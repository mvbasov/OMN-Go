package backend

// Tests for sections: the machinery that lets a result point at the bookmark
// or the timestamped entry it matched rather than at the top of the page.
//
// Two very different risks live here, and the tests are shaped around them.
//
// The bookmarks parser is a CORRECTNESS fix - text that was unfindable becomes
// findable - so those tests are about what can now be found, and about not
// corrupting the file's data while cleaning up its syntax.
//
// The heading anchors are a WRONGNESS risk. The ids are minted by goldmark at
// compile time and predicted here, and a prediction that is merely close sends
// the reader to a different section of the right page - which reads as a bug in
// the note, not in the search. So those tests check agreement with the real
// renderer, and check that every path that cannot guarantee agreement declines
// instead.

import (
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// forceAnchors pins the renderer self-check for one test.
//
// anchorsPredictable() compiles a probe through goldmark once per process, so a
// test cannot simply set a flag. Resetting the Once is safe here: tests in a
// package run one at a time unless they say otherwise, and nothing else reads
// the value concurrently.
func forceAnchors(t *testing.T, ok bool) {
	t.Helper()
	anchorsOnce = sync.Once{}
	anchorsOnce.Do(func() { anchorsGood = ok })
	t.Cleanup(func() {
		anchorsOnce = sync.Once{}
		anchorsGood = false
		anchorsOnce = sync.Once{}
	})
}

// ----------------------------------------------------------------------
// Anchor prediction
// ----------------------------------------------------------------------

// THE test for this phase. Everything else about headings is arrangement; this
// is the one that says the anchors are real.
//
// It does not compare against a recorded golden value, it compares against the
// renderer that is actually linked into this build. A golden file says "this is
// what goldmark did the day someone ran the test"; this says "this is what
// goldmark does here, now" - which is the claim a link into a section makes.
func TestHeadingIDsAgreeWithTheRenderer(t *testing.T) {
	a := &App{}

	docs := []string{
		"# Hello World",
		"## Mixed Case Heading",
		"### A: b, c! (d)",
		"##### 2026-07-27 07:23:17",
		"# a-b c",
		"# 2026",
		"# one\n\n## two\n\n### three",
		"# same\n\ntext\n\n# same\n\ntext\n\n# same", // collision suffixes
		"# Note Заметки 42",                          // non-ASCII runes are skipped, the rest survives
		"# a *b* c",                                  // emphasis contributes nothing under either reading
		"# a ~~b~~ c",                                // ... nor does strikethrough
		"# see https://x.co",                         // linkify does not change the text
	}

	for _, md := range docs {
		t.Run(strings.SplitN(md, "\n", 2)[0], func(t *testing.T) {
			forceAnchors(t, true)

			want := headingIDs2(a, md)
			got := predictedIDs(md)

			if len(got) != len(want) {
				t.Fatalf("predicted %d ids %q, renderer emitted %d %q", len(got), got, len(want), want)
			}
			for i := range want {
				if got[i] == "" {
					continue // declined: allowed, and covered by the test below
				}
				if got[i] != want[i] {
					t.Errorf("heading %d: predicted %q, renderer emitted %q\n"+
						"An anchor that disagrees sends the reader to the wrong "+
						"section. If goldmark's rule changed, headingSlug and "+
						"anchorProbe must change with it.", i+1, got[i], want[i])
				}
			}
		})
	}
}

// The refusals. Each of these is a construct where the source and the rendered
// text differ, so predicting from the source would be guessing - and the whole
// design here is that a missing anchor beats a wrong one.
func TestHeadingIDsDeclineWhatTheyCannotRead(t *testing.T) {
	forceAnchors(t, true)

	cases := []struct{ name, md, why string }{
		{"link", "# see [the docs](http://x.co/y)",
			"the URL is not in the rendered text"},
		{"inline code", "# the `fetch` call",
			"renderMarkdownToHTML shields it as a placeholder before goldmark sees it"},
		{"math", "# cost is $x^2$",
			"shielded the same way"},
		{"entity", "# Cats &amp; Dogs",
			"the entity renders as one character"},
		{"inline html", "# a <b>bold</b> word",
			"the tags are not text"},
		{"underscore", "# a_b",
			"an emphasis marker whose literal mapping differs from its markup one"},
		{"closing hashes", "# trailing hashes ##",
			"an ATX closing sequence is not part of the heading's text"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if ids := predictedIDs(c.md); len(ids) != 1 || ids[0] != "" {
				t.Errorf("predicted %q for %q; it must decline because %s", ids, c.md, c.why)
			}
		})
	}

	// And declining is contagious within a document: goldmark has consumed an
	// id this code cannot compute, so any later heading might legitimately be
	// its "-1" and every later prediction is a guess.
	ids := predictedIDs("# a `b` c\n\ntext\n\n# plain\n\ntext\n\n# plainer")
	for i, id := range ids {
		if id != "" {
			t.Errorf("heading %d still predicted %q after an unreadable heading; "+
				"the collision counter is out of step from that point on", i+1, id)
		}
	}
}

