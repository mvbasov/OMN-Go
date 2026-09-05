package backend

import (
	"net"
	"net/http"
	"strings"
)

func (a *App) isLocalConnection(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func (a *App) connectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// a.ActiveConns is read/written from every request's goroutine
		// concurrently; a bare ++/-- here was a data race. Use atomics.
		a.ActiveConns.Add(1)
		defer a.ActiveConns.Add(-1)

		// Each response goes through this function, thus this is the one
		// place that controls the cache of the client.
		//
		// The static files go out through http.ServeFile. It sends
		// Last-Modified, but it sends no expiry time. A browser then
		// applies its own rule and keeps the file for a part of its age.
		// The Android WebView does the same. After an update of the
		// application the new pages used the old omn-go-core.js for
		// days, and a user had to clear the cache by hand.
		//
		// "no-cache" does not stop the cache. The client keeps the file,
		// but it asks the server each time. The server answers 304 Not
		// Modified while the file does not change, thus the cost is one
		// small request.
		//
		// A handler that needs other words writes them later and wins.
		// The log stream does this (see logger.go).
		w.Header().Set("Cache-Control", "no-cache")

		next.ServeHTTP(&pageCacheWriter{ResponseWriter: w}, r)
	})
}

// pageCacheWriter changes "no-cache" to "no-store" for a page, and for a
// page only. It is the second half of the one cache decision above.
//
// "no-cache" is enough for a normal load. Chromium keeps the copy, asks
// the server, and gets the new page. A BACK or FORWARD load is different:
// Chromium reads the copy and does not ask the server at all. The page
// that OMN-Go changed while the page waited in the history thus comes
// back in its old form. The + button shows this: it writes a link into
// the page you started from, and Back then showed that page with no link
// until a second Back or a refresh.
//
// "no-store" is the only word that stops it. Chromium keeps no copy, thus
// Back has nothing to read and must ask the server.
//
// The rule covers a page and not an asset on purpose. KaTeX, highlight.js
// and the fonts are hundreds of kilobytes, and "no-store" on them would
// fetch that weight again for each page that you open.
//
// Two conditions guard the change, and each one keeps an existing rule:
//
//   - The response says "text/html". Each page handler sets this type
//     (serveHTMLPage, serveConfigPage, serveTagsPage, serveSearchPage,
//     serveFilesPage, serveStatusPage, serveDBBackupsPage, and the
//     editor). No asset does.
//   - Cache-Control still holds the "no-cache" from above. A handler that
//     wrote its own words keeps them, which is what logger.go needs.
type pageCacheWriter struct {
	http.ResponseWriter
	decided bool
}

// decide runs one time, at the moment the header goes out. Write also
// calls it, because a handler that writes a body with no WriteHeader
// sends the header at that moment.
func (w *pageCacheWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true
	if w.Header().Get("Cache-Control") != "no-cache" {
		return
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
}

func (w *pageCacheWriter) WriteHeader(status int) {
	w.decide()
	w.ResponseWriter.WriteHeader(status)
}

func (w *pageCacheWriter) Write(b []byte) (int, error) {
	w.decide()
	return w.ResponseWriter.Write(b)
}

// Flush keeps the log stream alive. logger.go asks the writer for
// http.Flusher, and a wrapper with no Flush would fail that question and
// hold each line until the response ends.
func (w *pageCacheWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ActiveConnCount returns the current number of in-flight requests.
func (a *App) ActiveConnCount() int64 {
	return a.ActiveConns.Load()
}

// hasRole answers "may this request do a thing that needs this role", and is
// the ONE definition of that. A connection from the device itself is always
// the owner - the Android WebView and the desktop browser both arrive that way
// - so the check only ever bites another machine on the network.
//
// It exists as a function because authMiddleware is not the only caller any
// more: the file index is a PAGE that needs authorization, and a page must
// answer a refusal with a page rather than the line of plain text below (see
// serveFilesPage). Two responses, one rule; the alternative was two copies of
// this condition drifting apart.
func (a *App) hasRole(r *http.Request, requireAdmin bool) bool {
	if a.isLocalConnection(r) {
		return true
	}
	// readSessionRole verifies the signature of the cookie and its expiry
	// time. It answers "" for a cookie that this install did not write.
	// Until 26.09.6 this function read the raw cookie value and trusted
	// it, thus a client on the network could name its own role. See the
	// banner of session.go.
	role := a.readSessionRole(r)
	if requireAdmin {
		return role == roleAdmin
	}
	return role == roleAdmin || role == roleGuest
}

func (a *App) authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.hasRole(r, requireAdmin) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
