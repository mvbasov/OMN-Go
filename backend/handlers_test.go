package backend

import (
	"bytes"
	"errors"
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
	name, err := a.saveUploadedFile(req, "image", destDir, imageUploadExtensions, 10<<20)
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

	if _, err := a.saveUploadedFile(req, "image", t.TempDir(), nil, 0); err == nil {
		t.Error("expected error for missing form field, got nil")
	}
}

func TestSaveUploadedFileRejectsDisallowedExtension(t *testing.T) {
	a := newTestApp(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("not json"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload_json", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	if _, err := a.saveUploadedFile(req, "file", t.TempDir(), jsonUploadExtensions, 10<<20); err == nil {
		t.Error("expected error for disallowed extension, got nil")
	} else {
		var rejected *uploadRejected
		if !errors.As(err, &rejected) {
			t.Errorf("expected *uploadRejected, got %T: %v", err, err)
		}
	}
}

func TestSaveUploadedFileRejectsTooLarge(t *testing.T) {
	a := newTestApp(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("image", "big.png")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(bytes.Repeat([]byte{0}, 2048))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	if _, err := a.saveUploadedFile(req, "image", t.TempDir(), imageUploadExtensions, 1024); err == nil {
		t.Error("expected error for oversized file, got nil")
	} else {
		var rejected *uploadRejected
		if !errors.As(err, &rejected) {
			t.Errorf("expected *uploadRejected, got %T: %v", err, err)
		}
	}
}

func postConfig(t *testing.T, a *App, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.handleConfig(rec, req)
	return rec
}

func TestHandleConfigSavesTheme(t *testing.T) {
	a := newTestApp(t)

	rec := postConfig(t, a, url.Values{"theme": {"dark"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if got := a.GetConfig().Theme; got != ThemeDark {
		t.Errorf("in-memory theme = %q, want dark", got)
	}
	// Must survive a restart: persisted to config.json.
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	if !strings.Contains(string(data), `"theme": "dark"`) {
		t.Errorf("config.json missing persisted theme:\n%s", data)
	}
}

func TestHandleConfigRejectsInvalidTheme(t *testing.T) {
	a := newTestApp(t)
	a.Config.Theme = ThemeDark // pre-existing valid value

	rec := postConfig(t, a, url.Values{"theme": {"purple; drop table"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	// Whitelist: garbage never lands in config, not even transiently -
	// it normalizes to auto.
	if got := a.GetConfig().Theme; got != ThemeAuto {
		t.Errorf("invalid theme stored as %q, want auto", got)
	}
}

func TestHandleConfigSavesShareLAN(t *testing.T) {
	a := newTestApp(t)

	// Checkbox checked -> true, persisted.
	rec := postConfig(t, a, url.Values{"share_lan": {"true"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !a.GetConfig().ShareLAN {
		t.Error("share_lan=true not stored")
	}
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	if !strings.Contains(string(data), `"share_lan": true`) {
		t.Errorf("config.json missing persisted share_lan:\n%s", data)
	}

	// Unchecked checkbox = field absent from the form -> back to false.
	rec = postConfig(t, a, url.Values{})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if a.GetConfig().ShareLAN {
		t.Error("absent share_lan field did not clear the option")
	}
}

func TestHandleConfigSavesMaxUploadSizeMB(t *testing.T) {
	a := newTestApp(t)
	a.Config.MaxUploadSizeMB = defaultMaxUploadSizeMB

	rec := postConfig(t, a, url.Values{"max_upload_size_mb": {"10"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if got := a.GetConfig().MaxUploadSizeMB; got != 10 {
		t.Errorf("MaxUploadSizeMB = %d, want 10", got)
	}
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	if !strings.Contains(string(data), `"max_upload_size_mb": 10`) {
		t.Errorf("config.json missing persisted max_upload_size_mb:\n%s", data)
	}

	// Same "parse, only apply if positive" shape as server_port: a blank
	// or zero submission must not silently zero out the limit (which
	// would reject every upload).
	rec = postConfig(t, a, url.Values{"max_upload_size_mb": {"0"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := a.GetConfig().MaxUploadSizeMB; got != 10 {
		t.Errorf("MaxUploadSizeMB changed to %d on a zero submission, want unchanged 10", got)
	}
}

// TestResolveAndroidEditName covers the bug where Android's external-editor
// handoff opened the compiled .html cache instead of the .md source: it
// picked md/ vs html/ purely by checking whether the name it was handed
// ended in ".md", but the name reaching handleEditExternal is whatever URL
// was being viewed (often "Name.html"), not necessarily the real source
// file. resolveAndroidEditName is the fix, normalizing that name before
// it's ever sent to the Android client.
func TestResolveAndroidEditName(t *testing.T) {
	tests := []struct {
		name         string
		reqName      string // what handleEditExternal received as ?name=
		baseName     string // resolvePageName's baseName for reqName
		isPage       bool   // resolvePageName's isPage for reqName
		wantEditName string
	}{
		{"page requested via its rendered view URL", "Welcome.html", "Welcome", true, "Welcome.md"},
		{"page requested by bare name", "Welcome", "Welcome", true, "Welcome.md"},
		{"page already given as .md", "Welcome.md", "Welcome", true, "Welcome.md"},
		{"nested page via view URL", "dir/Note.html", "dir/Note", true, "dir/Note.md"},
		{"non-page asset is left untouched", "omn-go-editor.js", "omn-go-editor.js", false, "omn-go-editor.js"},
		{"non-page asset in a subdirectory", "css/omn-go-core.css", "css/omn-go-core.css", false, "css/omn-go-core.css"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveAndroidEditName(tt.reqName, tt.baseName, tt.isPage)
			if got != tt.wantEditName {
				t.Errorf("resolveAndroidEditName(%q, %q, %v) = %q, want %q",
					tt.reqName, tt.baseName, tt.isPage, got, tt.wantEditName)
			}
		})
	}
}

// TestResolveAndroidEditNameAgreesWithResolvePageName drives
// resolveAndroidEditName off the SAME a.resolvePageName call
// handleEditExternal itself makes, rather than hand-picked baseName/isPage
// values - so it exercises the actual integration, not just the pure
// function in isolation. All three spellings a real request could arrive
// with ("Welcome", "Welcome.md", "Welcome.html" - see
// TestResolvePageNameEquivalence in paths_test.go) must normalize to the
// one correct Android edit name, "Welcome.md".
func TestResolveAndroidEditNameAgreesWithResolvePageName(t *testing.T) {
	a := &App{StorageDir: "/store"}

	for _, spelling := range []string{"Welcome", "Welcome.md", "Welcome.html"} {
		t.Run(spelling, func(t *testing.T) {
			_, _, baseName, isPage := a.resolvePageName(spelling)
			got := resolveAndroidEditName(spelling, baseName, isPage)
			if got != "Welcome.md" {
				t.Errorf("resolveAndroidEditName for %q = %q, want %q", spelling, got, "Welcome.md")
			}
		})
	}

	// A genuine static asset must come back unchanged, not have ".md"
	// appended - it was never a markdown-backed page to begin with.
	_, _, baseName, isPage := a.resolvePageName("omn-go-editor.js")
	if got := resolveAndroidEditName("omn-go-editor.js", baseName, isPage); got != "omn-go-editor.js" {
		t.Errorf("resolveAndroidEditName for static asset = %q, want unchanged %q", got, "omn-go-editor.js")
	}
}
