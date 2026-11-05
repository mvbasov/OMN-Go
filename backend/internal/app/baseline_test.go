package app

// BASELINE: the behavior that a change can break with no other test to say so.
//
// One question chose each test here. "What would break silently, and is
// asserted nowhere else?" The file pins:
//
//   - the set of routes that registerRoutes registers,
//   - the set of runtime variables that injectRuntimeVars puts into a page,
//   - the page dispatch for each kind of address,
//   - the shape of a compiled page across every write path,
//   - the /api/logs SSE lifecycle,
//   - the semantics of a POST to /api/config.
//
// A CHANGE OF A GOLDEN VALUE IS A DECISION. A new route or a new runtime
// variable changes a list below. Add it with one comment that says why it is
// there. A baseline test that fails for any other reason means the change
// under it was not as behavior-preserving as it looked.
//
// A convention inherited from the file that this replaces, and worth keeping.
// Every test says WHY it exists. A failure thus reads either as "you broke
// it", or as "you changed it on purpose, update the golden value".

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/search"
)

// ---------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------

// baseWriteMD writes a note into <storage>/md/<rel>, and it creates the
// directories. It is deliberately private to this file. A baseline suite that
// depends on helpers owned by other test files can be broken by an unrelated
// edit to those files.
func baseWriteMD(t *testing.T, a *App, rel, content string) string {
	t.Helper()
	p := filepath.Join(a.StorageDir, "md", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func postForm(t *testing.T, h http.HandlerFunc, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// getPage issues a browser-shaped GET, with Accept: text/html, through the
// real router. The test thus uses the real dispatch chain, and it does
// not call an inner handler directly.
func getPage(t *testing.T, a *App, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	return routeServe(a, req)
}

// ---------------------------------------------------------------------
// 1. Page dispatch
//
// The router sends Config, OMNGoTags and OMNGoSearch through the page-access
// table. serveFrontend sends an ordinary note, a missing note, ?refresh, ?edit
// and a non-page asset. The handler tests call the inner functions directly,
// thus only this test checks the dispatch AS A WHOLE. A new page can shadow
// an existing address, and this test finds that.
// ---------------------------------------------------------------------

func TestBaseline_ServeHTMLPageDispatch(t *testing.T) {
	a := newTestApp(t)
	baseWriteMD(t, a, "Note.md", "Title: A Note\nCategory: Test\n\nhello baseline")

	t.Run("root redirects to Welcome", func(t *testing.T) {
		for _, p := range []string{"/", "/index.html"} {
			rec := getPage(t, a, p)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("%s: status %d, want 303", p, rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "/Welcome.html" {
				t.Errorf("%s: Location %q, want /Welcome.html", p, loc)
			}
		}
	})

	t.Run("Config is a dynamic page", func(t *testing.T) {
		rec := getPage(t, a, "/Config.html")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `id="omn-config-form"`) {
			t.Error("Config.html did not render the config form")
		}
		// Dynamic, NOT cached: no md source and no compiled artefact.
		if _, err := os.Stat(filepath.Join(a.StorageDir, "md", "Config.md")); err == nil {
			t.Error("Config.html created an md/ source; it must stay dynamic")
		}
		if _, err := os.Stat(a.layout().PageHTML("Config")); err == nil {
			t.Error("Config.html wrote an html/ cache; it must stay dynamic")
		}
	})

	t.Run("OMNGoTags is generated and cached", func(t *testing.T) {
		rec := getPage(t, a, "/OMNGoTags.html")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		// Unlike Config, this one IS backed by a real (generated) note.
		if _, err := os.Stat(filepath.Join(a.StorageDir, "md", "OMNGoTags.md")); err != nil {
			t.Errorf("OMNGoTags.md not generated: %v", err)
		}
		if _, err := os.Stat(a.layout().PageHTML("OMNGoTags")); err != nil {
			t.Errorf("OMNGoTags.html not cached: %v", err)
		}
	})

	t.Run("OMNGoSearch is dynamic and gated", func(t *testing.T) {
		// With global search off, the page still answers. It explains how to
		// switch global search on. Unlike an unknown page name, it must not
		// synthesize a note either way.
		rec := getPage(t, a, "/OMNGoSearch.html")
		if rec.Code != http.StatusOK {
			t.Errorf("status %d, want 200 with an explanation while global search is off", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "/Config.html#cfg-search") {
			t.Error("the disabled page does not link to the setting that enables it")
		}
		if _, err := os.Stat(filepath.Join(a.StorageDir, "md", "OMNGoSearch.md")); err == nil {
			t.Error("a request for the search page created an md/ source")
		}

		a.config.Update(func(c *config.Config) { c.SearchEnabled = true })
		defer a.config.Update(func(c *config.Config) { c.SearchEnabled = false })
		if a.search == nil {
			a.search = &search.Index{}
		}
		rec = getPage(t, a, "/OMNGoSearch.html?q=hello")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d with global search on", rec.Code)
		}
		if _, err := os.Stat(a.layout().PageHTML("OMNGoSearch")); err == nil {
			t.Error("the search page wrote an html/ cache; it is dynamic like Config")
		}
	})

	t.Run("ordinary note compiles and caches", func(t *testing.T) {
		rec := getPage(t, a, "/Note.html")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "hello baseline") {
			t.Error("note body missing from response")
		}
		if _, err := os.Stat(a.layout().PageHTML("Note")); err != nil {
			t.Errorf("note was not cached to html/: %v", err)
		}
	})

	t.Run("missing note is synthesized and persisted", func(t *testing.T) {
		rec := getPage(t, a, "/Ghost.html")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		src, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "Ghost.md"))
		if err != nil {
			t.Fatalf("missing note was not persisted: %v", err)
		}
		if !strings.Contains(string(src), "Title: Ghost") {
			t.Errorf("synthesized note lacks a Title header:\n%s", src)
		}
	})

	t.Run("refresh recompiles the cache", func(t *testing.T) {
		// Poison the cache, then prove ?refresh=1 rebuilt it from the source.
		if err := os.WriteFile(a.layout().PageHTML("Note"), []byte("STALE-SENTINEL"), 0644); err != nil {
			t.Fatal(err)
		}
		rec := getPage(t, a, "/Note.html?refresh=1")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "STALE-SENTINEL") {
			t.Error("?refresh=1 served the poisoned cache instead of recompiling")
		}
		onDisk, err := os.ReadFile(a.layout().PageHTML("Note"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(onDisk), "STALE-SENTINEL") {
			t.Error("?refresh=1 did not rewrite the on-disk cache")
		}
	})

	t.Run("edit intent, external editor", func(t *testing.T) {
		// A person who turns the internal editor off gets the
		// external-editor flow and not the editor page.
		//
		// loadConfig sets UseInternalEd to true on a fresh install, thus
		// the subtest sets false itself and then sets the default again.
		a.config.Update(func(c *config.Config) { c.UseInternalEd = false })
		defer a.config.Update(func(c *config.Config) { c.UseInternalEd = true })

		rec := getPage(t, a, "/Note.html?edit=true")
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status %d, want 303", rec.Code)
		}
		if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/api/edit-external?name=") {
			t.Errorf("Location %q, want the external-edit redirect", loc)
		}
	})

	t.Run("edit intent, internal editor", func(t *testing.T) {
		// The value that loadConfig writes on a fresh install. The set
		// call stays, because a subtest must not depend on the order of
		// the subtests above it.
		a.config.Update(func(c *config.Config) { c.UseInternalEd = true })
		defer a.config.Update(func(c *config.Config) { c.UseInternalEd = true })

		rec := getPage(t, a, "/Note.html?edit=true")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "OMN_EDIT_NAME") {
			t.Error("editor page did not define OMN_EDIT_NAME")
		}
		// The editor fetches the source over /api/note; it must NOT be baked
		// into the page (this is what the standalone-editor rewrite bought).
		if strings.Contains(body, "hello baseline") {
			t.Error("editor page embeds the note source; it should fetch it")
		}
	})

	t.Run("non-page asset falls through to the asset path", func(t *testing.T) {
		rec := getPage(t, a, "/js/OMN-Go/omn-go-core.js")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
			t.Errorf("Content-Type %q, want text/javascript", ct)
		}
	})
}

