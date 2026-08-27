package backend

// ----------------------------------------------------------------------
// Sections: addressing a PART of a document
// ----------------------------------------------------------------------
//
// Some notes are not flat prose. QuickNotes is a run of entries separated by
// "---" and a "##### <timestamp>" heading; Bookmarks.md is a JSON array that
// Bookmarker.js renders into a list. In both cases the compiled page ALREADY
// carries a per-entry anchor id, so a search result can point at the entry it
// matched rather than at the top of a 3 000-line page - provided the search
// layer knows where the entries begin and end.
//
// That is all a section is: a line range, a label to show, and the anchor id
// the compiled HTML gives it. It is not a new scoring path. Ranking, weights,
// tiers and the mask prefilter are untouched.
//
// Two sectionizers, because there are two structures:
//
//   - headings, for every markdown document (§5.2)
//   - the bookmarks array, which is parsed rather than line-scanned - and that
//     one is a correctness fix, not a nicety. See parseBookmarksArray.
//
// The hard part is not finding the entries. It is that the anchor ids are
// assigned somewhere else - by goldmark at compile time, and by Bookmarker.js
// at render time - and this file has to predict both without reading either.
// Everything below is written to make a WRONG prediction impossible; a missing
// anchor is fine (the result falls back to linking at the page), a wrong one
// sends the reader to another entry entirely.

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// docSection is one addressable part of a document: [start, end] in file line
// numbers, inclusive.
//
// id is the anchor in the compiled HTML, or "" when the section exists but is
// not addressable - a note's preamble before its first heading, or a heading
// whose id could not be predicted safely. Such a section still labels its hits;
// it just links at the page instead of into it.
type docSection struct {
	start, end int
	id         string
	label      string
}

// sectionFor returns the section containing a line, or nil.
//
// Linear rather than binary: sections are few (a long QuickNotes has hundreds,
// not millions) and this runs only at result assembly, over the handful of
// snippets that survived ranking - never in the scoring loop.
func sectionFor(sections []docSection, line int) *docSection {
	for i := range sections {
		if line >= sections[i].start && line <= sections[i].end {
			return &sections[i]
		}
	}
	return nil
}

// ----------------------------------------------------------------------
// Anchor prediction
// ----------------------------------------------------------------------

// timestampAnchor mirrors Bookmarker.js:
//
//	li.setAttribute('id', bm.date.replaceAll(':','').replaceAll(' ','-'))
//
// so "2026-06-15 20:00:00" becomes "2026-06-15-200000". Nothing else is
// touched, because nothing else appears in a date this app writes.
//
// Note that goldmark's heading rule (below) collapses to exactly the same
// string for the same timestamp: digits and '-' survive, ':' is dropped, ' '
// becomes '-'. That agreement is not a coincidence worth relying on, so the two
// are computed separately - but it does mean a QuickNotes heading and a
// bookmark entry carrying the same instant get the same anchor, which is
// correct in both places.
func timestampAnchor(date string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(date), ":", ""), " ", "-")
}

// headingUnsafe lists the characters that make a heading's id unpredictable
// from its markdown source, so this file declines rather than guesses.
//
// goldmark builds an id from the heading's RENDERED TEXT, and these are the
// characters where the rendered text differs from the source in a way that
// changes the answer:
//
//	[ ]   a link: "[text](http://x)" renders as "text", so the URL must not
//	      contribute - but reading the source, it would
//	& < >  an entity or inline HTML: "&amp;" renders as "&"
//	`     inline code, which renderMarkdownToHTML replaces with a placeholder
//	      BEFORE goldmark sees it - so the id is built from "OMN_RAW_0_END"
//	$     KaTeX math, shielded by the same mechanism
//	\     a backslash escape changes what the next character means
//	_     an emphasis marker AND, unescaped, a character with its own mapping;
//	      "_a_" renders as "a" but reads as "_a_"
//	#     an ATX CLOSING sequence: "## Foo ##" is a heading whose text is
//	      "Foo", and telling a closing run apart from a literal '#' (as in
//	      "C#") is CommonMark trivia this file has no business relitigating
//
// Deliberately absent: '*' and '~'. They are emphasis markers too, but they
// contribute nothing to an id under either reading - dropped as punctuation if
// literal, removed as markup if not - so they cannot cause a disagreement.
const headingUnsafe = "[]`_$<>&#\\"

