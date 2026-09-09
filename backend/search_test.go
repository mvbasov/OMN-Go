package backend

// Tests for page-scope search: the always-on half of the feature.
//
// The point of most of these is not "does it find things". It is "does it find
// them without needing anything". That means no index, no config, and no state
// left behind. That is what lets the search button exist on a device that will
// never turn global search on. It is thus worth an assertion, and not an
// assumption.

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

// The end-to-end version of the worked example E4 of the plan. It has two
// terms. One matched in the title and one in the body, scored through the
// field weights. The matcher tests pin the arithmetic of each term. This test
// pins that the document layer multiplies and sums it the way the plan says.
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
	// newUnconfiguredApp and not newTestApp. The property under test is
	// that page search needs no configuration, thus this test must load
	// none. newTestApp loads the defaults since 26.09.18.
	a := newUnconfiguredApp(t)
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

// A query for something that is not there is an empty result, and not an
// error. It is never a way to find out what exists outside the storage
// directory.
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
// Search is read-only. Unlike create, quick-note and bookmark, the button is
// thus NOT .admin-only. A guest on the LAN can already read every page. To be
// unable to search what you are allowed to read is a strange place to draw a
// line. It IS .server-only, because an exported page has no /api/search. The
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

// The search box must not describe itself to the keyboard at all.
//
// spellcheck="false", autocorrect="off", autocapitalize and autocomplete all
// read like housekeeping on a search field. On Android they are not cosmetic.
// Each is a hint that the soft keyboard reads when it attaches. Several fold
// into the NO_SUGGESTIONS flag of the IME, which disables the COMPOSING
// region. That region is the mechanism that every non-Latin layout uses to
// enter text. Cyrillic input then silently produces nothing while Latin typing
// works. The field looks fine, and it refuses half the scripts of the world.
//
// None of them buys anything here. The field is not in a <form> and has no
// name, thus autofill never engages. Matching is case-folded, thus
// auto-capitalization is harmless. A red squiggle under a query is cosmetic.
// So the rule is the plain one. It is a plain text field, and nothing else.
//
// This is a shape test on the shipped asset, and not a behavior test. To
// observe the behavior needs a physical Android keyboard. These attributes are
// also exactly what a later tidy-up puts back.
func TestSearchInputDoesNotDisableTheIME(t *testing.T) {
	// The overlay moved to a file of its own in 26.09.24. It loads at the
	// first press of the magnifier, and not with each note page.
	src, err := staticFS.ReadFile("frontend/html/js/OMN-Go/omn-go-search.js")
	if err != nil {
		t.Fatal(err)
	}
	at := strings.Index(string(src), `class="omn-search-input"`)
	if at < 0 {
		t.Fatal("the search input markup moved; this test needs updating")
	}
	// The attribute list is built by string concatenation, so read a little
	// either side of the class rather than trying to parse it.
	markup := string(src[max(0, at-200):min(at+300, len(src))])
	markup = markup[strings.Index(markup, "<input"):]
	markup = markup[:strings.Index(markup, ">")]

	for _, forbidden := range []string{
		"spellcheck", "autocorrect", "autocapitalize", "autocomplete", "inputmode",
	} {
		if strings.Contains(markup, forbidden) {
			t.Errorf("%s is set on the search input: %s\n"+
				"Every keyboard hint here is a way for an IME to decide this is not "+
				"an ordinary text field, and none of them buys anything on a search box.",
				forbidden, markup)
		}
	}
}

// Focus is handed to the field twice on open. The second pass is the fix for
// the same class of bug. The overlay goes from display:none to display:flex,
// and takes focus in one tick. The keyboard can thus attach to an element with
// no layout yet, and come up with no composing region.
//
// A synchronous focus() alone is what the intermittent "cannot type Cyrillic
// until I open and close another panel" report was. Pinned as a shape test for
// the same reason as above - the failure needs a real soft keyboard to see.
func TestSearchOverlayReattachesFocusAfterLayout(t *testing.T) {
	src, err := staticFS.ReadFile("frontend/html/js/OMN-Go/omn-go-search.js")
	if err != nil {
		t.Fatal(err)
	}
	at := strings.Index(string(src), "function focusInput()")
	if at < 0 {
		t.Fatal("focusInput is gone; if the deferred re-attach moved, move this test with it")
	}
	body := string(src[at:min(at+900, len(src))])

	if !strings.Contains(body, "requestAnimationFrame") {
		t.Error("the re-attach is no longer deferred past layout")
	}
	if !strings.Contains(body, "input.blur()") {
		t.Error("the re-attach no longer blurs first; focus() on an already-focused " +
			"element is a no-op, which is the state that needs clearing")
	}
}

