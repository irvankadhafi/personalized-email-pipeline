package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestWebHandlerKeepsEnhancedRunStateAndCancellationInStableResult(t *testing.T) {
	// Given
	handler := newWebHandler()
	page := httptest.NewRecorder()

	// When
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	body := page.Body.String()

	// Then
	for _, value := range []string{
		`<link rel="icon" href="data:,">`,
		`class="evaluation-form"`,
		`class="actions"><button class="primary" id="evaluate-button" type="submit">Evaluate</button></div>`,
		`class="disclosure">Without enhanced cancellation`,
		`class="result-content"`,
		`class="running-content"`,
		`body:has(form.evaluation-form.htmx-request) .running-content { display:block; }`,
		`body:has(form.evaluation-form.htmx-request) .result-content { display:none; }`,
		`hx-post="/cancel" hx-target="#result" hx-swap="none"`,
	} {
		if !strings.Contains(body, value) {
			t.Fatalf("page missing %q: %s", value, body)
		}
	}
	resultStart := strings.Index(body, `<section class="region result result-swap" id="result"`)
	cancelStart := strings.Index(body, `>Cancel run</button>`)
	actionsStart := strings.Index(body, `<div class="actions">`)
	actionsEndOffset := strings.Index(body[actionsStart:], `</div>`)
	actionsEnd := actionsStart + actionsEndOffset
	if resultStart == -1 || cancelStart < resultStart {
		t.Fatalf("cancel is not owned by the stable result: %s", body)
	}
	if actionsStart == -1 || actionsEndOffset == -1 || cancelStart <= actionsEnd {
		t.Fatalf("cancel remained in the form action row: %s", body)
	}
	if strings.Contains(body[actionsStart:actionsEnd], "Cancel run") || strings.Contains(body[actionsStart:actionsEnd], "Local campaign running") {
		t.Fatalf("form action row retained enhanced-run controls: %s", body[actionsStart:actionsEnd])
	}
	validation := httptest.NewRecorder()
	invalid := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader("count=0&seed=7&workers=1&format=text"))
	invalid.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	invalid.Header.Set("HX-Request", "true")
	handler.ServeHTTP(validation, invalid)
	if validation.Code != http.StatusBadRequest || !hasDormantRunningMarkup(validation.Body.String()) {
		t.Fatalf("validation code=%d body=%s", validation.Code, validation.Body.String())
	}

	started := make(chan struct{})
	cancelled := make(chan struct{})
	controlled := newWebHandlerWithDependencies(webDependencies{
		newSource: emptyWebSource,
		run: func(ctx context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return interruptedWebReport(cfg.Workers)
		},
		now:      time.Now,
		newRunID: func() string { return "owned-run" },
	})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- performWebRequest(controlled, validWebForm("text"), "true") }()
	<-started

	matching := performCancelRequest(controlled, "owned-run")
	conflict := performCancelRequest(controlled, "stale-run")
	if matching.Code != http.StatusOK || !hasDormantRunningMarkup(matching.Body.String()) || conflict.Code != http.StatusConflict || conflict.Header().Get("HX-Reswap") != "outerHTML" || !hasDormantRunningMarkup(conflict.Body.String()) {
		t.Fatalf("matching=%d conflict=%d headers=%v", matching.Code, conflict.Code, conflict.Header())
	}
	<-cancelled
	terminal := <-done
	if terminal.Code != http.StatusOK || !strings.Contains(terminal.Body.String(), "Interrupted") || !hasDormantRunningMarkup(terminal.Body.String()) {
		t.Fatalf("terminal code=%d body=%s", terminal.Code, terminal.Body.String())
	}
}

func TestWebHandlerResultFragmentsRetainDormantRunningLifecycleMarkup(t *testing.T) {
	// Given
	handler := newWebHandlerWithDependencies(webDependencies{
		newSource: emptyWebSource,
		run: func(context.Context, campaign.NextFunc, campaign.RunConfig) campaign.RunReport {
			return successfulWebReport(1, 1)
		},
		now:      time.Now,
		newRunID: func() string { return "replacement-run" },
	})
	cases := []struct {
		name string
		page webPage
	}{
		{name: "ready", page: webPage{Form: defaultEvaluationForm(), RunID: "ready-run"}},
		{name: "completed", page: webPage{Form: defaultEvaluationForm(), Result: "report", Outcome: "success", RunID: "completed-run"}},
		{name: "interrupted", page: webPage{Form: defaultEvaluationForm(), Result: "report", Outcome: "interrupted", RunID: "interrupted-run"}},
		{name: "validation", page: webPage{Form: defaultEvaluationForm(), Error: "invalid_request", RunID: "validation-run"}},
		{name: "busy", page: webPage{Form: defaultEvaluationForm(), Error: "evaluation_busy", RunID: "busy-run"}},
		{name: "conflict", page: webPage{Form: defaultEvaluationForm(), Error: "cancellation_conflict", RunID: "conflict-run"}},
		{name: "cancellation acknowledgement", page: webPage{Form: defaultEvaluationForm(), Error: "cancellation_requested", RunID: "cancellation-run"}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// When
			request := httptest.NewRequest(http.MethodPost, "/evaluate", nil)
			request.Header.Set("HX-Request", "true")
			result := httptest.NewRecorder()
			handler.writePage(result, request, http.StatusOK, test.page)

			// Then
			body := result.Body.String()
			if result.Code != http.StatusOK || !strings.Contains(body, `id="evaluator-run-id" type="hidden" value="`+test.page.RunID+`"`) || !hasDormantRunningMarkup(body) {
				t.Fatalf("code=%d body=%s", result.Code, body)
			}
		})
	}
}

func hasDormantRunningMarkup(body string) bool {
	resultStart := strings.Index(body, `<section class="region result result-swap" id="result"`)
	runningStart := strings.Index(body, `class="running-content"`)
	return resultStart != -1 && runningStart > resultStart &&
		strings.Count(body, `class="running-content"`) == 1 &&
		!strings.Contains(body[runningStart:], `class="running-content" hidden`) &&
		strings.Count(body, `>Cancel run</button>`) == 1 &&
		strings.Contains(body, `hx-post="/cancel" hx-target="#result" hx-swap="none"`)
}