// A wholly non-ASCII heading degenerates to goldmark's "heading" fallback,
// which addresses nothing a reader would recognise and collides with the next
// one. Section, yes; anchor, no (§5.5).
func TestCyrillicHeadingGetsNoAnchor(t *testing.T) {
	forceAnchors(t, true)

	secs := sectionsOf(t, "# Заметки\n\nтекст\n")
	if len(secs) != 1 {
		t.Fatalf("expected one section, got %d", len(secs))
	}
	if secs[0].label != "Заметки" {
		t.Errorf("label %q; the section must still be NAMED", secs[0].label)
	}
	if secs[0].id != "" {
		t.Errorf("id %q; a degenerate anchor is worse than none", secs[0].id)
	}
}

// The setext form is a heading this file deliberately does not read. What
// matters is that the construct QuickNotes actually uses - "---" after a blank
// line, a thematic break - is not mistaken for one.
func TestSetextRuleIsHandledConservatively(t *testing.T) {
	forceAnchors(t, true)

	// A real setext heading: give up from there.
	ids := predictedIDs("A Heading\n=========\n\ntext\n\n# after\n")
	for i, id := range ids {
		if id != "" {
			t.Errorf("id %d is %q; a setext underline was seen and its id cannot be predicted", i+1, id)
		}
	}

	// QuickNotes' own shape. If this ever poisons, every quick note in the app
	// loses its anchor - which is the entire feature.
	quick := "\n---\n##### 2026-07-27 07:23:17\nfirst\n\n---\n##### 2026-07-27 07:23:18\nsecond\n"
	got := predictedIDs(quick)
	want := []string{"2026-07-27-072317", "2026-07-27-072318"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q, want %q - a '---' separator was read as a setext underline", got, want)
		}
	}
}

// A heading inside a fence is not a heading, and the cost of thinking it is
// runs past the fence: goldmark never counted it, so every collision suffix
// afterwards would be off by one.
func TestFencedHeadingIsNotASection(t *testing.T) {
	forceAnchors(t, true)

	secs := sectionsOf(t, "# real\n\n```\n# fake\n```\n\n# real\n")
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want 2: %+v", len(secs), secs)
	}
	if secs[0].id != "real" || secs[1].id != "real-1" {
		t.Errorf("ids %q/%q, want real/real-1", secs[0].id, secs[1].id)
	}
}

func TestSectionsCoverTheWholeBody(t *testing.T) {
	forceAnchors(t, true)

	secs := sectionsOf(t, "preamble text\n\n# one\n\nbody\n\n# two\n\nmore\n")
	if len(secs) != 3 {
		t.Fatalf("got %d sections, want 3 (preamble + two headings)", len(secs))
	}
	// The preamble belongs to no heading, so it is labelled by nothing and
	// linked by nothing - but it must not be swallowed by the first heading.
	if secs[0].label != "" || secs[0].id != "" {
		t.Errorf("preamble got label %q id %q; it is not a section anyone named", secs[0].label, secs[0].id)
	}
	if secs[0].end >= secs[1].start {
		t.Errorf("preamble [%d,%d] overlaps the first heading at %d", secs[0].start, secs[0].end, secs[1].start)
	}
	for i := 1; i < len(secs); i++ {
		if secs[i-1].end >= secs[i].start {
			t.Errorf("sections %d and %d overlap", i-1, i)
		}
	}
}

