package backend

// Tests for page-scope search: the always-on half of the feature.
//
// The point of most of these is not "does it find things" but "does it find
// them without needing anything" - no index, no config, no state left behind.
// That is what lets the search button exist on a device that will never turn
// global search on, so it is worth asserting rather than assuming.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The real line 15 of backend/frontend/md/Test/OMN-Go/Fetch.md, used as the
// fixture for the worked example the matcher tests already pin.
const fetchNote = `Title: Test/OMNGo/Fetch
Date: 2026-06-23 00:29:26
Category: Test
Author: Mikhail Basov
Modified: 2026-07-09 21:59:56
Tags: Test

### ` + "`fetch()`" + ` test
See console
<div id="fetchStatus">Waiting data ...</div>
<script type="module">
// Using async/await
async function loadJSON() {
    try {
        const response = await fetch('/json/test.json'); // Relative path to your JSON file
        if (!response.ok) throw new Error('Network response was not ok');
        const data = await response.json();
    } catch (error) {
        console.error('Fetch error:', error);
    }
}
loadJSON();
</script>
`

func searchReq(t *testing.T, a *App, query url.Values) (*httptest.ResponseRecorder, searchResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/search?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	a.handleSearch(rec, req)

	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not JSON (%v): %s", err, rec.Body.String())
	}
	return rec, resp
}

// The end-to-end version of the plan's worked example E4: two terms, one
// matched in the title and one in the body, scored through the field weights.
// The matcher tests pin the per-term arithmetic; this pins that the document
// layer multiplies and sums it the way the plan says.
func TestPageSearch_E4EndToEnd(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Test/OMN-Go/Fetch.md", fetchNote)

	rec, resp := searchReq(t, a, url.Values{
		"q": {"fetch json"}, "scope": {"page"}, "on": {"Test/OMN-Go/Fetch.html"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(resp.Results), resp)
	}
	got := resp.Results[0]

	// term "fetch": title 145 x3.0 = 435   (100 +30 after '/' +20 ends field -5 pos 11)
	// term "json":  content 135 x1.0 = 135 (100 +30 after '/' +20 before '/' -15 pos capped)
	// document: (435 + 135) x1.0 for kind md
	if want := 570; got.Score != want {
		t.Errorf("score = %d, want %d", got.Score, want)
	}
	if got.Kind != "md" || got.Name != "Test/OMN-Go/Fetch" {
		t.Errorf("kind/name = %q/%q, want md/Test/OMN-Go/Fetch", got.Kind, got.Name)
	}
	if got.URL != "/Test/OMN-Go/Fetch.html" {
		t.Errorf("url = %q", got.URL)
	}
	if got.Title != "Test/OMNGo/Fetch" {
		t.Errorf("title = %q, want the Title: header", got.Title)
	}

	// The best line is the one carrying both terms, and it is inside the
	// note's <script> block - which the result says out loud.
	if len(got.Matches) == 0 {
		t.Fatal("no snippets")
	}
	best := got.Matches[0]
	if !strings.Contains(best.Text, "await fetch('/json/test.json')") {
		t.Errorf("best snippet = %q", best.Text)
	}
	if best.Context != "script" {
		t.Errorf("context = %q, want script", best.Context)
	}
	if best.Line != 15 {
		t.Errorf("line = %d, want 15 (numbered from the top of the FILE, header included)", best.Line)
	}
	// Spans cover both terms, merged and in order.
	if len(best.Spans) < 4 {
		t.Errorf("spans = %v, want at least the 'fetch' hit and three 'json' hits", best.Spans)
	}
	for i := 1; i < len(best.Spans); i++ {
		if best.Spans[i][0] < best.Spans[i-1][0] {
			t.Errorf("spans out of order: %v", best.Spans)
			break
		}
	}
	// And they land on the right characters.
	runes := []rune(best.Text)
	for _, s := range best.Spans {
		if s[0] < 0 || s[0]+s[1] > len(runes) {
			t.Fatalf("span %v out of bounds for %q", s, best.Text)
		}
		frag := strings.ToLower(string(runes[s[0] : s[0]+s[1]]))
		if frag != "fetch" && frag != "json" {
			t.Errorf("span %v covers %q, want a query term", s, frag)
		}
	}
}

// Page search must work with nothing set up: no config, no index, no state
// left behind. This is the property that lets it be always-on.
func TestPageSearch_NeedsNothing(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nthe quick brown fox\n")

	// A zero App: no config loaded, no search settings, nothing warmed.
	if cfg := a.GetConfig(); cfg.ServerPort != 0 {
		t.Fatalf("test precondition: expected an unconfigured App, got %+v", cfg)
	}

	_, resp := searchReq(t, a, url.Values{"q": {"brown"}, "on": {"Note"}})
	if len(resp.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(resp.Results))
	}
	if resp.Scope != "page" {
		t.Errorf("scope = %q, want page as the default while global search does not exist", resp.Scope)
	}

	// Nothing was written: no cache, no index file, no md rewrite.
	entries, err := os.ReadDir(filepath.Join(a.StorageDir, "html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("page search wrote into html/: %v", entries)
	}
	src, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "Note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != "Title: A Note\n\nthe quick brown fox\n" {
		t.Error("page search modified the note")
	}
}

