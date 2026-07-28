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

func TestWebHandlerRejectsCrossOriginEvaluationBeforeWork(t *testing.T) {
	tests := []struct {
		name   string
		header string
		value  string
		origin string
	}{
		{name: "cross-site fetch metadata", header: "Sec-Fetch-Site", value: "cross-site", origin: "http://example.com"},
		{name: "mismatched origin", header: "Origin", value: "http://attacker.example"},
		{name: "opaque origin", header: "Origin", value: "null"},
		{name: "origin with userinfo", header: "Origin", value: "http://user@example.com"},
		{name: "origin with query", header: "Origin", value: "http://example.com?query=1"},
		{name: "origin with fragment", header: "Origin", value: "http://example.com/#fragment"},
		{name: "origin with path", header: "Origin", value: "http://example.com/form"},
		{name: "non-http origin", header: "Origin", value: "https://example.com"},
		{name: "mismatched referer", header: "Referer", value: "http://attacker.example/form"},
		{name: "malformed referer", header: "Referer", value: "://bad"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sourceCalls, runnerCalls int
			handler := newWebHandlerWithDependencies(webDependencies{
				newSource: func(evaluationInput) (campaign.NextFunc, error) {
					sourceCalls++
					return campaign.SliceSource(nil), nil
				},
				run: func(context.Context, campaign.NextFunc, campaign.RunConfig) campaign.RunReport {
					runnerCalls++
					return successfulWebReport(maxWebCount, 1)
				},
				now:      time.Now,
				newRunID: func() string { return "unused" },
			})
			request := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader("count=1000000&seed=7&workers=1&format=text"))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set(test.header, test.value)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			result := httptest.NewRecorder()

			handler.ServeHTTP(result, request)

			if result.Code != http.StatusForbidden || result.Body.String() != "forbidden\n" || sourceCalls != 0 || runnerCalls != 0 {
				t.Fatalf("code=%d body=%q source=%d runner=%d", result.Code, result.Body.String(), sourceCalls, runnerCalls)
			}
		})
	}
}

func TestWebHandlerPermitsLocalAndNonBrowserEvaluationRequests(t *testing.T) {
	for _, test := range []struct {
		name   string
		header string
		value  string
	}{
		{name: "same-origin origin", header: "Origin", value: "http://example.com"},
		{name: "same-origin referer", header: "Referer", value: "http://example.com/form?mode=local#controls"},
		{name: "no browser provenance"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var runs int
			handler := newWebHandlerWithDependencies(controlledWebDependencies(func(_ context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
				runs++
				return successfulWebReport(1, cfg.Workers)
			}))
			request := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader(validWebForm("text")))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if test.header != "" {
				request.Header.Set(test.header, test.value)
			}
			result := httptest.NewRecorder()

			handler.ServeHTTP(result, request)

			if result.Code != http.StatusOK || runs != 1 {
				t.Fatalf("code=%d runs=%d body=%s", result.Code, runs, result.Body.String())
			}
		})
	}
}

func TestWebHandlerRejectsCrossOriginCancelBeforeActiveWorkChanges(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	cancelled := make(chan struct{}, 1)
	handler := newWebHandlerWithDependencies(webDependencies{
		newSource: emptyWebSource,
		run: func(ctx context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				cancelled <- struct{}{}
			}
			return successfulWebReport(1, cfg.Workers)
		},
		now:      time.Now,
		newRunID: func() string { return "fresh-run" },
	})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- performEnhancedWebRequest(handler, "active-run") }()
	<-started
	request := httptest.NewRequest(http.MethodPost, "/cancel", strings.NewReader("run_id=active-run"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://attacker.example")
	result := httptest.NewRecorder()

	handler.ServeHTTP(result, request)

	if result.Code != http.StatusForbidden || result.Body.String() != "forbidden\n" {
		t.Fatalf("code=%d body=%q", result.Code, result.Body.String())
	}
	select {
	case <-cancelled:
		t.Fatal("cross-origin cancellation changed active work")
	default:
	}
	close(release)
	<-done
}

func TestWebHandlerPermitsSameOriginCancel(t *testing.T) {
	started := make(chan struct{})
	handler := newWebHandlerWithDependencies(webDependencies{
		newSource: emptyWebSource,
		run: func(ctx context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
			close(started)
			<-ctx.Done()
			return interruptedWebReport(cfg.Workers)
		},
		now:      time.Now,
		newRunID: func() string { return "fresh-run" },
	})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- performEnhancedWebRequest(handler, "active-run") }()
	<-started
	request := httptest.NewRequest(http.MethodPost, "/cancel", strings.NewReader("run_id=active-run"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://example.com")
	result := httptest.NewRecorder()

	handler.ServeHTTP(result, request)

	if result.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", result.Code, result.Body.String())
	}
	<-done
}
