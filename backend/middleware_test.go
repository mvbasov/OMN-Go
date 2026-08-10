package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
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

// A 64-bit atomic needs an 8-byte-aligned address. A 32-bit build aligns a
// struct to 4, and the rule that usually saves you - "the first word in an
// allocated struct can be relied upon to be 64-bit aligned" - only covers
// the FIRST word. App.ActiveConns sits after Config and a RWMutex, which on
// GOARCH=386 and armeabi-v7a puts it at an offset of 164: a multiple of 4
// and not of 8. connectionMiddleware wraps every route, so the first request
// on such a build panicked with "unaligned 64-bit atomic operation" and the
// browser saw an empty response. arm64 aligns to 8 naturally and hid it, so
// the whole 32-bit half of the ABI split was broken and nothing said so.
//
// atomic.Int64 carries its own alignment guarantee. This test keeps it that
// way, and keeps anyone from reintroducing the pattern somewhere else - the
// architecture that catches it is not the one this test runs on.
func TestNoBare64BitAtomics(t *testing.T) {
	field, ok := reflect.TypeOf(App{}).FieldByName("ActiveConns")
	if !ok {
		t.Fatal("App has no ActiveConns field")
	}
	if field.Type != reflect.TypeOf(atomic.Int64{}) {
		t.Errorf("App.ActiveConns is %s, want atomic.Int64 - a bare int64 with "+
			"atomic.AddInt64 panics on every 32-bit build", field.Type)
	}

	banned := []string{
		"atomic.AddInt64(", "atomic.LoadInt64(", "atomic.StoreInt64(",
		"atomic.SwapInt64(", "atomic.CompareAndSwapInt64(",
		"atomic.AddUint64(", "atomic.LoadUint64(", "atomic.StoreUint64(",
		"atomic.SwapUint64(", "atomic.CompareAndSwapUint64(",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		// Test files are not shipped to a device, and this one names every
		// banned call as a string.
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range banned {
			if strings.Contains(string(src), call) {
				t.Errorf("%s uses %s. On a 32-bit build that panics unless the "+
					"address happens to be 8-byte aligned, which a struct field "+
					"cannot promise. Use the atomic.Int64 / atomic.Uint64 types, "+
					"which align themselves.", name, call)
			}
		}
	}
}