// ---------------------------------------------------------------------
// 2. The route set
//
// IT RECORDS THE REAL REGISTRATIONS. http.ServeMux exposes no way to
// enumerate its patterns. A test of the SOURCE of server.go would prove what
// the file says and not what the mux holds.
//
// registerRoutes takes a routeTable. The recorder below satisfies that
// interface, thus this test calls the real function and reads the pattern
// of each route that it really registers.
// ---------------------------------------------------------------------

// routeRecorder is a routeTable that stores each pattern and calls no
// handler. It answers the question "what did registerRoutes register".
type routeRecorder struct{ patterns []string }

func (r *routeRecorder) Handle(pattern string, _ http.Handler) {
	r.patterns = append(r.patterns, pattern)
}

func (r *routeRecorder) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, pattern)
}

func TestBaseline_RouteSet(t *testing.T) {
	a := newTestApp(t)
	rec := &routeRecorder{}
	a.registerRoutes(rec)

	got := append([]string(nil), rec.patterns...)
	sort.Strings(got)

	// A pattern registered two times panics inside a real ServeMux at the
	// first start. The recorder does not panic, thus this test says so.
	seen := map[string]bool{}
	for _, p := range got {
		if seen[p] {
			t.Errorf("the pattern %q is registered two times. A real ServeMux "+
				"panics on that at the first start of the application.", p)
		}
		seen[p] = true
	}

	want := []string{
		"/",
		"/api/bookmark",
		"/api/config",
		"/api/db/backup",
		"/api/db/backups",
		"/api/db/restore",
		"/api/edit-external",
		// Note exchange. Two exact patterns under /api/, so they
		// shadow nothing. Both admin only - import writes files, and export
		// is a new way out of the note tree.
		"/api/export/note",
		"/api/import/note",
		// The log stream. An exact pattern, the same as the history ring
		// below. A trailing slash on either one would make it a subtree and
		// take the other address.
		"/api/logs",
		// The history ring. Both log routes are admin only. See
		// handleLogHistory.
		"/api/logs/history",
		"/api/newpage",
		"/api/note",
		"/api/quick",
		"/api/restart",
		"/api/save",
		"/api/search",
		"/api/sql",
		// The status endpoint. An exact pattern under /api/, so it shadows nothing.
		// Admin only - the answer carries LAN addresses, absolute paths and a commit
		// subject (see internal/status/status.go).
		"/api/status",
		"/api/sync",
		"/api/sync/preview",
		// This route stores a changed git server key. It is admin only.
		// See internal/gitsync/host_keys.go.
		"/api/sync/trust-host-key",
		"/api/upload",
		// One upload route and one tree for each row of config.UserFileTrees.
		// A contact and a calendar have a tree of their own, the same as a
		// JSON file.
		"/api/upload_calendars",
		"/api/upload_contacts",
		"/api/upload_json",
		"/css/",
		"/db_backups",
		"/images/",
		"/js/",
		"/json/",
		"/login",
		"/user_calendars/",
		"/user_contacts/",
		"/user_json/",
		// These are the rows of the page-access table in page_access.go. Each is an
		// exact pattern, thus the catch-all "/" still answers each note.
		"/Config.html",
		"/OMNGoFiles.html",
		"/OMNGoLogs.html",
		"/OMNGoSearch.html",
		"/OMNGoStatus.html",
		"/OMNGoTags.html",
		// These patterns give the method of each route. Each bare path
		// above answers 405 for another method. See route in routes.go.
		"GET /Config.html",
		"GET /OMNGoFiles.html",
		"GET /OMNGoLogs.html",
		"GET /OMNGoSearch.html",
		"GET /OMNGoStatus.html",
		"GET /OMNGoTags.html",
		"GET /api/config",
		"GET /api/db/backups",
		"GET /api/edit-external",
		"GET /api/export/note",
		"GET /api/logs",
		"GET /api/logs/history",
		"GET /api/note",
		"GET /api/search",
		"GET /api/status",
		"GET /api/sync/preview",
		"GET /db_backups",
		"POST /api/bookmark",
		"POST /api/config",
		"POST /api/db/backup",
		"POST /api/db/restore",
		"POST /api/import/note",
		"POST /api/newpage",
		"POST /api/quick",
		"POST /api/restart",
		"POST /api/save",
		"POST /api/sql",
		"POST /api/sync",
		"POST /api/sync/trust-host-key",
		"POST /api/upload",
		"POST /api/upload_calendars",
		"POST /api/upload_contacts",
		"POST /api/upload_json",
		"POST /login",
	}
	sort.Strings(want)

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("registered routes changed.\n got: %v\nwant: %v\n\n"+
			"If this is intentional, update the list above in the same commit "+
			"that adds the route - and check the new pattern does not shadow an "+
			"existing prefix (ServeMux matches longest-prefix, and a trailing "+
			"slash makes a pattern a subtree).", got, want)
	}
}

