package backend

// ----------------------------------------------------------------------
// The file index: /OMNGoFiles.html
// ----------------------------------------------------------------------
//
// Three trees, one question each, one directory at a time.
//
//	Bundled  what this build carries - staticFS, which embeds frontend/html
//	         and frontend/md. Templates are absent by construction, not by a
//	         filter: they live in a separate embed (see the comment on
//	         templatesFS). A test pins it anyway, because "by construction"
//	         stops being true the moment someone merges the two embeds.
//	Served   what a URL finds - StorageDir/html, minus db_backup/.
//	Source   what you wrote - StorageDir/md.
//
// Until 26.08.53 there were two of these on ONE screen, as two sections, and
// the reader had to pair the rows by eye. A name that appeared in both was
// printed twice and neither row said how the two were related. Each tree is
// its own screen now, and inside a tree each NAME has exactly one row that
// states the relation.
//
// TWO CHANNELS PER ROW. The word says what the file IS. The colour says what
// HAPPENS to it:
//
//	orange  the next version of the application replaces this file
//	red     ... and the copy on the device differs, so that work goes to a
//	        backup and stops being used. Also the one .txt case that no
//	        start repairs by itself (see filesMirrorState).
//	green   yours - OMN-Go keeps it
//	teal    OMN-Go makes it again when it needs to (a compiled page, a copy
//	        of a file in md/)
//	grey    nothing at stake
//
// Colour is never the only carrier: "app-owned" is a WORD on the second line
// of every row it applies to, in each of the three trees.
//
// NOTHING HERE MAY WRITE. In particular it must never call materializeAsset:
// a listing that extracted all 70 embedded files as a side effect of
// describing them would defeat lazy extraction entirely, and materializeAsset
// is exactly the function this code would otherwise reach for to resolve a
// path. Reading an embedded file to compare it is not writing.

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// filesDirLimit caps how many FILES one directory prints. Directory rows are
// never capped: there are never enough of them to matter, and hiding a
// subdirectory hides a branch of the tree rather than some leaves.
const filesDirLimit = 200

// filesExcludedDir is omitted at every level it would appear.
//
// Database dumps are named with the device hostname and are whole-database
// snapshots. Note what this is NOT: they are already fetchable by path,
// because the root catch-all resolves any non-.html URL under StorageDir/html.
// Leaving them out of a listing is a listing decision, not a security control,
// and the two should not be confused.
const filesExcludedDir = "db_backup"

// filesCompareMax bounds the byte comparison that tells "as shipped" from
// "changed here".
//
// Sizes come free with the walk, so a file of a different size is answered
// without opening anything. Two files of the SAME size still need their bytes:
// a typo repaired in a note keeps its length often enough to matter, and a row
// that reads "as shipped" for a file the user edited is worse than no row at
// all. The read is bounded because one directory is in view: html/js is the
// largest in the tree at about 620 KB. Above this cap the row says "same size"
// rather than claiming to know.
const filesCompareMax = 2 << 20

// The three trees. The key is what ?tree= carries and what the crumb shows.
const (
	filesTreeBundled = "bundled"
	filesTreeServed  = "served"
	filesTreeSource  = "source"
)

// indexedFile is one file in one tree, keyed by its LOGICAL path:
// slash-separated, relative to that tree's root, no leading slash.
//
// For the embedded tree the logical path folds two embed roots into one
// namespace - frontend/html/js/x.js becomes "js/x.js" and frontend/md/Note.md
// becomes "md/Note.md" - so "md" reads as an ordinary subdirectory of the
// Bundled tree. They cannot collide: md/ is a sibling of html/ in storage,
// never inside it.
type indexedFile struct {
	path string
	size int64
	mod  time.Time // zero for embedded files; embed.FS has no mtime
}

// filesEntry is one NAME in one tree, with the two sides that can hold it.
// Which side is which depends on the tree:
//
//	Bundled  ships only, so device is always nil
//	Served   ships = frontend/html/<path>, device = StorageDir/html/<path>
//	Source   ships = frontend/md/<path>,   device = StorageDir/md/<path>
type filesEntry struct {
	path   string
	ships  *indexedFile
	device *indexedFile
}

// ----------------------------------------------------------------------
// Walking
// ----------------------------------------------------------------------

