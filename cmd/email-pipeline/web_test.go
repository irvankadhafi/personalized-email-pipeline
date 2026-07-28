package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestWebListenAddressRejectsNonLoopbackHosts(t *testing.T) {
	tests := []string{"", ":8080", "localhost:8080", "0.0.0.0:8080", "192.0.2.1:8080", "127.0.0.1", "127.0.0.1:not-a-port"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			// Given
			// When
			_, err := loopbackListenAddress(input)

			// Then
			if err == nil {
				t.Fatalf("address %q was accepted", input)
			}
		})
	}
}

func TestWebListenAddressAcceptsLiteralLoopbackIPs(t *testing.T) {
	for _, input := range []string{"127.0.0.1:8080", "[::1]:8080"} {
		t.Run(input, func(t *testing.T) {
			// Given
			// When
			got, err := loopbackListenAddress(input)

			// Then
			if err != nil || got != input {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
}

func TestServeWebStopsAfterCancellation(t *testing.T) {
	// Given
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- serveWeb(ctx, listener, newWebHandler()) }()
	response, err := http.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d read=%v close=%v", response.StatusCode, readErr, closeErr)
	}

	// When
	cancel()

	// Then
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err == nil {
		connection.Close()
		t.Fatal("listener remained reachable after shutdown")
	}
}

func TestServeWebRejectsForgedHostBeforeRouting(t *testing.T) {
	// Given
	var runs int
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler := newWebHandlerWithDependencies(controlledWebDependencies(func(_ context.Context, _ campaign.NextFunc, cfg campaign.RunConfig) campaign.RunReport {
		runs++
		return successfulWebReport(1, cfg.Workers)
	}))
	result := make(chan error, 1)
	go func() { result <- serveWeb(ctx, listener, handler) }()

	request, err := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String()+"/evaluate", strings.NewReader(validWebForm("text")))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://attacker.example")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Host = "attacker.example"

	// When
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	// Then
	if response.StatusCode != http.StatusForbidden || runs != 0 {
		t.Fatalf("status=%d runs=%d", response.StatusCode, runs)
	}
	exactRequest, err := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String()+"/evaluate", strings.NewReader(validWebForm("text")))
	if err != nil {
		t.Fatal(err)
	}
	exactRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err = http.DefaultClient.Do(exactRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || runs != 1 {
		t.Fatalf("exact authority status=%d runs=%d", response.StatusCode, runs)
	}
	cancel()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestHostAuthorityHandlerRequiresBoundLiteralAuthority(t *testing.T) {
	// Given
	authority, err := listenerAuthorityFromString("[::1]:4321")
	if err != nil {
		t.Fatal(err)
	}
	handler := hostAuthorityHandler{
		authority: authority,
		next: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNoContent)
		}),
	}

	for _, test := range []struct {
		name string
		host string
		code int
	}{
		{name: "exact IPv6 authority", host: "[::1]:4321", code: http.StatusNoContent},
		{name: "hostname", host: "localhost:4321", code: http.StatusForbidden},
		{name: "wrong port", host: "[::1]:4322", code: http.StatusForbidden},
		{name: "portless", host: "[::1]", code: http.StatusForbidden},
		{name: "malformed", host: "attacker.example", code: http.StatusForbidden},
		{name: "mismatched IP", host: "[::2]:4321", code: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			// When
			request := httptest.NewRequest(http.MethodGet, "http://[::1]:4321/", nil)
			request.Host = test.host
			result := httptest.NewRecorder()
			handler.ServeHTTP(result, request)

			// Then
			if result.Code != test.code {
				t.Fatalf("host=%q code=%d want=%d", test.host, result.Code, test.code)
			}
		})
	}
}

