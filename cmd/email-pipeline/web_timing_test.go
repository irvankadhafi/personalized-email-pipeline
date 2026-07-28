package main

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestWebHandlerHTMLTimingIncludesOnlyFinalTemplateSerialization(t *testing.T) {
	for _, enhanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "full page", true: "HTMX fragment"}[enhanced], func(t *testing.T) {
			var renderCalls int
			times := []time.Time{time.Unix(100, 0), time.Unix(100, int64(37*time.Millisecond))}
			var nowCalls int
			handler := newWebHandlerWithDependencies(webDependencies{
				newSource: emptyWebSource,
				run: func(context.Context, campaign.NextFunc, campaign.RunConfig) campaign.RunReport {
					return successfulWebReport(1, 1)
				},
				now: func() time.Time {
					value := times[nowCalls]
					nowCalls++
					return value
				},
				newRunID: func() string { return "fresh-run" },
			})
			handler.templates = template.Must(template.New("response").Funcs(template.FuncMap{
				"recordRender": func() string {
					renderCalls++
					return "serialized"
				},
			}).Parse(`{{define "page"}}duration={{.RequestDuration}};{{recordRender}}{{end}}{{define "result"}}duration={{.RequestDuration}};{{recordRender}}{{end}}`))
			request := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader(validWebForm("text")))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if enhanced {
				request.Header.Set("HX-Request", "true")
				request.Header.Set(webRunIDHeader, "active-run")
			}
			result := httptest.NewRecorder()

			handler.ServeHTTP(result, request)

			body := result.Body.String()
			if result.Code != http.StatusOK || body != "duration=37ms;serialized" {
				t.Fatalf("code=%d body=%q", result.Code, result.Body.String())
			}
			if strings.Contains(body, requestDurationPlaceholder) || strings.Count(body, "37ms") != 1 {
				t.Fatalf("body=%q placeholderCount=%d durationCount=%d", body, strings.Count(body, requestDurationPlaceholder), strings.Count(body, "37ms"))
			}
			if result.Header().Get("Server-Timing") != "request;dur=37" || renderCalls != 1 || nowCalls != 2 {
				t.Fatalf("timing=%q renders=%d now=%d", result.Header().Get("Server-Timing"), renderCalls, nowCalls)
			}
		})
	}
}

func TestWebHandlerHTMLTimingRejectsDuplicateDurationPlaceholders(t *testing.T) {
	var campaignCalls int
	var renderCalls int
	var nowCalls int
	handler := newWebHandlerWithDependencies(webDependencies{
		newSource: emptyWebSource,
		run: func(context.Context, campaign.NextFunc, campaign.RunConfig) campaign.RunReport {
			campaignCalls++
			return successfulWebReport(1, 1)
		},
		now: func() time.Time {
			nowCalls++
			return time.Unix(100, int64(nowCalls)*int64(time.Millisecond))
		},
		newRunID: func() string { return "fresh-run" },
	})
	handler.templates = template.Must(template.New("response").Funcs(template.FuncMap{
		"recordRender": func() string {
			renderCalls++
			return "serialized"
		},
	}).Parse(`{{define "page"}}duration={{.RequestDuration}};duplicate={{.RequestDuration}};{{recordRender}}{{end}}`))
	request := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader(validWebForm("text")))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	result := httptest.NewRecorder()

	handler.ServeHTTP(result, request)

	if result.Code != http.StatusInternalServerError || result.Body.String() != "render_failed\n" {
		t.Fatalf("code=%d body=%q", result.Code, result.Body.String())
	}
	if result.Header().Get("Server-Timing") != "" || result.Header().Get(webFormatHeader) != "" {
		t.Fatalf("timing=%q format=%q", result.Header().Get("Server-Timing"), result.Header().Get(webFormatHeader))
	}
	if campaignCalls != 1 || renderCalls != 1 || nowCalls != 2 {
		t.Fatalf("campaigns=%d renders=%d now=%d", campaignCalls, renderCalls, nowCalls)
	}
}