func embeddedFiles() []indexedFile {
	var out []indexedFile
	for _, root := range []struct{ dir, prefix string }{
		{"frontend/html", ""},
		{"frontend/md", "md/"},
	} {
		fs.WalkDir(staticFS, root.dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, iErr := d.Info()
			if iErr != nil {
				return nil
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(p, root.dir), "/")
			out = append(out, indexedFile{path: root.prefix + rel, size: info.Size()})
			return nil
		})
	}
	return out
}

// walkStorage lists one directory of the storage tree. sub is "html" or "md".
func (a *App) walkStorage(sub string) []indexedFile {
	base := filepath.Join(a.StorageDir, sub)
	var out []indexedFile
	filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rErr := filepath.Rel(base, p)
		if rErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if sub == "html" && rel == filesExcludedDir {
				return fs.SkipDir
			}
			return nil
		}
		info, iErr := d.Info()
		if iErr != nil {
			return nil
		}
		out = append(out, indexedFile{path: rel, size: info.Size(), mod: info.ModTime()})
		return nil
	})
	return out
}

// treeEntries builds the entry list of one tree, with both sides paired by
// name. The pairing is the whole point of the page, so it happens once, here,
// and every row below reads the result.
func (a *App) treeEntries(tree string) []filesEntry {
	byPath := map[string]*filesEntry{}
	add := func(p string, f indexedFile, ships bool) {
		e := byPath[p]
		if e == nil {
			e = &filesEntry{path: p}
			byPath[p] = e
		}
		copyOf := f
		if ships {
			e.ships = &copyOf
		} else {
			e.device = &copyOf
		}
	}

	embedded := embeddedFiles()
	switch tree {
	case filesTreeBundled:
		for _, f := range embedded {
			add(f.path, f, true)
		}
	case filesTreeSource:
		for _, f := range embedded {
			// The md/ half of the embed is the shipped side of this tree.
			if rest, ok := strings.CutPrefix(f.path, "md/"); ok {
				add(rest, indexedFile{path: rest, size: f.size}, true)
			}
		}
		for _, f := range a.walkStorage("md") {
			add(f.path, f, false)
		}
	default: // served
		for _, f := range embedded {
			if strings.HasPrefix(f.path, "md/") {
				continue // a note is not served from html/
			}
			add(f.path, f, true)
		}
		for _, f := range a.walkStorage("html") {
			add(f.path, f, false)
		}
	}

	out := make([]filesEntry, 0, len(byPath))
	for _, e := range byPath {
		out = append(out, *e)
	}
	return out
}

// ----------------------------------------------------------------------
// Folding one tree down to one directory
// ----------------------------------------------------------------------

// normalizeFilesDir turns the ?dir= parameter into a logical directory prefix:
// either "" (the root) or something ending in "/".
//
// It is validated rather than trusted, then used ONLY as a string prefix
// against paths the walk already collected - it is never joined onto a
// filesystem path and handed to the OS. A dir that survives validation but
// names nothing renders an empty directory, which is the honest answer for a
// directory that does not exist and the safe one for a dir that was trying to
// leave.
func normalizeFilesDir(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(filepath.ToSlash(raw), "/")
	clean := path.Clean(raw)
	if clean == "." || clean == "/" {
		return ""
	}
	// path.Clean leaves a leading ".." in place, which is the whole point of
	// checking after cleaning rather than before.
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	if clean == filesExcludedDir || strings.HasPrefix(clean, filesExcludedDir+"/") {
		return ""
	}
	return clean + "/"
}

// normalizeFilesTree keeps ?tree= to the three known values.
//
// An address with a dir and no tree is the shape every link of 26.08.53 had,
// when the page served the html/ tree alone. Such a link still lands where it
// did.
func normalizeFilesTree(raw, dir string) string {
	switch raw {
	case filesTreeBundled, filesTreeServed, filesTreeSource:
		return raw
	}
	if dir != "" {
		return filesTreeServed
	}
	return ""
}