// Each recorded pattern must be one that http.ServeMux accepts.
//
// ServeMux panics on an empty pattern and on a duplicate. The recorder
// above catches neither, because it is a slice. This test feeds the real
// registrations to a real mux, thus a pattern that would kill the
// application at its first start fails here instead.
func TestBaseline_RouteSetLoadsIntoARealMux(t *testing.T) {
	a := newTestApp(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registerRoutes panics against a real ServeMux: %v", r)
		}
	}()
	a.registerRoutes(http.NewServeMux())
}

// ---------------------------------------------------------------------
// 3. Heading ids
//
// goldmark's parser.WithAutoHeadingID() is what gives every heading in a note
// an anchor, and nothing tests what it actually produces. The search feature
// links results at those anchors, so its behavior on duplicates, punctuation
// and non-ASCII stops being an implementation detail.
//
// This is a CHARACTERIZATION test: the golden file records whatever the current
// goldmark version does. On first run (or with -update) it writes the file;
// after that it compares. Commit the generated testdata file - a golden nobody
// commits pins nothing.
//
// The structural assertions below the golden comparison are the parts that must
// hold regardless of goldmark version.
// ---------------------------------------------------------------------

var headingIDRe = regexp.MustCompile(`<h[1-6][^>]*\sid="([^"]*)"`)

func headingIDs(a *App, md string) []string {
	out := []string{}
	for _, m := range headingIDRe.FindAllStringSubmatch(a.testRenderer().RenderMarkdown([]byte(md)), -1) {
		out = append(out, m[1])
	}
	return out
}

func TestBaseline_HeadingIDs(t *testing.T) {
	a := &App{}

	cases := []struct{ name, md string }{
		{"plain", "# Hello World"},
		{"mixed case", "## Mixed Case Heading"},
		{"punctuation", "### A: b, c! (d)"},
		{"quicknote timestamp", "##### 2026-07-27 07:23:17"},
		{"duplicate timestamps", "##### 2026-07-27 07:23:17\n\nx\n\n##### 2026-07-27 07:23:17"},
		{"underscores and dashes", "# a_b-c d"},
		{"cyrillic", "# Заметки"},
		{"cyrillic mixed", "# Note Заметки 42"},
		{"digits only", "# 2026"},
	}

	lines := []string{
		"# Generated by TestBaseline_HeadingIDs - the ids goldmark currently emits.",
		"# Recorded to pin behavior the search feature's anchors depend on.",
	}
	for _, c := range cases {
		lines = append(lines, c.name+" => "+strings.Join(headingIDs(a, c.md), ","))
	}
	got := strings.Join(lines, "\n") + "\n"

	golden := filepath.Join("testdata", "heading_ids.golden")
	want, err := os.ReadFile(golden)
	if os.IsNotExist(err) {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded new golden %s - COMMIT IT:\n%s", golden, got)
	} else if err != nil {
		t.Fatal(err)
	} else if string(want) != got {
		t.Errorf("heading ids changed.\n got:\n%s\nwant:\n%s\n\n"+
			"If a goldmark upgrade caused this, every existing anchor in every "+
			"exported page changed with it - decide deliberately, then delete "+
			"the golden and re-record.", got, want)
	}

	// Version-independent invariants.
	dup := headingIDs(a, "##### 2026-07-27 07:23:17\n\nx\n\n##### 2026-07-27 07:23:17")
	if len(dup) != 2 {
		t.Fatalf("expected 2 heading ids for two headings, got %v", dup)
	}
	if dup[0] == dup[1] {
		t.Errorf("two identical headings produced the same id %q; anchors must be unique", dup[0])
	}

	// A heading inside a fenced block is not a heading. This is the trap a
	// naive "^#{1,6} " line scan falls into.
	if ids := headingIDs(a, "```\n# Not A Heading\n```\n"); len(ids) != 0 {
		t.Errorf("fenced '# Not A Heading' produced heading ids %v; it is code", ids)
	}

	// Report the non-ASCII outcome explicitly: whether a Cyrillic heading gets
	// a usable anchor decides whether search results can link into such a
	// section at all.
	t.Logf("cyrillic heading ids: %v (empty or degenerate means no usable anchor)",
		headingIDs(a, "# Заметки"))
}