// headingSlug applies goldmark's id rule to a heading's text.
//
// The rule (parser.WithAutoHeadingID): ASCII alphanumerics are lowercased and
// kept, spaces and '-' become '-', every other ASCII character is dropped, and
// non-ASCII runes are skipped entirely - which is why a wholly Cyrillic heading
// degenerates (§5.5).
//
// ok is false when the text contains something from headingUnsafe. That is not
// a failure, it is a refusal: see the type comment.
func headingSlug(text string) (slug string, ok bool) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(text) {
		switch {
		case r > unicode.MaxASCII:
			// Skipped, exactly as goldmark skips multi-byte runes.
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r == ' ' || r == '\t' || r == '-':
			b.WriteByte('-')
		case strings.ContainsRune(headingUnsafe, r):
			return "", false
		default:
			// Dropped.
		}
	}
	return b.String(), true
}

// headingIDGen assigns ids the way goldmark does, including its collision
// suffixes, and knows when to stop.
//
// The dedup counter is the reason this is a type rather than a function. Two
// identical headings get "x" and "x-1", and the numbering is per document and
// in document order - so a heading this code fails to SEE (or fails to predict)
// desynchronises every id after it. Hence poison: once anything unpredictable
// appears, no further id is emitted for that document. Sections keep working;
// they just stop being addressable.
type headingIDGen struct {
	taken     map[string]bool
	poisoned  bool
	anchorsOK bool
}

func newHeadingIDGen() *headingIDGen {
	return &headingIDGen{taken: map[string]bool{}, anchorsOK: anchorsPredictable()}
}

// next returns the anchor for a heading, or "" when there must not be one.
//
// A degenerate id - goldmark's fallback "heading" for a heading with no ASCII
// alphanumerics, which is what a wholly Cyrillic heading produces - is
// registered so the numbering stays in step, but not returned: "heading",
// "heading-1", "heading-2" address nothing a reader would recognise, and
// linking to the page is the more honest answer (§5.5).
func (h *headingIDGen) next(text string) string {
	if h.poisoned || !h.anchorsOK {
		return ""
	}
	slug, ok := headingSlug(text)
	if !ok {
		h.poisoned = true
		return ""
	}
	degenerate := slug == ""
	if degenerate {
		slug = "heading"
	}
	id := slug
	for i := 1; h.taken[id]; i++ {
		id = slug + "-" + itoa(i)
	}
	h.taken[id] = true
	if degenerate {
		return ""
	}
	return id
}

// poison marks the rest of the document unpredictable. Called for a construct
// this file knows produces a heading but cannot read - see reSetextRule.
func (h *headingIDGen) poison() { h.poisoned = true }

// ----------------------------------------------------------------------
// The runtime self-check
// ----------------------------------------------------------------------

// headingIDAttrRe pulls the ids back out of compiled HTML.
var headingIDAttrRe = regexp.MustCompile(`<h[1-6][^>]*\bid="([^"]*)"`)

// anchorProbe exercises every rule headingSlug depends on: case folding, digits
// surviving, ' ' and '-' becoming '-', punctuation being dropped, a timestamp
// coming out the way Bookmarker.js writes one, and the collision suffix.
var anchorProbe = []struct{ md, want string }{
	{"# Aa Bb 09", "aa-bb-09"},
	{"# a-b c", "a-b-c"},
	{"# x: y. z! (w)", "x-y-z-w"},
	{"# 2026-07-27 07:23:17", "2026-07-27-072317"},
	{"# dup name", "dup-name"},
	{"# dup name", "dup-name-1"},
}

var (
	anchorsOnce sync.Once
	anchorsGood bool
)

