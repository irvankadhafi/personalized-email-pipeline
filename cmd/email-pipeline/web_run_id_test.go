package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestWebHandlerTerminalRunIDOwnsNextRunAfterActiveIDCancellation(t *testing.T) {
	var sequence atomic.Int64
	started := make(chan string, 2)
	handler := newWebHandlerWithDependencies(webDependencies{
		newSource: emptyWebSource,
		run: func(ctx context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
			started <- "started"
			<-ctx.Done()
			return interruptedWebReport(cfg.Workers)
		},
		now: time.Now,
		newRunID: func() string {
			return "run-" + strconv.FormatInt(sequence.Add(1), 10)
		},
	})
	initialPage := httptest.NewRecorder()
	handler.ServeHTTP(initialPage, httptest.NewRequest(http.MethodGet, "/", nil))
	initialID := hiddenRunID(t, initialPage.Body.String())
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- performEnhancedWebRequest(handler, initialID) }()
	<-started

	if result := performCancelRequest(handler, initialID); result.Code != http.StatusOK {
		t.Fatalf("initial cancel code=%d body=%s", result.Code, result.Body.String())
	}
	firstTerminal := <-firstDone
	freshID := hiddenRunID(t, firstTerminal.Body.String())
	if freshID == initialID {
		t.Fatalf("terminal reused active ID %q", freshID)
	}
	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { secondDone <- performEnhancedWebRequest(handler, freshID) }()
	<-started

	if stale := performCancelRequest(handler, initialID); stale.Code != http.StatusConflict {
		t.Fatalf("stale cancel code=%d body=%s", stale.Code, stale.Body.String())
	}
	if current := performCancelRequest(handler, freshID); current.Code != http.StatusOK {
		t.Fatalf("fresh cancel code=%d body=%s", current.Code, current.Body.String())
	}
	if terminal := <-secondDone; terminal.Code != http.StatusOK || !strings.Contains(terminal.Body.String(), "Interrupted") {
		t.Fatalf("second terminal code=%d body=%s", terminal.Code, terminal.Body.String())
	}
}

func performEnhancedWebRequest(handler http.Handler, runID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader(validWebForm("text")))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	request.Header.Set(webRunIDHeader, runID)
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	return result
}

func hiddenRunID(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`id="evaluator-run-id" type="hidden" value="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("run ID missing from %s", body)
	}
	return match[1]
}