func TestFlatDocumentHasNoSections(t *testing.T) {
	forceAnchors(t, true)
	if secs := sectionsOf(t, "just some prose\n\nand more of it\n"); secs != nil {
		t.Errorf("got %+v; a document with no headings has no parts to address", secs)
	}
}

// With the self-check failed, sections still exist - they are what labels a hit
// - but nothing is addressable. That is the safe degradation, and it needs to
// be the observable one.
func TestAnchorsOffStillProducesLabelledSections(t *testing.T) {
	forceAnchors(t, false)

	secs := sectionsOf(t, "# one\n\nbody\n")
	if len(secs) != 1 || secs[0].label != "one" {
		t.Fatalf("got %+v, want one section labelled 'one'", secs)
	}
	if secs[0].id != "" {
		t.Errorf("id %q emitted while the renderer self-check is failing", secs[0].id)
	}
}

func TestTimestampAnchorMatchesBookmarkerJS(t *testing.T) {
	// The rule from Bookmarker.js: remove ':', turn ' ' into '-'.
	for in, want := range map[string]string{
		"2026-06-15 20:00:00": "2026-06-15-200000",
		"2026-01-02 03:04:05": "2026-01-02-030405",
		"":                    "",
	} {
		if got := timestampAnchor(in); got != want {
			t.Errorf("timestampAnchor(%q) = %q, want %q", in, got, want)
		}
	}

	// And it agrees with the heading rule for the same instant, which is what
	// lets a quick note and a bookmark captured together address alike.
	forceAnchors(t, true)
	ids := predictedIDs("##### 2026-06-15 20:00:00")
	if len(ids) != 1 || ids[0] != timestampAnchor("2026-06-15 20:00:00") {
		t.Errorf("heading id %q != timestampAnchor %q", ids, timestampAnchor("2026-06-15 20:00:00"))
	}
}

// ----------------------------------------------------------------------
// The bookmarks array
// ----------------------------------------------------------------------

// bookmarksNote builds a Bookmarks.md the way handleBookmark writes one:
// entries newest-first, inserted directly after the marker comment that sits
// INSIDE the array.
func bookmarksSource(entries ...string) string {
	return "Title: Bookmarks\nCategory: System\nTags: Bookmarks\n\n" +
		"<script>bookmarks = [\n" +
		"<!-- Don't edit body below this line -->\n" +
		strings.Join(entries, "") +
		"];\n</script>\n\n<!-- end of bookmarks definition -->\n"
}

func bookmarkJSON(date, url, title string, tags, notes []string, trailingComma bool) string {
	b, err := json.MarshalIndent(map[string]any{
		"date": date, "url": url, "title": title, "tags": tags, "notes": notes,
	}, "  ", "  ")
	if err != nil {
		panic(err)
	}
	out := "  " + string(b)
	if trailingComma {
		out += ","
	}
	return out + "\n"
}

// THE bug this phase fixes. json.MarshalIndent writes '<', '>' and '&' as \u
// escapes so a bookmark cannot break out of the <script> it lives in - which
// means the readable text is nowhere in the file, and a line-based index could
// never match it. Every bookmark whose title contains an ampersand was
// invisible to search.
func TestBookmarkWithEscapedPunctuationIsFindable(t *testing.T) {
	a := newTestApp(t)
	src := bookmarksSource(bookmarkJSON(
		"2026-06-15 20:00:00", "https://example.org/cats",
		"Cats & Dogs <the sequel>", []string{"Pets"}, []string{"a > b"}, false))
	writeSearchNote(t, a, "Bookmarks.md", src)

	// The premise, asserted rather than assumed: the readable text really is
	// absent from the source.
	if strings.Contains(src, "Cats & Dogs") {
		t.Fatal("the fixture is not \\u-escaped; this test proves nothing")
	}
	if !strings.Contains(src, `\u0026`) {
		t.Fatal("expected a \\u-escaped ampersand in the fixture")
	}

	doc := loadDoc(t, a, "Bookmarks")
	if doc.Kind != SearchKindBookmarks {
		t.Errorf("kind %q, want %q", doc.Kind, SearchKindBookmarks)
	}
	for _, want := range []string{"Cats & Dogs", "the sequel", "a > b", "example.org/cats", "Pets"} {
		if !docContains(doc, want) {
			t.Errorf("%q is not searchable; the decoded entry did not reach the index", want)
		}
	}
}

