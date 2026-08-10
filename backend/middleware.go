package backend

import (
	"net"
	"net/http"
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

		next.ServeHTTP(w, r)
	})
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
	cookie, err := r.Cookie("session_role")
	if err != nil {
		return false
	}
	if requireAdmin {
		return cookie.Value == "admin"
	}
	return cookie.Value == "admin" || cookie.Value == "guest"
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
