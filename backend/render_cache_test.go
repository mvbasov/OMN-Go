package backend

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderAndCacheWritesCompiledHTML pins the single cache pipeline
// introduced in Phase 2: renderAndCache compiles a page and writes it to
// html/<name>.html, and the on-disk bytes equal both compilePage's output
// and the bytes it returns.
func TestRenderAndCacheWritesCompiledHTML(t *testing.T) {
	a := newTestApp(t)
	content := []byte("Title: Doc\n\nHello **bold**")

	compiled, err := a.renderAndCache("Doc", content)
	if err != nil {
		t.Fatalf("renderAndCache: %v", err)
	}

	// Returned bytes must equal a direct compilePage of the same input.
	if want := a.compilePage("Doc", content); !bytes.Equal(compiled, want) {
		t.Error("returned bytes differ from compilePage output")
	}

	// The on-disk cache must equal the returned bytes exactly.
	onDisk, err := os.ReadFile(filepath.Join(a.StorageDir, "html", "Doc.html"))
	if err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
	if !bytes.Equal(onDisk, compiled) {
		t.Error("on-disk cache differs from returned bytes")
	}

	// Sanity: it is a real compiled page, and it carries the raw runtime
	// marker (the cache is intentionally an incomplete template - see the
	// contract in render_cache.go).
	s := string(onDisk)
	if !strings.Contains(s, "<strong>bold</strong>") {
		t.Error("cache missing rendered markdown body")
	}
	if !strings.Contains(s, `<meta id="omn-go-runtime-vars-marker">`) {
		t.Error("cache missing the runtime-vars marker")
	}
}

// TestRenderAndCacheCreatesNestedDirs pins that renderAndCache materializes
// the parent directory tree for a nested page name (e.g. "local/deep/Note")
// rather than failing when html/local/deep does not exist yet.
func TestRenderAndCacheCreatesNestedDirs(t *testing.T) {
	a := newTestApp(t)

	if _, err := a.renderAndCache("local/deep/Note", []byte("Title: N\n\nx")); err != nil {
		t.Fatalf("renderAndCache nested: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "local", "deep", "Note.html")); err != nil {
		t.Fatalf("nested cache file not created: %v", err)
	}
}

// TestPageHTMLPath guards that the single path formula (pageHTMLPath) and
// resolvePageName agree - the whole reason resolvePageName now delegates to
// it. If a future edit reintroduces a second formula, this fails.
func TestPageHTMLPath(t *testing.T) {
	a := &App{StorageDir: "/store"}

	for _, name := range []string{"Note", "dir/Note", "a/b/c"} {
		want := filepath.Join("/store", "html", filepath.Clean(name+".html"))
		if got := a.pageHTMLPath(name); got != want {
			t.Errorf("pageHTMLPath(%q) = %q, want %q", name, got, want)
		}
		_, htmlPath, _, isPage := a.resolvePageName(name)
		if !isPage {
			t.Fatalf("resolvePageName(%q) unexpectedly not a page", name)
		}
		if got := a.pageHTMLPath(name); got != htmlPath {
			t.Errorf("pageHTMLPath(%q)=%q disagrees with resolvePageName htmlPath %q", name, got, htmlPath)
		}
	}
}
