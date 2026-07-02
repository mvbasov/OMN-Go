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

func (a *App) authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Automatically bypass authorization for internal OS/WebView connections
		if a.isLocalConnection(r) {
			next(w, r)
			return
		}

		cookie, err := r.Cookie("session_role")
		if err != nil || (requireAdmin && cookie.Value != "admin") || (!requireAdmin && cookie.Value != "admin" && cookie.Value != "guest") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