// The editor's find and replace fields carry no keyboard hints either.
//
// Same rule and same reason as the search overlay above. On Android those
// attributes fold into the NO_SUGGESTIONS flag of the IME. That flag disables
// the composing region that every non-Latin layout needs. A find field that
// cannot accept Cyrillic is a find field that cannot search a Russian note.
func TestEditorFindInputsDoNotDisableTheIME(t *testing.T) {
	src, err := templatesFS.ReadFile("frontend/templates/editor.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(src)
	for _, id := range []string{`id="findInput"`, `id="replaceInput"`} {
		at := strings.Index(page, id)
		if at < 0 {
			t.Fatalf("%s is gone; this test needs updating", id)
		}
		start := strings.LastIndex(page[:at], "<input")
		end := start + strings.Index(page[start:], ">")
		markup := page[start : end+1]
		for _, forbidden := range []string{"spellcheck", "autocorrect", "autocapitalize", "autocomplete"} {
			if strings.Contains(markup, forbidden) {
				t.Errorf("%s is set on %s: %s", forbidden, id, markup)
			}
		}
	}
}

// ---------------------------------------------------------------------
// The phrase rung
//
// A reader who types a whole sentence wants the note that holds that
// sentence. Before 26.08.80 the sum decided, and the sum favors a title.
// Five loose query words in one title scored 2001. The note that held the
// sentence in a body line scored 718. tierPhrase answers that, and the
// tests below pin each edge of the rule.
// ---------------------------------------------------------------------

// phraseDoc writes one note and gives its search document back.
func phraseDoc(t *testing.T, a *App, name, content string) *searchDocument {
	t.Helper()
	writeSearchNote(t, a, name+".md", content)
	doc, err := a.loadPageDocument(name)
	if err != nil || doc == nil {
		t.Fatalf("loadPageDocument(%q): %v", name, err)
	}
	return doc
}

func TestPhraseTierBeatsAHigherScore(t *testing.T) {
	a := newTestApp(t)
	// exact holds the sentence in a body line and says little in its title.
	// decoy holds four of the five words IN ITS TITLE, which weighs three
	// times a line, and the same words far apart in its body.
	exact := phraseDoc(t, a, "exact", "Title: A note\n\n"+
		"Build the project and run the tests.\nA second line about nothing.\n")
	decoy := phraseDoc(t, a, "decoy", "Title: Build the project run the tests\n\n"+
		"Build a shed.\nRun a marathon.\nThe tests of the water and the project of the bridge.\n")

	q := parseQuery("Build the project and run the tests")
	eScore, eTier, _, eOK := scoreDocument(q, exact)
	dScore, dTier, _, dOK := scoreDocument(q, decoy)
	if !eOK || !dOK {
		t.Fatal("both notes must match every term")
	}
	if eTier != tierPhrase {
		t.Errorf("the note holding the sentence has tier %s, want phrase", eTier)
	}
	if dTier == tierPhrase {
		t.Error("the decoy has no sentence, and it took the phrase rung")
	}
	if dScore <= eScore {
		t.Fatalf("the decoy scores %d and the exact note %d. This test proves "+
			"nothing unless the decoy scores MORE.", dScore, eScore)
	}
	if !betterMatch(eTier, eScore, dTier, dScore) {
		t.Errorf("the decoy still ranks first: exact=%s/%d decoy=%s/%d",
			eTier, eScore, dTier, dScore)
	}
}

func TestPhraseTierMarksTheLineToo(t *testing.T) {
	// The panel shows ten lines for one document. The line the reader typed
	// has to be the first of them, and not the line with the largest sum.
	a := newTestApp(t)
	doc := phraseDoc(t, a, "note", "Title: A note\n\n"+
		"The project and the tests and the project and the tests.\n"+
		"Build the project and run the tests.\n")
	_, _, hits, ok := scoreDocument(parseQuery("Build the project and run the tests"), doc)
	if !ok || len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].tier != tierPhrase {
		t.Errorf("the first line has tier %s, want phrase: %q",
			hits[0].tier, strings.TrimSpace(hits[0].line.raw))
	}
	if !strings.Contains(hits[0].line.raw, "Build the project and run the tests") {
		t.Errorf("the first line is %q, want the line holding the query",
			strings.TrimSpace(hits[0].line.raw))
	}
}

