package backend

// Tests for ?hl= - the terms a result carries so the page it opens marks them
// and scrolls to the first.
//
// The thing worth pinning here is not "does the parameter appear". It is that
// the terms are the ones the reader will actually SEE in the rendered page:
// unfolded, prefix-free, and long enough to be worth marking. Every one of
// those is a way to link to a page where nothing highlights, which is worse
// than not linking at all - the reader is told there is a match and then left
// to find it by eye.

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func hlOf(t *testing.T, query string) []string {
	t.Helper()
	return highlightTerms(parseQuery(query))
}

func TestHighlightTerms(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"plain", "fetch json", []string{"fetch", "json"}},

		// A field prefix restricts WHERE a term is looked for; it is not part
		// of the term. Marking the literal "tag:hydro" would find nothing on
		// any page, because no page contains that text.
		{"field prefixes stripped", "tag:hydro title:manual json",
			[]string{"hydro", "manual", "json"}},

		// kind: selects a corpus and contributes no term at all.
		{"kind carries nothing", "kind:md fetch", []string{"fetch"}},

		// An unknown prefix is not a prefix - the whole thing is the term,
		// exactly as parseQuery treats it for matching.
		{"unknown prefix is literal", "http://example.com", []string{"http://example.com"}},

		// One character marks half the page. Both ends drop it, so a link
		// never carries a term the client would refuse to use.
		{"single runes dropped", "a fetch я", []string{"fetch"}},

		// Two spellings of the same word are one highlight.
		{"deduplicated case-insensitively", "Fetch fetch FETCH", []string{"Fetch"}},

		{"empty", "   ", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hlOf(t, c.query)
			if len(got) != len(c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %q, want %q", got, c.want)
				}
			}
		})
	}
}

// The client marks LITERAL occurrences, so what travels on the URL has to be
// the text as typed. Folding is a matching device: it maps 'ё' to 'е' so a
// query for "еж" finds "ёж", but a page that says "ёж" contains no "еж" for
// the highlighter to wrap. Sending the folded form would produce a link that
// reliably highlights nothing.
func TestHighlightTermsAreNotFolded(t *testing.T) {
	for _, q := range []string{"Ёлка", "ПРИВЕТ", "CamelCase"} {
		got := hlOf(t, q)
		if len(got) != 1 || got[0] != q {
			t.Errorf("%q came back as %q; highlight terms must be unfolded", q, got)
		}
	}
	// And the folded form really is different, or this test proves nothing.
	if string(fold("Ёлка")) == "Ёлка" {
		t.Fatal("fold() no longer changes this input; pick another")
	}
}

func TestHighlightURL(t *testing.T) {
	cases := []struct {
		base  string
		terms []string
		want  string
	}{
		{"/Note.html", []string{"fetch", "json"}, "/Note.html?hl=fetch&hl=json"},
		{"/Note.html", nil, "/Note.html"},
		{"", []string{"fetch"}, ""},

		// An existing query string is appended to, not clobbered.
		{"/Note.html?refresh=1", []string{"fetch"}, "/Note.html?refresh=1&hl=fetch"},

		// Anything that would break out of the parameter is encoded. A term
		// comes from a URL a stranger on the LAN can hand over.
		{"/Note.html", []string{"a&b=c"}, "/Note.html?hl=a%26b%3Dc"},
		{"/Note.html", []string{"два слова"}, "/Note.html?hl=%D0%B4%D0%B2%D0%B0+%D1%81%D0%BB%D0%BE%D0%B2%D0%B0"},
	}
	for _, c := range cases {
		if got := highlightURL(c.base, c.terms); got != c.want {
			t.Errorf("highlightURL(%q, %q) = %q, want %q", c.base, c.terms, got, c.want)
		}
	}
}

// Repeated parameters rather than one comma-joined value: a term may contain a
// comma, and re-splitting on it at the far end would silently cut a search for
// "1,000" into two searches that both fail.
func TestHighlightURLKeepsCommasInsideOneTerm(t *testing.T) {
	got := highlightURL("/Note.html", []string{"1,000"})
	if got != "/Note.html?hl=1%2C000" {
		t.Fatalf("got %q", got)
	}
	if strings.Count(got, "hl=") != 1 {
		t.Errorf("a comma inside a term became two terms: %q", got)
	}
}