// ---------------------------------------------------------------------
// 4. On-disk formats the search sectionizers will parse
//
// handleQuickNote and handleBookmark define a structure inside a note. A "---"
// rule and a "#####" timestamp heading separate the quick notes. A bookmark
// set is a JSON array in a <script> block. Both shapes are load-bearing for
// anything that wants to address an individual entry, and neither is asserted
// today.
// ---------------------------------------------------------------------

func TestBaseline_QuickNoteEntryFormat(t *testing.T) {
	a := newTestApp(t)
	baseWriteMD(t, a, "QuickNotes.md", "Title: Quick Notes\nDate: 2026-01-01 00:00:00\nCategory: Log\n\n")

	if rec := postForm(t, a.handleQuickNote, "/api/quick",
		url.Values{"note": {"a captured thought"}}); rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}

	src, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "QuickNotes.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Exactly: blank line, "---", "##### <timestamp>", the text.
	entryRe := regexp.MustCompile(`\n---\n##### \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\na captured thought\n`)
	if !entryRe.Match(src) {
		t.Errorf("quick-note entry shape changed; the section format is:\n%s", src)
	}
	// The entry goes below the header, not above it.
	if strings.Index(string(src), "---") < strings.Index(string(src), "Title: Quick Notes") {
		t.Errorf("entry was inserted above the header:\n%s", src)
	}
}

func TestBaseline_BookmarkStorageFormat(t *testing.T) {
	a := newTestApp(t)
	const marker = "<!-- Don't edit body below this line -->"
	baseWriteMD(t, a, "Bookmarks.md",
		"Title: Incoming bookmarks\nTags: Bookmarks\n\n<script>bookmarks = [\n"+marker+"\n];\n</script>")

	if rec := postForm(t, a.handleBookmark, "/api/bookmark", url.Values{
		"url":   {"https://example.com/a?x=1&y=2"},
		"title": {"An <b>example</b>"},
		"tags":  {"a, b"},
		"notes": {"first; second"},
	}); rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "Bookmarks.md"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)

	// The entry is inserted directly after the marker line.
	at := strings.Index(src, marker)
	if at < 0 {
		t.Fatal("marker disappeared")
	}
	after := strings.TrimLeft(src[at+len(marker):], " \t\r\n")
	if !strings.HasPrefix(after, "{") {
		t.Errorf("entry is not inserted immediately after the marker:\n%s", src)
	}

	// encoding/json escapes <, > and & in every string value. The readable
	// text is therefore NOT present in the markdown source. Anything that
	// wants to search or display bookmark content must decode the JSON first.
	// This is the single most surprising property of this file.
	for _, hex := range []string{"003c", "003e", "0026"} {
		esc := jsonUnicodeEscape(hex)
		if !strings.Contains(src, esc) {
			t.Errorf("expected JSON escape %s in the stored entry; storage format changed:\n%s", esc, src)
		}
	}
	if strings.Contains(src, "<b>example</b>") {
		t.Error("raw markup reached the file unescaped; the JSON escaping contract changed")
	}

	// And it is still valid JSON once the wrapper is stripped.
	start := strings.Index(src, "bookmarks = [")
	end := strings.LastIndex(src, "];")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("script wrapper changed shape:\n%s", src)
	}
	body := strings.TrimSpace(src[start+len("bookmarks = [") : end])
	body = strings.TrimSuffix(body, ",")
	body = strings.ReplaceAll(body, marker, "")
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &entry); err != nil {
		t.Fatalf("stored entry is not parseable JSON (%v):\n%s", err, body)
	}
	for _, k := range []string{"date", "url", "title", "tags", "notes"} {
		if _, ok := entry[k]; !ok {
			t.Errorf("bookmark entry lost its %q field: %v", k, entry)
		}
	}
	if got, ok := entry["title"].(string); !ok || got != "An <b>example</b>" {
		t.Errorf("decoded title = %v, want the original text back", entry["title"])
	}
}

// jsonUnicodeEscape builds the six-character sequence that encoding/json emits
// for a character that it escapes. An example is "003c", which gives the
// escape for '<'. The function builds the sequence by concatenation on
// purpose. Written literally, the sequence is the kind of thing that an
// editor, or a copy-paste through a JSON-aware tool, silently rewrites. That
// would make this test assert nothing.
func jsonUnicodeEscape(hex string) string { return "\\" + "u" + hex }

// ---------------------------------------------------------------------
// 5. A plain view never rewrites its source
//
// recompileMarkdownPage carries a long comment about ensureHeaderModified. A
// call to it there would re-stamp "Modified:" on every VIEW that needs a cache
// rebuild. That bug was fixed one time. Nothing fails today if it comes back.
// Anything that adds work to the view path, indexing for one, must prove that
// it kept its hands off the source.
// ---------------------------------------------------------------------

