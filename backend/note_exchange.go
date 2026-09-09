package backend

// ----------------------------------------------------------------------
// Sending a note to another person, and receiving one back
// ----------------------------------------------------------------------
//
// One note leaves as its Markdown source, with one line added to the header
// block, and it arrives under md/incoming/. The transport is not the
// business of this file. The Android share sheet reaches Telegram, e-mail,
// LocalSend, Bluetooth and everything else installed. The desktop uses a
// download and an upload. All of them carry the same bytes.
//
// THE PATH TRAVELS INSIDE THE FILE. Every transport delivers a flat file
// name: md/project/Sub/WeeklyPlan.md arrives as an attachment called
// something, and the folder it lived in is gone. "FileName:" in the header
// block is the only place the note's own name survives Telegram, e-mail and
// LocalSend alike.
//
// AN EXPORT IS A READ. The header line is added to the copy that leaves. The
// stored note is never written to by an export.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// incomingDirName is the one place an arriving note may land, relative
	// to md/. Nothing that arrives can overwrite a note the user wrote, and
	// that holds even when the sanitizer below is wrong about something.
	incomingDirName = "incoming"

	// incomingIndexBase is the note that lists what arrived, inside that
	// same directory: md/incoming/incoming.md.
	incomingIndexBase = "incoming"

	// incomingIndexName is the NAME of that note. It is the path under md/
	// with no extension, which is what a URL and the PageName of the
	// frontend work in. injectRuntimeVars hands it to the browser as
	// OMN_INCOMING_PAGE. omn-go-sse.js can thus tell which page its receive
	// box belongs on, with no second copy of the name in JavaScript.
	incomingIndexName = incomingDirName + "/" + incomingIndexBase

	// incomingListMarker is where a new line goes: directly after it.
	//
	// Everything below it is the list, newest first. Everything above it is
	// whatever the user has written there. The marker is what lets a user
	// put their own text at the top of the page and keep it there. That is
	// why the note starts with nothing else.
	//
	// A note with no marker, from a user who deleted it, takes its lines at
	// the top of the body instead. That is what 26.08.34 did for every note.
	incomingListMarker = "<!-- omn-go-incoming-list -->"

	headerKeyFileName = "FileName"
	headerKeyImported = "Imported"

	// headerDescription carries a note's description to the Android side,
	// which puts it in the message that goes with the file.
	//
	// BASE64, because an HTTP header field is bytes and not text. A
	// description in Cyrillic, or one with an accented letter in it, is not
	// ISO-8859-1, and a raw value would arrive damaged. A newline in it
	// would be worse. It would end the header, and the rest of the
	// description would look like a field of its own. Base64 has neither
	// problem, and Android decodes it in one call.
	headerDescription = "X-OMN-Description"

	// descriptionMaxRunes caps what travels in that header.
	//
	// 1000 is under the caption limit of Telegram, which is 1024
	// characters. Telegram does not shorten a caption that is too long. It
	// refuses the whole caption. A description of 1100 characters would thus
	// arrive as no description at all.
	descriptionMaxRunes = 1000

	// exportNameMaxRunes caps the attachment name. A recipient's filesystem
	// is not ours to assume, and 100 is short of every limit in use.
	exportNameMaxRunes = 100

	// importSegmentMaxRunes / importPathMaxRunes / importMaxSegments bound
	// what an arriving FileName: can ask for. A path from another device is
	// not a promise about anything.
	importSegmentMaxRunes = 64
	importPathMaxRunes    = 200
	importMaxSegments     = 8
)

// ----------------------------------------------------------------------
// Export
// ----------------------------------------------------------------------

// exportNoteSource returns the Markdown of a note ready to send, and the file
// name the attachment should carry.
//
// The returned bytes are the own source of the note, with FileName: SET. It
// is set and not appended. A note that was itself imported once already
// carries one, and two of them would be meaningless. See setHeaderKey.
func (a *App) exportNoteSource(name string) (data []byte, filename string, err error) {
	mdPath, _, baseName, isPage := a.resolvePageName(name)
	if !isPage {
		return nil, "", fmt.Errorf("%q is not a note", name)
	}
	src, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, "", err
	}
	out := setHeaderKey(normalizeNewlines(string(src)), headerKeyFileName, baseName)
	return []byte(out), flattenExportName(baseName) + ".md", nil
}

