package backend

// Phase 0 of the code-review improvement plan: "lock in current behavior".
//
// These tests are a SAFETY NET, not a spec. Before the later phases
// consolidate the duplicated front-matter parsing (Phase 1), unify the
// HTML compile/cache write paths (Phase 2), and split the frontend SSE
// module (Phase 5), we pin the behavior those refactors must preserve -
// including a couple of current quirks that are surprising but real, so a
// change to them shows up as a failing test rather than a silent
// regression.
//
// Nothing here changes production code. Each test documents WHY it exists
// so a future maintainer knows whether a failure means "you broke it" or
// "you intentionally changed it, update the golden value".
//
// Reuses newTestApp / postConfig-style helpers from handlers_test.go (same
// package).

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------
// Phase 1 target: front-matter ("Pelican header") classification.
//
// The review flagged that "is this block a header?" is decided in four
// slightly different places. Phase 1 will replace them with one
// parseFrontMatter. There is no single parser to golden-test yet, so
// instead we characterize the CURRENT observable behavior of the two Go
// implementations - including the point where they DISAGREE - so the merge
// can be proven behavior-preserving (or a deliberate change is made
// visible).
// ---------------------------------------------------------------------

func TestPhase0_FrontMatterClassification_CompilePage(t *testing.T) {
	a := &App{}

	// A normal header block: every "key: value" line becomes a <meta> tag
	// and is excluded from the rendered body; the first blank line ends the
	// header, so a '#' heading after it renders as a real heading.
	out := string(a.compilePage("P", []byte("Title: Doc\nAuthor: Ann\n\n# Real Heading\n\nprose")))
	if !strings.Contains(out, `name="author" content="Ann"`) {
		t.Errorf("header line not emitted as meta tag:\n%s", out)
	}
	if !strings.Contains(out, "<h1") {
		t.Errorf("body heading (after the header block) did not render as <h1>:\n%s", out)
	}
	if strings.Contains(out, "%%") {
		t.Errorf("unfilled template placeholder in compiled page:\n%s", out)
	}

	// PHASE 1 (was a pinned quirk): the Phase 0 net recorded that
	// compilePageWithBody used to classify a header line purely by "does it
	// contain a colon", so a Markdown heading whose text happens to contain
	// a colon ("# Head: x") as the first line was swallowed as a bogus meta
	// tag and vanished from the body. Phase 1 unified header detection on
	// splitFrontMatter, whose first-line rule rejects a '#'-prefixed line -
	// so the heading now renders as <h1> and is NOT treated as metadata.
	// This is the intentional behavior change the Phase 0 test guarded.
	out = string(a.compilePage("P", []byte("# Head: x\n\nBody")))
	if strings.Contains(out, `name="# head"`) {
		t.Errorf("colon-bearing heading is still wrongly treated as a header line:\n%s", out)
	}
	if !strings.Contains(out, "<h1") {
		t.Errorf("colon-bearing heading should now render as <h1> in the body:\n%s", out)
	}
	if !strings.Contains(out, "<p>Body</p>") {
		t.Errorf("body after the heading not rendered as expected:\n%s", out)
	}
}

func TestPhase0_FrontMatterClassification_UnifiedAcrossFunctions(t *testing.T) {
	a := &App{}

	// Before Phase 1 the two implementations DISAGREED on "# Head: x":
	// compilePageWithBody treated it as a header line, ensureHeaderModified
	// treated it as body. Phase 1 routed both through splitFrontMatter, so
	// they now agree - the first line is body (a '#'-prefixed line is not a
	// metadata key line). This test guards that they stay unified.

	// ensureHeaderModified: classifies as body -> synthesizes a fresh header
	// above the verbatim content.
	em := a.ensureHeaderModified("# Head: x\n\nBody", "P")
	if !strings.Contains(em, "Title: P") {
		t.Errorf("ensureHeaderModified no longer synthesizes a header for a '#' first line:\n%s", em)
	}
	if !strings.Contains(em, "# Head: x\n\nBody") {
		t.Errorf("ensureHeaderModified did not preserve the original body verbatim:\n%s", em)
	}
	if strings.Count(em, "Modified:") != 1 {
		t.Errorf("expected exactly one Modified line in the synthesized header:\n%s", em)
	}

	// compilePageWithBody: also classifies as body -> renders as <h1>, emits
	// no header-derived meta tag. (The full assertions live in
	// TestPhase0_FrontMatterClassification_CompilePage; this is the
	// agreement check.)
	cp := string(a.compilePage("P", []byte("# Head: x\n\nBody")))
	if strings.Contains(cp, `name="# head"`) {
		t.Errorf("compilePageWithBody still disagrees - treats '# Head: x' as a header line:\n%s", cp)
	}
	if !strings.Contains(cp, "<h1") {
		t.Errorf("compilePageWithBody should render '# Head: x' as body <h1>:\n%s", cp)
	}
}

