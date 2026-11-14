package app

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

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/config"
)

// newUnconfiguredApp returns an App rooted in a fresh temp dir with the
// md/ and html/ layout that initStorage makes. It does not run
// initStorage, thus it extracts no embedded file. It loads NO
// configuration, thus the config.Store keeps each zero value.
//
// Use it for a test of loadConfig itself, and for a test that asks what
// the application does with no configuration. Each other test wants
// newTestApp below.
func newUnconfiguredApp(t *testing.T) *App {
	t.Helper()
	a := &App{StorageDir: t.TempDir()}
	a.connectGroups()
	for _, d := range []string{"md", "html"} {
		if err := os.MkdirAll(filepath.Join(a.StorageDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// newTestApp returns the App of a fresh install. It makes the storage
// layout and then loads the configuration, thus the config.Store holds the
// defaults that loadConfig writes and config.json exists on disk.
//
// WHY IT LOADS THE CONFIGURATION. No user ever runs an App with a zero
// Config. A zero Config makes each boolean false and each string empty,
// and a test would then read a default that the loader never writes. For
// example, loadConfig sets UseInternalEd to true on a fresh install.
//
// A test that wants the zero Config asks for it by name with
// newUnconfiguredApp. The two helpers make the choice visible in the
// test, which is where a reader looks for it.
func newTestApp(t *testing.T) *App {
	t.Helper()
	a := newUnconfiguredApp(t)
	a.loadConfig(a.layout().Config())
	return a
}

// The state of the test application must be the state of a fresh
// install. A change of loadConfig that makes a different default is a
// change that each test above must see.
//
// The fields below are the ones that a handler branches on. A zero
// Config makes each of them false or empty, and that hides a wrong
// default.
func TestTestAppHoldsTheFreshInstallDefaults(t *testing.T) {
	a := newTestApp(t)
	cfg := a.config.Get()

	if !cfg.UseInternalEd {
		t.Error("UseInternalEd is false, but a fresh install turns the internal editor on")
	}
	if cfg.ServerPort != 8080 {
		t.Errorf("ServerPort is %d, want 8080", cfg.ServerPort)
	}
	if cfg.Theme != config.ThemeAuto {
		t.Errorf("Theme is %q, want %q", cfg.Theme, config.ThemeAuto)
	}
	if cfg.SearchEnabled {
		t.Error("SearchEnabled is true, but global search is off on a fresh install")
	}
	if len(cfg.GitServers) != config.MaxGitServers {
		t.Errorf("GitServers holds %d slots, want %d", len(cfg.GitServers), config.MaxGitServers)
	}
	if len(cfg.LogTags) == 0 {
		t.Error("LogTags is empty, but a fresh install ticks each tag")
	}

	// The loader also writes the file. A test of an upgrade path needs
	// that file to exist.
	if _, err := os.Stat(filepath.Join(a.StorageDir, "config.json")); err != nil {
		t.Errorf("newTestApp wrote no config.json: %v", err)
	}
}

// The other half of the pair. A test that asks for the unconfigured App
// must get one, because that is the state it makes a claim about.
func TestUnconfiguredAppLoadsNoConfig(t *testing.T) {
	a := newUnconfiguredApp(t)

	if cfg := a.config.Get(); cfg.ServerPort != 0 || cfg.UseInternalEd {
		t.Errorf("newUnconfiguredApp loaded a configuration: %+v", cfg)
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "config.json")); err == nil {
		t.Error("newUnconfiguredApp wrote a config.json")
	}
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
		// whitespace trimmed. Empty stays empty.
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
	a.config.Update(func(c *config.Config) { c.Author = "Tester" })

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

// A shipped html/ asset that the user has never opened is not on disk yet.
// See the lazy extraction in materializeAsset. /api/note must answer with
// the EMBEDDED content, and not with a 404. The editor turns a 404 into an
// empty buffer, and the first Save would write that emptiness over the
// shipped file for good. Lazy extraction fills only a file that is MISSING.
func TestHandleGetNoteEmbeddedAssetFallsBackToEmbed(t *testing.T) {
	const rel = "json/bookmarker-tags.json"

	embedded, err := frontend.Static.ReadFile("html/" + rel)
	if err != nil {
		t.Skipf("%s is not embedded in this build: %v", rel, err)
	}

	a := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/api/note?name="+url.QueryEscape(rel), nil)
	rec := httptest.NewRecorder()
	a.handleGetNote(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if rec.Body.String() != string(embedded) {
		t.Error("served content is not the embedded content")
	}
	// The extraction is persisted, so the next request reads it from disk.
	onDisk, err := os.ReadFile(filepath.Join(a.StorageDir, "html", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("asset not extracted to disk: %v", err)
	}
	if !bytes.Equal(onDisk, embedded) {
		t.Error("extracted file differs from the embedded content")
	}
}

// serveEditor puts the shipped asset on disk before ANY editor opens it.
// The external editor and the Android omngo://edit intent open the file
// path directly, and they never call /api/note.
func TestServeEditorMaterializesEmbeddedAsset(t *testing.T) {
	const rel = "json/bookmarker-tags.json"

	embedded, err := frontend.Static.ReadFile("html/" + rel)
	if err != nil {
		t.Skipf("%s is not embedded in this build: %v", rel, err)
	}

	a := newTestApp(t)
	a.config.Update(func(c *config.Config) { c.UseInternalEd = true })
	req := httptest.NewRequest(http.MethodGet, "/"+rel+"?edit=true", nil)
	rec := httptest.NewRecorder()
	a.serveEditor(rec, req, "/"+rel)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	onDisk, err := os.ReadFile(filepath.Join(a.StorageDir, "html", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("asset not extracted before the editor opened: %v", err)
	}
	if !bytes.Equal(onDisk, embedded) {
		t.Error("extracted file differs from the embedded content")
	}
}

// /images/ and /user_json/ have routes of their own, so an edit request
// there never reaches serveFrontend. The trees still answer "?edit=true"
// with the editor, which is what the User Manual documents.
func TestServeStorageSubdirHonorsEditIntent(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) { c.UseInternalEd = true })

	dir := filepath.Join(a.StorageDir, "html", "user_json")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inventory.json"), []byte(`{"a":1}`), 0644); err != nil {
		t.Fatal(err)
	}

	h := a.serveStorageSubdir("user_json", "")

	req := httptest.NewRequest(http.MethodGet, "/user_json/inventory.json?edit=true", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit request: status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "omn-editor-save") {
		t.Error("edit request did not answer with the editor page")
	}

	// Without the query the file itself is served, exactly as before.
	req = httptest.NewRequest(http.MethodGet, "/user_json/inventory.json", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if body := rec.Body.String(); body != `{"a":1}` {
		t.Errorf("plain request served %q, want the file", body)
	}
}

// A picture, a font, an audio file or a video file has nothing to type
// into. A save through a textarea would damage it. Each of the four routes
// that can reach an editor refuses such a file with 415. serveEditor
// refuses it BEFORE it writes the file to disk.
func TestEditorRoutesRefuseBinaryFiles(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) { c.UseInternalEd = true })

	imgDir := filepath.Join(a.StorageDir, "html", "images")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\n binary")
	if err := os.WriteFile(filepath.Join(imgDir, "photo.png"), png, 0644); err != nil {
		t.Fatal(err)
	}

	const rel = "images/photo.png"

	// 1. the editor page
	rec := httptest.NewRecorder()
	a.serveEditor(rec, httptest.NewRequest(http.MethodGet, "/"+rel+"?edit=true", nil), "/"+rel)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("serveEditor: status %d, want 415", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "omn-editor-save") {
		t.Error("serveEditor answered with the editor page for a picture")
	}

	// 2. the external editor route
	rec = httptest.NewRecorder()
	a.handleEditExternal(rec, httptest.NewRequest(http.MethodGet, "/api/edit-external?name="+url.QueryEscape(rel), nil))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("handleEditExternal: status %d, want 415", rec.Code)
	}

	// 3. the content route the editor loads from
	rec = httptest.NewRecorder()
	a.handleGetNote(rec, httptest.NewRequest(http.MethodGet, "/api/note?name="+url.QueryEscape(rel), nil))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("handleGetNote: status %d, want 415", rec.Code)
	}

	// 4. the save, which is where the damage would happen
	form := url.Values{"name": {rel}, "content": {"text over a picture"}}
	req := httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	a.handleSaveNote(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("handleSaveNote: status %d, want 415", rec.Code)
	}
	after, err := os.ReadFile(filepath.Join(imgDir, "photo.png"))
	if err != nil || !bytes.Equal(after, png) {
		t.Error("the picture was changed by a save that must have been refused")
	}

	// A refused editor request must not extract a shipped binary either.
	rec = httptest.NewRecorder()
	a.serveEditor(rec, httptest.NewRequest(http.MethodGet, "/css/OMN-Go/fonts/material-icons.woff2?edit=true", nil),
		"/css/OMN-Go/fonts/material-icons.woff2")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("serveEditor on a font: status %d, want 415", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "css", "fonts", "material-icons.woff2")); !os.IsNotExist(err) {
		t.Error("a refused editor request extracted the font to disk")
	}
}