// The API hands the terms to the dialog, which puts them on the URL itself
// (withHighlight in omn-go-sse.js). One list, computed once, so a result
// behaves the same whether it was opened from the panel or the results page.
func TestSearchAPIReturnsHighlightTerms(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Fetch.md", "Title: Fetch\nTags: Net\n\nawait fetch('/json/test.json');\n")

	_, resp := searchReq(t, a, url.Values{"scope": {"all"}, "q": {"tag:Net fetch"}})
	if resp.Total != 1 {
		t.Fatalf("total %d, want 1", resp.Total)
	}
	want := []string{"Net", "fetch"}
	if len(resp.Highlight) != len(want) {
		t.Fatalf("highlight %q, want %q", resp.Highlight, want)
	}
	for i := range want {
		if resp.Highlight[i] != want[i] {
			t.Fatalf("highlight %q, want %q", resp.Highlight, want)
		}
	}

	// The result's own url stays canonical - it identifies the document, and
	// the index, the cache and every other consumer expect it unadorned. The
	// highlight is a property of the question, so it is reported once.
	if strings.Contains(resp.Results[0].URL, "hl=") {
		t.Errorf("result url carries hl=: %q", resp.Results[0].URL)
	}
}

// Page scope reports them too: the dialog highlights in place there rather
// than navigating, and it uses the same list.
func TestSearchAPIHighlightInPageScope(t *testing.T) {
	a := newTestApp(t)
	a.search = &searchIndex{}
	writeSearchNote(t, a, "Page.md", "Title: Page\n\nthe needle is here\n")

	_, resp := searchReq(t, a, url.Values{
		"scope": {"page"}, "on": {"Page.html"}, "q": {"needle"}})
	if len(resp.Highlight) != 1 || resp.Highlight[0] != "needle" {
		t.Errorf("highlight %q", resp.Highlight)
	}
}

// A query that matched nothing still reports its terms rather than omitting
// them: the field describes the query, and a client that reads it as "these
// are the terms that matched" would be reading it wrong.
func TestSearchAPIHighlightSurvivesNoResults(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: Note\n\nsomething\n")

	_, resp := searchReq(t, a, url.Values{"scope": {"all"}, "q": {"zzzznothing"}})
	if resp.Total != 0 {
		t.Fatalf("total %d", resp.Total)
	}
	if len(resp.Highlight) != 1 || resp.Highlight[0] != "zzzznothing" {
		t.Errorf("highlight %q", resp.Highlight)
	}
}

// Every link on the results page carries the query, and the parameter is
// escaped for the attribute it lands in - the query is attacker-controlled in
// the LAN-sharing case, and this page is assembled by hand.
func TestSearchPage_LinksCarryHighlight(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Fetch.md", "Title: Fetch\n\nawait fetch('/json/a.json');\n")
	writeSearchNote(t, a, "Json.md", "Title: Json\n\nthe json file\n")

	body := getSearchPage(t, a, "json").Body.String()
	for _, want := range []string{
		`href="/Fetch.html?hl=json"`,
		`href="/Json.html?hl=json"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}

	// A tag pill is not a search result and must not pick up the parameter.
	if strings.Contains(body, "OMNGoTags.html#") && strings.Contains(body, "OMNGoTags.html#Test?hl") {
		t.Error("a tag link picked up hl=")
	}

	// Two terms, two parameters, ampersand-escaped for the attribute.
	page := getSearchPage(t, a, "json+fetch").Body.String()
	if !strings.Contains(page, `href="/Fetch.html?hl=json&amp;hl=fetch"`) {
		t.Errorf("two-term link is missing or unescaped:\n%s", excerpt(page, "search-result-title"))
	}
	if strings.Contains(page, `hl=json&hl=`) {
		t.Error("a raw & reached an href attribute")
	}
}

// Every matching LINE is its own link, and each one carries THAT line's text
// as ?hlt=. Without it each snippet under a result went to the same place, and
// a reader who chose the second line arrived at the first match in the note.
func TestSearchPage_SnippetLinksCarryTheirOwnLine(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Many.md", "Title: Many\n\nfirst needle line\nfiller\nsecond needle line\n")

	body := getSearchPage(t, a, "needle").Body.String()
	for _, want := range []string{
		`href="/Many.html?hl=needle&amp;hlt=first+needle+line"`,
		`href="/Many.html?hl=needle&amp;hlt=second+needle+line"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q:\n%s", want, excerpt(body, "search-snippet"))
		}
	}
	if strings.Contains(body, `<div class="search-snippet"`) {
		t.Error("a snippet is still plain text; every line has to be a link")
	}
}

