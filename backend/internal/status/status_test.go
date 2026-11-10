package status

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
	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/testkit"
)

// getStatus runs the handler and returns the decoded document.
func getStatus(t *testing.T, a *testApp, query string) (*statusResponse, *httptest.ResponseRecorder) {
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
	// The config section is a list in Go and an object in JSON, thus the
	// decode reads it as a map.
	var doc struct {
		statusResponse
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	res := doc.statusResponse
	for k, v := range doc.Config {
		res.Config = append(res.Config, config.StatusValue{Key: k, Value: v})
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
	if res.Server.AppVersion != testkit.Version {
		t.Errorf("app_version = %q, want %q", res.Server.AppVersion, testkit.Version)
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
	a.Config.Update(func(c *config.Config) { c.ShareLAN = true })

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
	a.Config.Update(func(c *config.Config) { c.ShareLAN = false })
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
// The storage section has one group for each tree of config.UserFileTrees,
// in the JSON answer and in the Markdown answer. The group has the name of
// the directory. This test fails for a new tree with no group, because its
// files then count in the total only.
func TestStatusStorageHasAGroupForEachUserFileTree(t *testing.T) {
	a := newTestApp(t)
	for _, tree := range config.UserFileTrees {
		for i, ext := range tree.Exts {
			p := filepath.Join(a.StorageDir, "html", tree.Dir, "f"+ext)
			if i == 0 {
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	_, rec := getStatus(t, a, "sections=storage")
	var raw struct {
		Storage map[string]json.RawMessage `json:"storage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	md := httptest.NewRecorder()
	a.handleStatus(md, httptest.NewRequest(http.MethodGet, "/api/status?sections=storage&format=md", nil))
	for _, tree := range config.UserFileTrees {
		var group statusGroup
		if err := json.Unmarshal(raw.Storage[tree.Dir], &group); err != nil {
			t.Errorf("the storage section has no group %q: %v", tree.Dir, err)
			continue
		}
		if group.Files != len(tree.Exts) || group.Bytes != int64(4*len(tree.Exts)) {
			t.Errorf("%s: %+v, want %d file(s) of 4 bytes", tree.Dir, group, len(tree.Exts))
		}
		row := "| " + tree.Dir + " | "
		if !strings.Contains(md.Body.String(), row) {
			t.Errorf("the Markdown answer has no row for %s", tree.Dir)
		}
	}
}

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
	cfg := config.Config{
		ActiveGitIndex: 1,
		GitServers: []config.GitServer{
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
	a.Config.Update(func(c *config.Config) {
		c.GitServers = []config.GitServer{{
			Name: "home",
			URL:  "https://user:secret@example.com/notes.git",
		}}
		c.ActiveGitIndex = 0
	})

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
	a.Config.Update(func(c *config.Config) {
		c.AdminPassword = "admin_secret_value"
		c.GitServers = []config.GitServer{{
			Name: "home", URL: "https://u:pw_secret_value@example.com/n.git",
			SSHKeyData: "PRIVATE_KEY_VALUE", Password: "slot_secret_value",
		}}
	})

	for _, q := range []string{"sections=all", "sections=all&format=md"} {
		rec := httptest.NewRecorder()
		a.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status?"+q, nil))
		body := rec.Body.String()
		for _, secret := range []string{
			"admin_secret_value",
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
	for _, want := range []string{"# OMN-Go status", "## Server", "| app_version | " + testkit.Version + " |"} {
		if !strings.Contains(body, want) {
			t.Errorf("markdown misses %q:\n%s", want, body)
		}
	}
}

// The Status page shows the counters of the index, and the estimate grows
// with the index. The test builds a real index of one note.
func TestStatusSearchEstimate(t *testing.T) {
	a := enabledSearchApp(t)
	writeSearchNote(t, a, "One.md", "Title: One\nTags: Test\n\nfirst line\nsecond line\n")
	a.rebuildSearchIndex()
	want := a.search.Stats()
	if want.Docs != 1 || want.Lines == 0 || want.Bytes == 0 {
		t.Fatalf("test precondition: the index of one note is %+v", want)
	}

	res, _ := getStatus(t, a, "sections=search")
	if res.Search.Docs != want.Docs || res.Search.Lines != want.Lines || res.Search.Bytes != want.Bytes {
		t.Errorf("counters wrong: %+v, the index holds %+v", res.Search, want)
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
		"/api/status", "omn-st-storage", "omn-st-dirty", "omn-st-copy", "omn-st-reload",
		"sections=all&amp;format=md",
		"/js/OMN-Go/omn-go-status.js", "/css/OMN-Go/omn-go-status.css",
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
	// The script and the stylesheet are files, and not inline blocks of
	// the template.
	//
	// The check does not look for the word execCommand in the page. That
	// word can match a COMMENT and not the code, thus it proves nothing
	// about the clipboard. TestClipboardHasOneAuthority in
	// internal/repocheck/frontend_test.go is what holds that rule, and it
	// reads every embedded script and template.
	if strings.Contains(statusPageTmpl, "<script>") {
		t.Error("the template holds an inline script")
	}
	if strings.Contains(statusPageTmpl, "<style>") {
		t.Error("the template holds an inline style")
	}
}

// The Server section names the commit of the build. A hash gives the address
// of the commit, also with "-dirty". A build with no commit says "unknown",
// and other text stays as it is, with no address.
func TestStatusBuildCommit(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		commit, wantValue, wantURL string
	}{
		{"", "unknown", ""},
		{hash, hash, buildCommitURLBase + hash},
		{hash + "-dirty", hash + "-dirty", buildCommitURLBase + hash},
		{"0123abc", "0123abc", ""},
		{hash + "-other", hash + "-other", ""},
	}
	for _, c := range cases {
		value, url := buildCommitFields(c.commit)
		if value != c.wantValue || url != c.wantURL {
			t.Errorf("buildCommitFields(%q) = %q, %q, want %q, %q", c.commit, value, url, c.wantValue, c.wantURL)
		}
	}

	a := newTestApp(t)
	svc := a.statusService()
	svc.BuildCommit = hash + "-dirty"
	svc.BuildCommitTime = "2026-11-10T10:27:00+03:00"

	rec := httptest.NewRecorder()
	svc.HandleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status?sections=server", nil))
	var res statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Server == nil || res.Server.BuildCommit != hash+"-dirty" ||
		res.Server.BuildCommitTime != svc.BuildCommitTime ||
		res.Server.BuildCommitURL != buildCommitURLBase+hash {
		t.Errorf("server section = %+v", res.Server)
	}

	rec = httptest.NewRecorder()
	svc.HandleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status?format=md", nil))
	for _, want := range []string{
		"| build_commit | " + hash + "-dirty |",
		"| build_commit_time | 2026-11-10T10:27:00+03:00 |",
		"| build_commit_url | " + buildCommitURLBase + hash + " |",
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("markdown misses %q", want)
		}
	}

	res2, _ := getStatus(t, newTestApp(t), "")
	if res2.Server.BuildCommit != "unknown" || res2.Server.BuildCommitURL != "" {
		t.Errorf("no commit: build_commit = %q, url = %q, want unknown and no url",
			res2.Server.BuildCommit, res2.Server.BuildCommitURL)
	}
}