// foldToDir splits one tree at dir into the subdirectories directly below it
// and the entries directly in it.
//
// Directory totals are RECURSIVE - everything at or below that subdirectory -
// because "this subtree is 4 MB" is the question a file index is opened to
// answer, and the walk has collected it anyway. One name counts once, even
// when both sides hold it.
func foldToDir(entries []filesEntry, dir string) (dirs []filesDirRow, here []filesEntry, bytes int64, count int) {
	byDir := map[string]*filesDirRow{}
	for _, e := range entries {
		if dir != "" && !strings.HasPrefix(e.path, dir) {
			continue
		}
		rest := e.path[len(dir):]
		if rest == "" {
			continue
		}
		size := e.bytes()
		bytes += size
		count++

		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name := rest[:i]
			row := byDir[name]
			if row == nil {
				row = &filesDirRow{Name: name, Dir: dir + name + "/",
					everyShips: true, everyDevice: true}
				byDir[name] = row
			}
			row.Files++
			row.Bytes += size
			row.note(e)
			continue
		}
		here = append(here, e)
	}

	for _, row := range byDir {
		dirs = append(dirs, *row)
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(here, func(i, j int) bool { return here[i].path < here[j].path })
	return dirs, here, bytes, count
}

// bytes gives the size to report for one name: the copy on the device when
// there is one, because that is the copy that occupies the storage.
func (e filesEntry) bytes() int64 {
	if e.device != nil {
		return e.device.size
	}
	if e.ships != nil {
		return e.ships.size
	}
	return 0
}

// note collects the one fact a directory row can carry: a subtree that is
// entirely shipped, or entirely made on the device, says so.
func (d *filesDirRow) note(e filesEntry) {
	if e.device == nil {
		d.everyDevice = false
	} else {
		d.anyDevice = true
	}
	if e.ships == nil {
		d.everyShips = false
	} else {
		d.anyShips = true
	}
}

// ----------------------------------------------------------------------
// What a row says
// ----------------------------------------------------------------------

// The five colour classes. The CSS holds one token for each, with a value per
// theme (omn-go-core.css, section 1).
const (
	filesColorApp     = "files-c-app"     // the next version replaces this file
	filesColorAlert   = "files-c-alert"   // ... and the device copy differs
	filesColorKeep    = "files-c-keep"    // yours, kept
	filesColorDerived = "files-c-derived" // made again when needed
	filesColorPlain   = "files-c-plain"   // nothing at stake
)

// filesKindIcon gives the Material Icons ligature that marks what a file is.
//
// Every name here resolves in the bundled subset of the icon font
// (css/fonts/material-icons.woff2), which carries the full set - "javascript",
// "css" and "html" draw as JS, CSS and HTML monograms, which is what a file
// listing wants. No new asset is needed for this column.
func filesKindIcon(name string, isDir bool) string {
	if isDir {
		return "folder"
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "html"
	case ".md":
		return "article"
	case ".js":
		return "javascript"
	case ".css":
		return "css"
	case ".json", ".jsonl":
		return "data_object"
	case ".txt":
		return "subject"
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp":
		return "image"
	case ".woff", ".woff2", ".ttf", ".otf":
		return "text_fields"
	}
	return "insert_drive_file"
}

// filesEditable decides whether a row offers an "edit" link.
//
// Two exclusions, both asked for:
//
//   - a compiled .html page. Not because editing it would be wrong -
//     ?edit=true on a page resolves through resolvePageName and opens the
//     editor on the MARKDOWN SOURCE, which is entirely correct - but because
//     the rendered page already carries an Edit button that does exactly that.
//   - anything that is not text. There is nothing to type into a PNG.
//
// The second is decided by asking the one content-type table every route
// already consults, rather than adding a fourth hardcoded extension list
// beside versionDependentAssets, imageUploadExtensions and
// jsonUploadExtensions. A consequence worth knowing: SVG resolves to
// image/svg+xml and so gets no edit link, even though it is text. "Images are
// not editable here" is the rule; carving out an exception would make it
// harder to state than it is worth.
func (a *App) filesEditable(logical string) bool {
	if strings.HasSuffix(strings.ToLower(logical), ".html") {
		return false
	}
	return a.editableFileType(logical)
}

