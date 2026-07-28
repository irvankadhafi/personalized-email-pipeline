package main

import (
	"net/http"
	"net/url"
	"strings"
)

func allowsBrowserPost(request *http.Request) bool {
	if strings.EqualFold(request.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	values, originPresent := request.Header["Origin"]
	present := originPresent
	if !present {
		values, present = request.Header["Referer"]
	}
	if !present {
		return true
	}
	if len(values) != 1 {
		return false
	}
	parsed, err := url.Parse(values[0])
	if err != nil || !strings.EqualFold(parsed.Scheme, "http") || parsed.User != nil || parsed.Host != request.Host || parsed.Opaque != "" {
		return false
	}
	if originPresent {
		return parsed.Path == "" && parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == ""
	}
	return true
}
