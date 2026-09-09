package backend

// BASELINE: what OMN-Go does today, pinned before the search feature is built.
//
// This file replaces phase0_regression_test.go, which was the safety net for
// an EARLIER plan. Its comments are written in the future tense about
// refactors that have since shipped, such as "Phase 1 will replace them with
// one parseFrontMatter". Two of its four tests asserted post-refactor
// behavior while they still carried the pre-refactor name.
//
// What those two guarded is now covered directly by header_block_test.go and
// markdown_test.go. Two others are still load-bearing, and they moved here.
// Those are the shape of a compiled page across every write path, and the
// /api/logs SSE lifecycle. They are re-commented as statements about today,
// and not as promises about tomorrow.
//
// The rest of the file pins behavior that the search work is about to lean
// on or walk past. One question chose it. "What would break silently if the
// search work got something wrong, and is asserted nowhere today?"
//
// NOTHING HERE TESTS SEARCH. Every test must pass on the current tree, before a
// single line of search code exists, and keep passing after. Two exceptions are
// planned and named at the point they happen:
//
//   - S2 adds /api/search to TestBaseline_RouteSet (done)
//   - S4 adds OMN_SEARCH_GLOBAL to TestBaseline_InjectedRuntimeVarSet (done)
//   - S7 adds the OMNGoSearch arm to TestBaseline_ServeHTMLPageDispatch (done)
//   - 26.08.2 changes that same arm (done). With global search off, the page
//     no longer answers 404. It explains how to turn it on. This was not a
//     planned edit, but a reversal. S7 argued that a permanently empty
//     results page was worse than an honest miss. That was wrong about who
//     arrives here. The address is linkable, and people put a "Search" link
//     on their Welcome note. The 404 was thus a dead end that named neither
//     the cause nor the cure.
//   - 26.08.3 adds /OMNGoFiles.html to TestBaseline_RouteSet (done). The
//     directory index is a page, but it is admin-only. The catch-all that
//     serves every other page is unauthenticated. The index thus takes a
//     route of its own, next to /db_backups, which is there for the same
//     reason.
//   - 26.08.35 adds /api/export/note and /api/import/note to
//     TestBaseline_RouteSet (done). Note exchange, phase 2 of
//     claude/note-exchange-plan.md. Two exact patterns under /api/, both
//     behind authMiddleware with requireAdmin.
//   - 26.08.47 adds OMN_INCOMING_PAGE to TestBaseline_InjectedRuntimeVarSet
//     (done). The receive box moved out of the incoming index note and into
//     modals.html. omn-go-sse.js thus has to be told which page it belongs
//     on, and the name stays in Go, beside the code that writes that page.
//   - 26.08.71 adds OMN_LOG_DEBUG, OMN_LOG_INFO and OMN_LOG_TAGS to
//     TestBaseline_InjectedRuntimeVarSet (done), and log_debug, log_info and
//     log_tags to configFormFields. Every page mirrors the server log into
//     the browser console, and omn-go-sse.js reads these three to decide
//     what it prints. They must reach a page compiled before the switches
//     changed, which is what this mechanism is for.
//   - 26.09.39 adds /api/logs/history to TestBaseline_RouteSet (done). The
//     ring of the last 500 log lines. An exact pattern beside /api/logs,
//     which stays an exact pattern as well. A trailing slash on either one
//     would make it a subtree and take the other address. Both are admin
//     only since 26.09.59, and handleLogHistory says why.
//
// A baseline test failing for any other reason means the change under it was
// not as behaviour-preserving as it looked.
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
)

// ---------------------------------------------------------------------
// Shared helpers (moved from phase0_regression_test.go)
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

// getPage issues a browser-shaped GET, with Accept: text/html, through
// serveFrontend. That is the handler registered at "/". The test thus
// exercises the real dispatch chain, and it does not call an inner handler
// directly.
func getPage(t *testing.T, a *App, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	rec := httptest.NewRecorder()
	a.serveFrontend(rec, req)
	return rec
}

