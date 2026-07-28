package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestWebHandlerDefaultsAndExactControlBoundary(t *testing.T) {
	// Given
	handler := newWebHandler()

	// When
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/", nil))

	// Then
	body := result.Body.String()
	for _, field := range []string{`name="count"`, `name="seed"`, `name="workers"`, `name="format"`} {
		if strings.Count(body, field) != 1 {
			t.Fatalf("control %s count=%d", field, strings.Count(body, field))
		}
	}
	for _, value := range []string{`value="100000"`, `value="7"`, `value="` + strconv.Itoa(runtime.NumCPU()) + `"`, `value="text"`} {
		if !strings.Contains(body, value) {
			t.Fatalf("default %s missing", value)
		}
	}
}

func TestWebHandlerValidationPreservesRawFieldsBeforeStartingWork(t *testing.T) {
	// Given
	var sourceCalls, runnerCalls int
	handler := newWebHandlerWithDependencies(webDependencies{
		newSource: func(evaluationInput) (campaign.NextFunc, error) {
			sourceCalls++
			return campaign.SliceSource(nil), nil
		},
		run: func(context.Context, campaign.NextFunc, campaign.RunConfig) campaign.RunReport {
			runnerCalls++
			return successfulWebReport(0, 1)
		},
		now:      time.Now,
		newRunID: func() string { return "unused" },
	})
	raw := "count=1000001&seed=-1&workers=" + strconv.Itoa(runtime.NumCPU()+1) + "&format=markdown"

	// When
	result := performWebRequest(handler, raw, "")

	// Then
	if result.Code != http.StatusBadRequest || sourceCalls != 0 || runnerCalls != 0 {
		t.Fatalf("code=%d source=%d runner=%d", result.Code, sourceCalls, runnerCalls)
	}
	for _, value := range []string{"1000001", "-1", strconv.Itoa(runtime.NumCPU() + 1), "markdown"} {
		if !strings.Contains(result.Body.String(), value) {
			t.Fatalf("raw value %q missing from %s", value, result.Body.String())
		}
	}
	for _, field := range []string{"count", "seed", "workers", "format"} {
		if !strings.Contains(result.Body.String(), `data-error-for="`+field+`"`) {
			t.Fatalf("field error %q missing", field)
		}
	}
	if result.Header().Get("Server-Timing") != "" || result.Header().Get(webFormatHeader) != "" {
		t.Fatalf("invalid response fabricated metadata: %v", result.Header())
	}
}

func TestWebHandlerRejectsUnknownOrMissingControls(t *testing.T) {
	// Given
	handler := newWebHandler()
	cases := []string{
		"count=1&seed=7&workers=1",
		"count=1&seed=7&workers=1&format=text&input=private.csv",
		"count=1&count=2&seed=7&workers=1&format=text",
	}

	for _, body := range cases {
		// When
		result := performWebRequest(handler, body, "")

		// Then
		if result.Code != http.StatusBadRequest {
			t.Fatalf("body=%q code=%d", body, result.Code)
		}
	}
}

func TestWebHandlerRejectsEachInvalidFieldIndependently(t *testing.T) {
	// Given
	var starts int
	handler := newWebHandlerWithDependencies(webDependencies{
		newSource: func(evaluationInput) (campaign.NextFunc, error) {
			starts++
			return campaign.SliceSource(nil), nil
		},
		run: func(context.Context, campaign.NextFunc, campaign.RunConfig) campaign.RunReport {
			starts++
			return successfulWebReport(0, 1)
		},
		now:      time.Now,
		newRunID: func() string { return "unused" },
	})
	cases := []struct {
		field string
		body  string
	}{
		{field: "count", body: "count=0&seed=7&workers=1&format=text"},
		{field: "count", body: "count=1000001&seed=7&workers=1&format=text"},
		{field: "seed", body: "count=1&seed=-1&workers=1&format=text"},
		{field: "workers", body: "count=1&seed=7&workers=0&format=text"},
		{field: "workers", body: "count=1&seed=7&workers=" + strconv.Itoa(runtime.NumCPU()+1) + "&format=text"},
		{field: "format", body: "count=1&seed=7&workers=1&format=markdown"},
	}

	for _, test := range cases {
		// When
		result := performWebRequest(handler, test.body, "")

		// Then
		if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), `data-error-for="`+test.field+`"`) {
			t.Fatalf("field=%s code=%d body=%s", test.field, result.Code, result.Body.String())
		}
	}
	if starts != 0 {
		t.Fatalf("invalid inputs started work %d times", starts)
	}
}