func TestBaseline_ViewDoesNotRewriteSource(t *testing.T) {
	a := newTestApp(t)
	const body = "Title: Stable\nModified: 2020-01-01 00:00:00\n\nunchanged text"
	src := baseWriteMD(t, a, "Stable.md", body)

	// Compile once, then make the source strictly newer so the next view is
	// guaranteed to take the recompile branch.
	if _, err := a.renderAndCache("Stable", []byte(body)); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(src, future, future); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}

	if rec := getPage(t, a, "/Stable.html"); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	after, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("viewing the page rewrote its .md (mtime %v -> %v)", before.ModTime(), after.ModTime())
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Errorf("viewing the page changed the source:\n got: %q\nwant: %q", got, body)
	}
}

// ---------------------------------------------------------------------
// 6. Config POST semantics
//
// The rule: a field the request does not carry keeps its value. See
// doc/decisions/0014-change-only-the-settings-that-a-request-names.md. A field
// it DOES carry is applied, empty value included, so the Config page can
// still clear a text box.
//
// A checkbox has no value to send when it is unticked. The form thus declares
// the fields that it governs in one hidden "config_fields" input. A name in
// that list counts as sent. Add every new checkbox on that form to the list.
// A new text input or select needs no entry, because a browser always sends
// those.
// ---------------------------------------------------------------------

// configFormFields is what the Config page's hidden config_fields input
// carries. A test posts the same declaration that the real form sends.
//
// It is not a copy. config.CheckboxFields reads the table in
// internal/config/fields.go, and the page fills the input from the same call. A
// test therefore cannot drift from the page.
var configFormFields = config.CheckboxFields()

// assertConfigOnDisk decodes config.json and hands it to check. It is separate
// from the in-memory assertions, because "saved" in this app means both. A
// half-applied save is exactly the kind of fault that shows up only on the
// next restart.
func assertConfigOnDisk(t *testing.T, a *App, check func(config.Config)) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	var onDisk config.Config
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("config.json is not valid JSON: %v", err)
	}
	check(onDisk)
}

func TestBaseline_ConfigPostSemantics(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) {
		c.Author = "Ann"
		c.UseInternalEd = true
		c.Theme = config.ThemeDark
		c.ServerPort = 9999
		c.MaxUploadSizeMB = 7
		c.GitServers = make([]config.GitServer, config.MaxGitServers)
	})

	// A form carrying only "theme" changes only the theme.
	rec := postForm(t, a.handleConfigPost, "/api/config", url.Values{"theme": {"light"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	cfg := a.config.Get()
	if cfg.Theme != config.ThemeLight {
		t.Errorf("theme = %q, want light", cfg.Theme)
	}
	if cfg.Author != "Ann" {
		t.Errorf("an absent text field was cleared: author = %q", cfg.Author)
	}
	if !cfg.UseInternalEd {
		t.Error("an absent checkbox was cleared: use_internal_editor")
	}
	// The numeric fields also ignore an absent or invalid value. They do that
	// for their own older reason. A 0 would be a broken port, and a cap that
	// rejects every upload.
	if cfg.ServerPort != 9999 {
		t.Errorf("server_port = %d, want 9999 kept", cfg.ServerPort)
	}
	if cfg.MaxUploadSizeMB != 7 {
		t.Errorf("max_upload_size_mb = %d, want 7 kept", cfg.MaxUploadSizeMB)
	}

	// Every save lands in config.json too. That is the file that the Android
	// layer reads natively (MainActivity, ServerService). Memory and disk must
	// agree at every step, and not eventually.
	assertConfigOnDisk(t, a, func(onDisk config.Config) {
		if onDisk.Theme != config.ThemeLight {
			t.Errorf("config.json theme = %q, want light", onDisk.Theme)
		}
		if onDisk.ServerPort != 9999 {
			t.Errorf("config.json server_port = %d, want 9999", onDisk.ServerPort)
		}
		if onDisk.Author != "Ann" {
			t.Errorf("config.json author = %q, want kept", onDisk.Author)
		}
	})

	// Explicit zero/garbage is ignored the same way an absent value is.
	postForm(t, a.handleConfigPost, "/api/config", url.Values{
		"server_port":        {"0"},
		"max_upload_size_mb": {"not-a-number"},
	})
	cfg = a.config.Get()
	if cfg.ServerPort != 9999 || cfg.MaxUploadSizeMB != 7 {
		t.Errorf("invalid numerics overwrote good values: port=%d mb=%d", cfg.ServerPort, cfg.MaxUploadSizeMB)
	}
	// ... and that POST, carrying no theme, left the theme alone.
	if got := a.config.Get().Theme; got != config.ThemeLight {
		t.Errorf("theme after a POST that omitted it = %q, want light kept", got)
	}

	// A field that IS sent applies even when its value is empty. The Config
	// page clears a text box when it sends that box empty. That must keep
	// working now that absence means something else.
	postForm(t, a.handleConfigPost, "/api/config", url.Values{"author": {""}})
	if got := a.config.Get().Author; got != "" {
		t.Errorf("a sent-but-empty field did not clear: author = %q", got)
	}

	// Only a ShareLAN flip asks for a restart. Every other change applies
	// live. The frontend keys off this exact word.
	rec = postForm(t, a.handleConfigPost, "/api/config", url.Values{"share_lan": {"true"}})
	if body := strings.TrimSpace(rec.Body.String()); body != "RestartRequired" {
		t.Errorf("share_lan flip answered %q, want RestartRequired", body)
	}
	rec = postForm(t, a.handleConfigPost, "/api/config", url.Values{"share_lan": {"true"}})
	if body := strings.TrimSpace(rec.Body.String()); body != "Saved" {
		t.Errorf("unchanged share_lan answered %q, want Saved", body)
	}
}

// This is the bug that the rule was written for. A note that saves ONE setting
// used to empty the author name, both passwords, the external-editor command
// and the device label. It also unticked every checkbox on the Config page.
//
// The device label is the field of the wrapper, and it had the same fault in a
// worse form. An absent "hostname" was rewritten to the OS-derived default.
// That default then renamed every database backup that the device wrote next.
func TestConfigPost_PartialRequestKeepsTheRest(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) {
		c.Author = "Ann"
		c.AdminPassword = "adminpw"
		c.DesktopExtCmd = "vim %s"
		c.UseInternalEd = true
		c.ShareLAN = true
		c.EnableIntentURI = true
		c.EnableTermuxIntent = true
		c.SearchEnabled = true
		c.SearchBundled = true
		c.SearchScope = config.SearchScopeAll
		c.SearchKinds = []string{config.SearchKindMD, config.SearchKindJS}
		c.AndroidFullscreen = config.FullscreenImmersive
		c.Hostname = "pixel7"
		c.BackupPruneDepth = 5
		c.GitServers = make([]config.GitServer, config.MaxGitServers)
	})

	// Exactly what the Theme Customizer note sends.
	postForm(t, a.handleConfigPost, "/api/config", url.Values{
		"theme":               {"custom"},
		"custom_theme_bg":     {"#101010"},
		"custom_theme_accent": {"#4488ff"},
	})

	cfg := a.config.Get()
	for _, f := range []struct{ name, got, want string }{
		{"author", cfg.Author, "Ann"},
		{"admin_password", cfg.AdminPassword, "adminpw"},
		{"desktop_ext_cmd", cfg.DesktopExtCmd, "vim %s"},
		{"android_fullscreen", cfg.AndroidFullscreen, config.FullscreenImmersive},
		{"search_scope", cfg.SearchScope, config.SearchScopeAll},
		{"hostname", cfg.Hostname, "pixel7"},
	} {
		if f.got != f.want {
			t.Errorf("%s = %q, want %q kept", f.name, f.got, f.want)
		}
	}
	for _, f := range []struct {
		name string
		got  bool
	}{
		{"use_internal_editor", cfg.UseInternalEd},
		{"share_lan", cfg.ShareLAN},
		{"enable_intent_uri", cfg.EnableIntentURI},
		{"enable_termux_intent", cfg.EnableTermuxIntent},
		{"search_enabled", cfg.SearchEnabled},
		{"search_bundled", cfg.SearchBundled},
	} {
		if !f.got {
			t.Errorf("%s was cleared by a request that never named it", f.name)
		}
	}
	if got := strings.Join(cfg.SearchKinds, ","); got != "md,js" {
		t.Errorf("search_kinds = %q, want md,js kept", got)
	}
	if cfg.BackupPruneDepth != 5 {
		t.Errorf("backup_prune_depth = %d, want 5 kept", cfg.BackupPruneDepth)
	}
	// "custom" is not a theme this build knows, so it lands on auto. A
	// coercion, and no longer a loss.
	if cfg.Theme != config.ThemeAuto {
		t.Errorf("theme = %q, want auto", cfg.Theme)
	}
}

