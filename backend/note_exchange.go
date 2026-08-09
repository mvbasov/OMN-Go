package backend

// ----------------------------------------------------------------------
// Sending a note to another person, and receiving one back
// ----------------------------------------------------------------------
//
// One note leaves as its Markdown source with one line added to the header
// block, and arrives under md/incoming/. The transport is not this file's
// business: the Android share sheet reaches Telegram, e-mail, LocalSend,
// Bluetooth and everything else installed, and the desktop uses a download
// and an upload. All of them carry the same bytes.
//
// THE PATH TRAVELS INSIDE THE FILE. Every transport delivers a flat file
// name: md/project/Sub/WeeklyPlan.md arrives as an attachment called
// something, and the folder it lived in is gone. "FileName:" in the header
// block is the only place the note's own name survives Telegram, e-mail and
// LocalSend alike.
//
// AN EXPORT IS A READ. The header line is added to the copy that leaves. The
// stored note is never written to by an export.
//
// See claude/note-exchange-plan.md for the decisions behind this.

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
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

	headerKeyFileName = "FileName"
	headerKeyImported = "Imported"

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
// The returned bytes are the note's own source with FileName: SET - set and
// not appended, because a note that was itself imported once already carries
// one and two would be meaningless (see setHeaderKey).
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
// This name is A LABEL FOR A HUMAN and is never read back. A folder that
// already holds a "-" makes the flattening ambiguous to the eye, and that
// costs nothing: the importer reads FileName: from inside the file and only
// looks at the attachment name when that line is missing altogether.
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
	// was needed: "WeeklyPlan-2". It is the text of the index link.
	Base string
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

	// FileName: is taken, not read: it names where the note came FROM, and
	// the note is about to live somewhere else. Leaving it in would be a
	// line that lies about the file it sits in.
	original, src := takeHeaderKey(src, headerKeyFileName)

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

	base = freeNoteBase(fullDir, base)
	rel = path.Join(dir, base)

	// The moment it arrived, in the note itself. Date: and Modified: are the
	// sender's facts about their own note and stay as they are.
	src = setHeaderKey(src, headerKeyImported, now.Format("2006-01-02 15:04:05"))

	if err := os.WriteFile(filepath.Join(fullDir, base+".md"), []byte(src), 0644); err != nil {
		return importResult{}, err
	}

	res := importResult{
		Name: path.Join(incomingDirName, rel),
		Rel:  rel,
		Base: base,
	}
	if err := a.addIncomingIndexLine(res, now); err != nil {
		// The note is on disk and readable; only its line is missing. Say so
		// and keep the note rather than failing an import that succeeded.
		return res, fmt.Errorf("the note was saved, but the incoming index was not updated: %w", err)
	}
	return res, nil
}

// sanitizeImportPath turns a FileName: from another device into a path that
// is safe to join under md/incoming/, or "" when nothing usable is left.
//
// THIS IS THE ONLY ATTACKER-CONTROLLED PATH IN THE FEATURE. It is not a
// filename from our own disk; it is a line of text that arrived from a
// stranger's phone through Telegram. Every rule here exists because the
// alternative is writing a file where the sender chose.
//
// The containment check in incomingPath() runs afterwards regardless. Two
// defenses, because one of them is a regular expression's worth of thinking
// and the other is arithmetic on a resolved path.
func sanitizeImportPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// A Windows sender writes "project\Sub\Note". Treated as a separator
	// rather than refused: the segments that come out of it go through the
	// same rules as any other, so nothing is gained by rejecting the note.
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

	// Too long: drop from the FRONT. The deepest folder is the part a reader
	// needs least; the file's own name is the part they need most.
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
	// A leading dot hides the file; a trailing dot or space is a name
	// Windows cannot store; a trailing dash is just untidy.
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
// filepath.Join RESOLVES a "..", it does not refuse one - the same thing
// syncNoteFileToMD had to guard against in 26.08.31. sanitizeImportPath drops
// every ".." it sees; this asks the resolved path itself, which is the only
// question that actually matters.
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

// addIncomingIndexLine puts one line at the top of md/incoming/incoming.md.
//
// At the TOP, directly below the header block: the reason to open this note
// is "what just arrived", so the newest line is the first one. That is the
// same insertion point handleNewPage uses for the link it adds to a source
// note.
//
// The link target is relative to the index's own directory, because the index
// IS in that directory: a note at md/incoming/project/Sub/WeeklyPlan-2 is
// "project/Sub/WeeklyPlan-2" from here, which rewriteInternalLink turns into
// "project/Sub/WeeklyPlan-2.html" and the browser resolves under /incoming/.
//
// The link TEXT is the name as saved, carrying the collision index when there
// was one, so a second copy of a note reads as a second copy in the list.
// There is no "from" note of the original path: the target already spells it
// out, minus the incoming/ root.
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
		// First arrival on this device. The index is created here rather than
		// shipped with the application: a bundled note is extracted flat into
		// md/, so md/incoming/incoming.md cannot come from the embedded tree
		// (see initStorage). It is user-owned from this moment - nothing
		// rewrites it on a version change.
		//
		// A header block and nothing else. A line of introduction under it
		// would be pushed further down the page by every note that arrived,
		// because each new line goes in ABOVE the body - so the sentence
		// explaining the list would end up beneath the list.
		content = "Title: Incoming notes\nDate: " + now.Format("2006-01-02 15:04:05") +
			"\nCategory: Notes\n"
	}

	line := "* " + now.Format("2006-01-02 15:04") + " · [" + res.Base + "](" + res.Rel + ")"

	header, _, body := splitHeaderRegion(content)
	if header == "" {
		return os.WriteFile(indexPath, []byte(line+"\n\n"+content), 0644)
	}

	// A BLANK line between the header block and the line, always - the
	// separator that was there is not reused.
	//
	// "* 2026-08-09 12:34 · [x](y)" holds a colon and does not begin with a
	// space, '#' or '<', so isHeaderFirstLine reads it as another
	// "Key: value". With one newline in front of it the line joins the
	// HEADER BLOCK instead of starting the body: it never renders, and the
	// next arrival is appended after it, which turns "newest first" into
	// oldest first. One blank line is what makes the list a list.
	if body == "" {
		return os.WriteFile(indexPath, []byte(header+"\n\n"+line+"\n"), 0644)
	}
	return os.WriteFile(indexPath, []byte(header+"\n\n"+line+"\n"+body), 0644)
}

// ----------------------------------------------------------------------
// Small shared helpers
// ----------------------------------------------------------------------

// normalizeNewlines makes CRLF and CR into LF. A note that travelled through
// a mail client or a Windows machine arrives with whatever that leg used, and
// every rule in this file counts lines.
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
