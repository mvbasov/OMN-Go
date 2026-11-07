package app

import (
	"net/http"

	"net.basov.omngo/backend/internal/render"
)

// systemPage is one row of the page-access table: the address, the handler
// and the role that a caller on another machine needs. A refused caller gets
// a page, because a person can open the address from a link. The table is
// the registry of the system pages. registerRoutes and the Config page menu
// read it.
type systemPage struct {
	path  string
	title string // the title of the refusal page and of the menu line
	admin bool   // a caller on another machine needs the admin role
	serve http.HandlerFunc
	menu  *pageMenu // the line of the Config page menu, or nil
}

// pageMenu is the icon and the description of a menu line.
type pageMenu struct {
	icon string // a name of the Material Icons font
	desc string
}

func (a *App) systemPages() []systemPage {
	return []systemPage{
		{"/Config.html", "Config", true, a.serveConfigPage, nil},
		{"/OMNGoTags.html", "Tags", false, a.serveTagsPage, nil},
		{"/OMNGoSearch.html", "Search", false, a.serveSearchPage, nil},
		{"/OMNGoFiles.html", "Files", true, a.serveFilesPage, nil},
		{"/OMNGoStatus.html", "Status", true, a.serveStatusPage,
			&pageMenu{"monitor_heart", "Address, git commit, index, storage"}},
		{"/OMNGoLogs.html", "Log", true, a.serveLogsPage,
			&pageMenu{"subject", "What this device wrote, live and held"}},
		{"/db_backups", "Database Backups", true, a.serveDBBackupsPage, nil},
	}
}

// pageHandler adds the role check of one row to its handler.
func (a *App) pageHandler(p systemPage) http.HandlerFunc {
	if !p.admin {
		return p.serve
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.hasRole(r) {
			a.serveRefusalPage(w, p.title)
			return
		}
		p.serve(w, r)
	}
}

// serveRefusalPage tells a caller without the role how to get it.
func (a *App) serveRefusalPage(w http.ResponseWriter, title string) {
	body := `<div class="omn-card">` +
		`<h2 class="omn-card-title">` + title + `</h2>` +
		`<p class="omn-card-hint">This page is for the admin of this device. ` +
		`Log in as admin on a note page, then open the page again.</p>` +
		`</div>`
	a.renderPage(w, http.StatusOK, title, render.PageHeader(title, "System"), body)
}
