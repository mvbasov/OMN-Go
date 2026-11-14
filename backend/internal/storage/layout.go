package storage

import (
	"path/filepath"
	"strings"
)

// Layout is the storage directory. Its methods name each place in
// it, thus no other code joins a directory name to StorageDir.
//
//	md/             The notes as Markdown.
//	html/           Each file that the server sends.
//	db/             The SQLite files.
//	asset_backups/  The old copies of the application files.
//	.git/           The repository of the sync.
//
// The files at the top, for example config.json, use File.
type Layout string

// ConfigFilename is a file at the top of the storage directory.
const ConfigFilename = "config.json"

// under joins the parts. Up to two elements need no new slice, and a note
// path has one. BenchmarkSearchPage counts the allocations.
func (l Layout) under(dir string, elem []string) string {
	switch len(elem) {
	case 0:
		return filepath.Join(string(l), dir)
	case 1:
		return filepath.Join(string(l), dir, elem[0])
	case 2:
		return filepath.Join(string(l), dir, elem[0], elem[1])
	}
	return filepath.Join(append([]string{string(l), dir}, elem...)...)
}

func (l Layout) MD(elem ...string) string           { return l.under("md", elem) }
func (l Layout) HTML(elem ...string) string         { return l.under("html", elem) }
func (l Layout) DB(elem ...string) string           { return l.under("db", elem) }
func (l Layout) Git(elem ...string) string          { return l.under(".git", elem) }
func (l Layout) AssetBackups(elem ...string) string { return l.under("asset_backups", elem) }

// File answers a path below the top of the storage directory.
func (l Layout) File(elem ...string) string { return l.under("", elem) }

func (l Layout) Config() string { return l.File(ConfigFilename) }

// Contains tells if p is in the storage directory.
func (l Layout) Contains(p string) bool {
	_, ok := RelInside(string(l), p)
	return ok
}

// RelInside answers the path of p below root. It answers false for a path
// outside root, for example after a "../" in a name. It is the one
// containment test of the backend.
func RelInside(root, p string) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil || rel == ".." || filepath.IsAbs(rel) ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
