package app

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fileGroups puts each production file in one group. A later split makes
// each group a package, and the compiler then holds the layers.
var fileGroups = map[string]string{
	"log_app.go": "logx",

	"config_app.go": "config", "app_version.go": "config",

	"render_app.go":  "render",
	"storage_app.go": "storage", "start_stage.go": "storage",

	"files_app.go":    "files",
	"exchange_app.go": "exchange",
	"status_app.go":   "status",
	"search_app.go":   "search",
	"gitsync_app.go":  "gitsync",
	"db_app.go":       "db",

	"server.go": "app", "middleware.go": "app", "session.go": "app",
	"request_guard.go": "app", "handlers.go": "app", "page_access.go": "app",
	"log_handlers.go":  "app",
	"note_handlers.go": "app", "config_handlers.go": "app",
	"config_page.go": "app", "upload_handlers.go": "app",
	"serving.go": "app", "routes.go": "app", "storage.go": "app",
}

// groupLayers gives the layer of each group. A group can use a group of a
// lower layer only. Two groups of the same layer cannot use each other. Thus
// the groups have no cycle.
//
// The render cache writes html/ through the storage layout, thus render is
// above storage. The Status page reports on each feature, thus status is
// above the features.
var groupLayers = map[string]int{
	"logx":    0,
	"config":  1,
	"storage": 2,
	"render":  3,
	"files":   4, "exchange": 4, "search": 4, "gitsync": 4, "db": 4,
	"status": 5,
	"app":    6,
}

// groupLinkUses answers each use of a package-level name in another file, as
// "file uses name", with the file of the name. It type-checks the production
// files. A package outside the standard library becomes an empty package.
// The check then reports errors for it, and the scan ignores them. A name of
// this package still resolves.
func groupLinkUses(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: stdlibOnly{importer.Default()}, Error: func(error) {}}
	pkg, _ := conf.Check("net.basov.omngo/backend", fset, files, info)

	// The App struct holds the state of each group until Phase 3. A use of
	// App or of one of its fields is thus no link.
	app := pkg.Scope().Lookup("App")
	uses := map[string]string{}
	for id, obj := range info.Uses {
		if obj == nil || obj.Pkg() != pkg || obj == app {
			continue
		}
		if v, ok := obj.(*types.Var); ok && v.IsField() {
			continue
		}
		if _, ok := obj.(*types.Func); !ok && obj.Parent() != pkg.Scope() {
			continue
		}
		from := fset.Position(id.Pos()).Filename
		to := fset.Position(obj.Pos()).Filename
		if from != to {
			uses[filepath.Base(from)+" uses "+obj.Name()] = filepath.Base(to)
		}
	}
	return uses
}

// stdlibOnly imports each package of the standard library, and an empty
// package for each other path. The scan thus needs no module download.
type stdlibOnly struct{ std types.Importer }

func (s stdlibOnly) Import(path string) (*types.Package, error) {
	if !strings.Contains(strings.Split(path, "/")[0], ".") {
		return s.std.Import(path)
	}
	p := types.NewPackage(path, filepath.Base(path))
	p.MarkComplete()
	return p, nil
}

func TestEachFileHasAGroup(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") {
			continue
		}
		g, ok := fileGroups[n]
		if !ok {
			t.Errorf("%s has no group. Add it to fileGroups.", n)
			continue
		}
		if _, ok := groupLayers[g]; !ok {
			t.Errorf("the group %q of %s has no layer in groupLayers", g, n)
		}
	}
	for n := range fileGroups {
		if _, err := os.Stat(n); err != nil {
			t.Errorf("fileGroups names %s, and the file does not exist", n)
		}
	}
}

// A group uses only groups of a lower layer. The test allows no exception.
func TestGroupsUseOnlyLowerLayers(t *testing.T) {
	uses := groupLinkUses(t)
	if uses["config_handlers.go uses renderPage"] != "render_app.go" {
		t.Fatal("the scan did not find a use of renderPage. The scan is broken.")
	}
	for use, to := range uses {
		from, _, _ := strings.Cut(use, " uses ")
		gFrom, gTo := fileGroups[from], fileGroups[to]
		if gFrom == gTo || groupLayers[gTo] < groupLayers[gFrom] {
			continue
		}
		t.Errorf("%s of %s. The group %s (layer %d) cannot use the group %s (layer %d). "+
			"Move the name to a lower group, or connect the two groups with a hook "+
			"in connectGroups.", use, to, gFrom, groupLayers[gFrom], gTo, groupLayers[gTo])
	}
}
