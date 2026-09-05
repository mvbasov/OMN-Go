package backend

import "strings"

// ----------------------------------------------------------------------
// The single header-block ("Pelican header") parser
// ----------------------------------------------------------------------
//
// This file was frontmatter.go, and parseHeaderBlock was splitFrontMatter,
// until 26.08.42. "header block" is the name doc/TERMINOLOGY.md requires
// the documentation to use, and the code now uses the same one. Look in the
// history for the old names: nothing but names changed.
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
// heading. parseHeaderBlock is now the ONE authority; every Go caller goes
// through it, and the editor's firstLineAfterHeader mirrors isHeaderFirstLine
// exactly (see backend/frontend/html/js/OMN-Go/omn-go-editor.js). See CODE_REVIEW.md
// Phase 1.

// headerBlock is the parsed split of note content into its optional header
// and its body.
type headerBlock struct {
	// HasHeader is true when the content begins with a metadata header
	// block (see parseHeaderBlock for the exact rule).
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

// parseHeaderBlock parses content into its optional metadata header and its
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
func parseHeaderBlock(content string) headerBlock {
	firstLine := content
	if nl := strings.IndexByte(content, '\n'); nl >= 0 {
		firstLine = content[:nl]
	}
	if !isHeaderFirstLine(firstLine) {
		return headerBlock{Body: content}
	}

	lines := strings.Split(content, "\n")

	// makeResult builds the split given the body's starting line index and
	// whether the line before it was a dropped blank separator.
	makeResult := func(bodyStart int, headerEndExclusive int) headerBlock {
		offset := 0
		for i := 0; i < bodyStart; i++ {
			offset += len(lines[i]) + 1 // +1 for the '\n' strings.Split removed
		}
		if offset > len(content) {
			offset = len(content) // degenerate trailing-line-with-no-newline case
		}
		return headerBlock{
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
	return headerBlock{HasHeader: true, Header: content, BodyOffset: len(content)}
}

// ----------------------------------------------------------------------
// Reading and writing ONE header key
// ----------------------------------------------------------------------
//
// Note exchange (note_exchange.go) has to put "FileName:" on a note it sends
// and "Imported:" on a note it receives, and take "FileName:" off again at
// the other end. Both must SET a key - replace the line when it is already
// there - and never append a second line with the same key.
//
// That is not fussiness. A note can make more than one hop: A sends to B, B
// sends to C. If the second import appended, the note would carry two
// "Imported:" lines, and a header block with one key twice has no defined
// meaning - parseHeaderBlock would hand the first one to whatever reads it,
// and which of the two is first is an accident of the order the hops ran in.
//
// Both functions splice the header back into the ORIGINAL string rather than
// re-joining a parse of it. The separator between the header and the body is
// one newline when the header ended at a non-header line ("<style>" on the
// next line) and two when it ended at a blank one, and rebuilding with a
// fixed "\n\n" would silently insert a blank line into the first kind. The
// three pieces below always satisfy header + separator + body == content.

// splitHeaderRegion cuts content into its header text, the separator run that
// follows it, and the body. Concatenating the three reproduces content byte
// for byte. header and sep are empty when there is no header block.
func splitHeaderRegion(content string) (header, sep, body string) {
	hb := parseHeaderBlock(content)
	if !hb.HasHeader {
		return "", "", content
	}
	return content[:len(hb.Header)], content[len(hb.Header):hb.BodyOffset], content[hb.BodyOffset:]
}

// headerKeyOf returns the key of a "Key: value" header line, or "" when the
// line carries no colon. The key is returned as written.
func headerKeyOf(line string) string {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(line[:i])
}

// setHeaderKey returns content with "key: value" in its header block.
//
// An existing line with that key is REPLACED where it stands, so the order of
// a note's metadata does not change under it. A new key is appended as the
// last header line. A note with no header block gets one.
//
// The key is matched without regard to case (a sender may write "filename:"),
// and written back in the caller's spelling.
func setHeaderKey(content, key, value string) string {
	line := key + ": " + value
	header, sep, body := splitHeaderRegion(content)

	if header == "" {
		// No header block at all. One is created, with the blank line that
		// separates it from what is now the body.
		if body == "" {
			return line + "\n"
		}
		return line + "\n\n" + body
	}

	lines := strings.Split(header, "\n")
	for i, l := range lines {
		if strings.EqualFold(headerKeyOf(l), key) {
			lines[i] = line
			return strings.Join(lines, "\n") + sep + body
		}
	}
	return header + "\n" + line + sep + body
}