func TestBookmarkEntriesBecomeSections(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Bookmarks.md", bookmarksSource(
		bookmarkJSON("2026-06-16 17:58:30", "https://b.example", "Newer", nil, nil, true),
		bookmarkJSON("2026-06-15 20:00:00", "https://a.example", "Older", nil, nil, false),
	))

	doc := loadDoc(t, a, "Bookmarks")
	if len(doc.sections) != 2 {
		t.Fatalf("got %d sections, want 2: %+v", len(doc.sections), doc.sections)
	}
	if doc.sections[0].id != "2026-06-16-175830" || doc.sections[0].label != "Newer" {
		t.Errorf("first section %+v", doc.sections[0])
	}
	if doc.sections[1].id != "2026-06-15-200000" || doc.sections[1].label != "Older" {
		t.Errorf("second section %+v", doc.sections[1])
	}

	// Each entry's line lands inside its own section, which is the whole
	// contract - a hit attributed to the wrong entry links to the wrong place.
	for _, ln := range doc.lines {
		sec := sectionFor(doc.sections, ln.no)
		if sec == nil {
			continue // prose outside the array
		}
		if !strings.Contains(ln.raw, sec.label) {
			t.Errorf("line %d (%q) resolved to section %q", ln.no, ln.raw, sec.label)
		}
	}
}

// A bookmark with no title is shown by its URL - the same fallback
// Bookmarker.js uses when it builds the <li>.
func TestBookmarkWithoutTitleIsLabelledByURL(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Bookmarks.md", bookmarksSource(
		bookmarkJSON("2026-06-15 20:00:00", "https://a.example/x", "", nil, nil, false)))

	doc := loadDoc(t, a, "Bookmarks")
	if len(doc.sections) != 1 || doc.sections[0].label != "https://a.example/x" {
		t.Fatalf("sections %+v", doc.sections)
	}
}

// The two things in the file that are not JSON, and one that must survive
// intact. The marker comment and a trailing comma are syntax to be cleaned up;
// the same character sequences INSIDE a quoted string are the user's data.
func TestScanBookmarksArrayCleansSyntaxNotData(t *testing.T) {
	src := "x = 1;\nbookmarks = [\n<!-- Don't edit body below this line -->\n" +
		"  {\"title\": \"a <!-- b -->\", \"notes\": [\"ends with ,]\"]},\n" +
		"];\n"

	block, ok := scanBookmarksArray(src, 1)
	if !ok {
		t.Fatal("array not found")
	}

	var entries []bookmarkEntry
	if err := json.Unmarshal([]byte(block.json), &entries); err != nil {
		t.Fatalf("cleaned text is not JSON: %v\n%s", err, block.json)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}
	if entries[0].Title != "a <!-- b -->" {
		t.Errorf("title %q; a comment marker inside a string is data, not syntax", entries[0].Title)
	}
	if len(entries[0].Notes) != 1 || entries[0].Notes[0] != "ends with ,]" {
		t.Errorf("notes %q; a trailing-comma-looking sequence inside a string is data", entries[0].Notes)
	}

	if got, want := block.startLines, []int{4}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("entry start lines %v, want %v", got, want)
	}
	if block.firstLine != 2 || block.lastLine != 5 {
		t.Errorf("array spans lines %d..%d, want 2..5", block.firstLine, block.lastLine)
	}
}