func TestWebHandlerEvaluatesSyntheticFixtureAcrossRepresentations(t *testing.T) {
	// Given
	handler := newWebHandler()
	t.Setenv("EMAIL_PIPELINE_SMTP_PASSWORD", "SMTP_CANARY")
	t.Setenv("EMAIL_PIPELINE_REDIS_PASSWORD", "REDIS_CANARY")
	get := httptest.NewRequest(http.MethodGet, "/", nil)
	getResult := httptest.NewRecorder()
	handler.ServeHTTP(getResult, get)

	// When
	form := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader("count=4&seed=7&workers=1&format=text"))
	form.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	formResult := httptest.NewRecorder()
	handler.ServeHTTP(formResult, form)

	fragment := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader("count=4&seed=7&workers=1&format=text"))
	fragment.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	fragment.Header.Set("HX-Request", "true")
	fragment.Header.Set(webRunIDHeader, "fragment-run")
	fragmentResult := httptest.NewRecorder()
	handler.ServeHTTP(fragmentResult, fragment)

	plain := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader("count=4&seed=7&workers=1&format=text"))
	plain.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	plain.Header.Set("Accept", "text/plain")
	plainResult := httptest.NewRecorder()
	handler.ServeHTTP(plainResult, plain)

	// Then
	if getResult.Code != http.StatusOK || !strings.Contains(getResult.Body.String(), "<form class=\"evaluation-form\" action=\"/evaluate\" method=\"post\"") {
		t.Fatalf("GET code=%d body=%s", getResult.Code, getResult.Body.String())
	}
	for name, result := range map[string]*httptest.ResponseRecorder{"form": formResult, "fragment": fragmentResult} {
		if result.Code != http.StatusOK || result.Header().Get("Cache-Control") != "no-store" || result.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s code=%d headers=%v", name, result.Code, result.Header())
		}
		body := result.Body.String()
		if !strings.Contains(body, "completed") || strings.Contains(body, "@example.test") || strings.Contains(body, "SMTP_CANARY") || strings.Contains(body, "REDIS_CANARY") {
			t.Fatalf("%s unsafe result: %s", name, body)
		}
		if strings.Contains(body, "hx-swap-oob") {
			t.Fatalf("%s terminal result contained stale field update: %s", name, body)
		}
	}
	if plainResult.Code != http.StatusOK || plainResult.Header().Get("Content-Type") != "text/plain; charset=utf-8" || !strings.HasSuffix(plainResult.Body.String(), "\n") {
		t.Fatalf("plain code=%d headers=%v body=%q", plainResult.Code, plainResult.Header(), plainResult.Body.String())
	}
	var report struct {
		Counts struct {
			Examined  int64 `json:"examined"`
			Eligible  int64 `json:"eligible"`
			Completed int64 `json:"completed"`
		} `json:"counts"`
		Samples []struct {
			Category string `json:"category"`
		} `json:"samples"`
		Mode json.RawMessage `json:"mode"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(plainResult.Body.Bytes()), &report); err != nil {
		t.Fatal(err)
	}
	if report.Counts.Examined != 4 || report.Counts.Eligible != 4 || report.Counts.Completed != 4 || len(report.Samples) != 4 || report.Samples[0].Category != "named" || report.Samples[1].Category != "fallback" || report.Mode != nil {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestWebHandlerRendersFixedSafePreviewAndHTMXContracts(t *testing.T) {
	// Given
	handler := newWebHandler()

	// When
	initial := httptest.NewRecorder()
	handler.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/", nil))
	htmlPreview := httptest.NewRecorder()
	handler.ServeHTTP(htmlPreview, httptest.NewRequest(http.MethodGet, "/preview?format=html", nil))
	validation := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader("count=0&seed=7&workers=1&format=text"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	handler.ServeHTTP(validation, request)

	// Then
	for _, value := range []string{
		"Synthetic demonstration only. Uses deterministic .test recipients and an in-memory digest sink. No email is sent, no recipient data is accepted, and SMTP, Redis, Asynq, and the distributed ledger are not initialized. Use the CLI benchmark for authoritative one-million-record evidence.",
		"Customer 000001", "Exact plain-text message bytes", "Subject: Your exclusive offer", `id="result"`, "aria-live=\"polite\"",
		"Without enhanced cancellation, navigating away or disconnecting cancels this run.", "htmx:beforeSwap", `id="evaluator-run-id"`, "Cancel run", `class="evaluation-form" action="/evaluate" method="post" novalidate hx-post="/evaluate"`,
	} {
		if !strings.Contains(initial.Body.String(), value) {
			t.Fatalf("initial page missing %q", value)
		}
	}
	if htmlPreview.Code != http.StatusOK || !strings.Contains(htmlPreview.Body.String(), "sandbox") || !strings.Contains(htmlPreview.Body.String(), "Escaped HTML source") || strings.Contains(htmlPreview.Body.String(), "<script") || strings.Contains(htmlPreview.Body.String(), "<form") {
		t.Fatalf("html preview code=%d body=%s", htmlPreview.Code, htmlPreview.Body.String())
	}
	if validation.Code != http.StatusBadRequest || !strings.HasPrefix(strings.TrimSpace(validation.Body.String()), "<section") || !strings.Contains(validation.Body.String(), "Invalid benchmark controls") {
		t.Fatalf("validation code=%d body=%s", validation.Code, validation.Body.String())
	}
}

func TestWebHandlerHTMXValidationReturnsAffectedFieldOutOfBandUpdate(t *testing.T) {
	// Given
	handler := newWebHandler()
	request := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader("count=1000001&seed=7&workers=1&format=text"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	request.Header.Set(webRunIDHeader, "enhanced-invalid-run")

	// When
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)

	// Then
	body := result.Body.String()
	if result.Code != http.StatusBadRequest || !strings.HasPrefix(strings.TrimSpace(body), "<section") || !strings.Contains(body, "Invalid benchmark controls") {
		t.Fatalf("validation code=%d body=%s", result.Code, body)
	}
	for _, value := range []string{`<section class="region result result-swap" id="result"`, `id="field-count" hx-swap-oob="true"`, `value="1000001"`, `aria-invalid="true"`, `aria-describedby="count-help count-error"`, `id="count-error"`, `data-error-for="count"`} {
		if !strings.Contains(body, value) {
			t.Fatalf("validation update missing %q: %s", value, body)
		}
	}
	if strings.Count(body, `hx-swap-oob="true"`) != 1 {
		t.Fatalf("expected one OOB field replacement: %s", body)
	}
	if strings.Contains(body, `id="field-seed" hx-swap-oob="true"`) || strings.Contains(body, `id="field-workers" hx-swap-oob="true"`) || strings.Contains(body, `id="field-format" hx-swap-oob="true"`) {
		t.Fatalf("unaffected fields received OOB updates: %s", body)
	}
}

func TestWebHandlerRejectsInvalidOversizedBusyAndUnsupportedRequests(t *testing.T) {
	// Given
	handler := newWebHandler()
	cases := []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{name: "invalid count", method: http.MethodPost, body: "count=0&seed=7&workers=1&format=text", want: http.StatusBadRequest},
		{name: "unknown field", method: http.MethodPost, body: "count=1&seed=7&workers=1&format=text&input=private.csv", want: http.StatusBadRequest},
		{name: "oversized", method: http.MethodPost, body: "count=1&seed=" + strings.Repeat("1", 4096) + "&workers=1&format=text", want: http.StatusBadRequest},
		{name: "method", method: http.MethodPut, body: "", want: http.StatusMethodNotAllowed},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// When
			request := httptest.NewRequest(test.method, "/evaluate", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			result := httptest.NewRecorder()
			handler.ServeHTTP(result, request)

			// Then
			if result.Code != test.want {
				t.Fatalf("code=%d body=%s", result.Code, result.Body.String())
			}
		})
	}
}

func TestWebHandlerServesLocalHTMXAndNeverImportsOptionalServices(t *testing.T) {
	// Given
	handler := newWebHandler()
	request := httptest.NewRequest(http.MethodGet, "/assets/htmx-2.0.4.min.js", nil)
	result := httptest.NewRecorder()

	// When
	handler.ServeHTTP(result, request)

	// Then
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), "version:\"2.0.4\"") {
		t.Fatalf("asset code=%d body=%q", result.Code, result.Body.String())
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(result.Body.Bytes())); digest != "e209dda5c8235479f3166defc7750e1dbcd5a5c1808b7792fc2e6733768fb447" {
		t.Fatalf("unexpected HTMX digest: %s", digest)
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "web") || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"internal/distributed", "internal/testinbox", "runCommand(", "runDistributed(", "workerCommand("} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("%s contains %q", entry.Name(), forbidden)
			}
		}
	}
}

func TestExecuteIncludesWebInHelp(t *testing.T) {
	// Given
	var stdout, stderr bytes.Buffer

	// When
	code := execute([]string{"help"}, &stdout, &stderr)

	// Then
	if code != 0 || !strings.Contains(stdout.String(), "web [--listen") || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}
