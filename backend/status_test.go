package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// getStatus runs the handler and returns the decoded document.
func getStatus(t *testing.T, a *App, query string) (*statusResponse, *httptest.ResponseRecorder) {
	t.Helper()
	url := "/api/status"
	if query != "" {
		url += "?" + query
	}
	rec := httptest.NewRecorder()
	a.handleStatus(rec, httptest.NewRequest(http.MethodGet, url, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var res statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	return &res, rec
}

// The default answer carries the cheap sections and NEITHER slow one. That
// is the contract the Status page depends on: paint at once, then ask for
// the walks.
func TestStatusDefaultSectionsAreCheap(t *testing.T) {
	a := newTestApp(t)
	a.startedAt = time.Now().Add(-90 * time.Second)

	res, rec := getStatus(t, a, "")

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if res.Server == nil || res.Config == nil || res.Git == nil ||
		res.Search == nil || res.Runtime == nil {
		t.Fatal("a cheap section is missing from the default answer")
	}
	if res.Storage != nil {
		t.Error("storage walked without being asked for")
	}
	if res.GitDirty != nil {
		t.Error("the git worktree was read without being asked for")
	}
	if res.Server.AppVersion != APP_VERSION {
		t.Errorf("app_version = %q, want %q", res.Server.AppVersion, APP_VERSION)
	}
	if res.Server.UptimeS < 89 {
		t.Errorf("uptime_s = %d, want about 90", res.Server.UptimeS)
	}
	if res.Generated == "" || !strings.HasSuffix(res.Generated, "Z") {
		t.Errorf("generated = %q, want RFC3339 in UTC", res.Generated)
	}
}

// The listen address is not in the answer. "[::]:8080" and "::" tell a
// person nothing, on a phone and on a desktop alike. The port stays.
func TestStatusHasNoBindAddress(t *testing.T) {
	a := newTestApp(t)

	_, rec := getStatus(t, a, "sections=server")
	body := rec.Body.String()
	for _, gone := range []string{"bind_addr", "bind_host"} {
		if strings.Contains(body, gone) {
			t.Errorf("the answer still carries %q", gone)
		}
	}
	if !strings.Contains(body, "bind_port") {
		t.Error("bind_port is missing")
	}
}

// Each LAN address must be usable as an address: no loopback, no
// link-local, no duplicate. The list is empty on a machine with no
// network, which is why the test asserts the shape and not the count.
func TestStatusLANURLShape(t *testing.T) {
	a := newTestApp(t)
	a.Config.ShareLAN = true

	res, _ := getStatus(t, a, "sections=server")
	seen := map[string]bool{}
	for _, u := range res.Server.LANURLs {
		if !strings.HasPrefix(u, "http://") {
			t.Errorf("%q does not start with http://", u)
		}
		if strings.Contains(u, "127.0.0.1") || strings.Contains(u, "[::1]") {
			t.Errorf("%q is a loopback address", u)
		}
		if strings.Contains(u, "169.254.") || strings.Contains(u, "[fe80") {
			t.Errorf("%q is a link-local address", u)
		}
		if seen[u] {
			t.Errorf("%q appears two times", u)
		}
		seen[u] = true
	}

	// Sharing off: the list stays empty, because no other device can
	// reach this server.
	a.Config.ShareLAN = false
	res, _ = getStatus(t, a, "sections=server")
	if len(res.Server.LANURLs) != 0 {
		t.Errorf("lan_urls = %v with sharing off, want none", res.Server.LANURLs)
	}
}

// The Android layer hands its addresses to Go, because Go cannot read
// them on a phone. They must reach lan_urls, without a duplicate of the
// address that the probe already found.
func TestStatusLANAddressesFromAndroid(t *testing.T) {
	a := newTestApp(t)
	a.Config.ShareLAN = true

	SetLANAddresses(" 192.168.5.5 , 10.0.0.7 ,, 192.168.5.5 ")
	t.Cleanup(func() { SetLANAddresses("") })

	res, _ := getStatus(t, a, "sections=server")
	got := strings.Join(res.Server.LANURLs, " ")
	for _, want := range []string{"http://192.168.5.5:", "http://10.0.0.7:"} {
		if !strings.Contains(got, want) {
			t.Errorf("lan_urls %v misses %q", res.Server.LANURLs, want)
		}
	}
	count := 0
	for _, u := range res.Server.LANURLs {
		if strings.HasPrefix(u, "http://192.168.5.5:") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("192.168.5.5 appears %d times in %v, want 1", count, res.Server.LANURLs)
	}

	// Sharing off answers with no address, whatever Android sent.
	a.Config.ShareLAN = false
	res, _ = getStatus(t, a, "sections=server")
	if len(res.Server.LANURLs) != 0 {
		t.Errorf("lan_urls = %v with sharing off, want none", res.Server.LANURLs)
	}
}

// A named section, and only that one.
func TestStatusSectionsParameter(t *testing.T) {
	a := newTestApp(t)

	res, _ := getStatus(t, a, "sections=search")
	if res.Search == nil {
		t.Fatal("search section missing")
	}
	if res.Server != nil || res.Config != nil || res.Git != nil || res.Runtime != nil {
		t.Error("sections=search answered with more than search")
	}

	res, _ = getStatus(t, a, "sections=storage")
	if res.Storage == nil {
		t.Fatal("storage section missing")
	}
	if res.Server != nil {
		t.Error("sections=storage answered with more than storage")
	}

	// "all" reaches the slow sections too.
	res, _ = getStatus(t, a, "sections=all")
	if res.Storage == nil || res.GitDirty == nil || res.Server == nil {
		t.Error("sections=all left a section out")
	}

	// An unknown name is an error, not a silent omission.
	rec := httptest.NewRecorder()
	a.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status?sections=sarch", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown section: status %d, want 400", rec.Code)
	}
}

