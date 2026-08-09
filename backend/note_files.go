package backend

// ----------------------------------------------------------------------
// Plain files that live beside the notes
// ----------------------------------------------------------------------
//
// md/ is the notes tree. html/ is what the server hands out: a URL that is
// not a page resolves under html/ and nowhere else (materializeAsset).
//
// A user keeps a plain text file beside the note that refers to it - md/
// holds Log.md and log.txt together - because that is where the note is,
// that is what git sync carries, and that is what a file manager shows. The
// note then links to it with "[log](log.txt)", the browser asks for
// "/log.txt", and the server looks in html/, which has no such file. The
// link answered 404.
//
// So the two trees hold a copy each, and this file keeps the pair together.
// One direction each, because each direction has one event that causes it:
//
//   - AT START, md/x.txt is copied to html/x.txt when html/ has no copy or
//     the md/ copy is newer (syncNoteFilesToHTML). This is the direction
//     that a git pull, a file manager and a desktop editor all produce.
//   - AFTER A SAVE through the editor, the html/ copy is written back to
//     md/ (syncNoteFileToMD). The editor writes where the URL points, which
//     is html/, so without this the file beside the note would go stale.
//
// A copy carries the modification time of its source (os.Chtimes in
// copyFileWithTime). This is what keeps "newer" meaningful: a copy stamped
// with the time of the copy would be newer than the file it came from
// forever, and each start would see a pair that differs when it does not.
//
// NOTHING IS DELETED HERE. A file removed from md/ keeps its html/ copy. To
// delete, this code would have to tell an intentional removal apart from a
// tree it has not seen before, which it cannot; and the cost of the wrong
// answer is the user's data.

import (
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// syncedNoteFileExts names which files are mirrored.
//
// ".txt" is the kind that a note keeps beside it. Another plain-text kind
// that belongs there (".csv", ".log") is one line in this list, which is why
// it is a list and not a comparison against one string.
//
// ".md" IS NOT HERE AND MUST NOT BE. A markdown file in md/ is a NOTE. It
// already reaches the browser as its compiled page at "/Name.html", and a
// copy of it under html/ would be the same note at a second URL, with the
// page cache of the first sitting next to it.
var syncedNoteFileExts = []string{".txt"}

func isSyncedNoteFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range syncedNoteFileExts {
		if ext == e {
			return true
		}
	}
	return false
}

// syncNoteFilesToHTML copies each mirrored file in md/ to html/ when html/
// has no copy or the md/ copy is newer. It runs once, at start.
//
// Newer, not different: a comparison of content would read every one of
// these files on every start. The modification time is what a git checkout,
// a file manager, a desktop editor and copyFileWithTime all set, so it is
// the fact that is available.
//
// The walk is over md/ only and reads nothing until it finds a file to copy,
// so it stays cheap on a tree of some thousands of notes. It is synchronous
// for that reason: a link tapped in the first second after start must not
// race the copy that makes it work.
func (a *App) syncNoteFilesToHTML() {
	mdRoot := filepath.Join(a.StorageDir, "md")
	htmlRoot := filepath.Join(a.StorageDir, "html")
	copied := 0

	filepath.WalkDir(mdRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isSyncedNoteFile(d.Name()) {
			return nil
		}
		rel, relErr := filepath.Rel(mdRoot, p)
		if relErr != nil {
			return nil
		}
		src, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		dst := filepath.Join(htmlRoot, rel)
		if dstInfo, statErr := os.Stat(dst); statErr == nil &&
			!src.ModTime().After(dstInfo.ModTime()) {
			return nil
		}
		if copyErr := copyFileWithTime(p, dst); copyErr != nil {
			log.Printf("[note-files] md/%s to html/: %v", filepath.ToSlash(rel), copyErr)
			return nil
		}
		copied++
		return nil
	})

	if copied > 0 {
		log.Printf("[note-files] copied %d file(s) from md/ to html/", copied)
	}
}

// syncNoteFileToMD copies a file that was just written under html/ back to
// md/. It is the other half of syncNoteFilesToHTML and runs after a save.
//
// htmlPath is the path the caller already resolved (resolvePageName), not a
// name to resolve again: one resolution, one file.
//
// It writes md/ even when nothing is there yet. A .txt first created through
// the editor then joins the notes tree, where the next start finds it and
// where git sync carries it beside the notes that link to it.
//
// KNOWN GAP: the external editor (/api/edit-external) opens the html/ copy
// in another application and gets no completion event back - cmd.Start() and
// the Android intent are both fire and forget. An edit made that way reaches
// md/ at no point. It is not lost: html/ holds it and serves it. The pair is
// simply one-sided until someone saves that file through the in-app editor.
func (a *App) syncNoteFileToMD(htmlPath string) {
	if !isSyncedNoteFile(htmlPath) {
		return
	}
	htmlRoot := filepath.Join(a.StorageDir, "html")
	rel, err := filepath.Rel(htmlRoot, htmlPath)
	// A path that leaves html/ is not a file beside a note, whatever it is.
	// resolvePageName builds htmlPath with filepath.Join, which RESOLVES a
	// "../" in a name rather than refusing it, so a name is only as trusted
	// as its caller - and this function will not carry that into md/.
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(rel) {
		return
	}
	dst := filepath.Join(a.StorageDir, "md", rel)
	if copyErr := copyFileWithTime(htmlPath, dst); copyErr != nil {
		log.Printf("[note-files] html/%s to md/: %v", filepath.ToSlash(rel), copyErr)
	}
}

// copyFileWithTime copies src over dst, creating dst's directory, and gives
// dst the modification time of src.
//
// The time is the point. Without it each copy is newer than its source, so
// the next start reads an identical pair as "md/ changed" and copies it
// again, and a save that writes back to md/ makes md/ newer than the html/
// copy that produced it.
//
// It streams: one of these files is a log that a note points at, and its
// size is the user's business.
func copyFileWithTime(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}