// flattenExportName turns a note name into ONE file name for a recipient:
//
//	project/Sub/WeeklyPlan  ->  project-Sub-WeeklyPlan.md
//
// Each "/" becomes "-". Two notes of the same name from two folders are then
// two different attachments in one mail thread, which "WeeklyPlan.md" twice
// is not.
//
// This name is A LABEL FOR A HUMAN, and it is never read back. A folder that
// already holds a "-" makes the flattening ambiguous to the eye. That costs
// nothing. The importer reads FileName: from inside the file, and it looks
// at the attachment name only when that line is missing altogether.
//
// The character set is the one a recipient can save on Windows as well as on
// Android, which is stricter than what OMN-Go itself accepts.
func flattenExportName(noteName string) string {
	out := make([]rune, 0, len(noteName))
	lastDash := false
	for _, r := range strings.ReplaceAll(noteName, "/", "-") {
		keep := r == '.' || r == '_' || r == '-' ||
			(r >= '0' && r <= '9') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z')
		if !keep {
			r = '-'
		}
		if r == '-' {
			if lastDash {
				continue
			}
			lastDash = true
		} else {
			lastDash = false
		}
		out = append(out, r)
	}
	name := strings.Trim(string(out), "-.")
	if len([]rune(name)) > exportNameMaxRunes {
		name = string([]rune(name)[:exportNameMaxRunes])
		name = strings.Trim(name, "-.")
	}
	if name == "" {
		name = "note"
	}
	return name
}

// descriptionRe finds a note's description block:
//
//	<!--- DESCRIPTION:
//	There is some
//	description
//	--->
//
// An HTML comment. It is thus invisible on the rendered page, and it needs
// no change to the header block. That block holds one line for each key, and
// it cannot carry a paragraph.
//
// FORGIVING ON THE FENCE, DELIBERATELY. "<!---" and "--->" are what the note
// author writes. "<!--" and "-->" are the standard spelling of the same
// comment, and a note that uses them means the same thing. Any number of
// dashes is accepted at each end. DESCRIPTION is matched whatever its case,
// and the colon is optional.
//
// ONE LIMIT, from HTML and not from here: a comment ends at the first "-->",
// so a description cannot contain one. A line of dashes used as a rule ends
// the description early. There is no way around that while the block is a
// comment, and a comment is what keeps it off the page.
var descriptionRe = regexp.MustCompile(`(?is)<!--+\s*DESCRIPTION\b\s*:?\s*(.*?)\s*--+>`)

// noteDescription returns the text of the first description block in a note,
// or "" when the note has none.
//
// The FIRST one, and searched over the whole note rather than only the lines
// under the header block. A second block is a mistake either way, and a note
// that keeps its description a paragraph lower down still means it.
func noteDescription(src string) string {
	m := descriptionRe.FindStringSubmatch(normalizeNewlines(src))
	if m == nil {
		return ""
	}
	text := strings.TrimSpace(m[1])
	if r := []rune(text); len(r) > descriptionMaxRunes {
		text = strings.TrimSpace(string(r[:descriptionMaxRunes]))
	}
	return text
}

// ----------------------------------------------------------------------
// Import
// ----------------------------------------------------------------------

// importResult names where an arriving note landed.
type importResult struct {
	// Name is the note name under md/: "incoming/project/Sub/WeeklyPlan-2".
	// This is what a URL and resolvePageName work in.
	Name string
	// Rel is the path under md/incoming/: "project/Sub/WeeklyPlan-2". The
	// incoming index links to this, because the index lives in that
	// directory too.
	Rel string
	// Base is the file name as saved, carrying the collision index when one
	// was needed: "WeeklyPlan-2".
	Base string
	// Label is the text of the index link: the note's own Title, or Base
	// when the note has no usable one. See incomingLabel.
	Label string
}

