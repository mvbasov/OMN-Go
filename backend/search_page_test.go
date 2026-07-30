package backend

// Tests for the results page.
//
// It shares the query path with the API, so what needs proving here is not
// "does it find things" - that is covered - but that the HTML it builds is
// safe and honest: every value escaped for the context it lands in, and an
// empty result that says what was actually searched rather than leaving the
// reader to guess.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func getSearchPage(t *testing.T, a *App, query string) *httptest.ResponseRecorder {
	t.Helper()
	target := "/OMNGoSearch.html"
	if query != "" {
		target += "?q=" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	a.serveFrontend(rec, req)
	return rec
}

func TestSearchPage_RendersResults(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Fetch.md", "Title: Fetch\nTags: Test, Net\n\nawait fetch('/json/test.json');\n")
	writeSearchNote(t, a, "Other.md", "Title: Other\n\nnothing here\n")

	rec := getSearchPage(t, a, "fetch")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()

	if strings.Contains(body, "%%") {
		t.Error("unfilled template placeholder")
	}
	for _, want := range []string{
		`href="/Fetch.html?hl=fetch"`,   // links to the note, carrying the query
		"1 result for",                  // the summary counts
		`class="search-group"`,          // grouped by kind
		"Notes",                         // ... with a human label
		`<mark class="omn-search-hit">`, // the match is highlighted
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	if strings.Contains(body, "Other") {
		t.Error("a non-matching note appeared on the page")
	}

	// Tag pills use the same markup and the same anchor contract as a page
	// header, so a tag goes to the same place wherever it is shown.
	if !strings.Contains(body, `href="/OMNGoTags.html#Test"`) {
		t.Error("tag pill does not link into the Tags page")
	}

	// Dynamic like Config: no source, no cache, nothing written.
	if _, err := readIfExists(a.pageHTMLPath("OMNGoSearch")); err == nil {
		t.Error("the results page wrote an html/ cache; it must stay dynamic")
	}
}

// Everything on this page comes from either the user's URL or the user's
// notes. Both are attacker-controlled in the LAN-sharing case, and the page is
// assembled by hand rather than by html/template (see the note at the top of
// templates.go), so the escaping is this test's business.
func TestSearchPage_Escaping(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Evil.md",
		"Title: <script>alert('title')</script>\nTags: <b>tag</b>\n\nthe payload <img src=x onerror=alert(1)> sits here\n")

	rec := getSearchPage(t, a, "payload")
	body := rec.Body.String()

	for _, forbidden := range []string{
		"<script>alert('title')</script>",
		"<img src=x onerror=alert(1)>",
		"<b>tag</b>",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("unescaped markup from a note reached the page: %q", forbidden)
		}
	}
	if !strings.Contains(body, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Error("the snippet text was not escaped into the page")
	}

	// The query is echoed into an input value AND into the summary.
	rec = getSearchPage(t, a, "%22%3E%3Cscript%3Ealert(1)%3C/script%3E")
	body = rec.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("the query escaped its attribute and became markup")
	}
	if strings.Contains(body, `value=""><script`) {
		t.Error("the query broke out of the input's value attribute")
	}
}

// A snippet is escaped as it is spliced, so the only markup in the output is
// the <mark> this function writes itself.
func TestRenderSnippetHTML(t *testing.T) {
	got := renderSnippetHTML("a <b> c", [][2]int{{2, 3}})
	want := `a <mark class="omn-search-hit">&lt;b&gt;</mark> c`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	// Rune offsets, not bytes: the same span on Cyrillic must cover the same
	// characters it did on the Go side.
	got = renderSnippetHTML("Заметки тут", [][2]int{{0, 7}})
	if !strings.Contains(got, `<mark class="omn-search-hit">Заметки</mark>`) {
		t.Errorf("rune span landed wrong: %s", got)
	}

	// Nonsense spans are ignored rather than panicking or truncating.
	if got := renderSnippetHTML("abc", [][2]int{{5, 2}, {0, 99}, {-1, 1}}); got != "abc" {
		t.Errorf("out-of-range spans changed the text: %q", got)
	}
}

// An empty result has to say what WAS searched. "No results" from a config the
// reader has forgotten about is a trap, not an answer.
func TestSearchPage_EmptyStateNamesTheCorpus(t *testing.T) {
	a := enabledSearchApp(t, SearchKindMD)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nsomething\n")

	body := getSearchPage(t, a, "zzzznothing").Body.String()
	if !strings.Contains(body, "No matches") {
		t.Fatalf("no empty state: %s", body)
	}
	if !strings.Contains(body, "Notes") {
		t.Error("the empty state does not say which kinds are indexed")
	}
	if !strings.Contains(body, "/Config.html#cfg-search") {
		t.Error("the empty state does not offer a way to change what is searched")
	}
}

// With global search off the page does not exist. A permanently empty page
// would be worse than an honest 404 - and page search, which is what still
// works, lives in the dialog.
func TestSearchPage_404WhenGlobalSearchOff(t *testing.T) {
	a := newTestApp(t)
	a.search = &searchIndex{}

	rec := getSearchPage(t, a, "anything")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404 while global search is off", rec.Code)
	}

	// And it does not synthesize a note for itself on the way out, the way an
	// unknown page name would.
	if _, err := readIfExists(a.storagePath("md/OMNGoSearch.md")); err == nil {
		t.Error("a 404 for the search page created md/OMNGoSearch.md")
	}
}

func TestSearchPage_NoQueryIsJustTheForm(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nsomething\n")

	body := getSearchPage(t, a, "").Body.String()
	if !strings.Contains(body, `name="q"`) {
		t.Error("no search form")
	}
	if strings.Contains(body, "No matches") {
		t.Error("an empty query reported 'no matches'; it asked nothing")
	}
	if strings.Contains(body, `class="search-result"`) {
		t.Error("an empty query returned results")
	}
}
