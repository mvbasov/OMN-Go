package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// connectionMiddleware wraps each route (see server.go), thus each
// response must carry Cache-Control. Without the header http.ServeFile
// sends Last-Modified only, and a browser or the Android WebView then
// keeps a script for days. An update of the application then shows new
// pages that operate with the old scripts.
func TestConnectionMiddlewareSetsCacheControl(t *testing.T) {
	a := &App{}
	h := a.connectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/js/omn-go-core.js", nil))

	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
	}
}

// The middleware sets the header before the handler operates, thus a
// handler that needs other words can write them. The log stream does
// this (see InitLoggerAndRoute in logger.go).
func TestConnectionMiddlewareLetsAHandlerReplaceCacheControl(t *testing.T) {
	a := &App{}
	h := a.connectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/logs", nil))

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want the value of the handler %q", got, "no-store")
	}
}