// importNote writes an arriving note under md/incoming/ and adds a line to
// the incoming index. It returns where the note landed.
//
// displayName is the attachment's own name, used only when the note carries
// no FileName: line. now is passed in rather than read, so a test can state
// what the Imported: line and the index line must say.
func (a *App) importNote(content []byte, displayName string, now time.Time) (importResult, error) {
	src := normalizeNewlines(string(content))
	if strings.TrimSpace(src) == "" {
		return importResult{}, fmt.Errorf("the note is empty")
	}

	// FileName: is read and kept, not taken. It names where the note came
	// FROM. That is a fact the reader can use. It tells which note on which
	// device this copy is a copy of. A name that says
	// "project/Sub/WeeklyPlan" says more than "incoming" alone.
	//
	// The line thus does not name the file that holds it. That costs
	// nothing, because no code reads FileName: from a note on disk. The
	// import reads it here, from the bytes that arrived. The export sets
	// it (exportNoteSource -> setHeaderKey), and setHeaderKey replaces the
	// line that is there. A note that arrives, stays, and goes out again
	// thus leaves with the name it has HERE, and carries one FileName:
	// line, not two.
	original, _ := headerValue(src, headerKeyFileName)

	rel := sanitizeImportPath(original)
	if rel == "" {
		rel = sanitizeImportPath(strings.TrimSuffix(displayName, ".md"))
	}
	if rel == "" {
		title, _ := headerValue(src, "Title")
		rel = sanitizeImportPath(title)
	}
	if rel == "" {
		rel = "note-" + now.Format("2006-01-02-150405")
	}

	dir, base := path.Split(rel)
	fullDir, ok := a.incomingPath(dir)
	if !ok {
		return importResult{}, fmt.Errorf("the name %q leaves the incoming directory", original)
	}
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return importResult{}, err
	}

	wanted := base
	base = freeNoteBase(fullDir, base)
	rel = path.Join(dir, base)

	// The link text on the incoming index. A reader looks for the own Title
	// of the note, because that is what they were told it was called. The
	// file name is what OMN-Go had to call it, and that is the fallback and
	// not the first choice. The collision index is carried across, because
	// two copies of one note otherwise read as the same line twice.
	title, _ := headerValue(src, "Title")
	index := ""
	if base != wanted {
		// freeNoteBase appends "-2", "-3", ... to the name it was given.
		index = strings.TrimPrefix(base, wanted+"-")
	}
	label := incomingLabel(title, base, index)

	// The moment it arrived, in the note itself. Date: and Modified: are the
	// sender's facts about their own note and stay as they are.
	src = setHeaderKey(src, headerKeyImported, now.Format("2006-01-02 15:04:05"))

	if err := os.WriteFile(filepath.Join(fullDir, base+".md"), []byte(src), 0644); err != nil {
		return importResult{}, err
	}

	res := importResult{
		Name:  path.Join(incomingDirName, rel),
		Rel:   rel,
		Base:  base,
		Label: label,
	}
	if err := a.addIncomingIndexLine(res, now); err != nil {
		// The note is on disk and readable. Only its line is missing. Say
		// so and keep the note, and do not fail an import that succeeded.
		return res, fmt.Errorf("the note was saved, but the incoming index was not updated: %w", err)
	}
	return res, nil
}

// sanitizeImportPath turns a FileName: from another device into a path that
// is safe to join under md/incoming/, or "" when nothing usable is left.
//
// THIS IS THE ONLY ATTACKER-CONTROLLED PATH IN THE FEATURE. It is not a
// filename from our own disk. It is a line of text that arrived from the
// phone of a stranger through Telegram. Every rule here exists because the
// alternative is to write a file where the sender chose.
//
// The containment check in incomingPath() runs afterwards regardless. Two
// defenses, because one of them is a regular expression's worth of thinking
// and the other is arithmetic on a resolved path.
func sanitizeImportPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// A Windows sender writes "project\Sub\Note". It is treated as a
	// separator, and not refused. The segments that come out of it go
	// through the same rules as any other, thus a refusal of the note gains
	// nothing.
	raw = strings.ReplaceAll(raw, "\\", "/")
	raw = strings.TrimSuffix(raw, ".md")
	// A drive letter is not a path here, it is the first two characters of
	// one that was written for a different machine.
	if len(raw) > 1 && raw[1] == ':' {
		raw = raw[2:]
	}

	var segs []string
	for _, seg := range strings.Split(raw, "/") {
		seg = sanitizeImportSegment(seg)
		if seg == "" {
			continue // an empty, ".", ".." or all-punctuation segment
		}
		segs = append(segs, seg)
		if len(segs) >= importMaxSegments {
			break
		}
	}
	if len(segs) == 0 {
		return ""
	}

	// Too long, thus drop from the FRONT. The deepest folder is the part
	// that a reader needs least. The own name of the file is the part that
	// they need most.
	for len([]rune(path.Join(segs...))) > importPathMaxRunes && len(segs) > 1 {
		segs = segs[1:]
	}
	out := path.Join(segs...)
	if len([]rune(out)) > importPathMaxRunes {
		out = string([]rune(out)[:importPathMaxRunes])
		out = strings.Trim(out, " .-")
	}
	return out
}

