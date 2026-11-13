package app

import (
	"log"

	"net.basov.omngo/backend/internal/logx"
)

// log gives the logger of one tag.
func (a *App) log(tag logx.Tag) logx.Logger {
	return logx.New(tag, &a.logFilter, &a.logs)
}

// initLogger sends the standard logger into the /api/logs stream.
// registerRoutes in server.go registers the route. The function is not
// exported. See section 3 of CLAUDE.md for the exported names.
func (a *App) initLogger() {
	log.SetOutput(a.logs.StdWriter())
	log.SetFlags(log.LstdFlags | log.LUTC)
}