// editableFileType reports whether the content type of logical is text that
// an editor can open. It is the "anything that is not text" half of
// filesEditable, split out because the editor routes need the same answer
// without the .html rule: ?edit=true on a compiled page resolves to its
// markdown source and must stay editable, while ?edit=true on a picture, a
// font, an audio file or a video file must not open an editor at all (see
// serveEditor, handleEditExternal, handleGetNote and handleSaveNote).
func (a *App) editableFileType(logical string) bool {
	ct := a.resolveContentType(logical)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i] // drop "; charset=utf-8"
	}
	ct = strings.ToLower(strings.TrimSpace(ct))
	switch {
	case ct == "":
		return false // an unknown extension is not assumed to be text
	// Checked BEFORE the +xml / +json suffixes below, which would otherwise
	// claim image/svg+xml. SVG is text and would open in the editor perfectly
	// well; the rule that was asked for is about images, not about parsers.
	case strings.HasPrefix(ct, "image/"), strings.HasPrefix(ct, "font/"),
		strings.HasPrefix(ct, "audio/"), strings.HasPrefix(ct, "video/"):
		return false
	case strings.HasPrefix(ct, "text/"):
		return true
	// "application/jsonl" is not in the builtin table any more (.jsonl is
	// served as text/plain, so the "view" link on the Database Backups page
	// works in the Android WebView), but a config.json mime_types override
	// can still put it back - keep accepting it as text.
	case ct == "application/javascript", ct == "application/x-javascript",
		ct == "application/json", ct == "application/jsonl",
		ct == "application/xml":
		return true
	case strings.HasSuffix(ct, "+json"), strings.HasSuffix(ct, "+xml"):
		return true
	}
	return false
}

// isVersionDependent reports whether a path is one the application owns -
// replaced on the next version change, the user's copy backed up first. The
// list is read, never extended, from assets.go. want is the path as
// versionDependentAssets writes it: relative to the storage directory.
func isVersionDependent(want string) bool {
	for _, v := range versionDependentAssets {
		if v == want {
			return true
		}
	}
	return false
}

// filesSameBytes answers "are these two copies the same file".
//
// The sizes are compared first and answer most pairs for free. Equal sizes
// need the bytes, and only up to filesCompareMax; above it the caller reports
// "same size" instead of a claim it did not check.
func filesSameBytes(embeddedLogical, diskPath string, size int64) (same bool, checked bool) {
	if size > filesCompareMax {
		return false, false
	}
	emb, err := staticFS.ReadFile(embeddedLogical)
	if err != nil {
		return false, false
	}
	disk, err := os.ReadFile(diskPath)
	if err != nil {
		return false, false
	}
	return bytes.Equal(emb, disk), true
}

// filesMirrorState describes the pair that note_files.go keeps: md/x.txt is
// the file, html/x.txt is a copy of it.
//
// copyFileWithTime stamps the copy with the modification time of its source on
// purpose, so an equal (size, mtime) pair IS the answer and no byte reading is
// needed. The sign of a difference decides what happens next, and the two
// cases have different remedies:
//
//	the copy is older  the next start refreshes it (syncNoteFilesToHTML)
//	the copy is newer  nothing repairs this by itself. An external editor
//	                   wrote html/ and gave OMN-Go no signal. One save in the
//	                   internal editor copies it back (syncNoteFileToMD).
func filesMirrorState(source, copyOf *indexedFile) (word, color string, extra string) {
	if source == nil || copyOf == nil {
		return "", "", ""
	}
	if source.size == copyOf.size && source.mod.Equal(copyOf.mod) {
		return "", filesColorDerived, ""
	}
	if copyOf.mod.After(source.mod) {
		return "copy is ahead", filesColorAlert, "save it once to copy it back to md/"
	}
	return "copy is old", filesColorDerived, "the next start refreshes it"
}