// sanitizeImportSegment cleans ONE path segment. It returns "" for a segment
// that must not exist at all: empty, ".", "..", or one that has nothing left
// after the character rules.
func sanitizeImportSegment(seg string) string {
	seg = strings.TrimSpace(seg)
	if seg == "" || seg == "." || seg == ".." {
		return ""
	}
	out := make([]rune, 0, len(seg))
	lastDash := false
	for _, r := range seg {
		// A control character has no business in a file name, and a newline
		// in one would break the header line it came from.
		if r < 0x20 || r == 0x7f {
			continue
		}
		keep := r == ' ' || r == '.' || r == '_' || r == '-' ||
			(r >= '0' && r <= '9') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z')
		if !keep {
			r = '-'
		}
		// "Note (2)" would otherwise become "Note -2-": one dash per
		// discarded character, and a name ending in punctuation.
		if r == '-' {
			if lastDash {
				continue
			}
			lastDash = true
		} else {
			lastDash = false
		}
		out = append(out, r)
	}
	// A leading dot hides the file. A trailing dot or space is a name that
	// Windows cannot store. A trailing dash is untidy.
	seg = strings.Trim(string(out), " .-")
	if len([]rune(seg)) > importSegmentMaxRunes {
		seg = string([]rune(seg)[:importSegmentMaxRunes])
		seg = strings.Trim(seg, " .-")
	}
	return seg
}