func TestPhraseTierNeedsTwoTermsAndNoField(t *testing.T) {
	// A one-word query is its own phrase. The rung would then apply to
	// every note that holds the word, and it would say nothing. A query that names a
	// field has no natural reading as one phrase.
	a := newTestApp(t)
	doc := phraseDoc(t, a, "note", "Title: Build the project\n\nBuild the project.\n")
	for _, query := range []string{"project", "title:build project", "build title:project"} {
		if _, tier, _, ok := scoreDocument(parseQuery(query), doc); ok && tier == tierPhrase {
			t.Errorf("query %q took the phrase rung", query)
		}
	}
}

func TestPhraseTierIsLiteralAndAdjacent(t *testing.T) {
	// The words must be next to each other, with one space between each
	// pair. Anything looser makes the rung meaningless, because a document
	// holding every term somewhere already passes the AND rule.
	a := newTestApp(t)
	cases := []struct {
		name    string
		body    string
		phrase  bool
		comment string
	}{
		{"adjacent", "Build the project today.\n", true, "the plain case"},
		{"comma", "Build the, project today.\n", false, "a comma is not a space"},
		{"apart", "Build a shed. The project waits.\n", false, "same line, not adjacent"},
		{"twolines", "Build a shed.\nThe project waits.\n", false, "two lines"},
		{"folded", "BUILD THE PROJECT today.\n", true, "the fold applies"},
	}
	for _, c := range cases {
		doc := phraseDoc(t, a, c.name, "Title: N\n\n"+c.body)
		_, tier, _, ok := scoreDocument(parseQuery("build the project"), doc)
		got := ok && tier == tierPhrase
		if got != c.phrase {
			t.Errorf("%s (%s): phrase=%v want %v, body %q",
				c.name, c.comment, got, c.phrase, c.body)
		}
	}
}

// ---------------------------------------------------------------------
// The snippet cut
//
// A long note answers a common query on hundreds of lines. The panel asks
// for ten, and before 26.08.81 it got ten. The last rows thus carried the
// lines that matched a one-letter term and nothing else. cutSnippets takes
// those rows out of the answer.
// ---------------------------------------------------------------------

// cutDoc writes a note whose first lines carry the query and whose tail
// carries one letter of it.
func cutDoc(t *testing.T, a *App, name string, tail int) *searchDocument {
	t.Helper()
	body := "Title: A note\n\nBuild the project and run the tests.\n" +
		"The project waits here.\nThe tests wait here.\n"
	for i := 0; i < tail; i++ {
		body += "A line about nothing at all.\n"
	}
	return phraseDoc(t, a, name, body)
}

func TestCutSnippetsDropsALoneLetterLine(t *testing.T) {
	a := newTestApp(t)
	doc := cutDoc(t, a, "cut", 12)
	q := parseQuery("a project tests")
	_, _, hits, ok := scoreDocument(q, doc)
	if !ok {
		t.Fatal("the note must match")
	}
	got := cutSnippets(q, hits, 10, nil)
	if len(got) >= 10 {
		t.Errorf("kept %d lines of the ten. A line that matches the letter "+
			"\"a\" alone carries no word of the query.", len(got))
	}
	for _, h := range got {
		if strings.Contains(h.line.raw, "nothing at all") {
			t.Errorf("a lone-letter line survived: %q", strings.TrimSpace(h.line.raw))
		}
	}
	if len(got) == 0 {
		t.Error("every line went, and the reader is told the note matched")
	}
}

func TestCutSnippetsNeverPromotes(t *testing.T) {
	// The window comes FIRST and the drop comes second. A drop before the
	// window would let a line from a worse rung move up into the ten. This
	// test fails the moment a line appears that the plain window would not
	// have shown.
	a := newTestApp(t)
	doc := cutDoc(t, a, "cut", 12)
	q := parseQuery("a project tests")
	_, _, hits, _ := scoreDocument(q, doc)
	window := map[int]bool{}
	for i, h := range hits {
		if i >= 10 {
			break
		}
		window[h.line.no] = true
	}
	for _, h := range cutSnippets(q, hits, 10, nil) {
		if !window[h.line.no] {
			t.Errorf("line %d reached the answer, and the first ten did not "+
				"hold it: cutSnippets promoted a line", h.line.no)
		}
	}
}