// ---------------------------------------------------------------------
// 1. serveHTMLPage dispatch
//
// serveFrontend -> serveHTMLPage is a switch with several arms. Those arms are
// Config, OMNGoTags, an ordinary note, a missing note, ?refresh, ?edit and a
// non-page asset. Nothing tests the switch AS A WHOLE, because the handler
// tests all call the inner functions directly. The search feature adds another
// arm to it (OMNGoSearch). That is exactly the kind of edit that can silently
// reorder or shadow an existing arm.
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
		if !strings.Contains(rec.Body.String(), `id="configForm"`) {
			t.Error("Config.html did not render the config form")
		}
		// Dynamic, NOT cached: no md source and no compiled artefact.
		if _, err := os.Stat(filepath.Join(a.StorageDir, "md", "Config.md")); err == nil {
			t.Error("Config.html created an md/ source; it must stay dynamic")
		}
		if _, err := os.Stat(a.pageHTMLPath("Config")); err == nil {
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
		if _, err := os.Stat(a.pageHTMLPath("OMNGoTags")); err != nil {
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

		a.WithConfig(func(c *Config) { c.SearchEnabled = true })
		defer a.WithConfig(func(c *Config) { c.SearchEnabled = false })
		if a.search == nil {
			a.search = &searchIndex{}
		}
		rec = getPage(t, a, "/OMNGoSearch.html?q=hello")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d with global search on", rec.Code)
		}
		if _, err := os.Stat(a.pageHTMLPath("OMNGoSearch")); err == nil {
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
		if _, err := os.Stat(a.pageHTMLPath("Note")); err != nil {
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
		if err := os.WriteFile(a.pageHTMLPath("Note"), []byte("STALE-SENTINEL"), 0644); err != nil {
			t.Fatal(err)
		}
		rec := getPage(t, a, "/Note.html?refresh=1")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "STALE-SENTINEL") {
			t.Error("?refresh=1 served the poisoned cache instead of recompiling")
		}
		onDisk, err := os.ReadFile(a.pageHTMLPath("Note"))
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
		// This subtest set no value until 26.09.18. It read the zero
		// value of the field, and the comment here called that value the
		// default. loadConfig sets UseInternalEd to true on a fresh
		// install, thus the comment was wrong and the subtest tested the
		// state of no user.
		a.WithConfig(func(c *Config) { c.UseInternalEd = false })
		defer a.WithConfig(func(c *Config) { c.UseInternalEd = true })

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
		a.WithConfig(func(c *Config) { c.UseInternalEd = true })
		defer a.WithConfig(func(c *Config) { c.UseInternalEd = true })

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
// IT RECORDS THE REAL REGISTRATIONS SINCE 26.09.32. http.ServeMux exposes
// no way to enumerate its patterns. The block also sat inside the goroutine
// that binds the socket. This test therefore read the SOURCE of server.go
// with a regular expression, and it proved what the file says and not what
// the mux holds. A route that a helper registered, or a pattern that the
// expression did not match, was invisible to it.
//
// registerRoutes takes a routeTable now. The recorder below satisfies that
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
		// 26.08.35: note exchange. Two exact patterns under /api/, so they
		// shadow nothing. Both admin only - import writes files, and export
		// is a new way out of the note tree.
		"/api/export/note",
		"/api/import/note",
		// 26.09.32: registered by registerRoutes and no longer by
		// logger.go, thus one block holds every route.
		"/api/logs",
		// 26.09.39: the history ring. An exact pattern, thus it shadows
		// nothing and /api/logs still matches its own address alone. Both
		// are admin only since 26.09.59. See handleLogHistory.
		"/api/logs/history",
		"/api/newpage",
		"/api/note",
		"/api/quick",
		"/api/restart",
		"/api/save",
		"/api/search",
		"/api/sql",
		// 26.08.14: the status endpoint. An exact pattern under /api/, so
		// it shadows nothing. Admin only - the answer carries LAN
		// addresses, absolute paths and a commit subject (see status.go).
		"/api/status",
		"/api/sync",
		"/api/sync/preview",
		"/api/upload",
		"/api/upload_json",
		"/css/",
		"/db_backups",
		"/images/",
		"/js/",
		"/json/",
		"/login",
		"/user_json/",
		// 26.08.3: the directory index. An exact pattern, so it shadows
		// nothing - the catch-all "/" still answers every other page. It is
		// registered here rather than dispatched from serveHTMLPage because it
		// is a page that needs authorization, and the catch-all is
		// unauthenticated.
		"/OMNGoFiles.html",
		// 26.08.16: the Status page. An exact pattern, like the file index
		// above, and admin-only through hasRole inside the handler.
		"/OMNGoStatus.html",
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
// links results at those anchors, so its behaviour on duplicates, punctuation
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
	for _, m := range headingIDRe.FindAllStringSubmatch(a.renderMarkdownToHTML([]byte(md)), -1) {
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
		"# Recorded to pin behaviour the search feature's anchors depend on.",
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
// CHANGED IN 26.08.43. handleConfig used to rebuild the config from the form.
// An ABSENT field was thus not "leave it alone". A missing checkbox read as
// false, and a missing text input read as "". That was invisible while the
// Config page was the only caller, because that page sends the whole form.
// Then a note posted "theme" on its own. It emptied the author name, both
// passwords, the external-editor command and the device label.
//
// The rule now: a field the request does not carry is left as it is. A field
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
// It is not a copy any more. configCheckboxFields reads the table in
// config_fields.go, and the page fills the input from the same call since
// 26.09.19. A test therefore cannot drift from the page.
var configFormFields = configCheckboxFields()

// assertConfigOnDisk decodes config.json and hands it to check. It is separate
// from the in-memory assertions, because "saved" in this app means both. A
// half-applied save is exactly the kind of fault that shows up only on the
// next restart.
func assertConfigOnDisk(t *testing.T, a *App, check func(Config)) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	var onDisk Config
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("config.json is not valid JSON: %v", err)
	}
	check(onDisk)
}

func TestBaseline_ConfigPostSemantics(t *testing.T) {
	a := newTestApp(t)
	a.WithConfig(func(c *Config) {
		c.Author = "Ann"
		c.UseInternalEd = true
		c.Theme = ThemeDark
		c.ServerPort = 9999
		c.MaxUploadSizeMB = 7
		c.GitServers = make([]GitServerConfig, maxGitServers)
	})

	// A form carrying only "theme" changes only the theme.
	rec := postForm(t, a.handleConfig, "/api/config", url.Values{"theme": {"light"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	cfg := a.GetConfig()
	if cfg.Theme != ThemeLight {
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
	assertConfigOnDisk(t, a, func(onDisk Config) {
		if onDisk.Theme != ThemeLight {
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
	postForm(t, a.handleConfig, "/api/config", url.Values{
		"server_port":        {"0"},
		"max_upload_size_mb": {"not-a-number"},
	})
	cfg = a.GetConfig()
	if cfg.ServerPort != 9999 || cfg.MaxUploadSizeMB != 7 {
		t.Errorf("invalid numerics overwrote good values: port=%d mb=%d", cfg.ServerPort, cfg.MaxUploadSizeMB)
	}
	// ... and that POST, carrying no theme, left the theme alone.
	if got := a.GetConfig().Theme; got != ThemeLight {
		t.Errorf("theme after a POST that omitted it = %q, want light kept", got)
	}

	// A field that IS sent applies even when its value is empty. The Config
	// page clears a text box when it sends that box empty. That must keep
	// working now that absence means something else.
	postForm(t, a.handleConfig, "/api/config", url.Values{"author": {""}})
	if got := a.GetConfig().Author; got != "" {
		t.Errorf("a sent-but-empty field did not clear: author = %q", got)
	}

	// Only a ShareLAN flip asks for a restart. Every other change applies
	// live. The frontend keys off this exact word.
	rec = postForm(t, a.handleConfig, "/api/config", url.Values{"share_lan": {"true"}})
	if body := strings.TrimSpace(rec.Body.String()); body != "RestartRequired" {
		t.Errorf("share_lan flip answered %q, want RestartRequired", body)
	}
	rec = postForm(t, a.handleConfig, "/api/config", url.Values{"share_lan": {"true"}})
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
	a.WithConfig(func(c *Config) {
		c.Author = "Ann"
		c.AdminPassword = "adminpw"
		c.GuestPassword = "guestpw"
		c.DesktopExtCmd = "vim %s"
		c.UseInternalEd = true
		c.ShareLAN = true
		c.EnableIntentURI = true
		c.EnableTermuxIntent = true
		c.SearchEnabled = true
		c.SearchBundled = true
		c.SearchScope = SearchScopeAll
		c.SearchKinds = []string{SearchKindMD, SearchKindJS}
		c.AndroidFullscreen = FullscreenImmersive
		c.Hostname = "pixel7"
		c.BackupPruneDepth = 5
		c.GitServers = make([]GitServerConfig, maxGitServers)
	})

	// Exactly what the Theme Customizer note sends.
	postForm(t, a.handleConfig, "/api/config", url.Values{
		"theme":               {"custom"},
		"custom_theme_bg":     {"#101010"},
		"custom_theme_accent": {"#4488ff"},
	})

	cfg := a.GetConfig()
	for _, f := range []struct{ name, got, want string }{
		{"author", cfg.Author, "Ann"},
		{"admin_password", cfg.AdminPassword, "adminpw"},
		{"guest_password", cfg.GuestPassword, "guestpw"},
		{"desktop_ext_cmd", cfg.DesktopExtCmd, "vim %s"},
		{"android_fullscreen", cfg.AndroidFullscreen, FullscreenImmersive},
		{"search_scope", cfg.SearchScope, SearchScopeAll},
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
	if cfg.Theme != ThemeAuto {
		t.Errorf("theme = %q, want auto", cfg.Theme)
	}
}

// The Config page declares every checkbox it governs, so unticking one still
// clears it. Without that declaration the browser sends nothing and the
// server cannot tell "unticked" from "not mine to touch".
func TestConfigPost_DeclaredCheckboxesStillClear(t *testing.T) {
	a := newTestApp(t)
	a.WithConfig(func(c *Config) {
		c.UseInternalEd = true
		c.ShareLAN = true
		c.EnableIntentURI = true
		c.EnableTermuxIntent = true
		c.SearchEnabled = true
		c.SearchBundled = true
		c.SearchKinds = []string{SearchKindMD}
		c.GitServers = make([]GitServerConfig, maxGitServers)
	})

	postForm(t, a.handleConfig, "/api/config", url.Values{
		"config_fields": {configFormFields},
		"theme":         {"light"},
	})

	cfg := a.GetConfig()
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
	a.WithConfig(func(c *Config) { c.SearchEnabled = true; c.ShareLAN = true })
	postForm(t, a.handleConfig, "/api/config", url.Values{"search_enabled": {"false"}})
	cfg = a.GetConfig()
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
// THE TEST READS THE RENDERED PAGE. The list was a hand-written attribute
// of config_page.html until 26.09.19, and a test of the template caught a
// forgotten name. The page now fills the attribute from
// configCheckboxFields. This test therefore proves the whole path. It
// reads the table, the fill, and the markup that the browser gets.
func TestConfigPost_EveryCheckboxIsDeclared(t *testing.T) {
	// The template must hold the placeholder and no list of its own. A
	// hand-written list here would answer this test and still go out of
	// step with the table.
	if !strings.Contains(configPageTmpl, `name="config_fields" value="%%CONFIG_FIELDS%%"`) {
		t.Fatal("config_page.html does not fill config_fields from the table")
	}

	page := renderConfigPage(configPageView{})

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
	a.WithConfig(func(c *Config) {
		c.Hostname = "pixel7"
		c.GitServers = make([]GitServerConfig, maxGitServers)
	})
	postForm(t, a.handleConfig, "/api/config", url.Values{"hostname": {""}})
	if got := a.GetConfig().Hostname; got == "" || got == "pixel7" {
		t.Errorf("hostname = %q, want the OS-derived default", got)
	}
}

// ---------------------------------------------------------------------
// 7. versionDependentAssets and gitignorePatterns agree
//
// versionDependentAssets (assets.go) is the list of files that ship with the
// build and are refreshed on upgrade. gitignorePatterns (git_repo.go) keeps
// those same files out of the sync repo of the user. They are two hand-kept
// lists that must not drift. The search feature also makes the first list the
// single source of truth for the own code of OMN-Go. Its integrity thus
// matters more than it did. TestVersionDependentAssetsAllEmbedded covers the
// embed side. This test is the other half.
// ---------------------------------------------------------------------

func TestBaseline_VersionDependentAssetsAreGitignored(t *testing.T) {
	ignored := map[string]bool{}
	for _, p := range gitignorePatterns {
		ignored[p] = true
	}
	for _, rel := range versionDependentAssets {
		if !ignored["/"+rel] {
			t.Errorf("versionDependentAssets has %q but gitignorePatterns has no %q - "+
				"a shipped file that gets committed to the user's repo will "+
				"conflict on every upgrade", rel, "/"+rel)
		}
	}
}

// ---------------------------------------------------------------------
// 8. precompileAllPages
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
		if _, err := os.Stat(a.pageHTMLPath(name)); err != nil {
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
	if _, err := os.Stat(a.pageHTMLPath("OMNGoTags")); err != nil {
		t.Errorf("OMNGoTags.html not generated by precompileAllPages: %v", err)
	}
}

// ---------------------------------------------------------------------
// 9. The injected runtime-variable set
//
// Cached pages carry a marker that injectRuntimeVars fills per request with the
// values that must reflect the RUNNING server rather than compile time. Which
// globals those are is a contract with the frontend. A written set makes the
// addition of one global a deliberate edit, and not a silent drift.
// ---------------------------------------------------------------------

var runtimeVarRe = regexp.MustCompile(`var ([A-Za-z_][A-Za-z0-9_]*) =`)

func TestBaseline_InjectedRuntimeVarSet(t *testing.T) {
	a := newTestApp(t)

	page := a.injectRuntimeVars([]byte(runtimeVarsMarker))
	names := []string{}
	for _, m := range runtimeVarRe.FindAllStringSubmatch(string(page), -1) {
		names = append(names, m[1])
	}
	sort.Strings(names)

	// S4 added OMN_SEARCH_GLOBAL. The offer of the "All notes" scope in the
	// dialog depends on a setting that a person can change at any time. The
	// value must thus reach an already-cached page, the same way the theme
	// does.
	// 26.08.47 added OMN_INCOMING_PAGE. The receive box lives in the modals
	// block now. omn-go-sse.js must know which page the box belongs on, and it
	// must keep no second copy of the name of the note.
	// 26.08.71 added the three log switches. The console mirror in
	// omn-go-sse.js reads them to decide what it prints. A page compiled
	// before a switch changed must still get the new answer.
	want := []string{
		"APP_VERSION", "OMN_INCOMING_PAGE", "OMN_LOG_DEBUG", "OMN_LOG_INFO",
		"OMN_LOG_TAGS", "OMN_SEARCH_GLOBAL", "OMN_THEME", "USE_INTERNAL_ED",
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
	cached, err := os.ReadFile(a.pageHTMLPath("Marked"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cached), runtimeVarsMarker) {
		t.Error("cached page does not contain the raw runtime-vars marker")
	}
	if !strings.Contains(string(cached), modalsMarker) {
		t.Error("cached page does not contain the raw modals slot")
	}
	if strings.Contains(string(a.injectRuntimeVars(cached)), runtimeVarsMarker) {
		t.Error("marker survived injection at serve time")
	}
}

// ---------------------------------------------------------------------
// 10. The compiled-page shape, across every write path
//
// Moved from phase0_regression_test.go, where it was written to prove a
// refactor that has since shipped. It is kept because the invariant is still
// the one that matters. Five different handlers write html/<name>.html through
// renderAndCache. All five must keep making a page that the frontend can run.
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
		`<div id="preview">`,
		"var IS_MARKDOWN = true;",
		`content="OMN-Go ` + APP_VERSION + `"`,
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
// 11. The /api/logs SSE lifecycle
//
// Also moved from phase0_regression_test.go. The desktop connection-stall bug
// was every page that held its EventSource open forever. The client half
// closes it on pagehide. This is the server half. HandleLogsSSE must register
// a client on connect, and it must DE-register that client when the request
// context is canceled. Break the deferred cleanup and the leak returns.
// ---------------------------------------------------------------------

func countLogClients() int {
	logMutex.Lock()
	defer logMutex.Unlock()
	return len(logClients)
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
	srv := httptest.NewServer(http.HandlerFunc(a.HandleLogsSSE))
	defer srv.Close()

	base := countLogClients()

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

	if !waitFor(func() bool { return countLogClients() == base+1 }, 2*time.Second) {
		cancel()
		<-done
		t.Fatalf("SSE client was never registered (clients=%d, want %d)", countLogClients(), base+1)
	}

	cancel()
	if !waitFor(func() bool { return countLogClients() == base }, 2*time.Second) {
		<-done
		t.Fatalf("SSE client not released on disconnect (clients=%d, want %d)", countLogClients(), base)
	}
	<-done
}
