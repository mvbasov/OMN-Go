package backend

// ----------------------------------------------------------------------
// Sections: addressing a PART of a document
// ----------------------------------------------------------------------
//
// Some notes are not flat prose. QuickNotes is a run of entries separated by
// "---" and a "##### <timestamp>" heading. Bookmarks.md is a JSON array that
// Bookmarker.js renders into a list.
//
// In both cases the compiled page ALREADY carries an anchor id for each
// entry. A search result can thus point at the entry that it matched, and
// not at the top of a 3 000-line page. The search layer has to know where
// the entries begin and end.
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
// The hard part is not to find the entries. It is that the anchor ids are
// assigned somewhere else. goldmark assigns them at compile time, and
// Bookmarker.js at render time. This file has to predict both, and it reads
// neither.
//
// Everything below is written to make a WRONG prediction impossible. A
// missing anchor is fine, because the result falls back to a link at the
// page. A wrong one sends the reader to another entry entirely.

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
// id is the anchor in the compiled HTML. It is "" when the section exists
// but is not addressable. That covers the preamble of a note, before its
// first heading, and a heading whose id could not be predicted safely. Such
// a section still labels its hits. It links at the page instead of into it.
type docSection struct {
	start, end int
	id         string
	label      string
}

// sectionFor returns the section containing a line, or nil.
//
// Linear rather than binary. Sections are few, and a long QuickNotes has
// hundreds and not millions. This runs at result assembly only, over the
// handful of snippets that survived ranking. It never runs in the scoring
// loop.
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
// Note that the heading rule of goldmark below collapses to exactly the same
// string for the same timestamp. A digit and a '-' survive, a ':' is
// dropped, and a ' ' becomes a '-'. That agreement is not a coincidence
// worth relying on, thus the two are computed separately. It does mean that
// a QuickNotes heading and a bookmark entry that carry the same instant get
// the same anchor. That is correct in both places.
func timestampAnchor(date string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(date), ":", ""), " ", "-")
}

// headingUnsafe lists the characters that make a heading's id unpredictable
// from its markdown source, so this file declines rather than guesses.
//
// goldmark builds an id from the RENDERED TEXT of the heading. These are the
// characters where the rendered text differs from the source in a way that
// changes the answer:
//
//	[ ]   a link. "[text](http://x)" renders as "text", thus the URL must
//	      not contribute. Read as source, it would.
//	& < >  an entity or inline HTML. "&amp;" renders as "&".
//	`     inline code, which renderMarkdownToHTML replaces with a
//	      placeholder BEFORE goldmark sees it. The id is thus built from
//	      "OMN_RAW_0_END".
//	$     KaTeX math, shielded by the same mechanism.
//	\     a backslash escape changes what the next character means.
//	_     an emphasis marker AND, unescaped, a character with its own
//	      mapping. "_a_" renders as "a" but reads as "_a_".
//	#     an ATX CLOSING sequence. "## Foo ##" is a heading whose text is
//	      "Foo". A closing run and a literal '#', as in "C#", are hard to
//	      tell apart. That is CommonMark trivia, and this file has no
//	      business relitigating it.
//
// Deliberately absent: '*' and '~'. They are emphasis markers too, and they
// contribute nothing to an id under either reading. A literal one is dropped
// as punctuation, and markup is removed. They thus cannot cause a
// disagreement.
const headingUnsafe = "[]`_$<>&#\\"

// headingSlug applies goldmark's id rule to a heading's text.
//
// The rule is parser.WithAutoHeadingID. An ASCII alphanumeric is lowercased
// and kept. A space and a '-' become a '-'. Every other ASCII character is
// dropped, and a non-ASCII rune is skipped entirely. That is why a wholly
// Cyrillic heading degenerates (§5.5).
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
// The dedup counter is the reason this is a type, and not a function. Two
// identical headings get "x" and "x-1". The numbering is per document and in
// document order. A heading that this code fails to SEE, or fails to
// predict, thus desynchronises every id after it.
//
// Hence the poison. Once anything unpredictable appears, no further id is
// emitted for that document. Sections keep working, and they stop being
// addressable.
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
// A degenerate id is registered, thus the numbering stays in step, and it is
// not returned. It is the fallback "heading" of goldmark for a heading with
// no ASCII alphanumerics, which is what a wholly Cyrillic heading produces.
// "heading", "heading-1" and "heading-2" address nothing that a reader would
// recognise. A link to the page is the more honest answer (§5.5).
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

