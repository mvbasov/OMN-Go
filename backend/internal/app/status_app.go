package app

import (
	"net"
	"net/http"

	"net.basov.omngo/backend/internal/status"
)

// The App side of the status package. statusService gives the package the
// storage layout, a copy of the settings and the facts of the server process.

// statusService answers the Status page of the App for one request.
func (a *App) statusService() status.Service {
	_, _, addr := a.boundAddress()
	return status.Service{
		Layout:          a.layout(),
		Config:          a.config.Get(),
		Version:         version,
		BuildCommit:     buildCommit,
		BuildCommitTime: buildCommitTime,
		StartedAt:       a.startedAt,
		BoundAddr:       addr,
		ActiveConns:     a.activeConnCount(),
		FallbackPort:    a.fallbackPort(),
		AssetsRefreshed: AssetsRefreshed(),
		Android:         &a.android,
		Search:          a.search,
		Log:             a.log,
		RenderPage:      a.renderPage,
	}
}

// handleStatus answers GET /api/status. See status.Service.HandleStatus.
func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	a.statusService().HandleStatus(w, r)
}

// serveStatusPage sends the Status page. See status.Service.ServeStatusPage.
func (a *App) serveStatusPage(w http.ResponseWriter, r *http.Request) {
	a.statusService().ServeStatusPage(w, r)
}

// boundAddress reports the address of the listener as host, port and the
// joined form. Each value is empty before the bind. StartServer writes the
// address with setBoundAddress.
func (a *App) boundAddress() (host, port, addr string) {
	a.metaMu.RLock()
	addr = a.boundAddr
	a.metaMu.RUnlock()
	if addr == "" {
		return "", "", ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", addr
	}
	return host, port, addr
}

func (a *App) setBoundAddress(addr string) {
	a.metaMu.Lock()
	a.boundAddr = addr
	a.metaMu.Unlock()
}

func (a *App) activeConnCount() int64 {
	return a.ActiveConns.Load()
}
