package backend

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestApp returns an App rooted in a fresh temp dir with the md/ and
// html/ layout initStorage would create, without running initStorage (no
// embedded-file extraction, no config file - tests control all inputs).
func newTestApp(t *testing.T) *App {
	t.Helper()
	a := &App{StorageDir: t.TempDir()}
	for _, d := range []string{"md", "html"} {
		if err := os.MkdirAll(filepath.Join(a.StorageDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

func TestResolveNewPageTarget(t *testing.T) {
	a := &App{}
	tests := []struct {
		source, target, want string
	}{
		// bare target is a sibling of the source page
		{"local/notes", "test", "local/test"},
		{"a/b/c", "d", "a/b/d"},
		// root-level source: bare target stays root-level
		{"Welcome", "test", "test"},
		{"", "test", "test"},
		// absolute target: rooted, leading slash stripped
		{"local/notes", "/Top", "Top"},
		{"local/notes", "/x/y", "x/y"},
		// explicit relative path with a slash: taken as-is (cleaned)
		{"local/notes", "sub/page", "sub/page"},
		{"local/notes", "./sub/page", "sub/page"},
		// whitespace trimmed; empty stays empty
		{"a/b", "  c  ", "a/c"},
		{"a/b", "", ""},
	}
	for _, tt := range tests {
		if got := a.resolveNewPageTarget(tt.source, tt.target); got != tt.want {
			t.Errorf("resolveNewPageTarget(%q, %q) = %q, want %q", tt.source, tt.target, got, tt.want)
		}
	}
}

func TestHandleGetNoteExisting(t *testing.T) {
	a := newTestApp(t)
	content := "Title: T\n\nHello"
	if err := os.WriteFile(filepath.Join(a.StorageDir, "md", "T.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"T", "T.md", "T.html"} {
		req := httptest.NewRequest(http.MethodGet, "/api/note?name="+url.QueryEscape(name), nil)
		rec := httptest.NewRecorder()
		a.handleGetNote(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("name=%q: status %d", name, rec.Code)
		}
		if rec.Body.String() != content {
			t.Errorf("name=%q: body %q, want %q", name, rec.Body.String(), content)
		}
	}
}

func TestHandleGetNoteMissingSynthesizesAndPersists(t *testing.T) {
	a := newTestApp(t)
	a.Config.Author = "Tester"

	req := httptest.NewRequest(http.MethodGet, "/api/note?name=Fresh", nil)
	rec := httptest.NewRecorder()
	a.handleGetNote(rec, req)

	body := rec.Body.String()
	for _, want := range []string{"Title: Fresh", "Author: Tester"} {
		if !strings.Contains(body, want) {
			t.Errorf("synthesized note missing %q:\n%s", want, body)
		}
	}
	// The fallback must have been persisted so it only runs once.
	onDisk, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "Fresh.md"))
	if err != nil {
		t.Fatalf("synthesized note not persisted: %v", err)
	}
	if string(onDisk) != body {
		t.Error("persisted content differs from served content")
	}
}

func TestHandleGetNoteStaticAssetNotFound(t *testing.T) {
	a := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/api/note?name=missing.js", nil)
	rec := httptest.NewRecorder()
	a.handleGetNote(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing static asset: status %d, want 404", rec.Code)
	}
}

func TestHandleSaveNoteRoundTrip(t *testing.T) {
	a := newTestApp(t)

	form := url.Values{"name": {"Note"}, "content": {"Title: Note\n\nSome **bold** body"}}
	req := httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.handleSaveNote(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}

	md, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "Note.md"))
	if err != nil {
		t.Fatalf("markdown source not written: %v", err)
	}
	if !strings.Contains(string(md), "Modified: ") {
		t.Error("saved markdown missing Modified header")
	}
	if !strings.Contains(string(md), "Some **bold** body") {
		t.Error("saved markdown lost body content")
	}

	html, err := os.ReadFile(filepath.Join(a.StorageDir, "html", "Note.html"))
	if err != nil {
		t.Fatalf("compiled html not written: %v", err)
	}
	if !strings.Contains(string(html), "<strong>bold</strong>") {
		t.Error("compiled html missing rendered markdown")
	}
}

func TestHandleSaveNoteStaticAsset(t *testing.T) {
	a := newTestApp(t)

	form := url.Values{"name": {"custom.css"}, "content": {"body { color: red; }"}}
	req := httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.handleSaveNote(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	got, err := os.ReadFile(filepath.Join(a.StorageDir, "html", "custom.css"))
	if err != nil {
		t.Fatalf("asset not written: %v", err)
	}
	if string(got) != "body { color: red; }" {
		t.Errorf("asset content %q", got)
	}
	// A static asset must never grow a markdown header or an .md twin.
	if _, err := os.Stat(filepath.Join(a.StorageDir, "md", "custom.css.md")); err == nil {
		t.Error("static asset wrongly produced a markdown file")
	}
}

func TestHandleSaveNoteMissingName(t *testing.T) {
	a := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader("content=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.handleSaveNote(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing name: status %d, want 400", rec.Code)
	}
}

func TestSaveUploadedFile(t *testing.T) {
	a := newTestApp(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("image", "pic.png")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a}
	fw.Write(payload)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	destDir := filepath.Join(a.StorageDir, "html", "images")
	name, err := a.saveUploadedFile(req, "image", destDir)
	if err != nil {
		t.Fatalf("saveUploadedFile: %v", err)
	}
	if name != "pic.png" {
		t.Errorf("filename = %q", name)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "pic.png"))
	if err != nil {
		t.Fatalf("uploaded file not on disk: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("uploaded bytes corrupted")
	}
}

func TestSaveUploadedFileWrongField(t *testing.T) {
	a := newTestApp(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("other", "x.bin")
	fw.Write([]byte("x"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	if _, err := a.saveUploadedFile(req, "image", t.TempDir()); err == nil {
		t.Error("expected error for missing form field, got nil")
	}
}