// An empty list plus one added bookmark is exactly how a fresh install produces
// a trailing comma, so this is the common case rather than a pathological one.
func TestScanBookmarksArrayToleratesATrailingComma(t *testing.T) {
	src := bookmarksSource(bookmarkJSON("2026-06-15 20:00:00", "u", "t", nil, nil, true))
	block, ok := scanBookmarksArray(src, 1)
	if !ok {
		t.Fatal("array not found")
	}
	var entries []bookmarkEntry
	if err := json.Unmarshal([]byte(block.json), &entries); err != nil {
		t.Fatalf("a trailing comma survived the scan: %v\n%s", err, block.json)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
}

// Prose a user typed into Bookmarks.md by hand is still findable. The file is
// machine-managed, but "I typed it and search cannot find it" is a bad answer
// whatever the file's provenance.
func TestBookmarksNoteIndexesProseOutsideTheArray(t *testing.T) {
	a := newTestApp(t)
	src := "Title: Bookmarks\nCategory: System\n\nA reminder about pruning duplicates.\n\n" +
		"<script>bookmarks = [\n<!-- Don't edit body below this line -->\n" +
		bookmarkJSON("2026-06-15 20:00:00", "https://a.example", "T", nil, nil, false) +
		"];\n</script>\n"
	writeSearchNote(t, a, "Bookmarks.md", src)

	doc := loadDoc(t, a, "Bookmarks")
	if !docContains(doc, "pruning duplicates") {
		t.Error("hand-written prose above the array is not searchable")
	}
	// ... and the escaped JSON is not indexed twice, once decoded and once raw.
	for _, ln := range doc.lines {
		if strings.Contains(ln.raw, `"date"`) {
			t.Errorf("raw JSON line %d is still indexed: %q", ln.no, ln.raw)
		}
	}
}

// A file that is not the shape this expects degrades to plain line indexing
// rather than silently indexing nothing.
func TestUnreadableBookmarksFileFallsBackToLines(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Bookmarks.md",
		"Title: Bookmarks\n\n<script>bookmarks = [ {not json at all\n</script>\nplain text here\n")

	doc := loadDoc(t, a, "Bookmarks")
	if !docContains(doc, "plain text here") {
		t.Error("a malformed bookmarks file indexed nothing at all")
	}
}

// ----------------------------------------------------------------------
// End to end: the anchor reaches the result
// ----------------------------------------------------------------------

func TestGlobalResultCarriesTheSectionAnchor(t *testing.T) {
	forceAnchors(t, true)
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Bookmarks.md", bookmarksSource(
		bookmarkJSON("2026-06-15 20:00:00", "https://github.com/mvbasov/OMN-Go",
			"OMN-Go on GitHub", []string{"Local"}, nil, false)))

	_, resp := searchReq(t, a, url.Values{"scope": {"all"}, "q": {"mvbasov"}})
	if resp.Total != 1 {
		t.Fatalf("total %d, want 1", resp.Total)
	}
	r := resp.Results[0]
	if r.URL != "/Bookmarks.html#2026-06-15-200000" {
		t.Errorf("url %q; the result must open AT the entry", r.URL)
	}
	if len(r.Matches) != 1 || r.Matches[0].Section == nil {
		t.Fatalf("matches %+v", r.Matches)
	}
	if r.Matches[0].Section.Label != "OMN-Go on GitHub" {
		t.Errorf("section label %q", r.Matches[0].Section.Label)
	}
	if !strings.Contains(r.Matches[0].Text, "github.com/mvbasov") {
		t.Errorf("snippet %q does not carry the decoded entry", r.Matches[0].Text)
	}
}

func TestQuickNoteResultCarriesTheSectionAnchor(t *testing.T) {
	forceAnchors(t, true)
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "QuickNotes.md",
		"Title: Quick Notes\nCategory: System\n\n"+
			"\n---\n##### 2026-07-27 07:23:17\nremember the milk\n"+
			"\n---\n##### 2026-07-26 09:00:00\nsomething else\n")

	_, resp := searchReq(t, a, url.Values{"scope": {"all"}, "q": {"milk"}})
	if resp.Total != 1 {
		t.Fatalf("total %d", resp.Total)
	}
	if got := resp.Results[0].URL; got != "/QuickNotes.html#2026-07-27-072317" {
		t.Errorf("url %q, want the entry's anchor", got)
	}
	if s := resp.Results[0].Matches[0].Section; s == nil || s.Label != "2026-07-27 07:23:17" {
		t.Errorf("section %+v", s)
	}
}

