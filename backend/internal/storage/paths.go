package storage

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"net.basov.omngo/backend/internal/config"
)

// ContainedName makes a name from a request safe to join under a storage
// directory. It answers a clean, relative name with slashes. A ".." cannot
// climb above the root of the name: "../../x" gives "x", and "a/../../b"
// gives "b". The function removes a leading slash. On Windows, a backslash
// counts as a separator.
//
// filepath.Join resolves a ".." in a name, and the result can leave the
// storage directory. /api/save and /api/note would then write and read files
// outside it. Each path that a request names must pass through this function.
// TestContainedName holds the rule.
func ContainedName(name string) string {
	return strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(name)), "/")
}

// ResolvePageName is the one place that answers two questions about a name
// from a user or a URL. Is it a markdown page? Where are its .md source and
// its .html file? It accepts three forms of the same page:
//
//   - a page name, for example "Welcome" or "Report.2026"
//   - a markdown file name, for example "Welcome.md"
//   - a compiled file name, for example "Welcome.html"
//
// It answers mdPath and htmlPath together, thus no caller builds a path a
// second time. A name that ends in a known file extension, for example ".js"
// or ".txt", is a file under html/. isPage is then false, and mdPath is
// empty.
//
// config.HasKnownAssetExtension in internal/config/content_types.go is the one
// authority for that test: the LAST extension decides, and an unknown extension
// is a page. Keep the decision here, and do not copy it.
func ResolvePageName(l Layout, mimeTypes map[string]string, name string) (mdPath, htmlPath, baseName string, isPage bool) {
	switch {
	case strings.HasSuffix(name, ".md"):
		baseName = strings.TrimSuffix(name, ".md")
		isPage = true
	case strings.HasSuffix(name, ".html"):
		baseName = strings.TrimSuffix(name, ".html")
		isPage = true
	case !config.HasKnownAssetExtension(mimeTypes, name):
		baseName = name
		isPage = true
	default:
		// The name ends in an extension that this install serves as a file.
		// See config.HasKnownAssetExtension.
		name = ContainedName(name)
		return "", l.HTML(filepath.FromSlash(name)), name, false
	}

	// Layout.PageHTML is the one formula for the path of a compiled page.
	baseName = ContainedName(baseName)
	mdPath = l.MD(filepath.FromSlash(baseName + ".md"))
	htmlPath = l.PageHTML(baseName)
	return mdPath, htmlPath, baseName, true
}

// FileExists reports whether p is a file that can be read. A directory
// answers false.
func FileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// PageHTML is the one formula for the compiled HTML path of a page.
// ResolvePageName answers the same path for a page, and TestPageHTMLPath
// compares the two.
func (l Layout) PageHTML(name string) string {
	return l.HTML(filepath.FromSlash(ContainedName(name) + ".html"))
}

// DropCompiledPages removes each compiled page: each html/<name>.html that
// has the source md/<name>.md. An .html file with no source is a file of the
// user, and it stays. A new version calls it, because a compiled page holds
// the page shell of the old templates. See
// doc/decisions/0026-compile-each-page-again-after-a-new-version.md. The
// function answers the count of removed pages.
func DropCompiledPages(l Layout) (int, error) {
	root := l.HTML()
	removed := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".html") {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		if !FileExists(l.MD(strings.TrimSuffix(rel, ".html") + ".md")) {
			return nil
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		removed++
		return nil
	})
	if os.IsNotExist(err) {
		err = nil
	}
	return removed, err
}

// The local-only name rule: a file or a directory with a name that starts
// with "local-" stays on this device.
//
//	html/user_json/local-data.json     a file name
//	md/local-drafts/Monday.md          a directory name
//	html/db_backup/local-counters/...  a database backup
//
// The match is on a whole path segment, and it is case-sensitive, thus
// "mylocal-data.json" is a normal file. A commit does not take a local-only
// file. A force pull keeps it, because gitsync.Service.CleanUntrackedFiles in
// backend/internal/gitsync/sync.go keeps an ignored file.
const LocalOnlyPrefix = "local-"

// IsLocalOnlyPath tells if the name of the file, or of a directory above it,
// starts with "local-". It is the rule for the index.
// gitsync.GitignoreLocalOnlyPattern in backend/internal/gitsync/repo.go is the
// rule for a new file. TestGitignoreMatchesEachLocalOnlyPath compares the two.
func IsLocalOnlyPath(name string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(name), "/") {
		if strings.HasPrefix(segment, LocalOnlyPrefix) {
			return true
		}
	}
	return false
}