// anchorProbe exercises every rule that headingSlug depends on. Those are
// case folding, a digit that survives, a ' ' and a '-' that become a '-',
// and punctuation that is dropped. They also cover a timestamp that comes
// out the way Bookmarker.js writes one, and the collision suffix.
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
// codebase. For the tags page, one Go function produces both the anchor and
// the link, thus they cannot disagree. Here a dependency mints the anchor at
// compile time, and this file mints the link.
//
// A goldmark upgrade that changed the id rule would silently start to send
// readers to the wrong section of the right page. That kind of wrong looks
// like a bug in the note.
//
// A golden test catches that at build time. This catches it at RUN time, on
// a device that upgraded. Its failure mode is the safe one. There are no
// anchors at all, one log line, and results still link at the page.
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
// It costs nothing where it matters. QuickNotes separates entries with
// "---", and always after a blank line. That makes it a thematic break, and
// not an underline. The structure that this whole file exists to serve thus
// never trips it.
var reSetextRule = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)

// sectionsFromHeadings splits a document at its headings.
//
// The lines, contexts and firstLineNo parameters describe the BODY as
// addLines sees it. classifyContexts already labels a heading inside a
// fenced block or a <script> as non-prose, thus it is skipped here.
//
// That is the trap that a naive "^#{1,6} " scan falls into, and the trap is
// not cosmetic. goldmark does not see such a line as a heading either, thus
// to count it would shift every collision suffix after it.
//
// It answers nil when the document has no heading at all. There is nothing
// to address, and an "everything" section would only add noise to every
// result.
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
	// Everything above the first heading is the preamble. Nothing labels it,
	// and nothing addresses it. Its hits still belong to the document, thus
	// it must not fall into the section of the first heading.
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

// bookmarksBlock is what a scan of Bookmarks.md recovers. It holds the array
// as well-formed JSON. It also holds where each entry and the array itself
// live in the file. A hit can thus be attributed to a line that a reader
// could open.
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
// The scan is string-aware, and that is the whole reason it is a scan and
// not two regular expressions. A bookmark note may legitimately contain
// "<!--" or ",]". To rewrite those inside a quoted string would corrupt the
// data of the user, and not the syntax of the file.
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

// logAnchorsOff says so one time, loudly enough to be findable. The symptom
// otherwise is "search results stopped linking into long notes", with
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
// A line-based index cannot match "Cats & Dogs" against that. It can match
// "Cats", which is fine, and it cannot match "&", which is not the point
// either.
//
// The point is that the escaping is invisible in the browser and total in
// the source. Exactly the bookmarks that hold the commonest punctuation of
// an English title were thus the unfindable ones. To decode the JSON is the
// only way to search what the user can see.
//
// Anything in the note OUTSIDE the array is still indexed as ordinary prose.
// Bookmarks.md is machine-managed, and nothing stops someone from adding a
// note above the script. "I typed it and search cannot find it" is a bad
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

	// The own source lines of the array are blanked, and not skipped,
	// because addLines already drops a blank line. The prose around it is
	// thus indexed with its real line numbers, and none of the escaped JSON
	// is indexed.
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

		// ONE line for each entry, and not one for each field. The reason is
		// mechanical. scoreDocument keys its per-line hits by line number.
		// Two lines that share one would thus merge, and a span from the url
		// would land on the text of the title. An entry is one searchable
		// thing anyway, and snippetFor windows the join down to the 160
		// runes or so around whatever matched.
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