// anchorsPredictable compiles a probe document through the REAL renderer and
// checks that the ids coming back are the ids headingSlug predicted.
//
// This exists because the drift risk here is unlike anywhere else in the
// codebase. For the tags page one Go function produces both the anchor and the
// link, so they cannot disagree. Here the anchor is minted by a dependency at
// compile time and the link is minted by this file, and a goldmark upgrade that
// changed the id rule would silently start sending readers to the wrong section
// of the right page - the kind of wrong that looks like a bug in the note.
//
// A golden test catches that at build time. This catches it at RUN time, on a
// device that upgraded, and its failure mode is the safe one: no anchors at
// all, one log line, results still link at the page.
//
// Cost is one markdown compile of ~90 bytes, once per process.
func anchorsPredictable() bool {
	anchorsOnce.Do(func() { anchorsGood = probeAnchors() })
	return anchorsGood
}

func probeAnchors() bool {
	var src strings.Builder
	want := make([]string, 0, len(anchorProbe))
	for _, c := range anchorProbe {
		src.WriteString(c.md)
		src.WriteString("\n\n")
		want = append(want, c.want)
	}

	var got []string
	for _, m := range headingIDAttrRe.FindAllStringSubmatch(
		(&App{}).renderMarkdownToHTML([]byte(src.String())), -1) {
		got = append(got, m[1])
	}

	if len(got) != len(want) {
		logAnchorsOff("the renderer emitted " + itoa(len(got)) + " heading ids, expected " + itoa(len(want)))
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			logAnchorsOff("heading id " + itoa(i+1) + " is " + got[i] + ", expected " + want[i])
			return false
		}
	}
	return true
}

// ----------------------------------------------------------------------
// Sectionizer 1: markdown headings
// ----------------------------------------------------------------------

// reATXHeading matches a heading opener. The space after the '#' run is
// required by CommonMark - "#tag" is a word, not a heading - and a heading may
// also be empty ("###" alone).
var reATXHeading = regexp.MustCompile(`^ {0,3}(#{1,6})([ \t]+(.*))?$`)

// reSetextRule matches a line that might be a setext heading underline.
//
// This is not used to FIND headings, it is used to give up. A run of '=' or '-'
// directly under a paragraph line is an h1/h2 in CommonMark, and reading its
// text means re-implementing paragraph continuation rules. Rather than get that
// subtly wrong, a possible underline poisons the document's ids.
//
// It costs nothing where it matters: QuickNotes separates entries with "---",
// but always after a blank line, which makes it a thematic break rather than an
// underline - so the structure this whole file exists to serve never trips it.
var reSetextRule = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)

// sectionsFromHeadings splits a document at its headings.
//
// lines/contexts/firstLineNo describe the BODY as addLines sees it, so a
// heading inside a fenced block or a <script> is already labelled non-prose by
// classifyContexts and is skipped here. That is the trap a naive "^#{1,6} "
// scan falls into, and it is not a cosmetic one: goldmark does not see such a
// line as a heading either, so counting it would shift every collision suffix
// after it.
//
// Returns nil when the document has no headings at all - there is nothing to
// address, and an "everything" section would only add noise to every result.
func sectionsFromHeadings(lines, contexts []string, firstLineNo int) []docSection {
	ids := newHeadingIDGen()
	var out []docSection

	for i, line := range lines {
		no := firstLineNo + i
		if contexts[i] != "" {
			continue // inside a fence, <pre> or <script>: not a heading
		}
		if i > 0 && reSetextRule.MatchString(line) && strings.TrimSpace(lines[i-1]) != "" &&
			!reATXHeading.MatchString(lines[i-1]) {
			ids.poison()
			continue
		}
		m := reATXHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		text := strings.TrimSpace(m[3])

		if n := len(out); n > 0 {
			out[n-1].end = no - 1
		}
		out = append(out, docSection{
			start: no,
			end:   1 << 30, // closed by the next heading, or left open
			id:    ids.next(text),
			label: text,
		})
	}

	if len(out) == 0 {
		return nil
	}
	// Everything above the first heading is the preamble: labelled by nothing
	// and addressable by nothing, but its hits still belong to the document, so
	// it must not fall into the first heading's section.
	if out[0].start > firstLineNo {
		out = append([]docSection{{start: firstLineNo, end: out[0].start - 1}}, out...)
	}
	return out
}

