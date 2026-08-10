package backend

// Tests for the global index.
//
// Two of these matter more than the rest:
//
//   - TestIndexFilterHasNoFalseNegatives, because a document wrongly rejected
//     by the mask/trigram filter does not fail loudly - it silently never
//     appears in results, which is indistinguishable from "you have no note
//     about that". It is checked against a brute-force scan that reads and
//     scores every document, over a fuzzed corpus.
//   - TestIndexHoldsNoText, because the entire memory argument for this design
//     rests on the index not being a copy of the notes.

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func enabledSearchApp(t *testing.T, kinds ...string) *App {
	t.Helper()
	a := newTestApp(t)
	a.search = &searchIndex{}
	if len(kinds) == 0 {
		kinds = []string{SearchKindMD, SearchKindBookmarks}
	}
	a.WithConfig(func(c *Config) {
		c.SearchEnabled = true
		c.SearchKinds = kinds
	})
	return a
}

func writeAsset(t *testing.T, a *App, rel, content string) {
	t.Helper()
	p := filepath.Join(a.StorageDir, "html", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------
// What gets indexed
// ---------------------------------------------------------------------

func TestIndexRespectsConfiguredKinds(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nneedle in a note\n")
	writeAsset(t, a, "js/mine.js", "// needle in a script\n")
	writeAsset(t, a, "json/data.json", `{"needle": "in json"}`)
	writeAsset(t, a, "user_json/up.json", `{"needle": "uploaded"}`)

	a.rebuildSearchIndex()
	if got := indexedPaths(a); strings.Join(got, ",") != "md/Note.md" {
		t.Errorf("default kinds indexed %v, want only the note", got)
	}

	// Opting scripts in brings them, and nothing else.
	a.WithConfig(func(c *Config) { c.SearchKinds = []string{SearchKindMD, SearchKindJS} })
	a.rebuildSearchIndex()
	got := indexedPaths(a)
	if len(got) != 2 || !containsPath(got, "html/js/mine.js") {
		t.Errorf("with js enabled: %v", got)
	}

	// Unticking everything indexes nothing - and must not fall back to the
	// default (see normalizeSearchKinds).
	a.WithConfig(func(c *Config) { c.SearchKinds = []string{} })
	a.rebuildSearchIndex()
	if got := indexedPaths(a); len(got) != 0 {
		t.Errorf("no kinds selected, but indexed %v", got)
	}
}

func TestIndexExclusions(t *testing.T) {
	a := enabledSearchApp(t, SearchKindMD, SearchKindJS)
	writeSearchNote(t, a, "Keep.md", "Title: Keep\n\nkeep me\n")
	writeSearchNote(t, a, "OMNGoTags.md", "Title: Tags\n\ngenerated from the notes\n")
	writeSearchNote(t, a, "local/Scratch.md", "Title: Scratch\n\ngitignored scratch\n")
	writeAsset(t, a, "js/omn-go-core.js", "// the app's own code\n")
	writeAsset(t, a, "js/katex.min.js", "// a bundled library\n")
	writeAsset(t, a, "js/mine.js", "// my own script\n")

	a.rebuildSearchIndex()
	got := indexedPaths(a)

	for _, unwanted := range []string{
		"md/OMNGoTags.md",        // derived from the notes; indexing it duplicates them
		"md/local/Scratch.md",    // the gitignored scratch tree
		"html/js/omn-go-core.js", // shipped with the app
		"html/js/katex.min.js",   // ... and a bundled library
	} {
		if containsPath(got, unwanted) {
			t.Errorf("%s should not be indexed by default; got %v", unwanted, got)
		}
	}
	for _, wanted := range []string{"md/Keep.md", "html/js/mine.js"} {
		if !containsPath(got, wanted) {
			t.Errorf("%s should be indexed; got %v", wanted, got)
		}
	}

	// The opt-in brings the app's own code, which is the whole point of it.
	a.WithConfig(func(c *Config) { c.SearchBundled = true })
	a.rebuildSearchIndex()
	if got := indexedPaths(a); !containsPath(got, "html/js/omn-go-core.js") {
		t.Errorf("SearchBundled did not include the shipped scripts: %v", got)
	}
}

func TestIndexBookmarksAreTheirOwnKind(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nprose\n")
	writeSearchNote(t, a, "Bookmarks.md", "Title: Incoming bookmarks\n\n<script>bookmarks = [\n];\n</script>")

	a.rebuildSearchIndex()
	kinds := map[string]string{}
	for _, d := range a.snapshotDocs() {
		kinds[d.Path] = d.Kind
	}
	if kinds["md/Bookmarks.md"] != SearchKindBookmarks {
		t.Errorf("Bookmarks.md indexed as %q, want the bookmarks kind", kinds["md/Bookmarks.md"])
	}
	if kinds["md/Note.md"] != SearchKindMD {
		t.Errorf("a note indexed as %q", kinds["md/Note.md"])
	}

	// Turning notes off leaves bookmarks, and vice versa.
	a.WithConfig(func(c *Config) { c.SearchKinds = []string{SearchKindBookmarks} })
	a.rebuildSearchIndex()
	if got := indexedPaths(a); strings.Join(got, ",") != "md/Bookmarks.md" {
		t.Errorf("bookmarks only: %v", got)
	}
}

// The memory argument for this whole design is that the index is not a copy of
// the notes. If a text field ever appears on indexedDoc, that argument is
// gone - and it would be an easy thing to add without noticing.
func TestIndexHoldsNoText(t *testing.T) {
	a := enabledSearchApp(t)
	const secret = "supercalifragilistic"
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\n"+secret+" body text\n")
	a.rebuildSearchIndex()

	docs := a.snapshotDocs()
	if len(docs) != 1 {
		t.Fatalf("got %d docs", len(docs))
	}
	d := docs[0]

	// Metadata is kept on purpose - a result has to be describable without a
	// read. Body text is not.
	for _, s := range append([]string{d.Path, d.Kind, d.Name, d.Title, d.URL}, d.Tags...) {
		if strings.Contains(s, secret) {
			t.Errorf("document body leaked into the index: %q", s)
		}
	}
	if len(d.LineMasks) == 0 {
		t.Error("no line masks kept; the filter would have nothing to work with")
	}
	// One 64-bit mask per non-blank line, and that is all the per-line cost.
	if got, want := len(d.LineMasks), 1; got != want {
		t.Errorf("line masks = %d, want %d (blank lines are not kept)", got, want)
	}
}

// ---------------------------------------------------------------------
// The filter
// ---------------------------------------------------------------------

// A document wrongly rejected here never appears in any result, and nothing
// fails - so this compares the filtered path against a brute-force scan that
// reads and scores every document, over a corpus built to produce plenty of
// near misses.
func TestIndexFilterHasNoFalseNegatives(t *testing.T) {
	a := enabledSearchApp(t)

	words := []string{
		"fetch", "json", "android", "intent", "termux", "bookmark", "storage",
		"render", "markdown", "editor", "database", "backup", "sync", "commit",
		"response", "token", "handler", "template", "search", "index", "заметки",
	}
	for i := 0; i < 60; i++ {
		var b strings.Builder
		b.WriteString("Title: Note ")
		b.WriteString(itoa(i))
		b.WriteString("\nTags: ")
		b.WriteString(words[i%len(words)])
		b.WriteString("\n\n")
		for l := 0; l < 6; l++ {
			for w := 0; w < 5; w++ {
				b.WriteString(words[(i*7+l*3+w)%len(words)])
				b.WriteByte(' ')
			}
			b.WriteByte('\n')
		}
		writeSearchNote(t, a, "N"+itoa(i)+".md", b.String())
	}
	a.rebuildSearchIndex()

	queries := []string{
		"fetch", "json", "fetch json", "andro", "заметки", "tokn", "fecth",
		"handler template", "zzz", "sea", "backu", "intnt", "database backup",
	}
	for _, qs := range queries {
		q := parseQuery(qs)
		if len(q.terms) == 0 {
			continue
		}
		// Brute force: read and score everything, filter be damned.
		wanted := map[string]bool{}
		for _, d := range a.snapshotDocs() {
			doc := a.reloadDocument(d)
			if doc == nil {
				continue
			}
			if _, _, _, ok := scoreDocument(q, doc); ok {
				wanted[d.Path] = true
			}
		}
		// Filtered: what the index would let through.
		got := map[string]bool{}
		for _, d := range a.snapshotDocs() {
			feasible := true
			for _, term := range q.terms {
				if !d.couldMatchTerm(term) {
					feasible = false
					break
				}
			}
			if !feasible {
				continue
			}
			doc := a.reloadDocument(d)
			if doc == nil {
				continue
			}
			if _, _, _, ok := scoreDocument(q, doc); ok {
				got[d.Path] = true
			}
		}
		for path := range wanted {
			if !got[path] {
				t.Errorf("query %q: the filter rejected %s, which really does match", qs, path)
			}
		}
	}
}

// ---------------------------------------------------------------------
// Querying
// ---------------------------------------------------------------------

func TestGlobalSearchEndToEnd(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Fetch.md", "Title: Fetch\nTags: Test\n\nawait fetch('/json/test.json');\n")
	writeSearchNote(t, a, "Other.md", "Title: Other\n\nthis note mentions json once\n")
	writeSearchNote(t, a, "Unrelated.md", "Title: Unrelated\n\nnothing to see\n")

	rec, resp := searchReq(t, a, url.Values{"q": {"json"}, "scope": {"all"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %+v", rec.Code, resp)
	}
	if resp.Total != 2 {
		t.Fatalf("total = %d, want 2: %+v", resp.Total, resp.Results)
	}
	// Both matched in content; the note whose line carries it earlier and as a
	// whole word ranks first. What matters here is that ordering happens at
	// all and is stable.
	if resp.Results[0].Score < resp.Results[1].Score {
		t.Errorf("results are not ordered by score: %d then %d",
			resp.Results[0].Score, resp.Results[1].Score)
	}
	for _, r := range resp.Results {
		if r.Name == "Unrelated" {
			t.Error("a document with no match was returned")
		}
		if len(r.Matches) == 0 {
			t.Errorf("%s came back with no snippet", r.Name)
		}
	}

	// A title hit outranks a content hit, through the field weights.
	_, resp = searchReq(t, a, url.Values{"q": {"fetch"}, "scope": {"all"}})
	if len(resp.Results) == 0 || resp.Results[0].Name != "Fetch" {
		t.Errorf("title match did not come first: %+v", resp.Results)
	}
}

// The rung that gives the feature its name. Nothing tested it end to end
// before: page search never called it either, which is why this phase wired it
// into scoreDocument rather than leaving it available but unused.
func TestTypoRungFindsMisspelledWord(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Fetch.md", "Title: Notes\n\nconst r = await fetch(url);\n")

	// Global, through the index's trigram filter.
	_, resp := searchReq(t, a, url.Values{"q": {"fecth"}, "scope": {"all"}})
	if len(resp.Results) != 1 {
		t.Fatalf("global: %d results for a transposed query, want 1", len(resp.Results))
	}
	if len(resp.Results[0].Matches) == 0 {
		t.Fatal("no snippet for the typo match")
	}
	// The highlight covers the word that is really there, not the misspelling
	// the user typed.
	m := resp.Results[0].Matches[0]
	runes := []rune(m.Text)
	if len(m.Spans) == 0 {
		t.Fatal("no spans")
	}
	got := string(runes[m.Spans[0][0] : m.Spans[0][0]+m.Spans[0][1]])
	if !strings.EqualFold(got, "fetch") {
		t.Errorf("highlighted %q, want the real word fetch", got)
	}

	// And the same query in page scope, which uses no index at all.
	_, resp = searchReq(t, a, url.Values{"q": {"fecth"}, "scope": {"page"}, "on": {"Fetch"}})
	if len(resp.Results) != 1 {
		t.Errorf("page: %d results for a transposed query, want 1", len(resp.Results))
	}
}

func TestGlobalSearchLimits(t *testing.T) {
	a := enabledSearchApp(t)
	for i := 0; i < 12; i++ {
		writeSearchNote(t, a, "N"+itoa(i)+".md", "Title: Note "+itoa(i)+"\n\nneedle\n")
	}
	a.rebuildSearchIndex()

	_, resp := searchReq(t, a, url.Values{"q": {"needle"}, "scope": {"all"}, "limit": {"5"}})
	if len(resp.Results) != 5 {
		t.Errorf("returned %d results, want the requested 5", len(resp.Results))
	}
	if resp.Total != 12 {
		t.Errorf("total = %d, want the full 12 - a limit must not hide the count", resp.Total)
	}
	if !resp.Truncated {
		t.Error("truncation was not reported")
	}
}

// ---------------------------------------------------------------------
// Staleness
// ---------------------------------------------------------------------

func TestIndexPicksUpEdits(t *testing.T) {
	a := enabledSearchApp(t)
	// The two bodies are the SAME LENGTH on purpose: the stat stamp counts
	// bytes as well as times, and this test is about the other mechanism.
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\noriginal wording\n")

	if _, resp := searchReq(t, a, url.Values{"q": {"original"}, "scope": {"all"}}); resp.Total != 1 {
		t.Fatalf("first query: total %d", resp.Total)
	}

	// An edit made through the app marks the index dirty (renderAndCache), so
	// the change is visible without waiting out the stat-walk interval.
	//
	// The mtime is deliberately forced BACK to what it was before the edit.
	// That is not a contrived case: filesystems with second-granularity
	// timestamps - Android's external media, where every note lives - produce
	// exactly this when two writes land in the same second. An index that
	// re-stats to "confirm" an edit it was told about would conclude nothing
	// changed and serve the old text. This is the regression that reached a
	// build machine before it was caught.
	before, err := os.Stat(filepath.Join(a.StorageDir, "md", "Note.md"))
	if err != nil {
		t.Fatal(err)
	}
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nrewritten poetry\n") // same length
	if err := os.Chtimes(filepath.Join(a.StorageDir, "md", "Note.md"),
		before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	a.markSearchIndexDirty()

	if _, resp := searchReq(t, a, url.Values{"q": {"rewritten"}, "scope": {"all"}}); resp.Total != 1 {
		t.Errorf("edit not picked up: total %d - an edit the server itself made "+
			"must never depend on the filesystem's clock", resp.Total)
	}
	if _, resp := searchReq(t, a, url.Values{"q": {"original"}, "scope": {"all"}}); resp.Total != 0 {
		t.Errorf("removed text still matches: total %d", resp.Total)
	}
}

// The other half: a change made BEHIND the server's back, with a timestamp too
// coarse to move. Nothing marks the index dirty here, so the stat walk is the
// only thing that can notice - which is why the stamp counts bytes as well as
// times.
func TestIndexNoticesExternalEditWithUnchangedMtime(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\noriginal text\n")
	a.rebuildSearchIndex()

	path := filepath.Join(a.StorageDir, "md", "Note.md")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// An external editor rewrites the file; the clock has not visibly moved.
	if err := os.WriteFile(path, []byte("Title: A Note\n\nquite different wording here\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}

	// Past the rate-limit window, so the walk actually runs.
	a.search.mu.Lock()
	a.search.checked = time.Now().Add(-2 * indexStaleCheckEvery)
	a.search.dirty = false
	a.search.mu.Unlock()

	if _, resp := searchReq(t, a, url.Values{"q": {"different"}, "scope": {"all"}}); resp.Total != 1 {
		t.Error("an external edit with an unchanged mtime was never noticed; the stamp needs more than times")
	}
}

func TestIndexDoesNotRebuildWhenNothingChanged(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nstable text\n")
	a.rebuildSearchIndex()

	built := a.search.built
	// Two queries in a row, with the tree untouched: the first may re-stat,
	// neither may rebuild.
	searchReq(t, a, url.Values{"q": {"stable"}, "scope": {"all"}})
	searchReq(t, a, url.Values{"q": {"stable"}, "scope": {"all"}})

	if !a.search.built.Equal(built) {
		t.Error("the index rebuilt itself although nothing on disk changed")
	}
}

func TestIndexStatWalkIsRateLimited(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\ntext\n")
	a.rebuildSearchIndex()

	a.search.mu.Lock()
	a.search.checked = time.Now()
	a.search.dirty = false
	a.search.mu.Unlock()

	// A change made behind the server's back, with the interval not yet
	// elapsed: deliberately NOT seen. At 10 000 files the walk costs ~24 ms,
	// which a debounced search box would otherwise pay on every keystroke.
	writeSearchNote(t, a, "Sneaky.md", "Title: Sneaky\n\nadded externally\n")
	if _, resp := searchReq(t, a, url.Values{"q": {"externally"}, "scope": {"all"}}); resp.Total != 0 {
		t.Error("the stat walk ran inside the rate-limit window")
	}

	// Once the window passes, it is seen without anyone doing anything.
	a.search.mu.Lock()
	a.search.checked = time.Now().Add(-2 * indexStaleCheckEvery)
	a.search.mu.Unlock()
	if _, resp := searchReq(t, a, url.Values{"q": {"externally"}, "scope": {"all"}}); resp.Total != 1 {
		t.Error("an external change was never noticed")
	}
}

func TestIndexRebuildsWhenSettingsChange(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nneedle\n")
	writeAsset(t, a, "js/mine.js", "// needle in a script\n")
	a.rebuildSearchIndex()

	if _, resp := searchReq(t, a, url.Values{"q": {"needle"}, "scope": {"all"}}); resp.Total != 1 {
		t.Fatalf("baseline: total %d", resp.Total)
	}

	// Changing WHAT is covered is not a staleness question - no file changed,
	// but the answer must change anyway.
	a.WithConfig(func(c *Config) { c.SearchKinds = []string{SearchKindMD, SearchKindJS} })
	if _, resp := searchReq(t, a, url.Values{"q": {"needle"}, "scope": {"all"}}); resp.Total != 2 {
		t.Errorf("after enabling scripts: total %d, want 2", resp.Total)
	}
}

// ---------------------------------------------------------------------
// The switch
// ---------------------------------------------------------------------

func TestSearchToggleReleasesAndRebuilds(t *testing.T) {
	a := enabledSearchApp(t)
	a.WithConfig(func(c *Config) { c.GitServers = make([]GitServerConfig, maxGitServers) })
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nneedle\n")
	a.rebuildSearchIndex()

	if !a.searchIndexBuilt() {
		t.Fatal("index not built")
	}
	if !a.globalSearchAvailable() {
		t.Fatal("global search should be available once enabled and built")
	}

	// Switching off releases the memory immediately - no restart, because
	// turning this off is what someone does when a device is already short.
	postForm(t, a.handleConfig, "/api/config", url.Values{
		"search_enabled": {"false"},
		"search_kinds":   {"md"},
	})
	if a.searchIndexBuilt() {
		t.Error("the index survived being switched off")
	}
	if a.globalSearchAvailable() {
		t.Error("global search still reports available with the setting off")
	}

	// And page search is untouched by any of it.
	if _, resp := searchReq(t, a, url.Values{"q": {"needle"}, "scope": {"page"}, "on": {"Note"}}); len(resp.Results) != 1 {
		t.Error("page search broke when global search was switched off")
	}
}

func TestSearchIndexStatusLine(t *testing.T) {
	a := newTestApp(t)
	a.search = &searchIndex{}

	if got := a.searchIndexStatus(); !strings.Contains(got, "Off") {
		t.Errorf("status with search off = %q", got)
	}

	a.WithConfig(func(c *Config) { c.SearchEnabled = true; c.SearchKinds = []string{SearchKindMD} })
	if got := a.searchIndexStatus(); !strings.Contains(got, "Not built") {
		t.Errorf("status before the first build = %q", got)
	}

	writeSearchNote(t, a, "Note.md", "Title: A Note\n\none line\n")
	a.rebuildSearchIndex()
	got := a.searchIndexStatus()
	for _, want := range []string{"1 document", "MB indexed", "in memory", "built"} {
		if !strings.Contains(got, want) {
			t.Errorf("status %q is missing %q - the memory cost has to be visible where the switch is", got, want)
		}
	}
}

func TestOneDecimalAndItoa(t *testing.T) {
	for in, want := range map[float64]string{0: "0.0", 0.04: "0.0", 0.06: "0.1", 1.25: "1.3", 9.96: "10.0", 13: "13.0"} {
		if got := oneDecimal(in); got != want {
			t.Errorf("oneDecimal(%v) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[int]string{0: "0", 7: "7", 42: "42", 2607040: "2607040", -3: "-3"} {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func indexedPaths(a *App) []string {
	var out []string
	for _, d := range a.snapshotDocs() {
		out = append(out, d.Path)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// containsPath is a slice membership test. Named for what it holds rather than
// the obvious "contains", because sync_errors_test.go already has a function of
// that name with a different signature - and every _test.go file in a package
// shares one namespace.
func containsPath(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// readIfExists is a small helper for asserting that something was NOT created.
func readIfExists(path string) ([]byte, error) {
	return os.ReadFile(path)
}