// The Config page declares every checkbox it governs, so unticking one still
// clears it. Without that declaration the browser sends nothing and the
// server cannot tell "unticked" from "not mine to touch".
func TestConfigPost_DeclaredCheckboxesStillClear(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) {
		c.UseInternalEd = true
		c.ShareLAN = true
		c.EnableIntentURI = true
		c.EnableTermuxIntent = true
		c.SearchEnabled = true
		c.SearchBundled = true
		c.SearchKinds = []string{config.SearchKindMD}
		c.GitServers = make([]config.GitServer, config.MaxGitServers)
	})

	postForm(t, a.handleConfigPost, "/api/config", url.Values{
		"config_fields": {configFormFields},
		"theme":         {"light"},
	})

	cfg := a.config.Get()
	if cfg.UseInternalEd || cfg.ShareLAN || cfg.EnableIntentURI ||
		cfg.EnableTermuxIntent || cfg.SearchEnabled || cfg.SearchBundled {
		t.Errorf("a declared but unticked checkbox did not clear: %+v", cfg)
	}
	if len(cfg.SearchKinds) != 0 {
		t.Errorf("search_kinds = %v, want empty when every box is unticked", cfg.SearchKinds)
	}
	if cfg.SearchKinds == nil {
		t.Error("an empty kind list must stay non-nil, or the next load restores the default")
	}

	// A caller with no form behind it can do the same thing one field at a
	// time, by value, without declaring anything.
	a.config.Update(func(c *config.Config) { c.SearchEnabled = true; c.ShareLAN = true })
	postForm(t, a.handleConfigPost, "/api/config", url.Values{"search_enabled": {"false"}})
	cfg = a.config.Get()
	if cfg.SearchEnabled {
		t.Error("search_enabled=false did not clear it")
	}
	if !cfg.ShareLAN {
		t.Error("an unrelated checkbox was cleared by a one-field POST")
	}
}

