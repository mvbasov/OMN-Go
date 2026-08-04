package backend

// ----------------------------------------------------------------------
// The file index: /OMNGoFiles.html
// ----------------------------------------------------------------------
//
// Two trees, one directory at a time, the way a browser shows file:///.
//
//   - what the application SHIPS - staticFS, which embeds frontend/html and
//     frontend/md. Templates are absent by construction, not by a filter:
//     they live in a separate embed (see the comment on templatesFS), and
//     that separation exists partly so listings like this one do not have to
//     exclude them by hand. A test pins it anyway, because "by construction"
//     stops being true the moment someone merges the two embeds.
//
//   - what is on THIS DEVICE - StorageDir/html, minus db_backup/.
//
// The pair is the point. An html/ asset reaches disk lazily (materializeAsset)
// and, once there, is either yours forever or replaced on the next version
// change depending on whether it is in versionDependentAssets. Nothing in the
// app says which, and the answer decides whether editing a file is worth doing.
//
// NOTHING HERE MAY WRITE. In particular it must never call materializeAsset:
// a listing that extracted all 42 embedded files as a side effect of
// describing them would defeat lazy extraction entirely, and materializeAsset
// is exactly the function this code would otherwise reach for to resolve a
// path.

import (
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

// indexedFile is one file in one of the two trees, keyed by its LOGICAL path:
// slash-separated, relative to that tree's root, no leading slash.
//
// For the embedded tree the logical path folds two embed roots into one
// namespace - frontend/html/js/x.js becomes "js/x.js" and frontend/md/Note.md
// becomes "md/Note.md" - so "md" reads as an ordinary subdirectory. They
// cannot collide: md/ is a sibling of html/ in storage, never inside it.
type indexedFile struct {
	path string
	size int64
	mod  time.Time // zero for embedded files; embed.FS has no mtime
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

func (a *App) diskFiles() []indexedFile {
	base := filepath.Join(a.StorageDir, "html")
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
			if rel == filesExcludedDir {
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

// foldToDir splits one tree at dir into the subdirectories directly below it
// and the files directly in it.
//
// Directory totals are RECURSIVE - everything at or below that subdirectory -
// because "this subtree is 4 MB" is the question a file index is opened to
// answer, and the walk has collected it anyway.
func foldToDir(files []indexedFile, dir string) (dirs []filesDirRow, here []indexedFile, bytes int64, count int) {
	byDir := map[string]*filesDirRow{}
	for _, f := range files {
		if dir != "" && !strings.HasPrefix(f.path, dir) {
			continue
		}
		rest := f.path[len(dir):]
		if rest == "" {
			continue
		}
		bytes += f.size
		count++

		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name := rest[:i]
			row := byDir[name]
			if row == nil {
				row = &filesDirRow{Name: name, Dir: dir + name + "/"}
				byDir[name] = row
			}
			row.Files++
			row.Bytes += f.size
			continue
		}
		here = append(here, f)
	}

	for _, row := range byDir {
		dirs = append(dirs, *row)
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(here, func(i, j int) bool { return here[i].path < here[j].path })
	return dirs, here, bytes, count
}

// ----------------------------------------------------------------------
// Row construction
// ----------------------------------------------------------------------

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

// embeddedRow builds one row of the "shipped" section.
func (a *App) embeddedRow(f indexedFile) filesFileRow {
	row := filesFileRow{
		Name:     path.Base(f.path),
		Path:     f.path,
		Size:     f.size,
		AppOwned: isVersionDependent(f.path),
	}

	if md, ok := strings.CutPrefix(f.path, "md/"); ok {
		// A starter note is reached as its PAGE. No edit link, for the same
		// reason a compiled page has none: the page carries its own.
		name := strings.TrimSuffix(md, ".md")
		row.URL = "/" + name + ".html"
		row.OnDisk = fileExists(filepath.Join(a.StorageDir, "md", filepath.FromSlash(md)))
		return row
	}

	row.URL = "/" + f.path
	row.OnDisk = fileExists(filepath.Join(a.StorageDir, "html", filepath.FromSlash(f.path)))
	if a.filesEditable(f.path) {
		row.EditURL = row.URL + "?edit=true"
	}
	return row
}

// diskRow builds one row of the "on this device" section.
func (a *App) diskRow(f indexedFile) filesFileRow {
	row := filesFileRow{
		Name:    path.Base(f.path),
		Path:    f.path,
		Size:    f.size,
		Mod:     f.mod,
		URL:     "/" + f.path,
		OnDisk:  true,
		IsLocal: true,
	}
	if a.filesEditable(f.path) {
		row.EditURL = row.URL + "?edit=true"
	}
	return row
}

// isVersionDependent reports whether an embedded html/ path is one the app
// owns - replaced on the next version change, the user's copy backed up first.
// The list is read, never extended, from assets.go.
func isVersionDependent(logical string) bool {
	want := "html/" + logical
	if strings.HasPrefix(logical, "md/") {
		want = logical
	}
	for _, v := range versionDependentAssets {
		if v == want {
			return true
		}
	}
	return false
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
	view := filesPageView{Dir: normalizeFilesDir(r.URL.Query().Get("dir"))}

	if !a.hasRole(r, true) {
		view.Denied = true
		a.writeFilesPage(w, view)
		return
	}

	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	view.ShowingAll = all
	view.Crumbs = filesCrumbs(view.Dir)

	embedded := embeddedFiles()
	disk := a.diskFiles()

	view.Embedded = a.buildSection("Embedded in the application", embedded, view.Dir, all, a.embeddedRow)
	view.Disk = a.buildSection("On this device", disk, view.Dir, all, a.diskRow)

	a.writeFilesPage(w, view)
}

func (a *App) buildSection(title string, files []indexedFile, dir string, all bool,
	row func(indexedFile) filesFileRow) filesSection {

	dirs, here, bytes, count := foldToDir(files, dir)
	sec := filesSection{
		Title: title,
		Dirs:  dirs,
		Bytes: bytes,
		Count: count,
		Total: len(here),
	}
	if !all && len(here) > filesDirLimit {
		sec.Hidden = len(here) - filesDirLimit
		here = here[:filesDirLimit]
	}
	for _, f := range here {
		sec.Files = append(sec.Files, row(f))
	}
	sec.Empty = len(sec.Dirs) == 0 && len(sec.Files) == 0
	return sec
}

func (a *App) writeFilesPage(w http.ResponseWriter, view filesPageView) {
	title := "Files"
	if view.Dir != "" {
		title = "Files: " + strings.TrimSuffix(view.Dir, "/")
	}
	body := renderFilesPage(view)
	compiled := a.compilePageWithBody(title,
		[]byte("Title: "+title+"\nCategory: System\n\n"), body)
	w.Header().Set("Content-Type", "text/html")
	w.Write(a.injectRuntimeVars(compiled))
}

// filesCrumbs builds the breadcrumb, root first, the current directory last.
func filesCrumbs(dir string) []filesCrumb {
	out := []filesCrumb{{Label: "html/", Dir: ""}}
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