// incomingPath joins rel under md/incoming/ and reports whether the result is
// still inside it.
//
// filepath.Join RESOLVES a "..", and it does not refuse one. syncNoteFileToMD
// had to guard against the same thing in 26.08.31. sanitizeImportPath drops
// every ".." that it sees. This asks the resolved path itself, and that is
// the only question that matters.
func (a *App) incomingPath(rel string) (string, bool) {
	root := filepath.Join(a.StorageDir, "md", incomingDirName)
	full := filepath.Join(root, filepath.FromSlash(rel))
	inside, err := filepath.Rel(root, full)
	if err != nil || inside == ".." ||
		strings.HasPrefix(inside, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(inside) {
		return "", false
	}
	return full, true
}

// freeNoteBase returns base, or base-2, base-3 ... - the first that names no
// existing note in dir.
//
// "-2" and not " (2)" or "~2": the set that survives a URL, a goldmark
// heading id, git and a Windows checkout is A-Za-z0-9._-. The count starts at
// 2, the way a file manager numbers a second copy. An existing note that is
// really called "WeeklyPlan-2" only makes the loop take one more step.
func freeNoteBase(dir, base string) string {
	if !fileExists(filepath.Join(dir, base+".md")) {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !fileExists(filepath.Join(dir, candidate+".md")) {
			return candidate
		}
	}
}

// ----------------------------------------------------------------------
// The incoming index
// ----------------------------------------------------------------------

// hrefEscapePath percent-encodes a note path for use in an href.
//
// url.PathEscape is not the right tool. It escapes "/" as well, and the
// separators here have to stay separators.
//
// What needs encoding is short. sanitizeImportSegment already allows only
// "A-Za-z0-9 ._-", and the "/" between segments. Of those, only the space is
// a problem in an attribute.
//
// This does not read that allowlist. It encodes everything outside the
// unreserved set, thus it stays correct if the sanitizer is ever widened.
func hrefEscapePath(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~', c == '/':
			b.WriteByte(c)
		default:
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

// incomingLabelMaxRunes caps the link text on the incoming index. A Title:
// line is one line, and nothing says it is a short one. A title that runs
// past the width of the page turns the list into prose.
const incomingLabelMaxRunes = 80

// incomingLabelUnsafe is what a Title: may not carry into a Markdown link
// label, in the flavour this application renders (goldmark, GFM extensions,
// raw HTML allowed):
//
//	[ ]    end the label early, and the rest of the title becomes body text.
//	< > &  raw HTML and entities are passed through, and not escaped.
//	\      escapes whatever follows it.
//	` * ~  a code span, emphasis and strikethrough. "*" is here because it
//	       emphasizes inside a word, which "_" does not, thus "_" is left.
//	|      a table cell, if the line ever ends up in one.
//
// Each becomes a space, and the spaces then collapse. They are removed and
// not escaped. A backslash before each of these would keep the character.
// It would also leave the source of the note full of "\[", for a gain that
// nobody reading the list can see.
const incomingLabelUnsafe = "[]<>&\\`*~|"

// incomingLabel is the text of one line's link on the incoming index.
//
// The own Title of the note, because that is the name the sender knows it
// by, and the name a reader looks for. The FILE name is what OMN-Go had to
// call it. It comes from a path, stripped of everything a filesystem
// dislikes. It is the fallback for a note that carries no usable Title, and
// not the first choice.
//
// index is the collision suffix that the file name received, such as "2" for
// WeeklyPlan-2, or "". It is carried into the label. Two copies of one note
// otherwise read as the same line twice, with no way to tell which link is
// which. It is not added to the fallback, because the file name already
// carries it.
//
// The title is text from another device. It is cut down to one line of plain
// text here, and that is the whole of its treatment. The line is Markdown,
// thus nothing downstream will escape it later.
func incomingLabel(title, base, index string) string {
	clean := make([]rune, 0, len(title))
	space := true // leading whitespace is dropped by starting "inside" a run
	for _, r := range title {
		if r == '\t' || r == '\n' || r == '\r' || r == ' ' ||
			(r < 0x80 && strings.ContainsRune(incomingLabelUnsafe, r)) {
			if !space {
				clean = append(clean, ' ')
				space = true
			}
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue // a control character has no business on the page
		}
		clean = append(clean, r)
		space = false
	}
	label := strings.TrimSpace(string(clean))
	if len([]rune(label)) > incomingLabelMaxRunes {
		label = strings.TrimSpace(string([]rune(label)[:incomingLabelMaxRunes])) + "\u2026"
	}
	if label == "" {
		return base
	}
	if index != "" {
		label += " (" + index + ")"
	}
	return label
}

// addIncomingIndexLine puts one line at the top of md/incoming/incoming.md.
//
// At the TOP, directly below the header block. The reason to open this note
// is "what has arrived", thus the newest line is the first one. That is the
// same insertion point that handleNewPage uses for the link it adds to a
// source note.
//
// The link target is relative to the own directory of the index, because the
// index IS in that directory. A note at md/incoming/project/Sub/WeeklyPlan-2
// is "project/Sub/WeeklyPlan-2" from here. rewriteInternalLink turns that
// into "project/Sub/WeeklyPlan-2.html", and the browser resolves it under
// /incoming/.
//
// The link TEXT is the name as saved. It carries the collision index when
// there was one, thus a second copy of a note reads as a second copy in the
// list. There is no "from" note of the original path. The target already
// spells that out, minus the incoming/ root.
func (a *App) addIncomingIndexLine(res importResult, now time.Time) error {
	dir := filepath.Join(a.StorageDir, "md", incomingDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	indexPath := filepath.Join(dir, incomingIndexBase+".md")

	content := ""
	if data, err := os.ReadFile(indexPath); err == nil {
		content = normalizeNewlines(string(data))
	} else if !os.IsNotExist(err) {
		return err
	} else {
		content = incomingIndexStarter(now)
	}

	label := res.Label
	if label == "" {
		label = res.Base
	}
	// A Markdown link, so the line reads as a line of a note and not as
	// markup. incomingLabel has already taken out of the label everything
	// that would end the link early or start markup of its own.
	//
	// The destination is percent-encoded, because a note name may hold a
	// space and a bare Markdown destination cannot. "[x](My Notes/Plan)" is
	// not a link at all. The date keeps its <span>, because Markdown has no
	// way to give one part of a line its own size. The date is not a link,
	// and the link is the part that had to stop being HTML.
	line := "* <span class=\"omn-incoming-when\">" + now.Format("2006-01-02 15:04") +
		"</span> · [" + label + "](" + hrefEscapePath(res.Rel) + ")"

	header, sep, body := splitHeaderRegion(content)
	if header == "" {
		return os.WriteFile(indexPath, []byte(line+"\n\n"+content), 0644)
	}

	// Below the marker, when the note has one: the receive box sits above it
	// and has to stay reachable.
	if at := strings.Index(body, incomingListMarker); at >= 0 {
		at += len(incomingListMarker)
		if at < len(body) && body[at] == '\n' {
			at++
		}
		return os.WriteFile(indexPath,
			[]byte(header+sep+body[:at]+line+"\n"+body[at:]), 0644)
	}

	// No marker: the line becomes the FIRST body line, and then a blank line
	// between it and the header block is not decoration.
	//
	// "* 2026-08-09 12:34 · [x](y)" holds a colon, and it does not begin
	// with a space, a '#' or a '<'. isHeaderFirstLine thus reads it as
	// another "Key: value". With one newline in front of it, the line joins
	// the HEADER BLOCK and does not start the body. It never renders, and
	// the next arrival is appended after it. That turns "newest first" into
	// oldest first. One blank line is what makes the list a list.
	if body == "" {
		return os.WriteFile(indexPath, []byte(header+"\n\n"+line+"\n"), 0644)
	}
	return os.WriteFile(indexPath, []byte(header+"\n\n"+line+"\n"+body), 0644)
}

// incomingIndexStarter is the incoming index as first written.
//
// It is a TEMPLATE and not a string in this file. It is markup and a note
// script, and it is the receive box that the desktop application imports
// through. That belongs in frontend/templates/, with the other page
// fragments.
//
// It cannot ship in frontend/md/ with the other starter notes. initStorage
// extracts those FLAT into md/. A file there would thus land at
// md/incoming.md, and never at md/incoming/incoming.md.
//
// It is written one time, when the note is absent. From that moment it
// belongs to the user. Nothing rewrites it on a version change, and a user
// who deletes the receive box keeps a working list.
func incomingIndexStarter(now time.Time) string {
	return normalizeNewlines(fill(incomingIndexTmpl, map[string]string{
		"DATE": now.Format("2006-01-02 15:04:05"),
	}))
}

// ensureIncomingIndex writes the incoming index when it is absent.
//
// Called at startup, and by an import as well. On the desktop the receive
// box IS the way a first note arrives. The page has to exist before
// there is anything to list on it.
func (a *App) ensureIncomingIndex(now time.Time) error {
	dir := filepath.Join(a.StorageDir, "md", incomingDirName)
	indexPath := filepath.Join(dir, incomingIndexBase+".md")
	if fileExists(indexPath) {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(indexPath, []byte(incomingIndexStarter(now)), 0644)
}

// ----------------------------------------------------------------------
// Small shared helpers
// ----------------------------------------------------------------------

// normalizeNewlines makes CRLF and CR into LF. A note that traveled through
// a mail client or a Windows machine arrives with whatever that leg used.
// Every rule in this file counts lines.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// headerValue reads one header key without removing it.
func headerValue(content, key string) (string, bool) {
	header, _, _ := splitHeaderRegion(content)
	if header == "" {
		return "", false
	}
	for _, l := range strings.Split(header, "\n") {
		if strings.EqualFold(headerKeyOf(l), key) {
			if c := strings.IndexByte(l, ':'); c >= 0 {
				return strings.TrimSpace(l[c+1:]), true
			}
		}
	}
	return "", false
}

// ----------------------------------------------------------------------
// The HTTP surface
// ----------------------------------------------------------------------
//
// Two endpoints, both ADMIN-ONLY. Import writes files, and that is reason
// enough. Export is locked too, by decision. It is a new way out of the note
// tree, and a LAN guest has no business with one. A local connection
// bypasses authMiddleware entirely, thus the device itself is unaffected.
// That is the case that matters, because Android is where this feature is
// used.
//
// The Android side calls both over the loopback address, exactly as
// MainActivity already posts a quick note.

// exchangeJSON answers with one JSON object. An error from these two
// endpoints is shown to a person, as a toast on Android or as a line on the
// incoming page. The message is thus the thing that matters, and not the
// shape.
func (a *App) exchangeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		a.logErrf(logExchange, "encode response: %v", err)
	}
}

func (a *App) exchangeErr(w http.ResponseWriter, status int, err error) {
	a.exchangeJSON(w, status, map[string]string{"status": "error", "message": err.Error()})
}

// handleExportNote: GET /api/export/note?name=<note>
//
// The Markdown of the note with FileName: set, as a download. The frontend
// uses the same URL in two ways. The Send control is a link to it, and "send
// as text" fetches it and copies the body to the clipboard. MainActivity
// fetches it, writes the bytes to its cache, and hands the file to the share
// sheet.
func (a *App) handleExportNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.exchangeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("GET only"))
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		a.exchangeErr(w, http.StatusBadRequest, fmt.Errorf("no note named"))
		return
	}

	data, filename, err := a.exportNoteSource(name)
	if err != nil {
		if os.IsNotExist(err) {
			a.exchangeErr(w, http.StatusNotFound, fmt.Errorf("no note %q", name))
			return
		}
		a.exchangeErr(w, http.StatusBadRequest, err)
		return
	}

	// flattenExportName leaves only A-Za-z0-9._- , so the file name needs no
	// quoting rules applied to it here and cannot close the header early.
	// That is a property of the name, and this line depends on it.
	// The description, for the MESSAGE that carries the file - a Telegram
	// caption, a mail body. It stays in the note as well: it is part of the
	// note, and the receiver can send the note on with it.
	//
	// A header, and not a second endpoint. The Android side needs the bytes
	// and the text together in one answer, and it already has this response
	// open.
	if desc := noteDescription(string(data)); desc != "" {
		w.Header().Set(headerDescription, base64.StdEncoding.EncodeToString([]byte(desc)))
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Write(data)
}