func TestPageSearch_AndSemantics(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nalpha here\nbeta there\n")

	if _, resp := searchReq(t, a, url.Values{"q": {"alpha beta"}, "on": {"Note"}}); len(resp.Results) != 1 {
		t.Errorf("both terms present: got %d results, want 1", len(resp.Results))
	}
	if _, resp := searchReq(t, a, url.Values{"q": {"alpha gamma"}, "on": {"Note"}}); len(resp.Results) != 0 {
		t.Errorf("one term missing: got %d results, want 0 - AND, not OR", len(resp.Results))
	}
}

// Header lines are fields, not content: a search for a header VALUE finds the
// note, but never as a content snippet.
func TestPageSearch_HeaderIsFieldsNotContent(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\nCategory: Notes\nTags: Red, Blue\n\nbody text\n")

	_, resp := searchReq(t, a, url.Values{"q": {"Notes"}, "on": {"Note"}})
	if len(resp.Results) != 1 {
		t.Fatalf("header value did not match at all")
	}
	for _, m := range resp.Results[0].Matches {
		if strings.Contains(m.Text, "Category:") {
			t.Errorf("header line came back as a content snippet: %q", m.Text)
		}
	}

	// A tag scores through the tag field, at its own weight.
	_, resp = searchReq(t, a, url.Values{"q": {"tag:Blue"}, "on": {"Note"}})
	if len(resp.Results) != 1 {
		t.Error("tag:Blue did not match the note's tag")
	}
	_, resp = searchReq(t, a, url.Values{"q": {"tag:body"}, "on": {"Note"}})
	if len(resp.Results) != 0 {
		t.Error("tag:body matched, but 'body' is content, not a tag - the field restriction leaked")
	}
}