// filesRowFor turns one entry into one row of the tree in view.
func (a *App) filesRowFor(tree string, e filesEntry) filesFileRow {
	name := path.Base(e.path)
	row := filesFileRow{
		Name: name,
		Path: e.path,
		Kind: filesKindIcon(name, false),
		Size: filesSize(e.bytes()),
	}

	switch tree {
	case filesTreeBundled:
		row.AppOwned = isVersionDependent(filesStoragePath(tree, e.path))
		row.OwnerColor = filesColorApp
		// A starter note is reached as its PAGE, and every other embedded
		// file at its own address. No edit link in this tree: an edit always
		// operates on the copy on the device, and that copy has a row of its
		// own in the Served or the Source tree.
		if md, ok := strings.CutPrefix(e.path, "md/"); ok {
			row.URL = "/" + strings.TrimSuffix(md, ".md") + ".html"
		} else {
			row.URL = "/" + e.path
		}
		return row

	case filesTreeSource:
		row.URL = "/" + strings.TrimSuffix(e.path, ".md") + ".html"
		if !strings.HasSuffix(strings.ToLower(e.path), ".md") {
			row.URL = "/" + e.path
		}
		row.AppOwned = isVersionDependent(filesStoragePath(tree, e.path))
		if a.filesEditable(e.path) {
			row.EditURL = row.URL + "?edit=true"
			if strings.HasSuffix(strings.ToLower(e.path), ".md") {
				// The page address carries the editor to the source, the way
				// the Edit button of the page does.
				row.EditURL = "/" + strings.TrimSuffix(e.path, ".md") + ".html?edit=true"
			}
		}
		if isLocalOnlyPath("md/"+e.path) || strings.HasPrefix(e.path, "local/") {
			row.Extra = append(row.Extra, "local only")
		}

	default: // served
		row.URL = "/" + e.path
		row.AppOwned = isVersionDependent(filesStoragePath(tree, e.path))
		if a.filesEditable(e.path) {
			row.EditURL = row.URL + "?edit=true"
		}
	}

	a.filesState(tree, e, &row)
	return row
}

// filesStoragePath maps a logical path of one tree to the path
// versionDependentAssets uses.
func filesStoragePath(tree, logical string) string {
	switch tree {
	case filesTreeSource:
		return "md/" + logical
	case filesTreeBundled:
		if strings.HasPrefix(logical, "md/") {
			return logical
		}
		return "html/" + logical
	}
	return "html/" + logical
}

// filesEmbeddedPath maps a logical path back into staticFS.
func filesEmbeddedPath(tree, logical string) string {
	if tree == filesTreeSource {
		return "frontend/md/" + logical
	}
	if strings.HasPrefix(logical, "md/") {
		return "frontend/" + logical
	}
	return "frontend/html/" + logical
}

// filesState fills in the word on the first line, its colour, and the
// remaining facts. This is where the two channels of the page are decided.
func (a *App) filesState(tree string, e filesEntry, row *filesFileRow) {
	if e.device != nil {
		row.Mod = e.device.mod.Format("2006-01-02")
		row.ModFull = e.device.mod.Format("2006-01-02 15:04")
	}
	row.OwnerColor = filesColorApp

	switch {
	case e.ships != nil && e.device == nil:
		// It ships, and no request has ever asked for it.
		row.State, row.StateColor = "not extracted", filesColorPlain
		if row.AppOwned {
			row.StateColor = filesColorApp
		}
		row.Size = filesSize(e.ships.size)
		row.Mod, row.ModFull = "", ""

	case e.ships != nil && e.device != nil:
		same, checked := true, true
		if e.ships.size != e.device.size {
			same = false
		} else {
			same, checked = filesSameBytes(filesEmbeddedPath(tree, e.path), a.filesDiskPath(tree, e.path), e.device.size)
		}
		switch {
		case !checked && e.ships.size == e.device.size:
			row.State, row.StateColor = "same size", filesColorPlain
		case same:
			row.State, row.StateColor = "as shipped", filesColorPlain
			if row.AppOwned {
				row.StateColor = filesColorApp
			}
		default:
			row.State, row.StateColor = "changed here", filesColorKeep
			if row.AppOwned {
				row.StateColor = filesColorAlert
				row.OwnerColor = filesColorAlert
			}
			// Both sizes, but only when they READ differently: two files
			// that differ by a line still round to the same "68 KB", and
			// "68 KB → 68 KB" says nothing twice.
			if from, to := filesSize(e.ships.size), filesSize(e.device.size); from != to {
				row.Size = from + " → " + to
			}
		}

	default:
		// Only on the device. Which of three things it is decides the word.
		row.State, row.StateColor = "yours", filesColorKeep
		low := strings.ToLower(e.path)
		switch {
		case tree == filesTreeServed && strings.HasSuffix(low, ".html") &&
			fileExists(filepath.Join(a.StorageDir, "md", filepath.FromSlash(strings.TrimSuffix(e.path, ".html")+".md"))):
			row.State, row.StateColor = "compiled", filesColorDerived
		case tree == filesTreeServed && isSyncedNoteFile(e.path):
			if src := a.filesStat("md", e.path); src != nil {
				row.State, row.StateColor = "copy of md/"+e.path, filesColorDerived
				if word, color, extra := filesMirrorState(src, e.device); word != "" {
					row.State, row.StateColor = word, color
					row.Extra = append(row.Extra, extra)
				}
			}
		case tree == filesTreeSource && isSyncedNoteFile(e.path):
			row.State, row.StateColor = "copied to html/", filesColorDerived
			if copyOf := a.filesStat("html", e.path); copyOf != nil {
				if word, color, extra := filesMirrorState(e.device, copyOf); word != "" {
					row.State, row.StateColor = word, color
					row.Extra = append(row.Extra, extra)
				}
			}
		}
	}
}