func TestCutSnippetsKeepsAnHonestList(t *testing.T) {
	a := newTestApp(t)
	cases := []struct {
		name  string
		query string
		limit int
		want  int
		why   string
	}{
		{"one term", "project", 10, 10, "one term cannot be a lone short term"},
		{"the limit rules", "a project tests", 3, 3, "the caller asked for three"},
	}
	for _, c := range cases {
		doc := cutDoc(t, a, c.name, 12)
		q := parseQuery(c.query)
		_, _, hits, ok := scoreDocument(q, doc)
		if !ok {
			t.Fatalf("%s: no match", c.name)
		}
		if got := len(cutSnippets(q, hits, c.limit, nil)); got > c.want {
			t.Errorf("%s: kept %d, want at most %d (%s)", c.name, got, c.want, c.why)
		}
	}
}

func TestShortTermKeepsAWholeWord(t *testing.T) {
	// One Han rune is a word. A rule that calls it short takes the score of
	// a real search: the query "猫" scored 390 in the laboratory, and 0 with
	// such a rule.
	for _, s := range []string{"猫", "犬", "の", "ア", "한"} {
		if isShortTerm([]rune(s)) {
			t.Errorf("%q reads as a short term, and it is a whole word", s)
		}
	}
	for _, s := range []string{"a", "и", "1", "_"} {
		if !isShortTerm([]rune(s)) {
			t.Errorf("%q does not read as a short term", s)
		}
	}
	if isShortTerm([]rune("ab")) || isShortTerm(nil) {
		t.Error("only a term of exactly one rune can be short")
	}
}

func TestCutSnippetsInTheResponse(t *testing.T) {
	// End to end through the API. The note answers the query on many lines,
	// and only the first few say anything.
	a := newTestApp(t)
	cutDoc(t, a, "cut", 12)

	_, resp := searchReq(t, a, url.Values{
		"q": {"a project tests"}, "on": {"cut.md"}, "snippets": {"10"},
	})
	if len(resp.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(resp.Results))
	}
	got := len(resp.Results[0].Matches)
	if got == 0 || got >= 10 {
		t.Errorf("the answer holds %d snippets. The lines that match the "+
			"letter \"a\" alone must not fill the list.", got)
	}
	for _, m := range resp.Results[0].Matches {
		if strings.Contains(m.Text, "nothing at all") {
			t.Errorf("a line that matches one letter reached the answer: %q", m.Text)
		}
	}
}

// ----------------------------------------------------------------------
// The snippet cut, with the common words of the collection
// ----------------------------------------------------------------------
//
// 26.09.39 measured the panel for the query "the tag and the level".
// **33 of 50 rows carried no content word, and the panel drew 1360
// marks.** See claude/s1-search-panel-report-2026-09-06.md.
//
// The rule of 26.09.40 keeps a row that holds a term which is not one
// rune, is not common in this collection, and matches VERBATIM.
//
// EACH OF THE THREE PARTS DOES WORK. The tests below break each part in
// turn. Remove the common test and the reported query keeps 75 percent
// of its empty rows. Remove the verbatim test and it keeps 89 percent.

// csDoc writes a note that answers a two-word query on some lines with
// the rare word and on others with the common word alone.
func csDoc(t *testing.T, a *App, name string) *searchDocument {
	t.Helper()
	body := "Title: A note\n\n" +
		"The kingfisher waits by the water.\n" +
		"The kingfisher is a bird.\n" +
		"The morning was cold and long.\n" +
		"The evening came and the light went.\n" +
		"The road turned and the wall ended.\n"
	return phraseDoc(t, a, name, body)
}

// A row that holds the common word alone goes.
//
// "the" is in every note of the collection. A row that carries it and
// nothing else of the query tells the reader nothing.
func TestCutSnippetsDropsARowOfCommonWordsAlone(t *testing.T) {
	a := newTestApp(t)
	doc := csDoc(t, a, "cs-common")
	q := parseQuery("the kingfisher")
	_, _, hits, ok := scoreDocument(q, doc)
	if !ok {
		t.Fatal("the note must match")
	}
	common := map[string]bool{"the": true}

	got := cutSnippets(q, hits, 10, common)
	if len(got) == 0 {
		t.Fatal("every row went, and two rows hold the rare word")
	}
	for _, h := range got {
		if !strings.Contains(strings.ToLower(h.line.raw), "kingfisher") {
			t.Errorf("a row of common words alone survived: %q",
				strings.TrimSpace(h.line.raw))
		}
	}
	if len(got) != 2 {
		t.Errorf("kept %d rows, want the two that hold the rare word", len(got))
	}
}

