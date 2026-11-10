package status

import (
	"net/http"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/search"
	"net.basov.omngo/backend/internal/storage"
)

// Service is the Status page for one request. The App makes one with its
// storage layout, a copy of the settings and the facts of the server process.
// See statusService in backend/internal/app/status_app.go.
type Service struct {
	Layout          storage.Layout
	Config          config.Config
	Version         string        // APP_VERSION of the build
	BuildCommit     string        // the commit of the build, or "" when the build gave none
	BuildCommitTime string        // the commit time in RFC 3339, or ""
	StartedAt       time.Time     // the start of the server
	BoundAddr       string        // the address of the listener, or "" before the bind
	ActiveConns     int64         // the open connections
	AssetsRefreshed bool          // this start wrote an application file
	FallbackPort    int           // the port of a config.json with none
	Android         *Android      // the facts that the Android layer sets
	Search          *search.Index // nil before initStorage makes the index
	Log             func(tag logx.Tag) logx.Logger
	RenderPage      func(w http.ResponseWriter, code int, name string, header []byte, body string)
}

// writeJSON answers with one JSON value. See render.WriteJSON.
func (svc Service) writeJSON(w http.ResponseWriter, code int, v any) {
	render.WriteJSON(w, code, v, svc.Log(logx.Server))
}