// ----------------------------------------------------------------------
// Sectionizer 2: the bookmarks array
// ----------------------------------------------------------------------

// bookmarkEntry mirrors the struct handleBookmark writes.
type bookmarkEntry struct {
	Date  string   `json:"date"`
	URL   string   `json:"url"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Notes []string `json:"notes"`
}

// reBookmarksArray finds the opening of the array Bookmarker.js reads.
var reBookmarksArray = regexp.MustCompile(`\bbookmarks\s*=\s*\[`)

// bookmarksBlock is what a scan of Bookmarks.md recovers: the array as
// well-formed JSON, plus where each entry and the array itself live in the
// file, so a hit can be attributed to a line a reader could open.
type bookmarksBlock struct {
	json       string
	startLines []int // source line of each entry's '{', in array order
	firstLine  int   // the line the array opens on
	lastLine   int   // the line it closes on
}

// scanBookmarksArray extracts the array as parseable JSON.
//
// Two things in the file are not JSON and have to be removed, and both are
// there for good reasons:
//
//   - the "<!-- Don't edit body below this line -->" marker sits INSIDE the
//     array. handleBookmark inserts new entries directly after it, which is how
//     newest-first ordering costs nothing. Browsers accept it because "<!--" is
//     a legal comment opener in JavaScript; encoding/json is not so relaxed.
//   - a trailing comma before the closing ']'. A file whose entries were all
//     removed by hand, or written by the original OMN, ends up with one.
//
// The scan is string-aware, which is the whole reason it is a scan and not two
// regexes: a bookmark note may legitimately contain "<!--" or ",]", and
// rewriting those inside a quoted string would corrupt the user's data rather
// than the file's syntax.
func scanBookmarksArray(content string, firstLineNo int) (bookmarksBlock, bool) {
	loc := reBookmarksArray.FindStringIndex(content)
	if loc == nil {
		return bookmarksBlock{}, false
	}
	start := loc[1] - 1 // the '['

	var (
		out       []byte
		block     = bookmarksBlock{firstLine: firstLineNo + strings.Count(content[:start], "\n")}
		line      = block.firstLine
		depth     int
		inStr     bool
		esc       bool
		lastComma = -1 // index in out of a comma with only space since
	)

	for i := start; i < len(content); i++ {
		c := content[i]
		if c == '\n' {
			line++
		}
		if inStr {
			out = append(out, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr, lastComma = true, -1
			out = append(out, c)
		case c == '<' && strings.HasPrefix(content[i:], "<!--"):
			end := strings.Index(content[i:], "-->")
			if end < 0 {
				return bookmarksBlock{}, false
			}
			line += strings.Count(content[i:i+end+3], "\n")
			i += end + 2
		case c == '[', c == '{':
			if c == '{' && depth == 1 {
				block.startLines = append(block.startLines, line)
			}
			depth++
			lastComma = -1
			out = append(out, c)
		case c == ']', c == '}':
			if lastComma >= 0 {
				out[lastComma] = ' ' // the trailing comma, neutralised in place
			}
			depth--
			lastComma = -1
			out = append(out, c)
			if depth == 0 {
				block.json, block.lastLine = string(out), line
				return block, true
			}
		case c == ',':
			out = append(out, c)
			lastComma = len(out) - 1
		case c == ' ', c == '\t', c == '\n', c == '\r':
			out = append(out, c) // whitespace does not end a trailing comma
		default:
			out = append(out, c)
			lastComma = -1
		}
	}
	return bookmarksBlock{}, false // unterminated array
}

// logAnchorsOff says so once, loudly enough to be findable, because the
// symptom otherwise is "search results stopped linking into long notes" with
// nothing to explain it.
func logAnchorsOff(why string) {
	log.Printf("[search] (error) section anchors disabled - the renderer no longer "+
		"assigns heading ids the way this build predicts (%s). Results will "+
		"link at the page instead of the section.", why)
}

// bookmarksNote is the one note with a bespoke parser. Its base name, not its
// path, because that is what both resolvePageName and the index walk key on.
const bookmarksNote = "Bookmarks"

// bookmarkJoin separates an entry's fields inside its single searchable line.
// A middle dot, matching the separator the UI already uses, and one no URL,
// tag or timestamp this app writes can contain.
const bookmarkJoin = " · "

// addBookmarks indexes Bookmarks.md as ENTRIES rather than as lines.
//
// This is a correctness fix, not a presentation nicety. handleBookmark writes
// the array with json.MarshalIndent, which emits '<', '>' and '&' as \u
// escapes so a bookmark can never break out of the surrounding <script>. A
// bookmark titled "Cats & Dogs" is therefore stored as
//
//	"title": "Cats & Dogs"
//
// and a line-based index cannot match "Cats & Dogs" against it - nor against
// "Cats", which is fine, nor against "&", which is not the point either. The
// point is that the escaping is invisible in the browser and total in the
// source, so exactly the bookmarks containing the commonest punctuation in
// English titles are the ones that were unfindable. Decoding the JSON is the
// only way to search what the user can actually see.
//
// Anything in the note OUTSIDE the array is still indexed as ordinary prose:
// Bookmarks.md is machine-managed, but nothing stops someone adding a note
// above the script, and "I typed it and search cannot find it" is a bad
// answer.
//
// If the file is not the shape this expects - hand-edited, half-written, from
// another tool - it falls back to plain line indexing. Degraded, not broken.
func (d *searchDocument) addBookmarks(body string, firstLineNo int) {
	block, ok := scanBookmarksArray(body, firstLineNo)
	var entries []bookmarkEntry
	if ok {
		if err := json.Unmarshal([]byte(block.json), &entries); err != nil {
			ok = false
		} else if len(entries) != len(block.startLines) {
			// The scan and the decoder disagree about how many entries there
			// are, so the line attribution below would be fiction.
			ok = false
		}
	}
	if !ok {
		log.Printf("[search] (error) %s: not a readable bookmarks array, indexing it as plain text", d.Path)
		d.addLines(body, firstLineNo)
		return
	}

	// The array's own source lines are blanked rather than skipped, because
	// addLines already drops blank lines - so the prose around it is indexed
	// with its real line numbers and none of the escaped JSON is.
	lines := strings.Split(body, "\n")
	for i := range lines {
		if no := firstLineNo + i; no >= block.firstLine && no <= block.lastLine {
			lines[i] = ""
		}
	}
	d.addLines(strings.Join(lines, "\n"), firstLineNo)

	for i, e := range entries {
		start := block.startLines[i]
		end := block.lastLine
		if i+1 < len(block.startLines) {
			end = block.startLines[i+1] - 1
		}

		label := strings.TrimSpace(e.Title)
		if label == "" {
			label = strings.TrimSpace(e.URL)
		}
		d.sections = append(d.sections, docSection{
			start: start,
			end:   end,
			id:    timestampAnchor(e.Date),
			label: label,
		})

		// ONE line per entry, not one per field, and the reason is mechanical:
		// scoreDocument keys its per-line hits by line number, so two lines
		// sharing one would merge - spans from the url landing on the title's
		// text. An entry is a single searchable thing anyway, and snippetFor
		// windows the join down to the ~160 runes around whatever matched.
		var parts []string
		for _, p := range []string{e.Title, e.URL, strings.Join(e.Tags, ", "), strings.Join(e.Notes, "; ")} {
			if p = strings.TrimSpace(p); p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 {
			continue
		}
		text := strings.Join(parts, bookmarkJoin)
		f := fold(text)
		d.lines = append(d.lines, docLine{no: start, raw: text, fold: f, mask: runeMask(f)})
	}
}
