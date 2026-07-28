package main

import (
	"bytes"
	"net/http"
)

func (h *webHandler) servePage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	h.writePage(writer, request, http.StatusOK, webPage{Form: defaultEvaluationForm()})
}

func (h *webHandler) cancel(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	if !allowsBrowserPost(request) {
		http.Error(writer, "forbidden", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxWebBody)
	if err := request.ParseForm(); err != nil || len(request.Form) != 1 || len(request.PostForm) != 1 || len(request.PostForm["run_id"]) != 1 || h.controller.cancel(request.PostForm.Get("run_id")) != nil {
		writer.Header().Set("HX-Reswap", "outerHTML")
		h.writePage(writer, request, http.StatusConflict, webPage{Form: defaultEvaluationForm(), Error: "cancellation_conflict"})
		return
	}
	h.writePage(writer, request, http.StatusOK, webPage{Form: defaultEvaluationForm(), Error: "cancellation_requested"})
}

func (h *webHandler) serveAsset(writer http.ResponseWriter, request *http.Request, path, contentType string, immutable bool) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	writer.Header().Set("Content-Type", contentType)
	if immutable {
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeFileFS(writer, request, webFiles, path)
}

func (h *webHandler) writePage(writer http.ResponseWriter, request *http.Request, status int, page webPage) {
	name := "page"
	if request.Header.Get("HX-Request") == "true" {
		name = "result"
	}
	if page.RunID == "" {
		page.RunID = h.deps.newRunID()
		if page.RunID == "" {
			http.Error(writer, "render_failed", http.StatusInternalServerError)
			return
		}
	}
	var body bytes.Buffer
	if h.templates == nil || h.templates.ExecuteTemplate(&body, name, prepareWebPage(page)) != nil {
		http.Error(writer, "render_failed", http.StatusInternalServerError)
		return
	}
	if status == http.StatusBadRequest && request.Header.Get("HX-Request") == "true" {
		for _, field := range []string{"count", "seed", "workers", "format"} {
			if _, hasError := page.Form.Errors[field]; !hasError {
				continue
			}
			form := page.Form
			form.OutOfBand = true
			if err := h.templates.ExecuteTemplate(&body, "field-"+field, form); err != nil {
				http.Error(writer, "render_failed", http.StatusInternalServerError)
				return
			}
		}
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write(body.Bytes())
}

func (h *webHandler) shutdown() {
	h.controller.shutdown()
}