// ---------------------------------------------------------------------
// Phase 2 target: the compiled-HTML cache write paths.
//
// Five handlers each open-code "recompile this page to html/<name>.html".
// Phase 2 will funnel them through one writer. This test pins the invariant
// shape all of them currently produce, so the funnel can be proven to keep
// producing the same on-disk cache. It exercises four of the five write
// paths end to end (save, quick note, bookmark, new page).
// ---------------------------------------------------------------------

// assertCachedPageShape checks the invariants every compile path currently
// bakes into an on-disk html/<name>.html cache file: the runtime-vars
// marker (filled per request by injectRuntimeVars at serve time, so it is
// present RAW on disk), the preview container, the markdown-page flag, the
// generator meta stamp, and no unfilled %% placeholders.
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

func postForm(t *testing.T, h http.HandlerFunc, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestPhase0_CompiledHTMLShapeAcrossWritePaths(t *testing.T) {
	a := newTestApp(t)
	mdDir := filepath.Join(a.StorageDir, "md")
	htmlDir := filepath.Join(a.StorageDir, "html")

	// Seed the two pages whose handlers require pre-existing content.
	// QuickNotes just needs a header + blank line; Bookmarks needs its
	// "don't edit below" marker or handleBookmark writes nothing.
	if err := os.WriteFile(filepath.Join(mdDir, "QuickNotes.md"),
		[]byte("Title: Quick Notes\nDate: 2026-01-01 00:00:00\nCategory: Log\n\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mdDir, "Bookmarks.md"),
		[]byte("Title: Incoming bookmarks\nTags: Bookmarks\n\n<script>bookmarks = [\n<!-- Don't edit body below this line -->\n];\n</script>"), 0644); err != nil {
		t.Fatal(err)
	}
	// Source page for the new-page path (handleNewPage recompiles the
	// SOURCE page's html when creating a child from it).
	if err := os.WriteFile(filepath.Join(mdDir, "Home.md"),
		[]byte("Title: Home\nDate: 2026-01-01 00:00:00\n\nWelcome"), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Save a note.
	if rec := postForm(t, a.handleSaveNote, "/api/save",
		url.Values{"name": {"SaveMe"}, "content": {"Title: SaveMe\n\nHello **bold**"}}); rec.Code != http.StatusOK {
		t.Fatalf("save: status %d, body %s", rec.Code, rec.Body.String())
	}
	assertCachedPageShape(t, filepath.Join(htmlDir, "SaveMe.html"))

	// 2. Quick note.
	if rec := postForm(t, a.handleQuickNote, "/api/quick",
		url.Values{"note": {"a captured thought"}}); rec.Code != http.StatusOK {
		t.Fatalf("quicknote: status %d, body %s", rec.Code, rec.Body.String())
	}
	assertCachedPageShape(t, filepath.Join(htmlDir, "QuickNotes.html"))

	// 3. Bookmark.
	if rec := postForm(t, a.handleBookmark, "/api/bookmark",
		url.Values{"url": {"https://example.com"}, "title": {"Example"}, "tags": {"a, b"}, "notes": {"note"}}); rec.Code != http.StatusOK {
		t.Fatalf("bookmark: status %d, body %s", rec.Code, rec.Body.String())
	}
	assertCachedPageShape(t, filepath.Join(htmlDir, "Bookmarks.html"))

	// 4. New page (recompiles the SOURCE page, Home).
	if rec := postForm(t, a.handleNewPage, "/api/newpage",
		url.Values{"source": {"Home"}, "target": {"Child"}, "title": {"Child"}}); rec.Code != http.StatusOK {
		t.Fatalf("newpage: status %d, body %s", rec.Code, rec.Body.String())
	}
	assertCachedPageShape(t, filepath.Join(htmlDir, "Home.html"))
}

// ---------------------------------------------------------------------
// Phase 5 target: the SSE log bridge lifecycle.
//
// The desktop connection-stall bug was that each page held its /api/logs
// EventSource open forever; the client-side fix closes it on pagehide.
// This is the SERVER-SIDE half of the same contract: HandleLogsSSE must
// register a client on connect and DE-register it when the request context
// is cancelled (the client disconnecting). If a future refactor of the log
// bridge breaks the deferred cleanup, the held-connection leak comes back -
// this test fails first.
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

func TestPhase0_LogsSSERegistersAndReleasesClient(t *testing.T) {
	a := newTestApp(t)
	srv := httptest.NewServer(http.HandlerFunc(a.HandleLogsSSE))
	defer srv.Close()

	base := countLogClients()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The SSE handler blocks (it only flushes when a log line arrives), so
	// Do() won't return until we cancel - run it in the background. The
	// server-side registration happens as soon as the handler runs,
	// regardless of whether any bytes reached the client yet.
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

	// Disconnect. The handler's deferred cleanup must remove this client.
	cancel()
	if !waitFor(func() bool { return countLogClients() == base }, 2*time.Second) {
		<-done
		t.Fatalf("SSE client not released on disconnect (clients=%d, want %d)", countLogClients(), base)
	}
	<-done
}