func TestWebHandlerValidContentionReturnsImmediatelyWithoutStartingOrCancelling(t *testing.T) {
	// Given
	started := make(chan struct{})
	release := make(chan struct{})
	unexpectedCancellation := make(chan struct{}, 1)
	var mu sync.Mutex
	runs := 0
	handler := newWebHandlerWithDependencies(controlledWebDependencies(func(ctx context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
		mu.Lock()
		runs++
		mu.Unlock()
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			unexpectedCancellation <- struct{}{}
		}
		return successfulWebReport(1, cfg.Workers)
	}))
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- performWebRequest(handler, validWebForm("text"), "") }()
	<-started

	// When
	busy := performWebRequest(handler, validWebForm("html"), "")

	// Then
	mu.Lock()
	gotRuns := runs
	mu.Unlock()
	if busy.Code != http.StatusTooManyRequests || gotRuns != 1 {
		t.Fatalf("code=%d runs=%d", busy.Code, gotRuns)
	}
	select {
	case <-unexpectedCancellation:
		t.Fatal("busy request cancelled active run")
	default:
	}
	if busy.Header().Get("Server-Timing") != "" || busy.Header().Get(webFormatHeader) != "" {
		t.Fatalf("busy response fabricated metadata: %v", busy.Header())
	}
	close(release)
	if result := <-firstDone; result.Code != http.StatusOK {
		t.Fatalf("active code=%d body=%s", result.Code, result.Body.String())
	}
}

func TestWebHandlerEnhancedCancellationRequiresMatchingOwnership(t *testing.T) {
	// Given
	started := make(chan struct{})
	cancelled := make(chan struct{})
	handler := newWebHandlerWithDependencies(webDependencies{
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
	go func() { done <- performWebRequest(handler, validWebForm("text"), "true") }()
	<-started

	// When
	stale := performCancelRequest(handler, "stale-run")
	missing := performCancelRequest(handler, "")
	matching := performCancelRequest(handler, "owned-run")

	// Then
	if stale.Code != http.StatusConflict || missing.Code != http.StatusConflict || matching.Code != http.StatusOK {
		t.Fatalf("stale=%d missing=%d matching=%d", stale.Code, missing.Code, matching.Code)
	}
	<-cancelled
	result := <-done
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), "interrupted") {
		t.Fatalf("code=%d body=%s", result.Code, result.Body.String())
	}
	if repeat := performCancelRequest(handler, "owned-run"); repeat.Code != http.StatusConflict {
		t.Fatalf("stale completed identity code=%d", repeat.Code)
	}
}

func TestWebHandlerEnhancedCancelKeepsTerminalResultOwnedByEvaluation(t *testing.T) {
	// Given
	started := make(chan struct{})
	cancelled := make(chan struct{})
	handler := newWebHandlerWithDependencies(webDependencies{
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
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- performWebRequest(handler, validWebForm("text"), "true") }()
	<-started

	// When
	matching := performCancelRequest(handler, "owned-run")
	conflict := performCancelRequest(handler, "stale-run")

	// Then
	if !strings.Contains(page.Body.String(), `hx-post="/cancel" hx-target="#result" hx-swap="none"`) {
		t.Fatalf("successful cancel must not replace the evaluation-owned result: %s", page.Body.String())
	}
	if matching.Code != http.StatusOK || !strings.Contains(matching.Body.String(), "Cancellation requested") {
		t.Fatalf("matching code=%d body=%s", matching.Code, matching.Body.String())
	}
	if conflict.Code != http.StatusConflict || conflict.Header().Get("HX-Reswap") != "outerHTML" || !strings.Contains(conflict.Body.String(), "No applicable active run to cancel") {
		t.Fatalf("conflict code=%d headers=%v body=%s", conflict.Code, conflict.Header(), conflict.Body.String())
	}
	<-cancelled
	result := <-done
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), "Interrupted") {
		t.Fatalf("terminal code=%d body=%s", result.Code, result.Body.String())
	}
}

func TestWebHandlerOrdinaryRunUsesRequestCancellation(t *testing.T) {
	// Given
	started := make(chan struct{})
	cancelled := make(chan struct{})
	handler := newWebHandlerWithDependencies(controlledWebDependencies(func(ctx context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return interruptedWebReport(cfg.Workers)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader(validWebForm("text"))).WithContext(ctx)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
		close(done)
	}()
	<-started

	// When
	cancel()

	// Then
	<-cancelled
	<-done
}

func TestWebHandlerOrdinaryRunCannotBeCancelledByOwnershipEndpoint(t *testing.T) {
	// Given
	started := make(chan struct{})
	release := make(chan struct{})
	handler := newWebHandlerWithDependencies(controlledWebDependencies(func(ctx context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			t.Error("ownership endpoint cancelled an ordinary run")
		}
		return successfulWebReport(1, cfg.Workers)
	}))
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- performWebRequest(handler, validWebForm("text"), "") }()
	<-started

	// When
	result := performCancelRequest(handler, "controlled-run")

	// Then
	if result.Code != http.StatusConflict {
		t.Fatalf("code=%d", result.Code)
	}
	close(release)
	<-done
}

