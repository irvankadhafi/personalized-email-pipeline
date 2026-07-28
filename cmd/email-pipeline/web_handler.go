package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/recipientcsv"
)

const (
	webFormatHeader            = "X-Evaluator-Format"
	webRunIDHeader             = "X-Evaluator-Run-ID"
	requestDurationPlaceholder = "__EMAIL_PIPELINE_INTERNAL_REQUEST_DURATION_7B1D638E__"
)

type webRunFunc func(context.Context, campaign.NextFunc, campaign.RunConfig) campaign.RunReport

type webDependencies struct {
	newSource func(evaluationInput) (campaign.NextFunc, error)
	run       webRunFunc
	now       func() time.Time
	newRunID  func() string
}

type webHandler struct {
	templates  *template.Template
	controller webRunController
	deps       webDependencies
}

type webPage struct {
	Form              evaluationForm
	Preview           webPreview
	Result            string
	Error             string
	RunID             string
	Outcome           string
	Count             int64
	Seed              uint64
	Workers           int
	Format            string
	ProcessingElapsed time.Duration
	RequestDuration   string
}

func newWebHandler() *webHandler {
	return newWebHandlerWithDependencies(webDependencies{
		newSource: generatedWebSource, run: campaign.Run, now: time.Now,
		newRunID: func() string {
			var identity [16]byte
			if _, err := rand.Read(identity[:]); err != nil {
				return ""
			}
			return hex.EncodeToString(identity[:])
		},
	})
}

func newWebHandlerWithDependencies(deps webDependencies) *webHandler {
	templates, err := template.ParseFS(webFiles, "web/*.html")
	if err != nil {
		return &webHandler{deps: deps}
	}
	return &webHandler{templates: templates, deps: deps}
}

func generatedWebSource(input evaluationInput) (campaign.NextFunc, error) {
	source, err := recipientcsv.NewGeneratedSource(recipientcsv.FixtureOptions{
		Algorithm: recipientcsv.FixtureAlgorithmV1, Seed: input.Seed, Count: input.Count,
	})
	if err != nil {
		return nil, err
	}
	return func() (campaign.SourceRecord, bool, error) {
		record, ok := source.Next()
		return campaign.SourceRecord{Ordinal: record.Ordinal, Email: record.Email, Name: record.Name}, ok, nil
	}, nil
}

func (h *webHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	switch request.URL.Path {
	case "/":
		h.servePage(writer, request)
	case "/evaluate":
		h.evaluate(writer, request)
	case "/preview":
		h.servePreview(writer, request)
	case "/cancel":
		h.cancel(writer, request)
	case "/assets/htmx-2.0.4.min.js":
		h.serveAsset(writer, request, "web/htmx-2.0.4.min.js", "text/javascript; charset=utf-8", true)
	case "/assets/htmx-LICENSE.txt":
		h.serveAsset(writer, request, "web/htmx-LICENSE.txt", "text/plain; charset=utf-8", false)
	default:
		http.NotFound(writer, request)
	}
}

func (h *webHandler) evaluate(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	if !allowsBrowserPost(request) {
		http.Error(writer, "forbidden", http.StatusForbidden)
		return
	}
	input, form, err := parseEvaluationInput(writer, request)
	if err != nil {
		h.writePage(writer, request, http.StatusBadRequest, webPage{Form: form, Error: "invalid_request"})
		return
	}
	started := h.deps.now()
	enhanced := request.Header.Get("HX-Request") == "true"
	runID := ""
	if enhanced {
		runID = request.Header.Get(webRunIDHeader)
		if runID == "" {
			h.writePage(writer, request, http.StatusBadRequest, webPage{Form: form, Error: "invalid_request"})
			return
		}
	}
	parent := request.Context()
	if enhanced {
		parent = context.WithoutCancel(parent)
	}
	runCtx, lease, err := h.controller.admit(parent, enhanced, runID)
	if err != nil {
		h.writePage(writer, request, http.StatusTooManyRequests, webPage{Form: form, Error: "evaluation_busy"})
		return
	}
	defer h.controller.finish(lease)
	source, err := h.deps.newSource(input)
	if err != nil {
		h.writePage(writer, request, http.StatusInternalServerError, webPage{Form: form, Error: "evaluation_failed"})
		return
	}
	report := h.deps.run(runCtx, source, campaign.RunConfig{Workers: input.Workers, Format: input.Format})
	reportBytes, err := campaign.MarshalReport(report)
	if err != nil {
		h.writePage(writer, request, http.StatusInternalServerError, webPage{Form: form, Error: "evaluation_failed"})
		return
	}
	page := webPage{
		Form: form, Result: string(reportBytes), Outcome: string(report.Outcome), Count: input.Count,
		Seed: input.Seed, Workers: input.Workers, Format: input.Format.String(), ProcessingElapsed: report.ProcessingElapsed,
	}
	if page.RunID = h.deps.newRunID(); page.RunID == "" {
		h.writePage(writer, request, http.StatusInternalServerError, webPage{Form: form, Error: "evaluation_failed"})
		return
	}
	if !wantsPlainText(request) {
		page.RequestDuration = requestDurationPlaceholder
	}
	body, contentType, err := h.responseBody(request, page, reportBytes)
	if err != nil {
		h.writePage(writer, request, http.StatusInternalServerError, webPage{Form: form, Error: "evaluation_failed"})
		return
	}
	duration := requestDuration(started, h.deps.now())
	if !wantsPlainText(request) {
		if bytes.Count(body, []byte(requestDurationPlaceholder)) != 1 {
			http.Error(writer, "render_failed", http.StatusInternalServerError)
			return
		}
		body = bytes.Replace(body, []byte(requestDurationPlaceholder), []byte(duration.String()), 1)
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set(webFormatHeader, input.Format.String())
	writer.Header().Set("Server-Timing", fmt.Sprintf("request;dur=%g", float64(duration.Microseconds())/1000))
	_, _ = writer.Write(body)
}

func (h *webHandler) servePreview(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	format, err := campaign.ParseFormat(request.URL.Query().Get("format"))
	if err != nil || request.URL.Query().Get("format") == "" {
		format = campaign.TextFormat
	}
	var body bytes.Buffer
	if h.templates == nil || h.templates.ExecuteTemplate(&body, "preview", makeWebPreview(format.String())) != nil {
		http.Error(writer, "render_failed", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = writer.Write(body.Bytes())
}

func (h *webHandler) responseBody(request *http.Request, page webPage, report []byte) ([]byte, string, error) {
	if wantsPlainText(request) {
		return append(append([]byte(nil), report...), '\n'), "text/plain; charset=utf-8", nil
	}
	name := "page"
	if request.Header.Get("HX-Request") == "true" {
		name = "result"
	}
	var body bytes.Buffer
	if h.templates == nil {
		return nil, "", errors.New("web templates unavailable")
	}
	if err := h.templates.ExecuteTemplate(&body, name, prepareWebPage(page)); err != nil {
		return nil, "", err
	}
	return body.Bytes(), "text/html; charset=utf-8", nil
}

func wantsPlainText(request *http.Request) bool {
	for accepted := range strings.SplitSeq(request.Header.Get("Accept"), ",") {
		if strings.TrimSpace(strings.SplitN(accepted, ";", 2)[0]) == "text/plain" {
			return true
		}
	}
	return false
}