// The storage section counts each group of files in one walk.
func TestStatusStorageCounts(t *testing.T) {
	a := newTestApp(t)

	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(a.StorageDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("md/One.md", "Title: One\n\nbody")
	write("md/sub/Two.md", "Title: Two\n\nbody")
	write("html/One.html", "<html></html>")
	write("html/images/a.png", "0123456789")
	write("html/user_json/data.json", `{"a":1}`)
	write("db/mydata.sqlite", "sqlite")
	write("html/db_backup/mydata/20260804T100000Z_dev.jsonl", "{}\n")
	write("asset_backups/26.08.1/md/UserManual.md", "old")

	res, _ := getStatus(t, a, "sections=storage")
	st := res.Storage
	if st == nil {
		t.Fatal("no storage section")
	}
	for _, c := range []struct {
		name string
		got  statusGroup
		want int
	}{
		{"notes", st.Notes, 2},
		{"pages", st.Pages, 1},
		{"images", st.Images, 1},
		{"user_json", st.UserJSON, 1},
		{"databases", st.Databases, 1},
		{"backups", st.Backups, 1},
		{"asset_backups", st.AssetBackups, 1},
	} {
		if c.got.Files != c.want {
			t.Errorf("%s: %d file(s), want %d", c.name, c.got.Files, c.want)
		}
		if c.got.Bytes <= 0 {
			t.Errorf("%s: %d bytes, want more than 0", c.name, c.got.Bytes)
		}
	}
	if st.Total.Files < 8 {
		t.Errorf("total: %d file(s), want at least 8", st.Total.Files)
	}
	if st.Dir != a.StorageDir {
		t.Errorf("dir = %q, want %q", st.Dir, a.StorageDir)
	}
}

// An install that never synced has no repository, and status must report
// that instead of creating one.
func TestStatusGitCreatesNothing(t *testing.T) {
	a := newTestApp(t)

	res, _ := getStatus(t, a, "sections=git,git_dirty")
	if res.Git == nil || res.Git.RepoExists {
		t.Error("git section reports a repository that does not exist")
	}
	if res.Git.RemoteHead != nil || res.Git.RemoteRef != "" {
		t.Error("git section reports a remote head without a repository")
	}
	if res.GitDirty == nil || res.GitDirty.Dirty {
		t.Error("git_dirty reports changes without a repository")
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, ".git")); !os.IsNotExist(err) {
		t.Error("a status request created a git repository")
	}
}