// filesDiskPath is the physical path of a logical path in the tree in view.
func (a *App) filesDiskPath(tree, logical string) string {
	sub := "html"
	if tree == filesTreeSource {
		sub = "md"
	}
	return filepath.Join(a.StorageDir, sub, filepath.FromSlash(logical))
}

// filesStat reads one file of the storage tree, for the mirror comparison.
func (a *App) filesStat(sub, logical string) *indexedFile {
	st, err := os.Stat(filepath.Join(a.StorageDir, sub, filepath.FromSlash(logical)))
	if err != nil || st.IsDir() {
		return nil
	}
	return &indexedFile{path: logical, size: st.Size(), mod: st.ModTime()}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// ----------------------------------------------------------------------
// The handler
// ----------------------------------------------------------------------

// serveFilesPage answers GET /OMNGoFiles.html.
//
// Registered as its own exact route rather than as an arm of serveHTMLPage,
// because it is the first PAGE that needs authorization and serveHTMLPage is
// reached through the unauthenticated catch-all. /db_backups is registered the
// same way for the same reason.
//
// The route deliberately does NOT wrap authMiddleware. That middleware answers
// a refusal with one line of plain text, which is right for /api/* and wrong
// for an address a person can link to from their own note - the lesson from
// the search page's 404 in 26.08.2. It asks the same question through hasRole
// and answers it with a page.
func (a *App) serveFilesPage(w http.ResponseWriter, r *http.Request) {
	dir := normalizeFilesDir(r.URL.Query().Get("dir"))
	view := filesPageView{
		Dir:  dir,
		Tree: normalizeFilesTree(r.URL.Query().Get("tree"), dir),
	}

	if !a.hasRole(r, true) {
		view.Denied = true
		a.writeFilesPage(w, view)
		return
	}

	if view.Tree == "" {
		view.Cards = a.filesCards()
		a.writeFilesPage(w, view)
		return
	}

	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	view.ShowingAll = all
	view.Crumbs = filesCrumbs(view.Tree, view.Dir)

	dirs, here, subtreeBytes, subtreeCount := foldToDir(a.treeEntries(view.Tree), view.Dir)
	view.Dirs = dirs
	view.Total = len(here)
	if !all && len(here) > filesDirLimit {
		view.Hidden = len(here) - filesDirLimit
		here = here[:filesDirLimit]
	}
	for _, e := range here {
		view.Files = append(view.Files, a.filesRowFor(view.Tree, e))
	}
	view.Empty = len(view.Dirs) == 0 && len(view.Files) == 0
	view.Summary = filesSummary(subtreeCount, subtreeBytes, view.Dir != "" || len(dirs) > 0, view.Files)
	view.Legend = filesLegend(view.Tree, view.Files)

	a.writeFilesPage(w, view)
}

// filesCards builds the first screen: one button per tree, with the size of
// each. The three walks run once, here, and nothing else on this screen needs
// them.
func (a *App) filesCards() []filesTreeCard {
	count := func(entries []filesEntry) (int, int64) {
		var b int64
		for _, e := range entries {
			b += e.bytes()
		}
		return len(entries), b
	}
	nb, bb := count(a.treeEntries(filesTreeBundled))
	ns, bs := count(a.treeEntries(filesTreeServed))
	nm, bm := count(a.treeEntries(filesTreeSource))
	return []filesTreeCard{
		{Key: filesTreeBundled, Icon: "inventory_2", Title: "Bundled",
			Where: "inside the application",
			Count: filesCountLabel(nb) + " · " + filesSize(bb)},
		{Key: filesTreeServed, Icon: "public", Title: "Served", Class: "files-card-served",
			Where: "html/ — what a URL finds",
			Count: filesCountLabel(ns) + " · " + filesSize(bs)},
		{Key: filesTreeSource, Icon: "article", Title: "Source", Class: "files-card-source",
			Where: "md/ — your notes",
			Count: filesCountLabel(nm) + " · " + filesSize(bm)},
	}
}

// filesSummary is the one line under the crumb.
//
// The count and the size are RECURSIVE, so they answer "how large is this
// whole folder". The state counts are for the rows of THIS directory, because
// those are the rows a reader can act on. The wording keeps the two apart.
func filesSummary(count int, bytes int64, below bool, rows []filesFileRow) string {
	out := filesCountLabel(count) + " · " + filesSize(bytes)
	if below {
		out += " below"
	}
	var changed, absent int
	for _, r := range rows {
		switch r.State {
		case "changed here":
			changed++
		case "not extracted":
			absent++
		}
	}
	if changed > 0 {
		out += fmt.Sprintf(" · %d changed here", changed)
	}
	if absent > 0 {
		out += fmt.Sprintf(" · %d not extracted", absent)
	}
	return out
}

// filesLegend explains the colours that this directory actually uses. A key
// that lists every possible state is noise on a phone.
func filesLegend(tree string, rows []filesFileRow) []filesLegendItem {
	seen := map[string]bool{}
	for _, r := range rows {
		if r.State != "" {
			seen[r.StateColor] = true
		}
		if r.AppOwned {
			seen[r.OwnerColor] = true
		}
	}
	all := []filesLegendItem{
		{Color: filesColorApp, Word: "app-owned",
			Text: "the next version of OMN-Go replaces this file"},
		{Color: filesColorAlert, Word: "changed here",
			Text: "… and you changed it, so OMN-Go backs up your copy and stops using it"},
		{Color: filesColorKeep, Word: "yours",
			Text: "OMN-Go keeps this file, always"},
		{Color: filesColorDerived, Word: "compiled",
			Text: "OMN-Go makes this file again when it needs to"},
		{Color: filesColorPlain, Word: "as shipped",
			Text: "nothing at stake"},
	}
	var out []filesLegendItem
	for _, item := range all {
		if seen[item.Color] {
			out = append(out, item)
		}
	}
	return out
}

func (a *App) writeFilesPage(w http.ResponseWriter, view filesPageView) {
	title := "Files"
	switch view.Tree {
	case filesTreeBundled:
		title = "Bundled files"
	case filesTreeServed:
		title = "Served files"
	case filesTreeSource:
		title = "Note source"
	}
	if view.Dir != "" {
		title += ": " + strings.TrimSuffix(view.Dir, "/")
	}
	body := renderFilesPage(view)
	compiled := a.compilePageWithBody(title,
		[]byte("Title: "+title+"\nCategory: System\n\n"), body)
	w.Header().Set("Content-Type", "text/html")
	w.Write(a.injectRuntimeVars(compiled))
}

// filesCrumbs builds the breadcrumb, the tree root first, the current
// directory last.
//
// Each crumb carries its own trailing slash and NOTHING separates two crumbs.
// Until 26.08.53 the labels carried a slash and a separator span was written
// between them as well, so the crumb of html/js/ read "html/ / js/".
func filesCrumbs(tree, dir string) []filesCrumb {
	root := "bundled/"
	switch tree {
	case filesTreeServed:
		root = "html/"
	case filesTreeSource:
		root = "md/"
	}
	out := []filesCrumb{{Label: root, Dir: ""}}
	if dir == "" {
		return out
	}
	acc := ""
	for _, part := range strings.Split(strings.TrimSuffix(dir, "/"), "/") {
		acc += part + "/"
		out = append(out, filesCrumb{Label: part + "/", Dir: acc})
	}
	return out
}

// filesSize renders a byte count the way a file listing should: short, and
// never wider than it needs to be.
func filesSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func filesCountLabel(n int) string {
	if n == 1 {
		return "1 file"
	}
	return itoa(n) + " files"
}
