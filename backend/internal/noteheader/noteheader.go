// Package noteheader parses the header block at the start of a note.
package noteheader

import (
	"strings"
	"time"
)

// ----------------------------------------------------------------------
// The one header block parser
// ----------------------------------------------------------------------
//
// doc/TERMINOLOGY.md calls this the "header block", and the code uses the
// same name. A note can start with "Key: Value" lines that an empty line
// ends:
//
//	Title: My Note
//	Date: 2026-01-01 00:00:00
//	Category: Notes
//
//	Body starts here.
//
// Parse is the ONE authority for the question "where does the
// header end?". Each Go caller uses it. isHeaderFirstLine and
// firstLineAfterHeader in omn-go-editor.js are a port of this rule.
// TestHeaderPortAgreesWithTheRealJavaScript compares them. A heading such as
// "# Head: subtitle" holds a ':', and a copy of the rule gets such a case
// wrong.

// Block is a note split into its header and its body.
type Block struct {
	// HasHeader is true when the content starts with a header block. See
	// Parse.
	HasHeader bool
	// Header holds the header lines joined by "\n", WITHOUT the empty line
	// that ends them. It is empty when HasHeader is false.
	Header string
	// Body is the text after the header, or the whole content when there is
	// no header.
	Body string
	// BodyOffset is the byte offset in the ORIGINAL content where Body
	// starts, or 0 with no header. The caret of the editor uses the same
	// position.
	BodyOffset int
}

// IsFirstLine reports whether the FIRST line of a note is a header
// line, as in "Key: Value". It must hold a ':', and it must NOT start with a
// space, a '#' or a '<'. Those three mark Markdown or HTML with a colon, for
// example "# Heading: subtitle" or "<script>let x: 1". The function ignores a
// trailing CR, thus a CRLF file gives the same answer. Keep the JavaScript
// port the same.
func IsFirstLine(line string) bool {
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

// Parse splits content into its optional header and its body. A
// header exists only when the FIRST line passes isHeaderFirstLine. The header
// ends at the FIRST of two lines:
//
//   - An empty line, also one with only spaces or tabs. The parser drops
//     it, and the body starts after it.
//   - A line that fails isHeaderFirstLine, for example "<style>" or prose
//     with no colon. That line is the first BODY line, and it stays.
//
// Both ends are necessary. A "<style>" block can follow the header directly.
// With only the first rule, the parser would read a CSS line "--var: #hex;"
// as a header line. A note with only header lines has an empty body.
func Parse(content string) Block {
	firstLine := content
	if nl := strings.IndexByte(content, '\n'); nl >= 0 {
		firstLine = content[:nl]
	}
	if !IsFirstLine(firstLine) {
		return Block{Body: content}
	}

	lines := strings.Split(content, "\n")

	// makeResult makes the split from the first body line and the end of the
	// header.
	makeResult := func(bodyStart int, headerEndExclusive int) Block {
		offset := 0
		for i := 0; i < bodyStart; i++ {
			offset += len(lines[i]) + 1 // +1 for the '\n' strings.Split removed
		}
		if offset > len(content) {
			offset = len(content) // degenerate trailing-line-with-no-newline case
		}
		return Block{
			HasHeader:  true,
			Header:     strings.Join(lines[:headerEndExclusive], "\n"),
			Body:       strings.Join(lines[bodyStart:], "\n"),
			BodyOffset: offset,
		}
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			// This is an empty line. The parser drops it, and the body starts
			// on the next line.
			return makeResult(i+1, i)
		}
		if !IsFirstLine(lines[i]) {
			// This line is not a header line, thus it is the first body line.
			return makeResult(i, i)
		}
	}

	// Each line is a header line, thus the body is empty.
	return Block{HasHeader: true, Header: content, BodyOffset: len(content)}
}

// ----------------------------------------------------------------------
// Read and write ONE header key
// ----------------------------------------------------------------------
//
// internal/exchange/exchange.go SETS "FileName:" on a note that it sends, and
// "Imported:" on a note that it receives. SET means that it replaces the line
// when it exists. A note can travel from A to B to C. An append would then give
// two "Imported:" lines, and a header with one key twice has no defined
// meaning.
//
// These functions put the header back into the ORIGINAL string, and they do
// not join a parsed copy. The separator is one newline after a header that
// ends at a body line, and two after an empty line. A fixed "\n\n" would add
// an empty line to the first kind. header + separator + body is always equal
// to content.

// SplitRegion cuts content into the header, the separator after it, and
// the body. The three together are content, byte for byte. header and sep are
// empty when there is no header block.
func SplitRegion(content string) (header, sep, body string) {
	hb := Parse(content)
	if !hb.HasHeader {
		return "", "", content
	}
	return content[:len(hb.Header)], content[len(hb.Header):hb.BodyOffset], content[hb.BodyOffset:]
}

// KeyOf answers the key of a "Key: value" line as written, or "" when
// the line has no colon.
func KeyOf(line string) string {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(line[:i])
}

// SetKey answers content with "key: value" in its header block. It
// REPLACES an existing line with that key where it is, thus the order of the
// metadata stays. It adds a new key as the last header line. A note with no
// header block gets one. The key match ignores case, and the new line uses
// the spelling of the caller.
func SetKey(content, key, value string) string {
	line := key + ": " + value
	header, sep, body := SplitRegion(content)

	if header == "" {
		// With no header block, make one, with the empty line before the
		// body.
		if body == "" {
			return line + "\n"
		}
		return line + "\n\n" + body
	}

	lines := strings.Split(header, "\n")
	for i, l := range lines {
		if strings.EqualFold(KeyOf(l), key) {
			lines[i] = line
			return strings.Join(lines, "\n") + sep + body
		}
	}
	return header + "\n" + line + sep + body
}

// StampLayout is the form of each time that the application writes in a note.
const StampLayout = "2006-01-02 15:04:05"

// Stamp answers t in UTC, in StampLayout. The desktop and Android then
// write the same text for one moment. See
// doc/decisions/0023-write-each-time-in-utc.md.
func Stamp(t time.Time) string {
	return t.UTC().Format(StampLayout)
}

// New answers the header block of a new note, with the empty line after
// it. Date and Modified hold the same time. An empty author gives no
// Author line.
func New(title, author string, now time.Time) string {
	stamp := Stamp(now)
	header := "Title: " + title + "\nDate: " + stamp + "\nModified: " + stamp + "\nCategory: Notes\n"
	if author != "" {
		header += "Author: " + author + "\n"
	}
	return header + "\n"
}
