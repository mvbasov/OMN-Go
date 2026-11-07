package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/exchange"
	"net.basov.omngo/backend/internal/render"
)

// Each row of the page-access table gets its role check from the router. A
// remote caller with an old guest cookie gets the refusal page for an admin row,
// and the page itself for each other row. The device itself gets each page.
func TestEachSystemPageFollowsItsRow(t *testing.T) {
	a := newTestApp(t)
	const refusal = "for the admin of this device"

	for _, p := range a.systemPages() {
		req := httptest.NewRequest(http.MethodGet, p.path, nil)
		req.RemoteAddr = "192.168.1.44:51000"
		req.AddCookie(sessionCookie(t, a, "guest"))
		rec := routeServe(a, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: a remote caller got %d, want a page", p.path, rec.Code)
			continue
		}
		refused := strings.Contains(rec.Body.String(), refusal)
		if refused != p.admin {
			t.Errorf("%s: a remote caller refused=%v, want %v", p.path, refused, p.admin)
		}
		if refused && !strings.Contains(rec.Body.String(), p.title) {
			t.Errorf("%s: the refusal page does not name %q", p.path, p.title)
		}

		local := routeReq(a, http.MethodGet, p.path)
		if strings.Contains(local.Body.String(), refusal) {
			t.Errorf("%s: the device itself got the refusal page", p.path)
		}
	}
}

// renderPage is the one shell of a page that the server makes. A call of
// CompilePageWithBody in another file of package app fails this test.
func TestOnlyRenderPageCompilesAServerPage(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "render_app.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "CompilePageWithBody(") {
			t.Errorf("%s compiles a page itself. Call a.renderPage.", f)
		}
	}
}

// connectGroups gives the page shell two values of other groups. Each page must
// show them, although internal/render/pages.go names no search or exchange
// code.
func TestEachPageShowsTheValuesOfOtherGroups(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nneedle\n")
	a.rebuildSearchIndex()

	page := string(a.injectRuntimeVars([]byte(render.RuntimeVarsMarker)))
	for _, want := range []string{
		"var OMN_SEARCH_GLOBAL = true;",
		fmt.Sprintf("var OMN_INCOMING_PAGE = %q;", exchange.IncomingIndexName),
		// The receive box reads this value. It holds each extension of
		// config.UserFileTrees with the upload route of its tree.
		`var OMN_USER_FILE_UPLOADS = {".json":"/api/upload_json",".jsonl":"/api/upload_json",` +
			`".vcf":"/api/upload_contacts",".ics":"/api/upload_calendars",".vcs":"/api/upload_calendars"};`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page misses %s: %s", want, page)
		}
	}
}

// testRenderer answers the renderer of the App as a pointer, thus a test can
// call a method of it in one expression.
func (a *App) testRenderer() *render.Renderer {
	rd := a.renderer()
	return &rd
}

// The Config page menu shows one link for each row of the page-access table
// that has a menu line. It shows no other link of that kind.
func TestEachMenuRowHasALinkOnTheConfigPage(t *testing.T) {
	a := newTestApp(t)
	body := a.getConfigPageBody()
	rows := 0
	for _, p := range a.systemPages() {
		if p.menu == nil {
			continue
		}
		rows++
		if !strings.Contains(body, `<a class="omn-card-menu-item" href="`+p.path+`">`) {
			t.Errorf("the Config page menu has no link to %s", p.path)
		}
	}
	if n := strings.Count(body, `<a class="omn-card-menu-item" href=`); n != rows {
		t.Errorf("the Config page menu has %d links, and the table has %d menu rows", n, rows)
	}
}

// No bundled note may take the address of a system page. The route of the
// system page wins, thus the note could never open.
func TestNoBundledNoteTakesASystemPagePath(t *testing.T) {
	for _, p := range newTestApp(t).systemPages() {
		name := strings.TrimSuffix(strings.TrimPrefix(p.path, "/"), ".html")
		if _, err := frontend.Static.ReadFile("md/" + name + ".md"); err == nil {
			t.Errorf("md/%s.md has the address of the system page %s", name, p.path)
		}
	}
}