// Every checkbox of the Config page must be in the config_fields value of
// that page. A checkbox that the list does not name cannot be cleared. A
// browser sends nothing at all for an unticked box. The server reads that
// absence as "not my business" and keeps the old value.
//
// THE TEST READS THE RENDERED PAGE. The page fills the attribute from
// config.CheckboxFields. This test therefore proves the whole path. It
// reads the table, the fill, and the markup that the browser gets.
func TestConfigPost_EveryCheckboxIsDeclared(t *testing.T) {
	// The template must hold the placeholder and no list of its own. A
	// hand-written list here would answer this test and still go out of
	// step with the table.
	if !strings.Contains(configPageTmpl, `name="config_fields" value="%%CONFIG_FIELDS%%"`) {
		t.Fatal("config_page.html does not fill config_fields from the table")
	}

	page := configPageOf(config.Config{})

	m := regexp.MustCompile(`name="config_fields" value="([^"]*)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("the rendered page carries no config_fields declaration")
	}
	declared := map[string]bool{}
	for _, f := range strings.Split(m[1], ",") {
		if f = strings.TrimSpace(f); f != "" {
			declared[f] = true
		}
	}
	if len(declared) == 0 {
		t.Fatal("the rendered config_fields value is empty")
	}

	for _, box := range regexp.MustCompile(`<input type="checkbox"[^>]*name="([^"]+)"`).
		FindAllStringSubmatch(page, -1) {
		if !declared[box[1]] {
			t.Errorf("checkbox %q is on the Config page but not in config_fields, "+
				"so unticking it cannot be saved", box[1])
		}
	}

	// And the other way. A name in the list that no checkbox carries is
	// dead text, and it makes a request govern a field it cannot send.
	boxes := map[string]bool{}
	for _, box := range regexp.MustCompile(`<input type="checkbox"[^>]*name="([^"]+)"`).
		FindAllStringSubmatch(page, -1) {
		boxes[box[1]] = true
	}
	for name := range declared {
		if !boxes[name] {
			t.Errorf("config_fields names %q, but the page carries no checkbox of that name", name)
		}
	}
}

// Clearing the hostname field on the Config page still falls back to the
// OS-derived default. That is a SENT empty value, which is a different thing
// from the absent field of the test above.
func TestConfigPost_HostnameClearedFallsBack(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) {
		c.Hostname = "pixel7"
		c.GitServers = make([]config.GitServer, config.MaxGitServers)
	})
	postForm(t, a.handleConfigPost, "/api/config", url.Values{"hostname": {""}})
	if got := a.config.Get().Hostname; got == "" || got == "pixel7" {
		t.Errorf("hostname = %q, want the OS-derived default", got)
	}
}

// ---------------------------------------------------------------------
// 7. precompileAllPages
//
// The startup pass that compiles every note. It is untested today. It is also
// where any future "warm up something at startup" work will be attached. Pin
// what it guarantees now: every note compiled, nested notes included, the Tags
// page generated, and no source touched.
// ---------------------------------------------------------------------