// The same note with no common set keeps every row.
//
// This is the half of the rule that document frequency does. Without the
// set, "the" is an ordinary term and each row carries it.
func TestCutSnippetsWithNoCommonSetKeepsTheCommonRows(t *testing.T) {
	a := newTestApp(t)
	doc := csDoc(t, a, "cs-nocommon")
	q := parseQuery("the kingfisher")
	_, _, hits, _ := scoreDocument(q, doc)

	if got := len(cutSnippets(q, hits, 10, nil)); got <= 2 {
		t.Errorf("kept %d rows with no common set, want each row of the "+
			"window. The common set is then doing no work.", got)
	}
}

// A row that matches only as a SUBSEQUENCE goes.
//
// scoreTerm answers true for a line that holds the runes of the term in
// order and apart. A line that says "the cabinet at the top" carries
// "cat" that way, and it says nothing about a cat.
//
// This is the half that document frequency cannot do. It is why the
// measurement of the report, which used a plain substring test, promised
// more than document frequency alone delivered.
func TestCutSnippetsNeedsAVerbatimMatch(t *testing.T) {
	a := newTestApp(t)
	body := "Title: A note\n\n" +
		"The cat sat on the mat.\n" +
		"The cabinet at the top holds the tea.\n" +
		"A certificate arrived this morning.\n"
	doc := phraseDoc(t, a, "cs-subseq", body)

	q := parseQuery("cat morning")
	_, _, hits, ok := scoreDocument(q, doc)
	if !ok {
		t.Fatal("the note must match")
	}

	// Every line of the window carries "cat" through scoreTerm, thus the
	// rule before 26.09.40 kept each of them.
	loose := 0
	for _, h := range hits {
		if _, _, _, ok := scoreTerm([]rune("cat"), h.line.fold); ok {
			loose++
		}
	}
	if loose < 3 {
		t.Skipf("only %d lines match \"cat\" loosely, thus this note proves "+
			"nothing about the subsequence rung", loose)
	}

	for _, h := range cutSnippets(q, hits, 10, nil) {
		low := strings.ToLower(h.line.raw)
		if !strings.Contains(low, "cat ") && !strings.Contains(low, "morning") {
			t.Errorf("a row that holds no whole word of the query survived: %q",
				strings.TrimSpace(h.line.raw))
		}
	}
}

// When every term of the query is common, the window is the answer.
//
// The document matched, thus it has something to say. A cut that leaves
// no row would tell the reader that a result exists and show nothing of
// it.
//
// This guard stood before 26.09.40. It now carries the whole risk of the
// common set. A person whose notes are all about one subject pushes the
// words of that subject above the half, and lands here.
func TestCutSnippetsAnswersTheWindowWhenEachTermIsCommon(t *testing.T) {
	a := newTestApp(t)
	doc := csDoc(t, a, "cs-allcommon")
	q := parseQuery("the kingfisher")
	_, _, hits, _ := scoreDocument(q, doc)

	common := map[string]bool{"the": true, "kingfisher": true}
	got := cutSnippets(q, hits, 10, common)

	window := hits
	if len(window) > 10 {
		window = window[:10]
	}
	if len(got) != len(window) {
		t.Errorf("kept %d rows of the %d of the window. With every term "+
			"common the window is the answer.", len(got), len(window))
	}
}

// A one-term query never reaches the rule.
//
// cutSnippets returns before it reads a term. A single common term is
// the whole query, and hiding each row would answer a search with
// nothing at all.
func TestCutSnippetsKeepsAOneTermQueryWhole(t *testing.T) {
	a := newTestApp(t)
	doc := csDoc(t, a, "cs-oneterm")
	q := parseQuery("the")
	_, _, hits, ok := scoreDocument(q, doc)
	if !ok {
		t.Fatal("the note must match")
	}
	common := map[string]bool{"the": true}
	if got := len(cutSnippets(q, hits, 10, common)); got == 0 {
		t.Error("a one-term query of a common word answered no row at all")
	}
}
