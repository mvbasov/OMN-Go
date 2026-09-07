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
// re-implemented in four places, and each one differed a little. Three were
// in Go, in compilePageWithBody, ensureHeaderModified and handleNewPage. The
// fourth was firstLineAfterHeader, in the JavaScript of the editor.
//
// Those variants disagreed on edge cases. Most visibly,
// compilePageWithBody read any first line that held a ':' as a header line.
// A Markdown heading such as "# Head: subtitle" was thus swallowed as
// metadata, and it did not render as a heading.
//
// parseHeaderBlock is now the ONE authority. Every Go caller goes through
// it, and the firstLineAfterHeader of the editor mirrors isHeaderFirstLine
// exactly. See backend/frontend/html/js/OMN-Go/omn-go-editor.js, and
// CODE_REVIEW.md Phase 1.

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

// isHeaderFirstLine reports whether line looks like a metadata key line, as
// in "Key: Value". The line is the FIRST line of a note. It must contain a
// ':', and it must NOT start with a space, a '#' or a '<'.
//
// Those three mark a line of Markdown or of raw HTML body that happens to
// contain a colon. Examples are "# Heading: subtitle", an indented
// continuation, and "<script>let x: 1".
//
// A trailing CR is ignored, thus a CRLF file classifies the same as an LF
// one.
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
// isHeaderFirstLine. The header then continues line by line, and it ends at
// the FIRST of these two:
//   - a blank line, which is empty after a trim of the whitespace. A
//     "separator" line that carries stray spaces or tabs thus still counts,
//     and real notes have such a line. The blank line is the separator and
//     is dropped, and the body starts after it.
//   - a line that is not itself a "Key: Value" header line, which is a line
//     that fails isHeaderFirstLine. Examples are "<style>" and a prose line
//     with no colon. That line is the first BODY line, and it is kept.
//
// Both conditions matter. An earlier version required a blank line alone.
// Content could follow a header at once, as a "<style>" block, a prose
// paragraph, or a whitespace-only separator. The header then ran on until
// the first truly empty line, and it swallowed a CSS "--var: #hex;" line as
// bogus metadata.
//
// A header with neither a blank line nor a non-header line after it is a
// note that is only metadata. Such a note has an empty body. With no header
// at all, the whole content is the body.
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
			// Blank separator line. It is dropped, and the body starts
			// on the next line.
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
// Note exchange, in note_exchange.go, has to put "FileName:" on a note that
// it sends, and "Imported:" on a note that it receives. It has to take
// "FileName:" off again at the other end. Both must SET a key, which means
// to replace the line when it is already there. Neither may append a second
// line with the same key.
//
// That is not fussiness. A note can make more than one hop, as when A sends
// to B and B sends to C. An append on the second import would leave the note
// with two "Imported:" lines. A header block with one key twice has no
// defined meaning. parseHeaderBlock would hand the first one to whatever
// reads it. Which of the two is first is an accident of the order that the
// hops ran in.
//
// Both functions splice the header back into the ORIGINAL string. They do
// not re-join a parse of it. The separator between the header and the body
// is one newline when the header ended at a non-header line. "<style>" on
// the next line is such a line. It is two when the header ended at a blank
// line. A rebuild with a fixed "\n\n" would thus silently insert a blank
// line into the first kind. The three pieces below always satisfy
// header + separator + body == content.

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