// A note's own text reaches an href through ?hlt=, so it passes the same two
// encoders the query does: percent-encoding for the query string, then
// HTML-escaping for the attribute.
//
// The payload holds no "<script", on purpose. classifyContexts labels each
// line that contains it "script", and such a line gets no ?hlt= at all (the
// test below), so a payload built around one would test nothing here.
func TestSearchPage_SnippetLineInLinkIsEscaped(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: Note\n\nthe \"><img src=x onerror=alert(1)> payload sits here\n")

	link := excerpt(getSearchPage(t, a, "payload").Body.String(), "search-snippet")
	if !strings.Contains(link, `hlt=the+%22%3E%3Cimg`) {
		t.Errorf("the line was not percent-encoded into the link:\n%s", link)
	}
	// The visible snippet shows this as "&lt;img", so the tag can only be here
	// if the line closed the attribute it travelled in.
	if strings.Contains(link, `<img`) {
		t.Errorf("the line broke out of the href:\n%s", link)
	}
}

// A hit inside a <script> block gets no ?hlt=. The text is in the index but
// the page never renders it, so there is no word to go to and a search of the
// page for that text could only find a coincidence somewhere else.
func TestSearchPage_ScriptLineGetsNoTarget(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Script.md", "Title: Script\n\n<script>\nvar payload = 1;\n</script>\n")

	link := excerpt(getSearchPage(t, a, "payload").Body.String(), "search-snippet")
	if !strings.Contains(link, `href="/Script.html?hl=payload"`) {
		t.Errorf("the link is not the plain one:\n%s", link)
	}
	if strings.Contains(link, "hlt=") {
		t.Errorf("a line that the page never renders asked to be scrolled to:\n%s", link)
	}
}

// The term travelling onto the link is attacker-controlled in the LAN-sharing
// case: it comes straight off the URL. It passes through two encoders on the
// way - percent-encoding for the query string, then HTML-escaping for the
// attribute - and both have to be there.
func TestSearchPage_QueryInLinkIsEscaped(t *testing.T) {
	a := enabledSearchApp(t)
	// The note contains the payload, so the query matches and the term
	// actually reaches a link rather than the empty state.
	writeSearchNote(t, a, "Note.md", "Title: Note\n\nthe \"><script> payload sits here\n")

	page := getSearchPage(t, a, `%22%3E%3Cscript%3E`).Body.String()
	link := excerpt(page, "search-result-title")

	if !strings.Contains(link, `href="/Note.html?hl=%22%3E%3Cscript%3E"`) {
		t.Errorf("the term was not percent-encoded into the link:\n%s", link)
	}
	// Whatever else is on the page, this attribute must not have been closed
	// early by the term inside it.
	if strings.Contains(link, `<script`) || strings.Contains(link, `">`+`<`) {
		t.Errorf("the query broke out of the href:\n%s", link)
	}
}