func TestServeWebShutdownCancelsActiveRunAndClosesAdmission(t *testing.T) {
	// Given
	started := make(chan struct{})
	cancelled := make(chan struct{})
	handler := newWebHandlerWithDependencies(controlledWebDependencies(func(ctx context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return interruptedWebReport(cfg.Workers)
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() { serveDone <- serveWeb(ctx, listener, handler) }()
	request, err := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String()+"/evaluate", strings.NewReader(validWebForm("text")))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	requestDone := make(chan error, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			err = response.Body.Close()
		}
		requestDone <- err
	}()
	<-started

	// When
	stop()

	// Then
	<-cancelled
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	if _, _, err := handler.controller.admit(context.Background(), false, ""); !errors.Is(err, errWebBusy) {
		t.Fatalf("admission after shutdown err=%v", err)
	}
}

func TestWebHandlerPropagatesControlsAndKeepsReportBytesAndTimingsSeparate(t *testing.T) {
	// Given
	report := successfulWebReport(4, 2)
	wantBody, err := campaign.MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	times := []time.Time{time.Unix(100, 0), time.Unix(100, int64(37*time.Millisecond))}
	var nowCalls int
	var gotConfig campaign.RunConfig
	var gotInput evaluationInput
	handler := newWebHandlerWithDependencies(webDependencies{
		newSource: func(input evaluationInput) (campaign.NextFunc, error) {
			gotInput = input
			return campaign.SliceSource(nil), nil
		},
		run: func(_ context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
			gotConfig = cfg
			return report
		},
		now: func() time.Time {
			value := times[nowCalls]
			nowCalls++
			return value
		},
		newRunID: func() string { return "unused" },
	})

	// When
	result := performWebRequest(handler, "count=4&seed=18446744073709551615&workers=2&format=html", "plain")

	// Then
	if result.Code != http.StatusOK || !bytes.Equal(result.Body.Bytes(), append(wantBody, '\n')) {
		t.Fatalf("code=%d body=%q want=%q", result.Code, result.Body.Bytes(), append(wantBody, '\n'))
	}
	if gotConfig.Workers != 2 || gotConfig.Format != campaign.HTMLFormat {
		t.Fatalf("config=%+v", gotConfig)
	}
	if gotInput.Count != 4 || gotInput.Seed != ^uint64(0) || gotInput.Workers != 2 || gotInput.Format != campaign.HTMLFormat {
		t.Fatalf("input=%+v", gotInput)
	}
	if result.Header().Get(webFormatHeader) != "html" || result.Header().Get("Server-Timing") != `request;dur=37` {
		t.Fatalf("metadata=%v", result.Header())
	}
}

func TestWebHandlerFullPageResultDisplaysSeparateEvidenceMetadata(t *testing.T) {
	// Given
	handler := newWebHandlerWithDependencies(webDependencies{
		newSource: emptyWebSource,
		run: func(context.Context, campaign.NextFunc, campaign.RunConfig) campaign.RunReport {
			return successfulWebReport(4, 2)
		},
		now:      time.Now,
		newRunID: func() string { return "fresh-run" },
	})

	// When
	result := performWebRequest(handler, "count=4&seed=7&workers=2&format=html", "")

	// Then
	for _, label := range []string{"Effective count", "Effective seed", "Effective worker count", "Selected format", "Campaign-processing elapsed", "Total server-request duration", "Request duration begins after successful validation", "machine-specific interactive evidence"} {
		if !strings.Contains(result.Body.String(), label) {
			t.Fatalf("result missing %q: %s", label, result.Body.String())
		}
	}
}

func controlledWebDependencies(run webRunFunc) webDependencies {
	return webDependencies{newSource: emptyWebSource, run: run, now: time.Now, newRunID: func() string { return "controlled-run" }}
}

func emptyWebSource(evaluationInput) (campaign.NextFunc, error) {
	return campaign.SliceSource(nil), nil
}

func validWebForm(format string) string {
	return "count=1&seed=7&workers=1&format=" + format
}

func performWebRequest(handler http.Handler, body, mode string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if mode == "true" {
		request.Header.Set("HX-Request", "true")
		request.Header.Set(webRunIDHeader, "owned-run")
	}
	if mode == "plain" {
		request.Header.Set("Accept", "text/plain")
	}
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	return result
}

func performCancelRequest(handler http.Handler, runID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/cancel", strings.NewReader("run_id="+runID))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	return result
}

func successfulWebReport(count int64, workers int) campaign.RunReport {
	counts := campaign.Counts{Examined: count, Eligible: count, Completed: count}
	return campaign.RunReport{
		Outcome: campaign.DeriveOutcome(counts, false, false), AccountingScope: "full", Counts: counts,
		InvalidReasons: map[campaign.Reason]campaign.ReasonSummary{}, FailedReasons: map[campaign.Reason]campaign.ReasonSummary{},
		ProcessingElapsed: 11 * time.Millisecond, Workers: workers, Settlement: campaign.DefaultSettlement,
		ResponseBound: campaign.DefaultResponseBound, Started: count,
	}
}

func interruptedWebReport(workers int) campaign.RunReport {
	counts := campaign.Counts{Examined: 1, Eligible: 1, Unprocessed: 1}
	return campaign.RunReport{
		Outcome: campaign.DeriveOutcome(counts, true, false), AccountingScope: "full", Counts: counts,
		InvalidReasons: map[campaign.Reason]campaign.ReasonSummary{}, FailedReasons: map[campaign.Reason]campaign.ReasonSummary{},
		ProcessingElapsed: time.Millisecond, Workers: workers, Settlement: campaign.DefaultSettlement,
		ResponseBound: campaign.DefaultResponseBound, Cancelled: true,
	}
}
