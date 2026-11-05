package app

import (
	"fmt"
	"net/http"

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
)

func (a *App) handleLogsSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := a.logs.Subscribe()
	defer a.logs.Unsubscribe(ch)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

var logsPageTmpl = render.LoadTemplate("logs_page.html")

// serveLogsPage answers /OMNGoLogs.html. The page reads /api/logs/history one
// time, and then it adds each new line of /api/logs. omn-go-logs.js does that
// work.
//
// Android has no terminal. Without this page, a person on a phone needs adb
// logcat, or a second browser at the history endpoint.
func (a *App) serveLogsPage(w http.ResponseWriter, r *http.Request) {
	a.renderPage(w, http.StatusOK, "Log", render.PageHeader("Log", "System", "css/OMN-Go/omn-go-logs.css"), logsPageTmpl)
}

// handleLogHistory answers the ring of the last logx.HistoryCap lines, oldest
// first. It is a separate endpoint, because a replay on /api/logs breaks the
// sync overlay. See the ring banner in internal/logx/hub.go.
//
// IT IS ADMIN ONLY, and so is /api/logs. A remote caller reads no log line. See
// doc/decisions/0013-send-each-log-line-to-three-places-and-to-the-admin-only.md.
// The answer follows section 1.4 of doc/API.md: JSON with a status word.
func (a *App) handleLogHistory(w http.ResponseWriter, r *http.Request) {
	lines := a.logs.Snapshot()
	a.writeJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"cap":    logx.HistoryCap,
		"lines":  lines,
	})
}