// Page scope deliberately does not anchor: the reader is already on the page,
// and the dialog highlights in place rather than navigating.
func TestPageScopeDoesNotAnchor(t *testing.T) {
	forceAnchors(t, true)
	a := newTestApp(t)
	a.search = &searchIndex{}
	writeSearchNote(t, a, "Notes.md", "Title: Notes\n\n# One\n\nthe needle\n")

	_, resp := searchReq(t, a, url.Values{
		"scope": {"page"}, "on": {"Notes.html"}, "q": {"needle"}})
	if len(resp.Results) != 1 {
		t.Fatalf("results %+v", resp.Results)
	}
	if strings.Contains(resp.Results[0].URL, "#") {
		t.Errorf("url %q carries an anchor in page scope", resp.Results[0].URL)
	}
	// The section is still reported: it is what labels the row.
	if s := resp.Results[0].Matches[0].Section; s == nil || s.Label != "One" {
		t.Errorf("section %+v; page scope still says WHERE the hit is", s)
	}
}

// ?hl= and #anchor have to coexist, and a query string goes before a fragment.
func TestHighlightURLKeepsTheFragmentLast(t *testing.T) {
	got := highlightURL("/Bookmarks.html#2026-06-15-200000", []string{"cats", "dogs"})
	want := "/Bookmarks.html?hl=cats&hl=dogs#2026-06-15-200000"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// ----------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------

// predictedIDs runs the sectionizer over a body and returns the ids it would
// emit, in document order. "" means "declined".
func predictedIDs(body string) []string {
	lines := strings.Split(body, "\n")
	out := []string{}
	for _, s := range sectionsFromHeadings(lines, classifyContexts(lines), 1) {
		if s.label == "" && s.id == "" {
			continue // the preamble is not a heading
		}
		out = append(out, s.id)
	}
	return out
}

// headingIDs2 asks the REAL renderer what ids it emits. Named apart from the
// baseline suite's helper so the two files stay independent.
func headingIDs2(a *App, md string) []string {
	out := []string{}
	for _, m := range headingIDAttrRe.FindAllStringSubmatch(a.renderMarkdownToHTML([]byte(md)), -1) {
		out = append(out, m[1])
	}
	return out
}

func sectionsOf(t *testing.T, body string) []docSection {
	t.Helper()
	lines := strings.Split(body, "\n")
	return sectionsFromHeadings(lines, classifyContexts(lines), 1)
}

func loadDoc(t *testing.T, a *App, name string) *searchDocument {
	t.Helper()
	doc, err := a.loadPageDocument(name)
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		t.Fatalf("%s did not resolve to a document", name)
	}
	return doc
}

// docContains reports whether any indexed line carries the text - the question
// "would search find this", asked of the document rather than of a query, so a
// failure points at the parser rather than at the matcher.
func docContains(d *searchDocument, want string) bool {
	for _, ln := range d.lines {
		if strings.Contains(ln.raw, want) {
			return true
		}
	}
	return false
}

// A result links to its best hit's section, but each section HEADING on the
// results page links to its own. Both come off the same document URL, which
// already carries one anchor - so the second must replace it rather than pile
// onto it: "/Q.html#a#b" is a link to nothing.
func TestSearchPageSectionLinksDoNotStackFragments(t *testing.T) {
	forceAnchors(t, true)
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Q.md",
		"Title: Q\nCategory: System\n\n"+
			"\n---\n##### 2026-07-27 07:23:17\nneedle one\n"+
			"\n---\n##### 2026-07-26 09:00:00\nneedle two\n")

	body := getSearchPage(t, a, "needle").Body.String()
	if strings.Contains(body, "#2026-07-27-072317#") || strings.Contains(body, "#2026-07-26-090000#") {
		t.Fatalf("two fragments landed in one href:\n%s", excerpt(body, "search-section"))
	}
	for _, want := range []string{
		`href="/Q.html?hl=needle#2026-07-27-072317"`,
		`href="/Q.html?hl=needle#2026-07-26-090000"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q\n%s", want, excerpt(body, "search-section"))
		}
	}
}