// excerpt pulls the line containing needle out of a page, for readable
// failures on a document that is otherwise thousands of characters long.
func excerpt(page, needle string) string {
	for _, line := range strings.Split(page, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return "(" + needle + " not found)"
}

// ---------------------------------------------------------------------
// The frontend copy of the fold table
//
// The server folds before it matches, and it sends the UNFOLDED term in
// ?hl= (see TestHighlightTermsAreNotFolded above). omn-go-core.js thus has
// to fold both sides itself, or a note titled "Ёлка" opens with nothing
// marked after a search for "елка". It did exactly that until 26.08.79.
//
// The table therefore exists twice, in Go and in JavaScript. That is the
// same arrangement as isHeaderFirstLine, and it needs the same guard: a
// test that fails when one copy moves and the other does not.
// ---------------------------------------------------------------------

// jsFoldTableRe reads the body of OMN_FOLD_TABLE from omn-go-core.js.
var jsFoldTableRe = regexp.MustCompile(`(?s)var OMN_FOLD_TABLE = \{(.*?)\n\};`)

// jsFoldPairRe reads one row. Both sides are \u escapes or plain ASCII.
var jsFoldPairRe = regexp.MustCompile(`'(\\u[0-9a-fA-F]{4}|[ -~])':\s*'(\\u[0-9a-fA-F]{4}|[ -~])'`)

// jsFoldRune turns one side of a row into the rune it names.
func jsFoldRune(t *testing.T, lit string) rune {
	t.Helper()
	if !strings.HasPrefix(lit, `\u`) {
		return []rune(lit)[0]
	}
	n, err := strconv.ParseInt(lit[2:], 16, 32)
	if err != nil {
		t.Fatalf("bad escape %q in OMN_FOLD_TABLE: %v", lit, err)
	}
	return rune(n)
}

func TestFoldTableHasAFrontendCopy(t *testing.T) {
	raw, err := staticFS.ReadFile("frontend/html/js/omn-go-core.js")
	if err != nil {
		t.Fatalf("omn-go-core.js is not embedded: %v", err)
	}
	body := jsFoldTableRe.FindStringSubmatch(string(raw))
	if body == nil {
		t.Fatal("omn-go-core.js holds no OMN_FOLD_TABLE. " +
			"The page highlight then misses every folded character, and a " +
			"search for \"елка\" opens \"Ёлка\" with nothing marked.")
	}

	js := map[rune]rune{}
	for _, m := range jsFoldPairRe.FindAllStringSubmatch(body[1], -1) {
		js[jsFoldRune(t, m[1])] = jsFoldRune(t, m[2])
	}

	// Same size, and the same entry for each key. A missing row is a
	// character the reader sees marked in the panel and not in the page.
	// An extra row marks a word that the server never matched.
	for from, to := range foldTable {
		got, ok := js[from]
		switch {
		case !ok:
			t.Errorf("foldTable maps %q to %q, and OMN_FOLD_TABLE has no row "+
				"for it. Add '\\u%04x': '\\u%04x' to omn-go-core.js.", from, to, from, to)
		case got != to:
			t.Errorf("foldTable maps %q to %q, OMN_FOLD_TABLE maps it to %q", from, to, got)
		}
	}
	for from, to := range js {
		if _, ok := foldTable[from]; !ok {
			t.Errorf("OMN_FOLD_TABLE maps %q to %q, and foldTable has no row "+
				"for it. The page then marks a word the server never matched.", from, to)
		}
	}

	// Every row must keep the length, in both copies. A fold that grows
	// moves every span after it, and the marks land on the wrong words.
	for from, to := range foldTable {
		if len(string(from)) != len(string(from)) || to == 0 {
			t.Errorf("row %q is not one rune to one rune", from)
		}
		if utf8.RuneCountInString(string(to)) != 1 {
			t.Errorf("foldTable maps %q to %d runes, want 1", from, utf8.RuneCountInString(string(to)))
		}
	}

	// The two call sites must use the fold, or the table is dead code.
	for _, want := range []string{
		"omnFold(t)",      // the needles of omnHighlightTerms
		"omnFold(value)",  // the text it searches
		"omnFoldChar(c)",  // omnFlatten, which places the mark
		"folded.indexOf(", // the search itself
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("omn-go-core.js does not hold %q. The fold table is "+
				"then present and unused, which is worse than absent.", want)
		}
	}
}
