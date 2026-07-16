package backend

import "strings"

// ----------------------------------------------------------------------
// The single front-matter ("Pelican header") parser
// ----------------------------------------------------------------------
//
// A note may begin with a block of "Key: Value" metadata lines terminated
// by a blank line, e.g.:
//
//	Title: My Note
//	Date: 2026-01-01 00:00:00
//	Category: Notes
//
//	Body starts here.
//
// The decision "where does the header end and the body begin?" used to be
// re-implemented, subtly differently, in three Go places
// (compilePageWithBody, ensureHeaderModified, handleNewPage) and a fourth
// in the editor's JavaScript (firstLineAfterHeader). Those variants
// disagreed on edge cases - most visibly, compilePageWithBody treated any
// first line containing a ':' as a header line, so a Markdown heading like
// "# Head: subtitle" was swallowed as metadata instead of rendering as a
// heading. splitFrontMatter is now the ONE authority; every Go caller goes
// through it, and the editor's firstLineAfterHeader mirrors isHeaderFirstLine
// exactly (see backend/frontend/html/js/omn-go-editor.js). See CODE_REVIEW.md
// Phase 1.

// frontMatter is the parsed split of note content into its optional header
// and its body.
type frontMatter struct {
	// HasHeader is true when the content begins with a metadata header
	// block (see splitFrontMatter for the exact rule).
	HasHeader bool
	// Header is the raw header block - the metadata lines joined by "\n",
	// WITHOUT the terminating blank line. Empty when HasHeader is false.
	Header string
	// Body is everything after the header's terminating blank line, or the
	// entire content when there is no header.
	Body string
	// BodyOffset is the byte offset into the ORIGINAL content at which Body
	// begins (0 when there is no header). This is the authoritative
	// "first line after the header" position, matched by the editor caret.
	BodyOffset int
}

// isHeaderFirstLine reports whether line - the FIRST line of a note -
// looks like a metadata key line ("Key: Value"). It must contain a ':' and
// must NOT start with a space, '#', or '<': those three mark a line that is
// Markdown or raw HTML body which merely happens to contain a colon (a
// "# Heading: subtitle", an indented continuation, a "<script>let x: 1").
// A trailing CR is ignored so CRLF files classify the same as LF ones.
//
// The editor's isHeaderFirstLine (JS) is a direct port of this rule; keep
// the two in sync.
func isHeaderFirstLine(line string) bool {
	line = strings.TrimSuffix(line, "\r")
	if !strings.Contains(line, ":") {
		return false
	}
	if strings.HasPrefix(line, " ") ||
		strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "<") {
		return false
	}
	return true
}

// splitFrontMatter parses content into its optional metadata header and its
// body. A header is present only when the FIRST line satisfies
// isHeaderFirstLine. The header then continues line by line and ends at the
// FIRST of either:
//   - a blank line (empty after trimming whitespace - so a "separator" line
//     that carries stray spaces/tabs still counts, which real notes have);
//     the blank line is the separator and is dropped, and the body starts
//     after it, or
//   - a line that is not itself a "Key: Value" header line (fails
//     isHeaderFirstLine, e.g. "<style>" or a prose line with no colon); that
//     line is the first BODY line and is kept.
//
// Both conditions matter. Requiring only a blank line (as an earlier version
// did) let a note whose header was followed immediately by content - a
// "<style>" block, a prose paragraph, or a whitespace-only separator - run
// the header on until the first truly-empty line, swallowing CSS
// "--var: #hex;" lines as bogus metadata. A header with neither a blank line
// nor a non-header line after it (a note that is only metadata) has an empty
// body. With no header at all, the whole content is the body.
func splitFrontMatter(content string) frontMatter {
	firstLine := content
	if nl := strings.IndexByte(content, '\n'); nl >= 0 {
		firstLine = content[:nl]
	}
	if !isHeaderFirstLine(firstLine) {
		return frontMatter{Body: content}
	}

	lines := strings.Split(content, "\n")

	// makeResult builds the split given the body's starting line index and
	// whether the line before it was a dropped blank separator.
	makeResult := func(bodyStart int, headerEndExclusive int) frontMatter {
		offset := 0
		for i := 0; i < bodyStart; i++ {
			offset += len(lines[i]) + 1 // +1 for the '\n' strings.Split removed
		}
		if offset > len(content) {
			offset = len(content) // degenerate trailing-line-with-no-newline case
		}
		return frontMatter{
			HasHeader:  true,
			Header:     strings.Join(lines[:headerEndExclusive], "\n"),
			Body:       strings.Join(lines[bodyStart:], "\n"),
			BodyOffset: offset,
		}
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			// Blank separator line: dropped; body starts on the next line.
			return makeResult(i+1, i)
		}
		if !isHeaderFirstLine(lines[i]) {
			// Not a header line: this line itself is the start of the body.
			return makeResult(i, i)
		}
	}

	// Every line was a header line (header-only note, no body).
	return frontMatter{HasHeader: true, Header: content, BodyOffset: len(content)}
}
