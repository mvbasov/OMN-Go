package backend

import (
	"net"
	"net/http"
	"sync/atomic"
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
		atomic.AddInt64(&a.ActiveConns, 1)
		defer atomic.AddInt64(&a.ActiveConns, -1)
		next.ServeHTTP(w, r)
	})
}

// ActiveConnCount returns the current number of in-flight requests.
func (a *App) ActiveConnCount() int64 {
	return atomic.LoadInt64(&a.ActiveConns)
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