// handleImportNote: POST /api/import/note?name=<display name>
//
// Two callers, two body shapes, one rule set behind them:
//
//   - Android POSTs the bytes it read from the shared content:// URI, with
//     the attachment's own name as ?name= . Raw body, because the native side
//     has bytes and a name and no reason to build a multipart request.
//   - the desktop upload control POSTs a form file, because that is what a
//     browser sends from a file input.
//
// ?name= is only a fallback for a note that carries no FileName: line. It is
// not trusted any more than FileName: is, and it goes through the same
// sanitizer.
func (a *App) handleImportNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.exchangeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("POST only"))
		return
	}

	limit := a.maxUploadBytes()
	displayName := r.URL.Query().Get("name")
	var content []byte

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(limit); err != nil {
			a.exchangeErr(w, http.StatusBadRequest, fmt.Errorf("cannot read the upload: %w", err))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			a.exchangeErr(w, http.StatusBadRequest, fmt.Errorf("no file in the upload"))
			return
		}
		defer file.Close()
		if content, err = readImportBody(file, limit); err != nil {
			a.exchangeErr(w, http.StatusRequestEntityTooLarge, err)
			return
		}
		if displayName == "" && header != nil {
			displayName = header.Filename
		}
	} else {
		var err error
		if content, err = readImportBody(r.Body, limit); err != nil {
			a.exchangeErr(w, http.StatusRequestEntityTooLarge, err)
			return
		}
	}

	res, err := a.importNote(content, displayName, time.Now())
	if res.Name == "" {
		// Nothing was written. This is the only real failure.
		a.exchangeErr(w, http.StatusBadRequest, err)
		return
	}

	out := map[string]string{
		"status": "success",
		"name":   res.Name,
		"base":   res.Base,
		"url":    "/" + res.Name + ".html",
	}
	if err != nil {
		// The note is on disk and readable. Only its line on the incoming
		// index is missing. A report of a failure here would tell the user
		// to send it again, and a second copy is not the repair.
		out["warning"] = err.Error()
		a.logErrf(logExchange, "%v", err)
	}
	a.logInfof(logExchange, "imported %s", res.Name)
	a.exchangeJSON(w, http.StatusOK, out)
}

// readImportBody reads at most limit bytes and reports an error when there
// were more.
//
// This is NOT the readCapped of search.go. This function carried that name
// until the two collided, and it must not reach for that one. The other one
// takes a PATH and TRUNCATES on purpose. "Found nothing in the part I looked
// at" is a useful answer about a 2 MB note. Half a note is not a
// useful import. The two have opposite behavior at the cap, and they must
// stay apart.
func readImportBody(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("the note is larger than the upload limit of %d MB",
			limit/(1024*1024))
	}
	return data, nil
}