func TestBaseline_PrecompileAllPages(t *testing.T) {
	a := newTestApp(t)
	notes := map[string]string{
		"Top.md":          "Title: Top\n\ntop body",
		"sub/Nested.md":   "Title: Nested\nTags: X\n\nnested body",
		"sub/deep/Far.md": "Title: Far\n\nfar body",
	}
	for rel, body := range notes {
		baseWriteMD(t, a, rel, body)
	}

	a.precompileAllPages()

	for rel := range notes {
		name := strings.TrimSuffix(rel, ".md")
		if _, err := os.Stat(a.layout().PageHTML(name)); err != nil {
			t.Errorf("%s was not compiled: %v", rel, err)
		}
		got, err := os.ReadFile(filepath.Join(a.StorageDir, "md", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != notes[rel] {
			t.Errorf("%s: precompile modified the source", rel)
		}
	}

	// The Tags index is generated at the end of the pass, so an export taken
	// after startup always contains it.
	if _, err := os.Stat(a.layout().PageHTML("OMNGoTags")); err != nil {
		t.Errorf("OMNGoTags.html not generated by precompileAllPages: %v", err)
	}
}

// ---------------------------------------------------------------------
// 8. The injected runtime-variable set
//
// Cached pages carry a marker that injectRuntimeVars fills per request with the
// values that must reflect the RUNNING server rather than compile time. Which
// globals those are is a contract with the frontend. A written set makes the
// addition of one global a deliberate edit, and not a silent drift.
// ---------------------------------------------------------------------

var runtimeVarRe = regexp.MustCompile(`var ([A-Za-z_][A-Za-z0-9_]*) =`)

func TestBaseline_InjectedRuntimeVarSet(t *testing.T) {
	a := newTestApp(t)

	page := a.injectRuntimeVars([]byte(render.RuntimeVarsMarker))
	names := []string{}
	for _, m := range runtimeVarRe.FindAllStringSubmatch(string(page), -1) {
		names = append(names, m[1])
	}
	sort.Strings(names)

	// OMN_SEARCH_GLOBAL: the offer of the "All notes" scope in the dialog
	// depends on a setting that a person can change at any time. The value
	// must thus reach an already-cached page, the same way the theme does.
	//
	// OMN_INCOMING_PAGE: the receive box lives in the modals block.
	// omn-go-api.js must know which page the box belongs on, and it must keep
	// no second copy of the name of the note.
	//
	// OMN_USER_FILE_UPLOADS: the receive box sends a JSON file, a contact
	// and a calendar to the upload route of its tree. The value comes from
	// config.UserFileTrees, thus omn-go-api.js keeps no copy of that table.
	//
	// The three log switches: the console mirror in omn-go-api.js reads them
	// to decide what it prints. A page compiled before a switch changed must
	// still get the new answer.
	want := []string{
		"APP_VERSION", "OMN_INCOMING_PAGE", "OMN_LOG_DEBUG", "OMN_LOG_INFO",
		"OMN_LOG_TAGS", "OMN_SEARCH_GLOBAL", "OMN_THEME", "OMN_USER_FILE_UPLOADS",
		"USE_INTERNAL_ED",
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("injected runtime globals changed: got %v, want %v\n"+
			"Adding one is fine - update this list in the same commit, and "+
			"remember every already-cached page picks it up for free, which is "+
			"the whole reason this mechanism exists.", names, want)
	}

	// The marker survives into the on-disk cache RAW. It is filled at serve
	// time. If it were filled at compile time, changing the theme would mean
	// recompiling every page.
	baseWriteMD(t, a, "Marked.md", "Title: Marked\n\nbody")
	if _, err := a.renderAndCache("Marked", []byte("Title: Marked\n\nbody")); err != nil {
		t.Fatal(err)
	}
	cached, err := os.ReadFile(a.layout().PageHTML("Marked"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cached), render.RuntimeVarsMarker) {
		t.Error("cached page does not contain the raw runtime-vars marker")
	}
	if !strings.Contains(string(cached), render.ModalsMarker) {
		t.Error("cached page does not contain the raw modals slot")
	}
	if strings.Contains(string(a.injectRuntimeVars(cached)), render.RuntimeVarsMarker) {
		t.Error("marker survived injection at serve time")
	}
}

// ---------------------------------------------------------------------
// 9. The compiled-page shape, across every write path
//
// Five different handlers write html/<name>.html through renderAndCache. All
// five must make a page that the frontend can run.
// ---------------------------------------------------------------------

func assertCachedPageShape(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("compiled html not written at %s: %v", path, err)
	}
	html := string(data)
	for _, want := range []string{
		`<meta id="omn-go-runtime-vars-marker">`,
		`<div id="omn-preview">`,
		"var IS_MARKDOWN = true;",
		`content="OMN-Go ` + version + `"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("%s: cached page missing %q", filepath.Base(path), want)
		}
	}
	if strings.Contains(html, "%%") {
		t.Errorf("%s: cached page has an unfilled template placeholder", filepath.Base(path))
	}
}

func TestBaseline_CompiledHTMLShapeAcrossWritePaths(t *testing.T) {
	a := newTestApp(t)
	htmlDir := filepath.Join(a.StorageDir, "html")

	baseWriteMD(t, a, "QuickNotes.md", "Title: Quick Notes\nDate: 2026-01-01 00:00:00\nCategory: Log\n\n")
	baseWriteMD(t, a, "Bookmarks.md",
		"Title: Incoming bookmarks\nTags: Bookmarks\n\n<script>bookmarks = [\n<!-- Don't edit body below this line -->\n];\n</script>")
	baseWriteMD(t, a, "Home.md", "Title: Home\nDate: 2026-01-01 00:00:00\n\nWelcome")

	if rec := postForm(t, a.handleSaveNote, "/api/save",
		url.Values{"name": {"SaveMe"}, "content": {"Title: SaveMe\n\nHello **bold**"}}); rec.Code != http.StatusOK {
		t.Fatalf("save: status %d, body %s", rec.Code, rec.Body.String())
	}
	assertCachedPageShape(t, filepath.Join(htmlDir, "SaveMe.html"))

	if rec := postForm(t, a.handleQuickNote, "/api/quick",
		url.Values{"note": {"a captured thought"}}); rec.Code != http.StatusOK {
		t.Fatalf("quicknote: status %d, body %s", rec.Code, rec.Body.String())
	}
	assertCachedPageShape(t, filepath.Join(htmlDir, "QuickNotes.html"))

	if rec := postForm(t, a.handleBookmark, "/api/bookmark",
		url.Values{"url": {"https://example.com"}, "title": {"Example"}, "tags": {"a, b"}, "notes": {"note"}}); rec.Code != http.StatusOK {
		t.Fatalf("bookmark: status %d, body %s", rec.Code, rec.Body.String())
	}
	assertCachedPageShape(t, filepath.Join(htmlDir, "Bookmarks.html"))

	if rec := postForm(t, a.handleNewPage, "/api/newpage",
		url.Values{"source": {"Home"}, "target": {"Child"}, "title": {"Child"}}); rec.Code != http.StatusOK {
		t.Fatalf("newpage: status %d, body %s", rec.Code, rec.Body.String())
	}
	assertCachedPageShape(t, filepath.Join(htmlDir, "Home.html"))
}

// ---------------------------------------------------------------------
// 10. The /api/logs SSE lifecycle
//
// A page that holds its EventSource open forever stalls the desktop
// connections. The client closes it on pagehide. This is the server half.
// handleLogsSSE must register a client on connect, and it must DE-register
// that client when the request context ends.
// ---------------------------------------------------------------------

func countLogClients(a *App) int {
	return a.logs.ClientCount()
}

func waitFor(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func TestBaseline_LogsSSERegistersAndReleasesClient(t *testing.T) {
	a := newTestApp(t)
	srv := httptest.NewServer(http.HandlerFunc(a.handleLogsSSE))
	defer srv.Close()

	base := countLogClients(a)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The handler blocks until a log line arrives, so Do() will not return
	// until we cancel - run it in the background. Registration happens as soon
	// as the handler runs, regardless of whether bytes reached the client.
	done := make(chan struct{})
	go func() {
		resp, derr := (&http.Client{}).Do(req)
		if derr == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		close(done)
	}()

	if !waitFor(func() bool { return countLogClients(a) == base+1 }, 2*time.Second) {
		cancel()
		<-done
		t.Fatalf("SSE client was never registered (clients=%d, want %d)", countLogClients(a), base+1)
	}

	cancel()
	if !waitFor(func() bool { return countLogClients(a) == base }, 2*time.Second) {
		<-done
		t.Fatalf("SSE client not released on disconnect (clients=%d, want %d)", countLogClients(a), base)
	}
	<-done
}