// A query for something that is not there is an empty result, not an error -
// and never a way to find out what exists outside the storage directory.
func TestPageSearch_MissingAndTraversal(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nsecret sauce\n")

	outside := filepath.Join(filepath.Dir(a.StorageDir), "outside.md")
	if err := os.WriteFile(outside, []byte("Title: Outside\n\nsecret sauce\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, on := range []string{
		"",
		"NoSuchNote",
		"../outside",
		"../../outside",
		"/etc/passwd",
	} {
		rec, resp := searchReq(t, a, url.Values{"q": {"secret"}, "on": {on}})
		if rec.Code != http.StatusOK {
			t.Errorf("on=%q: status %d, want 200 - a miss is not an error", on, rec.Code)
		}
		if len(resp.Results) != 0 {
			t.Errorf("on=%q returned %d results: %+v", on, len(resp.Results), resp.Results)
		}
	}
}

func TestPageSearch_AssetTarget(t *testing.T) {
	a := newTestApp(t)
	jsPath := filepath.Join(a.StorageDir, "html", "js", "mine.js")
	if err := os.MkdirAll(filepath.Dir(jsPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsPath, []byte("// helper\nfunction loadJSON() { return 42; }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, resp := searchReq(t, a, url.Values{"q": {"loadjson"}, "on": {"js/mine.js"}})
	if len(resp.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(resp.Results))
	}
	got := resp.Results[0]
	if got.Kind != "js" {
		t.Errorf("kind = %q, want js", got.Kind)
	}
	if got.URL != "/js/mine.js" {
		t.Errorf("url = %q, want the file's own served path", got.URL)
	}
	if len(got.Matches) == 0 || got.Matches[0].Line != 2 {
		t.Errorf("matches = %+v, want line 2", got.Matches)
	}
}

// A "<script>" mentioned inside a fenced example must not mark the rest of the
// file as script. This is the same failure the renderer's combined-scan comment
// in markdown.go records, in a different guise.
func TestClassifyContexts(t *testing.T) {
	lines := strings.Split(strings.TrimPrefix(`
prose one
`+"```"+`
# not a heading
<script>
`+"```"+`
prose two
<script>
var x = 1;
</script>
prose three
<pre>
literal
</pre>
prose four
`, "\n"), "\n")

	got := classifyContexts(lines)
	want := map[int]string{
		0: "",       // prose one
		1: "code",   // fence open
		2: "code",   // # not a heading
		3: "code",   // <script> INSIDE the fence
		4: "code",   // fence close
		5: "",       // prose two  <- the crucial one
		6: "script", // real <script>
		7: "script", // var x = 1;
		8: "script", // </script>
		9: "",       // prose three
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d (%q): context %q, want %q", i, lines[i], got[i], w)
		}
	}
}

func TestSnippetFor(t *testing.T) {
	// Short line: whitespace trimmed, spans shifted with it.
	text, spans := snippetFor("    hello world  ", []span{{Start: 10, Len: 5}})
	if text != "hello world" {
		t.Errorf("text = %q", text)
	}
	if len(spans) != 1 || spans[0].Start != 6 {
		t.Errorf("spans = %v, want the hit shifted to 6", spans)
	}

	// Long line: a window around the first hit, with ellipses, spans shifted.
	long := strings.Repeat("x", 200) + "needle" + strings.Repeat("y", 200)
	text, spans = snippetFor(long, []span{{Start: 200, Len: 6}})
	runes := []rune(text)
	if len(runes) > snippetMaxRunes+2 {
		t.Errorf("snippet is %d runes, want at most %d plus ellipses", len(runes), snippetMaxRunes)
	}
	if !strings.HasPrefix(text, "…") || !strings.HasSuffix(text, "…") {
		t.Errorf("expected ellipses on both sides: %q", text)
	}
	if len(spans) != 1 {
		t.Fatalf("spans = %v, want 1", spans)
	}
	if got := string(runes[spans[0].Start : spans[0].Start+spans[0].Len]); got != "needle" {
		t.Errorf("span covers %q, want needle", got)
	}
}

// Global scope has no index yet. It must say so rather than answer "nothing
// matched", which is a different and misleading statement.
func TestSearch_GlobalScopeUnavailable(t *testing.T) {
	a := newTestApp(t)
	rec, resp := searchReq(t, a, url.Values{"q": {"anything"}, "scope": {"all"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", rec.Code)
	}
	if resp.Status != "disabled" {
		t.Errorf("status field = %q, want disabled", resp.Status)
	}

	rec, _ = searchReq(t, a, url.Values{"q": {"anything"}, "scope": {"sideways"}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown scope: status %d, want 400", rec.Code)
	}
}

func TestSearch_EmptyQueryIsEmptyResult(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\ncontent\n")
	for _, q := range []string{"", "   "} {
		rec, resp := searchReq(t, a, url.Values{"q": {q}, "on": {"Note"}})
		if rec.Code != http.StatusOK {
			t.Errorf("q=%q: status %d, want 200", q, rec.Code)
		}
		if len(resp.Results) != 0 {
			t.Errorf("q=%q returned results", q)
		}
	}
}

func TestPageSearch_SnippetLimit(t *testing.T) {
	a := newTestApp(t)
	var b strings.Builder
	b.WriteString("Title: Many\n\n")
	for i := 0; i < 30; i++ {
		b.WriteString("needle line\n")
	}
	writeSearchNote(t, a, "Many.md", b.String())

	_, resp := searchReq(t, a, url.Values{"q": {"needle"}, "on": {"Many"}})
	if n := len(resp.Results[0].Matches); n != searchDefaultSnippets {
		t.Errorf("default snippets = %d, want %d", n, searchDefaultSnippets)
	}

	_, resp = searchReq(t, a, url.Values{"q": {"needle"}, "on": {"Many"}, "snippets": {"99"}})
	if n := len(resp.Results[0].Matches); n != searchMaxSnippets {
		t.Errorf("snippets=99 gave %d, want the cap %d", n, searchMaxSnippets)
	}
}

func TestPageSearch_TruncatesLargeFile(t *testing.T) {
	a := newTestApp(t)
	var b strings.Builder
	b.WriteString("Title: Big\n\n")
	for b.Len() < maxIndexFileBytes+4096 {
		b.WriteString("filler line of ordinary text\n")
	}
	b.WriteString("needle past the cap\n")
	writeSearchNote(t, a, "Big.md", b.String())

	_, resp := searchReq(t, a, url.Values{"q": {"needle"}, "on": {"Big"}})
	if len(resp.Results) != 0 {
		t.Error("found a term past the 500 KiB cap; the cap is not being applied")
	}

	_, resp = searchReq(t, a, url.Values{"q": {"filler"}, "on": {"Big"}})
	if len(resp.Results) != 1 {
		t.Fatal("nothing matched inside the cap")
	}
	if !resp.Results[0].Truncated || !resp.Truncated {
		t.Error("truncation was not reported; the user would read this as 'not found'")
	}
}

func TestPageSearch_KindFilter(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nneedle\n")

	if _, resp := searchReq(t, a, url.Values{"q": {"needle"}, "on": {"Note"}, "kind": {"md"}}); len(resp.Results) != 1 {
		t.Error("kind=md excluded a note")
	}
	if _, resp := searchReq(t, a, url.Values{"q": {"needle"}, "on": {"Note"}, "kind": {"js,json"}}); len(resp.Results) != 0 {
		t.Error("kind=js,json returned a note")
	}
	if _, resp := searchReq(t, a, url.Values{"q": {"kind:js needle"}, "on": {"Note"}}); len(resp.Results) != 0 {
		t.Error("in-query kind: prefix was ignored")
	}
}

func TestParseQuery(t *testing.T) {
	q := parseQuery("tag:Hydro TITLE:Manual plain kind:md,js")
	if len(q.terms) != 3 {
		t.Fatalf("terms = %d, want 3: %+v", len(q.terms), q.terms)
	}
	if q.terms[0].field != "tag" || string(q.terms[0].runes) != "hydro" {
		t.Errorf("term 0 = %+v", q.terms[0])
	}
	if q.terms[1].field != "title" || string(q.terms[1].runes) != "manual" {
		t.Errorf("term 1 = %+v", q.terms[1])
	}
	if q.terms[2].field != "" {
		t.Errorf("term 2 should be unrestricted: %+v", q.terms[2])
	}
	if strings.Join(q.kinds, ",") != "md,js" {
		t.Errorf("kinds = %v", q.kinds)
	}

	// An unknown prefix is not a prefix: a URL must stay a search for a URL.
	q = parseQuery("https://example.com")
	if len(q.terms) != 1 || q.terms[0].field != "" {
		t.Errorf("URL was parsed as a field restriction: %+v", q.terms)
	}
}

// The dialog is reachable from the page chrome, and reachable by everyone.
//
// Search is read-only, so unlike create/quick-note/bookmark the button is NOT
// .admin-only: a guest on the LAN can already read every page, and being
// unable to search what you are allowed to read is a strange place to draw a
// line. It IS .server-only, because an exported page has no /api/search - the
// existing applyOfflineUI() hides it there with no extra code.
func TestSearchButtonIsInTheRenderedPage(t *testing.T) {
	a := newTestApp(t)
	page := string(a.compilePage("Note", []byte("Title: A Note\n\nbody")))

	if !strings.Contains(page, "omnSearchOpen()") {
		t.Fatal("rendered page has no search button")
	}

	line := ""
	for _, l := range strings.Split(page, "\n") {
		if strings.Contains(l, "omnSearchOpen()") {
			line = l
			break
		}
	}
	if !strings.Contains(line, "server-only") {
		t.Errorf("search button is not .server-only, so an exported page would show a button that cannot work: %s", line)
	}
	if strings.Contains(line, "admin-only") {
		t.Errorf("search button is .admin-only; reading and searching should need the same rights: %s", line)
	}
}

func writeSearchNote(t *testing.T, a *App, rel, content string) {
	t.Helper()
	p := filepath.Join(a.StorageDir, "md", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// The search box must not tell the keyboard to stop making suggestions.
//
// spellcheck="false" and autocorrect="off" read like housekeeping on a search
// field - no red squiggles, no "helpful" rewriting of a query. On Chrome for
// Android they are not cosmetic: they set the IME's NO_SUGGESTIONS flag, which
// disables the COMPOSING region, which is the mechanism every non-Latin layout
// uses to enter text. Cyrillic input silently produces nothing.
//
// This is a shape test on the shipped asset rather than a behaviour test
// because the behaviour needs a physical Android keyboard to observe, and the
// attributes are exactly the kind of thing a later tidy-up puts back.
func TestSearchInputDoesNotDisableTheIME(t *testing.T) {
	src, err := staticFS.ReadFile("frontend/html/js/omn-go-sse.js")
	if err != nil {
		t.Fatal(err)
	}
	at := strings.Index(string(src), "omn-search-input")
	if at < 0 {
		t.Fatal("the search input markup moved; this test needs updating")
	}
	// The attribute list is built by string concatenation across a few lines.
	markup := string(src[at:min(at+400, len(src))])
	for _, forbidden := range []string{`spellcheck="false"`, `autocorrect="off"`} {
		if strings.Contains(markup, forbidden) {
			t.Errorf("%s is set on the search input; it disables composing input, "+
				"so Cyrillic (and every other IME-entered script) cannot be typed", forbidden)
		}
	}
	if !strings.Contains(markup, `autocomplete="off"`) {
		t.Error("autocomplete=\"off\" was dropped too; that one is fine and worth keeping - " +
			"it suppresses the browser's saved-values dropdown, not the keyboard")
	}
}