// The rule is about binary files, not about the editor. A page, a note and
// a text asset must still open.
func TestEditorRoutesStillOpenTextFiles(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) { c.UseInternalEd = true })

	if err := os.WriteFile(filepath.Join(a.StorageDir, "md", "Welcome.md"),
		[]byte("Title: Welcome\n\nbody"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/Welcome.html", "/css/omn-go-custom.css", "/js/omn-go-custom.js"} {
		rec := httptest.NewRecorder()
		a.serveEditor(rec, httptest.NewRequest(http.MethodGet, p+"?edit=true", nil), p)
		if rec.Code != http.StatusOK {
			t.Errorf("serveEditor(%q): status %d, want 200", p, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "omn-editor-save") {
			t.Errorf("serveEditor(%q) did not answer with the editor page", p)
		}
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

	if _, err := a.saveUploadedFile(req, "file", t.TempDir(), config.UserFileTrees[0].Exts, 10<<20); err == nil {
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
	a.handleConfigPost(rec, req)
	return rec
}

func TestHandleConfigSavesTheme(t *testing.T) {
	a := newTestApp(t)

	rec := postConfig(t, a, url.Values{"theme": {"dark"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if got := a.config.Get().Theme; got != config.ThemeDark {
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
	a.config.Update(func(c *config.Config) { c.Theme = config.ThemeDark }) // pre-existing valid value

	rec := postConfig(t, a, url.Values{"theme": {"purple; drop table"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	// Whitelist: garbage never lands in config, not even transiently -
	// it normalizes to auto.
	if got := a.config.Get().Theme; got != config.ThemeAuto {
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
	if !a.config.Get().ShareLAN {
		t.Error("share_lan=true not stored")
	}
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	if !strings.Contains(string(data), `"share_lan": true`) {
		t.Errorf("config.json missing persisted share_lan:\n%s", data)
	}

	// Unchecked checkbox: the form DECLARES the field in config_fields and
	// sends no value for it -> back to false. A request that does not
	// declare it leaves it alone instead (TestBaseline_ConfigPostSemantics).
	rec = postConfig(t, a, url.Values{"config_fields": {configFormFields}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if a.config.Get().ShareLAN {
		t.Error("absent share_lan field did not clear the option")
	}
}

func TestHandleConfigSavesMaxUploadSizeMB(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) { c.MaxUploadSizeMB = config.DefaultMaxUploadSizeMB })

	rec := postConfig(t, a, url.Values{"max_upload_size_mb": {"10"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if got := a.config.Get().MaxUploadSizeMB; got != 10 {
		t.Errorf("MaxUploadSizeMB = %d, want 10", got)
	}
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	if !strings.Contains(string(data), `"max_upload_size_mb": 10`) {
		t.Errorf("config.json missing persisted max_upload_size_mb:\n%s", data)
	}

	// Same "parse, only apply if positive" shape as server_port. A blank
	// or zero submission must not silently zero out the limit, because
	// that would reject every upload.
	rec = postConfig(t, a, url.Values{"max_upload_size_mb": {"0"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := a.config.Get().MaxUploadSizeMB; got != 10 {
		t.Errorf("MaxUploadSizeMB changed to %d on a zero submission, want unchanged 10", got)
	}
}

func TestHandleConfigSavesAndroidIntentToggles(t *testing.T) {
	a := newTestApp(t)

	// Both boxes checked -> both true, both persisted. The dependency of
	// Termux on the master toggle is enforced on the Android side at tap
	// time, and not here. The config layer stores whatever was submitted.
	rec := postConfig(t, a, url.Values{
		"enable_intent_uri":    {"true"},
		"enable_termux_intent": {"true"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if !a.config.Get().EnableIntentURI {
		t.Error("enable_intent_uri=true not stored")
	}
	if !a.config.Get().EnableTermuxIntent {
		t.Error("enable_termux_intent=true not stored")
	}
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	for _, want := range []string{`"enable_intent_uri": true`, `"enable_termux_intent": true`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config.json missing persisted %s:\n%s", want, data)
		}
	}

	// Unchecked checkboxes: declared in config_fields, no value sent -> both
	// clear back to false (same shape as share_lan above).
	rec = postConfig(t, a, url.Values{"config_fields": {configFormFields}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if a.config.Get().EnableIntentURI {
		t.Error("absent enable_intent_uri field did not clear the option")
	}
	if a.config.Get().EnableTermuxIntent {
		t.Error("absent enable_termux_intent field did not clear the option")
	}
}

// TestResolveAndroidEditName covers a bug. The external-editor handoff of
// Android opened the compiled .html cache instead of the .md source. It
// picked md/ or html/ by one test alone, whether the name it was handed
// ended in ".md". The name that reaches handleEditExternal is whatever URL
// was on screen, and that is often "Name.html". It is not necessarily the
// real source file. resolveAndroidEditName is the fix. It normalizes that
// name before the name is ever sent to the Android client.
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
		{"non-page asset in a subdirectory", "css/OMN-Go/omn-go-core.css", "css/OMN-Go/omn-go-core.css", false, "css/OMN-Go/omn-go-core.css"},
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
// resolveAndroidEditName off the SAME a.resolvePageName call that
// handleEditExternal itself makes. It does not use a hand-picked baseName
// or isPage value. It thus exercises the real integration, and not the
// pure function alone.
//
// A real request can arrive with three spellings, "Welcome", "Welcome.md"
// and "Welcome.html". See TestResolvePageNameEquivalence in paths_test.go.
// All three must normalize to the one correct Android edit name, which is
// "Welcome.md".
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

// handleBookmark must split the notes field into several entries on ';'.
// It trims each entry and drops an empty one. It must store them safely,
// with each quote JSON-escaped, thus the entry embeds cleanly in the
// <script> block of Bookmarks.md.
func TestHandleBookmarkSplitsNotesBySemicolon(t *testing.T) {
	a := newTestApp(t)
	bmPath := filepath.Join(a.StorageDir, "md", "Bookmarks.md")
	marker := "<!-- Don't edit body below this line -->"
	if err := os.WriteFile(bmPath,
		[]byte("Title: Incoming bookmarks\n\n<script>bookmarks = [\n"+marker+"\n];\n</script>"), 0644); err != nil {
		t.Fatal(err)
	}

	form := url.Values{
		"url":   {"https://example.com"},
		"title": {"Ex"},
		"tags":  {"a, b"},
		// Three pieces separated by a semicolon. The trailing blank must
		// be dropped. The double quotes sit mid-note, thus the empty-note
		// check below cannot be fooled. A note that *ends* in a quote
		// would marshal to "\"\"", and that is the trap.
		"notes": {` a "b" c ; it's second ; `},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/bookmark", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.handleBookmark(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}

	got, err := os.ReadFile(bmPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)

	// First note: split out, trimmed, and its double quotes JSON-escaped.
	if !strings.Contains(s, `"a \"b\" c"`) {
		t.Errorf("first note not split/escaped as expected:\n%s", s)
	}
	// Second note: a standalone JSON string (proves the ';' split happened) with
	// its apostrophe preserved (no bogus JSON escaping of ').
	if !strings.Contains(s, `"it's second"`) {
		t.Errorf("second note (apostrophe) missing or not split:\n%s", s)
	}
	// The trailing empty piece must not become an empty note.
	if strings.Contains(s, `""`) {
		t.Errorf("an empty note leaked from the trailing ';':\n%s", s)
	}
}

// saveBlock puts a directory or a regular file at rel inside the storage
// directory. A save that must write a file or a directory at that place
// then fails. The block stands for a full disk or a permission fault.
func saveBlock(t *testing.T, a *App, rel string, asDir bool) {
	t.Helper()
	p := filepath.Join(a.StorageDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	var err error
	if asDir {
		err = os.MkdirAll(p, 0755)
	} else {
		err = os.WriteFile(p, []byte("x"), 0644)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// A save that fails must answer 500. The editor then shows "Save failed"
// and keeps the text, thus the person does not lose the edit. The body
// holds a general message only, because a LAN client can read it. This
// test covers each of the four write steps of handleSaveNote.
func TestHandleSaveNoteReportsEachWriteFault(t *testing.T) {
	for _, tc := range []struct {
		why   string
		name  string
		block string
		asDir bool
	}{
		{"the directory of a note is a file", "sub/Page", "md/sub", false},
		{"the note path is a directory", "Page", "md/Page.md", true},
		{"the directory of a file is a file", "css/site.css", "html/css", false},
		{"the file path is a directory", "site.css", "html/site.css", true},
	} {
		a := newTestApp(t)
		saveBlock(t, a, tc.block, tc.asDir)
		rec := postForm(t, a.handleSaveNote, "/api/save",
			url.Values{"name": {tc.name}, "content": {"Title: T\n\nbody"}})
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status %d, want 500", tc.why, rec.Code)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "Failed to save" {
			t.Errorf("%s: body %q, want only %q", tc.why, got, "Failed to save")
		}
	}
}

// A browser on Windows sends CRLF line ends. The note on disk must hold
// LF only, because git sync and the header parser expect LF. The rule
// applies to a note and to a text file.
func TestHandleSaveNoteStoresLFLineEnds(t *testing.T) {
	a := newTestApp(t)
	for _, tc := range []struct{ name, path string }{
		{"Page", "md/Page.md"},
		{"site.css", "html/site.css"},
	} {
		rec := postForm(t, a.handleSaveNote, "/api/save",
			url.Values{"name": {tc.name}, "content": {"Title: T\r\n\r\none\r\ntwo\r\n"}})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d, want 200", tc.name, rec.Code)
		}
		got, err := os.ReadFile(filepath.Join(a.StorageDir, filepath.FromSlash(tc.path)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(got), "\r") {
			t.Errorf("%s: the file holds a CR: %q", tc.name, got)
		}
		if !strings.Contains(string(got), "one\ntwo\n") {
			t.Errorf("%s: the body changed: %q", tc.name, got)
		}
	}
}

// A second save replaces the note. It does not add to it, and the
// compiled page shows the new text only.
func TestHandleSaveNoteReplacesTheNote(t *testing.T) {
	a := newTestApp(t)
	for _, body := range []string{"first text", "second text"} {
		rec := postForm(t, a.handleSaveNote, "/api/save",
			url.Values{"name": {"Page"}, "content": {"Title: Page\n\n" + body}})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, want 200", rec.Code)
		}
	}
	md, _ := os.ReadFile(filepath.Join(a.StorageDir, "md", "Page.md"))
	html, _ := os.ReadFile(filepath.Join(a.StorageDir, "html", "Page.html"))
	for what, got := range map[string]string{"md": string(md), "html": string(html)} {
		if strings.Contains(got, "first text") || !strings.Contains(got, "second text") {
			t.Errorf("the %s file does not hold only the second save:\n%s", what, got)
		}
	}
}

// The first quick note makes QuickNotes.md. An absent note is the normal
// state of a new install, and not a fault.
func TestQuickNoteMakesTheNoteWhenItIsAbsent(t *testing.T) {
	a := newTestApp(t)
	p := filepath.Join(a.StorageDir, "md", "QuickNotes.md")
	os.Remove(p)
	rec := postForm(t, a.handleQuickNote, "/api/quick", url.Values{"note": {"x"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Title: Quick Notes") || !strings.Contains(string(got), "\nx\n") {
		t.Errorf("QuickNotes.md does not hold the header and the note:\n%s", got)
	}
}

// A quick note that does not reach the disk must answer 500. The page then
// keeps the text, and the person does not lose it. A directory at the path
// of the note makes the write fail, also for root, who runs the gate.
func TestQuickNoteReportsAWriteFault(t *testing.T) {
	a := newTestApp(t)
	saveBlock(t, a, "md/QuickNotes.md", true)
	rec := postForm(t, a.handleQuickNote, "/api/quick", url.Values{"note": {"x"}})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "Failed to save" {
		t.Errorf("body %q, want %q", got, "Failed to save")
	}
}

// A bookmark that the server cannot store must answer 500, and not
// "Saved". Each case below lost the bookmark with a "Saved" answer before.
func TestBookmarkReportsEachFault(t *testing.T) {
	const noMarker = "Title: Incoming bookmarks\n\nno marker line\n"
	for _, tc := range []struct {
		why     string
		prepare func(t *testing.T, a *App)
	}{
		{"Bookmarks.md is absent", func(t *testing.T, a *App) {
			os.Remove(filepath.Join(a.StorageDir, "md", "Bookmarks.md"))
		}},
		{"Bookmarks.md has no marker line", func(t *testing.T, a *App) {
			baseWriteMD(t, a, "Bookmarks.md", noMarker)
		}},
		{"a directory is at the path", func(t *testing.T, a *App) {
			os.Remove(filepath.Join(a.StorageDir, "md", "Bookmarks.md"))
			saveBlock(t, a, "md/Bookmarks.md", true)
		}},
	} {
		a := newTestApp(t)
		tc.prepare(t, a)
		rec := postForm(t, a.handleBookmark, "/api/bookmark", url.Values{"url": {"https://example.org"}})
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status %d, want 500", tc.why, rec.Code)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "Failed to save" {
			t.Errorf("%s: body %q, want %q", tc.why, got, "Failed to save")
		}
		if tc.why == "Bookmarks.md has no marker line" {
			got, _ := os.ReadFile(filepath.Join(a.StorageDir, "md", "Bookmarks.md"))
			if string(got) != noMarker {
				t.Errorf("%s: the note changed:\n%s", tc.why, got)
			}
		}
	}
}

// A new page that cannot be written must answer 500. Before, the handler
// made no page and answered the name, and the editor then opened a page
// that did not exist. A FILE where the directory must be makes the fault.
func TestNewPageReportsAWriteFault(t *testing.T) {
	a := newTestApp(t)
	saveBlock(t, a, "md/sub", false)
	rec := postForm(t, a.handleNewPage, "/api/newpage", url.Values{"target": {"sub/New"}, "title": {"T"}})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "Failed to create the page" {
		t.Errorf("body %q, want %q", got, "Failed to create the page")
	}
}

// A new page gets the header of noteheader.New. A second request for the
// same page must not write over the page that exists.
func TestNewPageHeader(t *testing.T) {
	a := newTestApp(t)
	rec := postForm(t, a.handleNewPage, "/api/newpage", url.Values{"target": {"Fresh"}, "title": {"My title"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	p := filepath.Join(a.StorageDir, "md", "Fresh.md")
	first, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	src := string(first)
	if !strings.HasPrefix(src, "Title: My title\nDate: ") ||
		!strings.Contains(src, "\nModified: ") || !strings.Contains(src, "\nCategory: Notes\n") {
		t.Errorf("the header of the new page is wrong:\n%s", src)
	}
	rec = postForm(t, a.handleNewPage, "/api/newpage", url.Values{"target": {"Fresh"}, "title": {"Other"}})
	if rec.Code != http.StatusOK {
		t.Errorf("second request: status %d, want 200", rec.Code)
	}
	if again, _ := os.ReadFile(p); string(again) != src {
		t.Errorf("the second request changed the page:\n%s", again)
	}
}