// The remote head comes from the remote-tracking ref that the last sync
// left in this repository. Nothing asks the remote server.
func TestStatusRemoteHeadFromLocalRefs(t *testing.T) {
	a := newTestApp(t)

	repo, err := git.PlainInit(a.StorageDir, false)
	if err != nil {
		t.Skipf("cannot init a repository here: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.StorageDir, "note.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("note.txt"); err != nil {
		t.Fatal(err)
	}
	local, err := wt.Commit("a local commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Tester", Email: "t@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	branch := head.Name().Short()

	// Without a remote-tracking ref the two fields stay absent.
	res, _ := getStatus(t, a, "sections=git")
	if res.Git.RemoteHead != nil || res.Git.RemoteRef != "" {
		t.Errorf("a repository with no fetch reports %v / %q",
			res.Git.RemoteHead, res.Git.RemoteRef)
	}

	// A fetch writes such a ref. This writes one the same way.
	ref := plumbing.NewHashReference(plumbing.NewRemoteReferenceName("origin", branch), local)
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatal(err)
	}

	res, _ = getStatus(t, a, "sections=git")
	if res.Git.RemoteHead == nil {
		t.Fatal("no remote head after a remote-tracking ref exists")
	}
	if res.Git.RemoteHead.Hash != local.String() {
		t.Errorf("remote head hash = %q, want %q", res.Git.RemoteHead.Hash, local.String())
	}
	if res.Git.RemoteHead.Short != local.String()[:7] {
		t.Errorf("remote head short = %q, want %q", res.Git.RemoteHead.Short, local.String()[:7])
	}
	if res.Git.RemoteHead.Subject != "a local commit" {
		t.Errorf("remote head subject = %q", res.Git.RemoteHead.Subject)
	}
	if res.Git.RemoteRef != "origin/"+branch {
		t.Errorf("remote ref = %q, want %q", res.Git.RemoteRef, "origin/"+branch)
	}
}

// The remote to look in: the remote of the active server slot first, then
// the bootstrap remote of an older installation.
func TestRemoteRefCandidates(t *testing.T) {
	cfg := Config{
		ActiveGitIndex: 1,
		GitServers: []GitServerConfig{
			{Name: "first", URL: "https://example.com/a.git"},
			{Name: "second", URL: "https://example.com/b.git"},
		},
	}
	got := remoteRefCandidates(cfg, "master")
	want := []string{"gitserver1", "origin"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("candidates = %v, want %v", got, want)
	}

	// A slot with no address contributes no remote.
	cfg.GitServers[1].URL = "  "
	if got := remoteRefCandidates(cfg, "master"); strings.Join(got, ",") != "origin" {
		t.Errorf("candidates = %v, want [origin]", got)
	}

	// A detached HEAD has no remote-tracking ref.
	if got := remoteRefCandidates(cfg, ""); len(got) != 0 {
		t.Errorf("candidates = %v for a detached HEAD, want none", got)
	}
}

// The remote URL is reported without its password.
func TestStatusRedactsGitPassword(t *testing.T) {
	a := newTestApp(t)
	a.Config.GitServers = []GitServerConfig{{
		Name: "home",
		URL:  "https://user:secret@example.com/notes.git",
	}}
	a.Config.ActiveGitIndex = 0

	res, _ := getStatus(t, a, "sections=git")
	if res.Git == nil || res.Git.Remote == nil {
		t.Fatal("no remote in the git section")
	}
	if strings.Contains(res.Git.Remote.URL, "secret") {
		t.Errorf("the remote URL still carries the password: %q", res.Git.Remote.URL)
	}
	if !strings.Contains(res.Git.Remote.URL, "user@") {
		t.Errorf("the user name was dropped as well: %q", res.Git.Remote.URL)
	}
}

// A password of the application must never reach this endpoint, in either
// format.
func TestStatusNeverCarriesSecrets(t *testing.T) {
	a := newTestApp(t)
	a.Config.AdminPassword = "admin_secret_value"
	a.Config.GuestPassword = "guest_secret_value"
	a.Config.GitServers = []GitServerConfig{{
		Name: "home", URL: "https://u:pw_secret_value@example.com/n.git",
		SSHKeyData: "PRIVATE_KEY_VALUE", Password: "slot_secret_value",
	}}

	for _, q := range []string{"sections=all", "sections=all&format=md"} {
		rec := httptest.NewRecorder()
		a.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status?"+q, nil))
		body := rec.Body.String()
		for _, secret := range []string{
			"admin_secret_value", "guest_secret_value",
			"pw_secret_value", "PRIVATE_KEY_VALUE", "slot_secret_value",
		} {
			if strings.Contains(body, secret) {
				t.Errorf("%s: the answer carries %q", q, secret)
			}
		}
	}
}

// format=md answers with the same facts as text a person can paste into a
// bug report. text/plain, so the Android WebView shows it.
func TestStatusMarkdownFormat(t *testing.T) {
	a := newTestApp(t)

	rec := httptest.NewRecorder()
	a.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status?format=md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"# OMN-Go status", "## Server", "| app_version | " + APP_VERSION + " |"} {
		if !strings.Contains(body, want) {
			t.Errorf("markdown misses %q:\n%s", want, body)
		}
	}
}

// Only GET.
func TestStatusRejectsPost(t *testing.T) {
	a := newTestApp(t)
	rec := httptest.NewRecorder()
	a.handleStatus(rec, httptest.NewRequest(http.MethodPost, "/api/status", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d, want 405", rec.Code)
	}
}

// SetAndroidPackage wins over the derivation, and the derivation answers
// when the setter never ran.
func TestStatusAndroidPackage(t *testing.T) {
	a := &App{StorageDir: "/storage/emulated/0/Android/media/net.basov.omngo.fdroid"}

	SetAndroidPackage("")
	if got := a.statusAndroidPackage(); got != "net.basov.omngo.fdroid" {
		t.Errorf("derived package = %q, want net.basov.omngo.fdroid", got)
	}

	SetAndroidPackage("net.basov.omngo")
	defer SetAndroidPackage("")
	if got := a.statusAndroidPackage(); got != "net.basov.omngo" {
		t.Errorf("set package = %q, want net.basov.omngo", got)
	}
}

// The estimate counts what the index holds, and it grows with the index.
func TestStatusSearchEstimate(t *testing.T) {
	a := newTestApp(t)
	a.Config.SearchEnabled = true
	a.search = &searchIndex{
		docs: map[string]*indexedDoc{
			"md/One.md": {
				Path: "md/One.md", Kind: "note", Name: "One", Title: "One",
				URL: "/One.html", Tags: []string{"Test"},
				LineMasks: make([]uint64, 20),
			},
		},
		lines: 20,
		bytes: 400,
		built: time.Now(),
	}

	res, _ := getStatus(t, a, "sections=search")
	if res.Search.Docs != 1 || res.Search.Lines != 20 || res.Search.Bytes != 400 {
		t.Errorf("counters wrong: %+v", res.Search)
	}
	if res.Search.IndexBytesEstimate <= 160 {
		t.Errorf("index_bytes_estimate = %d, want more than the flat allowance",
			res.Search.IndexBytesEstimate)
	}
	if res.Search.Built == "" {
		t.Error("built time missing")
	}
}

// ----------------------------------------------------------------------
// The page (phase 2)
// ----------------------------------------------------------------------

// The page holds no facts of its own. It must carry the reader script and
// the buttons for the two slow sections. It must carry no value that only
// /api/status knows.
func TestStatusPageIsAReaderOfTheEndpoint(t *testing.T) {
	a := newTestApp(t)

	// httptest.NewRequest gives each request the address 192.0.2.1, which
	// is another machine as far as hasRole is concerned. The owner of the
	// device connects from the loopback address, and that is the request
	// this test makes (see isLocalConnection in middleware.go).
	req := httptest.NewRequest(http.MethodGet, "/OMNGoStatus.html", nil)
	req.RemoteAddr = "127.0.0.1:41000"

	rec := httptest.NewRecorder()
	a.serveStatusPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"/api/status", "stStorage", "stDirty", "stCopy", "stReload",
		"sections=all&amp;format=md",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page misses %q", want)
		}
	}
	// The body of the page starts empty. Each value comes from the
	// endpoint, so the template can carry no status value of its own.
	if !strings.Contains(body, "Loading…") {
		t.Error("the page does not start empty; it must read /api/status")
	}
	if !strings.Contains(body, "execCommand") {
		t.Error("copy must use select + execCommand, which is what the Android WebView has")
	}
}

// A guest gets a page, not the line of plain text that authMiddleware
// writes. This is the rule the file index follows.
func TestStatusPageAnswersAGuestWithAPage(t *testing.T) {
	a := newTestApp(t)
	a.Config.ShareLAN = true

	req := httptest.NewRequest(http.MethodGet, "/OMNGoStatus.html", nil)
	req.RemoteAddr = "192.168.1.44:51000" // another machine on the network
	// A signed cookie, and not the bare word "guest": the server refuses
	// an unsigned value since 26.09.6. See session.go.
	req.AddCookie(sessionCookie(t, a, roleGuest))

	rec := httptest.NewRecorder()
	a.serveStatusPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want a page", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "for the admin of this device") {
		t.Error("a guest did not get the refusal page")
	}
	if strings.Contains(body, "stStorage") {
		t.Error("a guest got the reader script")
	}
}
