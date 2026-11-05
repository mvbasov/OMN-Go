package files

// ----------------------------------------------------------------------
// The file index: /OMNGoFiles.html
// ----------------------------------------------------------------------
//
// Three trees, one directory at a time:
//
//	Bundled  What this build carries: frontend.Static. The templates are in a
//	         separate embed. TestFilesPage_NeverListsTemplates holds that.
//	Served   What a URL finds: StorageDir/html, without db_backup/.
//	Source   What the person wrote: StorageDir/md.
//
// Each NAME has one row in its tree. SILENCE IS THE NORMAL CASE: a row speaks
// ONLY when the app is involved. The word says what the file IS, and the
// color says what HAPPENS to it. The table in filesLegend lists each word and
// each color. The color never works alone.
//
// NOTHING HERE MAY WRITE. Never call materializeAsset, because a listing
// would then extract each embedded file. TestFilesPage_WritesNothing holds
// the rule.

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/render"
)

// filesDirLimit limits the FILES that one directory shows. It never hides a
// directory row, because a hidden directory hides a whole branch.
const filesDirLimit = 200

// The listing leaves filesExcludedDir out at each level. It holds the
// database backups. A URL can still fetch them, thus this is a listing
// decision and not a security control.
const filesExcludedDir = "db_backup"

// filesCompareMax limits the byte comparison of two files with the SAME size,
// because a fixed typo often keeps the length. Above this limit, the row says
// "same size".
const filesCompareMax = 2 << 20

// These are the three trees. The key is the value of ?tree= and the text of
// the crumb.
const (
	filesTreeBundled = "bundled"
	filesTreeServed  = "served"
	filesTreeSource  = "source"
)

// indexedFile is one file in one tree, keyed by its LOGICAL path: relative to
// the root of the tree, with slashes. In the Bundled tree,
// frontend/html/js/x.js is "js/x.js", and frontend/md/Note.md is
// "md/Note.md".
type indexedFile struct {
	path string
	size int64
	mod  time.Time // zero for embedded files; embed.FS has no mtime
}

// filesEntry is one NAME in one tree, with its two sides. In the Bundled
// tree, device is always nil. In the Served and Source trees, ships is the
// embedded file, and device is the file in StorageDir.
type filesEntry struct {
	path   string
	ships  *indexedFile
	device *indexedFile
}

func embeddedFiles() []indexedFile {
	var out []indexedFile
	for _, root := range []struct{ dir, prefix string }{
		{"html", ""},
		{"md", "md/"},
	} {
		fs.WalkDir(frontend.Static, root.dir, func(p string, d fs.DirEntry, err error) error {
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

// walkStorage lists one tree of the storage directory. sub is "html" or "md".
func (svc Service) walkStorage(sub string) []indexedFile {
	base := svc.Layout.File(sub)
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

// treeEntries makes the entry list of one tree, with the two sides paired by
// name. Each row below reads the result.
func (svc Service) treeEntries(tree string) []filesEntry {
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
		for _, f := range svc.walkStorage("md") {
			add(f.path, f, false)
		}
	default: // served
		for _, f := range embedded {
			if strings.HasPrefix(f.path, "md/") {
				continue // a note is not served from html/
			}
			add(f.path, f, true)
		}
		for _, f := range svc.walkStorage("html") {
			add(f.path, f, false)
		}
	}

	out := make([]filesEntry, 0, len(byPath))
	for _, e := range byPath {
		out = append(out, *e)
	}
	return out
}

// normalizeFilesDir changes ?dir= into a logical directory prefix, "" or a
// string that ends with "/". The code uses it ONLY as a string prefix, and
// never joins it into a file system path.
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
	// path.Clean keeps a leading "..", thus check after Clean.
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	if clean == filesExcludedDir || strings.HasPrefix(clean, filesExcludedDir+"/") {
		return ""
	}
	return clean + "/"
}

// normalizeFilesTree accepts only the three known values. A link with a dir
// and no tree is from an older page that showed only html/, thus it goes to
// the Served tree.
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

// foldToDir splits one tree at dir into the directories below it and the
// entries in it. The directory totals are RECURSIVE, and a name counts one
// time.
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

// bytes answers the size to show for one name: the copy on the device when it
// exists, because that copy uses the storage.
func (e filesEntry) bytes() int64 {
	if e.device != nil {
		return e.device.size
	}
	if e.ships != nil {
		return e.ships.size
	}
	return 0
}

// ServePage answers GET /OMNGoFiles.html. The page-access table in
// page_access.go refuses a caller without the admin role.
func (svc Service) ServePage(w http.ResponseWriter, r *http.Request) {
	dir := normalizeFilesDir(r.URL.Query().Get("dir"))
	view := filesPageView{
		Dir:  dir,
		Tree: normalizeFilesTree(r.URL.Query().Get("tree"), dir),
	}

	if view.Tree == "" {
		view.Cards = svc.filesCards()
		svc.writeFilesPage(w, view)
		return
	}

	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	view.ShowingAll = all
	view.Crumbs = filesCrumbs(view.Tree, view.Dir)

	dirs, here, subtreeBytes, subtreeCount := foldToDir(svc.treeEntries(view.Tree), view.Dir)
	view.Dirs = dirs
	view.Total = len(here)
	if !all && len(here) > filesDirLimit {
		view.Hidden = len(here) - filesDirLimit
		here = here[:filesDirLimit]
	}
	for _, e := range here {
		view.Files = append(view.Files, svc.filesRowFor(view.Tree, e))
	}
	view.Empty = len(view.Dirs) == 0 && len(view.Files) == 0
	view.Summary = filesSummary(subtreeCount, subtreeBytes, view.Dir != "" || len(dirs) > 0, view.Files)
	view.Legend = filesLegend(view.Tree, view.Files, view.Dirs)

	svc.writeFilesPage(w, view)
}

// filesCards makes the first screen: one button for each tree, with its size.
func (svc Service) filesCards() []filesTreeCard {
	count := func(entries []filesEntry) (int, int64) {
		var b int64
		for _, e := range entries {
			b += e.bytes()
		}
		return len(entries), b
	}
	nb, bb := count(svc.treeEntries(filesTreeBundled))
	ns, bs := count(svc.treeEntries(filesTreeServed))
	nm, bm := count(svc.treeEntries(filesTreeSource))
	return []filesTreeCard{
		{Key: filesTreeBundled, Icon: "inventory_2", Title: "Bundled",
			Where: "inside the application",
			Count: filesCountLabel(nb) + " · " + filesSize(bb)},
		{Key: filesTreeServed, Icon: "public", Title: "Served", Class: "omn-files-card-served",
			Where: "html/ — what a URL finds",
			Count: filesCountLabel(ns) + " · " + filesSize(bs)},
		{Key: filesTreeSource, Icon: "article", Title: "Source", Class: "omn-files-card-source",
			Where: "md/ — your notes",
			Count: filesCountLabel(nm) + " · " + filesSize(bm)},
	}
}

// filesSummary is the one line under the crumb. The count and the size are
// RECURSIVE: "how large is this whole folder". The state counts are for the
// rows of THIS directory, because a reader can act on those rows.
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

// filesLegend explains the words that THIS page uses. The key is the pair of
// word and color, because "changed here" is green on a kept file and red on a
// replaced one.
func filesLegend(tree string, rows []filesFileRow, dirs []filesDirRow) []filesLegendItem {
	type key struct{ word, color string }
	seen := map[key]bool{}
	for _, r := range rows {
		if r.State != "" {
			seen[key{r.State, r.StateColor}] = true
		}
		if r.AppOwned {
			seen[key{"app-owned", r.OwnerColor}] = true
		}
		for _, x := range r.Extra {
			seen[key{x, filesColorPlain}] = true
		}
	}
	for _, d := range dirs {
		if w, c := filesDirNote(tree, d); w != "" {
			seen[key{w, c}] = true
		}
	}

	// Give one line for each pair, in a fixed order. A pair without a line
	// here gets none.
	all := []filesLegendItem{
		{Color: filesColorAlert, Word: "changed here",
			Text: "you changed it, and the next version of OMN-Go replaces it. OMN-Go backs up your copy first"},
		{Color: filesColorKeep, Word: "changed here",
			Text: "you changed a file that came with OMN-Go. OMN-Go keeps your copy"},
		{Color: filesColorApp, Word: "app-owned",
			Text: "the next version of OMN-Go replaces this file"},
		// This is the same word in the alert color. filesState makes BOTH
		// words of such a row red, because both describe one outcome.
		{Color: filesColorAlert, Word: "app-owned",
			Text: "the next version of OMN-Go replaces this file, and your change goes to a backup"},
		{Color: filesColorApp, Word: "not extracted",
			Text: "OMN-Go carries this file, and this device has no copy of it yet"},
		{Color: filesColorPlain, Word: "not extracted",
			Text: "OMN-Go carries this file, and this device has no copy of it yet"},
		{Color: filesColorPlain, Word: "same size",
			Text: "the file is too large to compare, and the two copies have the same size"},
		{Color: filesColorAlert, Word: "edited outside",
			Text: "an editor outside OMN-Go wrote the copy in html/. Save the file one time in the editor to copy it back to md/"},
		{Color: filesColorDerived, Word: "waits for restart",
			Text: "the file in md/ is newer. The next start of OMN-Go copies it into html/"},
		{Color: filesColorPlain, Word: "local only",
			Text: "git synchronization does not carry this file. It stays on this device"},
	}
	var out []filesLegendItem
	for _, item := range all {
		k := key{item.Word, item.Color}
		if seen[k] {
			out = append(out, item)
			delete(seen, k)
		}
	}
	// A directory line holds a count, thus it cannot be in the table above.
	// One line covers each of them.
	for k := range seen {
		if strings.Contains(k.word, filesFromTheApp) {
			out = append(out, filesLegendItem{Color: k.color, Word: "… " + filesFromTheApp,
				Text: "OMN-Go delivered that many of the files below this directory"})
			break
		}
	}
	return out
}

func (svc Service) writeFilesPage(w http.ResponseWriter, view filesPageView) {
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
	svc.RenderPage(w, http.StatusOK, title, render.PageHeader(title, "System", "css/OMN-Go/omn-go-files.css"), body)
}

// filesCrumbs makes the breadcrumb, from the root of the tree to the current
// directory. Each crumb carries its own trailing slash, and NOTHING separates
// two crumbs. A separator would show "html/ / js/".
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

// filesSize shows a byte count in a short form.
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
	return strconv.Itoa(n) + " files"
}
