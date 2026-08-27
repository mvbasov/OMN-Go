package backend

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// ----------------------------------------------------------------------
// GET /api/status - what this build is doing right now
// ----------------------------------------------------------------------
//
// One endpoint answers the questions that a bug report asks. Which
// address does the server listen on? Which commit are the notes at? How
// large did the search index grow? Which Android package runs? The Status
// page is a later phase, and it reads this endpoint. It is not a second
// source of the same facts.
//
// ADMIN ONLY. The answer carries LAN addresses, absolute paths and a
// commit subject. hasRole treats each local connection as the owner. The
// Android WebView and a desktop browser therefore reach this endpoint
// with no login. Only another machine on the network needs the admin
// cookie.
//
// Two sections cost real work. They are NEVER in the default answer:
// "storage" walks the storage directory, and "git_dirty" walks the git
// worktree. A caller asks for them by name. The page therefore paints the
// cheap facts at once, and runs a progress bar over the slow request
// alone.
//
// Nothing here opens a network connection, and nothing here creates or
// changes a file. In particular the git section opens the repository
// read-only: getOrInitRepo() would CREATE one, which a status request
// must never do.

// statusCheapSections is the default answer. statusSlowSections must be
// named in the "sections" parameter, one at a time or through "all".
var (
	statusCheapSections = []string{"server", "config", "git", "search", "runtime", "android"}
	statusSlowSections  = []string{"storage", "git_dirty"}
)

// androidPackage is set by the Android layer before StartServer, through
// SetAndroidPackage. The Go runtime cannot ask Android which
// applicationId it runs under. The two values are net.basov.omngo and
// net.basov.omngo.fdroid. This package only reports the name.
var (
	androidPackageMu sync.RWMutex
	androidPackage   string
)

// SetAndroidPackage records the applicationId of the running Android
// application. ServerService.java calls it before Backend.startServer.
// /api/status then reports the package that this process belongs to.
//
// The function is exported for the gomobile binding, and it is additive.
// A caller that never calls it still works. statusAndroidPackage then
// takes the last element of the storage directory, which IS the package
// name on Android (/storage/emulated/0/Android/media/<package>).
func SetAndroidPackage(name string) {
	androidPackageMu.Lock()
	androidPackage = strings.TrimSpace(name)
	androidPackageMu.Unlock()
}

// lanAddressList holds what the Android layer enumerated. Go cannot read
// the addresses on a phone. java.net.NetworkInterface uses getifaddrs(),
// which an application may call. Go asks the kernel over a NETLINK_ROUTE
// socket, which Android denies to an application since Android 11.
var (
	lanAddressesMu sync.RWMutex
	lanAddressList []string
)

// SetLANAddresses records the addresses of this device, as one
// comma-separated list. ServerService.java calls it with each
// non-loopback site-local IPv4 address that it finds. The call happens
// at each build of the notification, so the notification and the Status
// page name the same addresses.
//
// One string and not a slice, because the gomobile binding carries no
// slice of strings. An empty list clears the value.
func SetLANAddresses(list string) {
	out := []string{}
	for _, part := range strings.Split(list, ",") {
		if text := strings.TrimSpace(part); text != "" {
			out = append(out, text)
		}
	}
	lanAddressesMu.Lock()
	lanAddressList = out
	lanAddressesMu.Unlock()
}

func androidLANAddresses() []string {
	lanAddressesMu.RLock()
	defer lanAddressesMu.RUnlock()
	return append([]string(nil), lanAddressList...)
}

func (a *App) statusAndroidPackage() string {
	androidPackageMu.RLock()
	name := androidPackage
	androidPackageMu.RUnlock()
	if name != "" {
		return name
	}
	base := filepath.Base(filepath.Clean(a.StorageDir))
	if strings.Contains(base, ".") {
		return base // derived, see SetAndroidPackage
	}
	return ""
}

// ----------------------------------------------------------------------
// The document
// ----------------------------------------------------------------------

type statusResponse struct {
	Generated string            `json:"generated"`
	Server    *statusServer     `json:"server,omitempty"`
	Config    *statusConfig     `json:"config,omitempty"`
	Git       *statusGit        `json:"git,omitempty"`
	Search    *statusSearch     `json:"search,omitempty"`
	Runtime   *statusRuntime    `json:"runtime,omitempty"`
	Android   *statusAndroid    `json:"android,omitempty"`
	Storage   *statusStorage    `json:"storage,omitempty"`
	GitDirty  *statusGitDirty   `json:"git_dirty,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
}

// statusServer holds no listen ADDRESS. The listener binds "::" or
// "0.0.0.0", and "[::]:8080" answers no question that a person asks. The
// port is the useful half, and share_lan with lan_urls says the rest.
type statusServer struct {
	AppVersion  string   `json:"app_version"`
	Started     string   `json:"started"`
	UptimeS     int64    `json:"uptime_s"`
	BindPort    int      `json:"bind_port"`
	ShareLAN    bool     `json:"share_lan"`
	LANURLs     []string `json:"lan_urls"`
	ActiveConns int64    `json:"active_conns"`
	Hostname    string   `json:"hostname"`
	GOOS        string   `json:"goos"`
	GOARCH      string   `json:"goarch"`
}

type statusConfig struct {
	InternalEditor    bool     `json:"internal_editor"`
	Theme             string   `json:"theme"`
	MaxUploadMB       int      `json:"max_upload_mb"`
	SearchEnabled     bool     `json:"search_enabled"`
	SearchKinds       []string `json:"search_kinds"`
	SearchScope       string   `json:"search_scope"`
	SearchBundled     bool     `json:"search_bundled"`
	IntentURI         bool     `json:"intent_uri"`
	TermuxIntent      bool     `json:"termux_intent"`
	AndroidFullscreen string   `json:"android_fullscreen"`
	BackupPruneDepth  int      `json:"backup_prune_depth"`
	Hostname          string   `json:"hostname"`
	Author            string   `json:"author"`
}

type statusGitHead struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

type statusGitRemote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type statusGit struct {
	RepoExists bool             `json:"repo_exists"`
	Configured bool             `json:"configured"`
	Branch     string           `json:"branch,omitempty"`
	Head       *statusGitHead   `json:"head,omitempty"`
	Remote     *statusGitRemote `json:"remote,omitempty"`

	// RemoteRef and RemoteHead are what the LAST sync left in this
	// repository, for example "gitserver0/master". Nothing here asks the
	// remote server. A branch that this device never fetched leaves both
	// fields absent. A reader compares head.hash with remote_head.hash to
	// see whether the two ends agree.
	RemoteRef  string         `json:"remote_ref,omitempty"`
	RemoteHead *statusGitHead `json:"remote_head,omitempty"`
}

type statusGitDirty struct {
	Dirty     bool `json:"dirty"`
	Changed   int  `json:"changed"`
	Untracked int  `json:"untracked"`
}

type statusSearch struct {
	Enabled            bool     `json:"enabled"`
	Docs               int      `json:"docs"`
	Lines              int      `json:"lines"`
	Bytes              int64    `json:"bytes"`
	IndexBytesEstimate int64    `json:"index_bytes_estimate"`
	Built              string   `json:"built,omitempty"`
	Checked            string   `json:"checked,omitempty"`
	Dirty              bool     `json:"dirty"`
	Kinds              []string `json:"kinds"`
	Scope              string   `json:"scope"`
}

type statusRuntime struct {
	GoVersion     string `json:"go_version"`
	Goroutines    int    `json:"goroutines"`
	HeapAlloc     uint64 `json:"heap_alloc"`
	Sys           uint64 `json:"sys"`
	AssetsVersion string `json:"assets_version"`
	// AssetsRefreshed tells if this start wrote a version-dependent
	// asset. It is true one time, after an update of the application.
	AssetsRefreshed bool `json:"assets_refreshed"`
}

type statusAndroid struct {
	Package     string `json:"package"`
	DefaultPort int    `json:"default_port"`
	Fullscreen  string `json:"fullscreen"`
}

// statusGroup is one counted group of files inside the storage directory.
type statusGroup struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

type statusStorage struct {
	Dir          string      `json:"dir"`
	Notes        statusGroup `json:"notes"`
	Pages        statusGroup `json:"pages"`
	Images       statusGroup `json:"images"`
	UserJSON     statusGroup `json:"user_json"`
	Databases    statusGroup `json:"databases"`
	Backups      statusGroup `json:"backups"`
	AssetBackups statusGroup `json:"asset_backups"`
	Total        statusGroup `json:"total"`
}

// ----------------------------------------------------------------------
// The handler
// ----------------------------------------------------------------------

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}

	want, unknown := parseStatusSections(r.URL.Query().Get("sections"))
	if len(unknown) > 0 {
		http.Error(w, "unknown section: "+strings.Join(unknown, ", "), http.StatusBadRequest)
		return
	}

	res := a.buildStatus(want)

	if strings.EqualFold(r.URL.Query().Get("format"), "md") {
		// text/plain, not text/markdown. A browser paints text/plain
		// everywhere, and the Android WebView paints nothing else. This
		// is the reason .jsonl is text/plain too (see builtinMIME).
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(renderStatusMarkdown(res)))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		a.logErrf(logStatus, "encode failed: %v", err)
	}
}

// statusDeniedBody is what a guest sees. A page answers with a page, like
// the file index does - not with the line of plain text that
// authMiddleware writes.
const statusDeniedBody = `<div class="config-panel">` +
	`<h2 class="config-title">Status</h2>` +
	`<p class="config-hint">This page is for the admin of this device. ` +
	`Log in as admin on a note page, then open the page again.</p>` +
	`</div>`

// serveStatusPage answers /OMNGoStatus.html. The page holds no facts of
// its own: it reads /api/status and draws what comes back. Phase 2 of the
// status work, and the reason the endpoint came first.
func (a *App) serveStatusPage(w http.ResponseWriter, r *http.Request) {
	body := statusPageTmpl
	if !a.hasRole(r, true) {
		body = statusDeniedBody
	}
	compiled := a.compilePageWithBody("Status",
		[]byte("Title: Status\nCategory: System\n\n"), body)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(a.injectRuntimeVars(compiled))
}

// parseStatusSections turns the "sections" parameter into a set. Empty
// means the cheap sections. "all" means every section, the slow ones
// included. An unknown name is an error rather than a silent omission:
// a caller that asks for "sarch" must hear about it.
func parseStatusSections(raw string) (want map[string]bool, unknown []string) {
	want = map[string]bool{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		for _, s := range statusCheapSections {
			want[s] = true
		}
		return want, nil
	}
	known := map[string]bool{}
	for _, s := range append(append([]string{}, statusCheapSections...), statusSlowSections...) {
		known[s] = true
	}
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		switch {
		case name == "":
		case name == "all":
			for s := range known {
				want[s] = true
			}
		case known[name]:
			want[name] = true
		default:
			unknown = append(unknown, name)
		}
	}
	return want, unknown
}

func (a *App) buildStatus(want map[string]bool) *statusResponse {
	cfg := a.GetConfig()
	res := &statusResponse{Generated: statusTime(time.Now())}
	fail := func(section string, err error) {
		if res.Errors == nil {
			res.Errors = map[string]string{}
		}
		res.Errors[section] = err.Error()
	}

	if want["server"] {
		res.Server = a.statusServerSection(cfg)
	}
	if want["config"] {
		res.Config = statusConfigSection(cfg)
	}
	if want["git"] {
		// Not named "git": that is the package this file imports.
		section, err := a.statusGitSection(cfg)
		res.Git = section
		if err != nil {
			fail("git", err)
		}
	}
	if want["search"] {
		res.Search = a.statusSearchSection(cfg)
	}
	if want["runtime"] {
		res.Runtime = a.statusRuntimeSection()
	}
	if want["android"] && runtime.GOOS == "android" {
		res.Android = &statusAndroid{
			Package:     a.statusAndroidPackage(),
			DefaultPort: a.fallbackPort(),
			Fullscreen:  cfg.AndroidFullscreen,
		}
	}
	if want["storage"] {
		storage, err := a.statusStorageSection()
		res.Storage = storage
		if err != nil {
			fail("storage", err)
		}
	}
	if want["git_dirty"] {
		dirty, err := a.statusGitDirtySection()
		res.GitDirty = dirty
		if err != nil {
			fail("git_dirty", err)
		}
	}
	return res
}

// ----------------------------------------------------------------------
// Sections
// ----------------------------------------------------------------------

func (a *App) statusServerSection(cfg Config) *statusServer {
	_, portStr, addr := a.boundAddress()
	port, _ := strconv.Atoi(portStr)
	if addr == "" {
		// Asked before the listener came up. The config then answers.
		port = cfg.ServerPort
	}

	hostname := sanitizeHostname(cfg.Hostname)
	if hostname == "" {
		hostname = defaultHostname()
	}

	s := &statusServer{
		AppVersion:  APP_VERSION,
		Started:     statusTime(a.startedAt),
		UptimeS:     int64(time.Since(a.startedAt).Seconds()),
		BindPort:    port,
		ShareLAN:    cfg.ShareLAN,
		LANURLs:     []string{},
		ActiveConns: a.ActiveConnCount(),
		Hostname:    hostname,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}
	if cfg.ShareLAN {
		s.LANURLs = lanURLs(port)
	}
	return s
}

func statusConfigSection(cfg Config) *statusConfig {
	kinds := cfg.SearchKinds
	if kinds == nil {
		kinds = []string{}
	}
	return &statusConfig{
		InternalEditor:    cfg.UseInternalEd,
		Theme:             cfg.Theme,
		MaxUploadMB:       cfg.MaxUploadSizeMB,
		SearchEnabled:     cfg.SearchEnabled,
		SearchKinds:       kinds,
		SearchScope:       cfg.SearchScope,
		SearchBundled:     cfg.SearchBundled,
		IntentURI:         cfg.EnableIntentURI,
		TermuxIntent:      cfg.EnableTermuxIntent,
		AndroidFullscreen: cfg.AndroidFullscreen,
		BackupPruneDepth:  cfg.BackupPruneDepth,
		Hostname:          cfg.Hostname,
		Author:            cfg.Author,
	}
}

// statusGitSection reads HEAD without touching the repository. An install
// that never synced has no .git at all, which is an answer, not an error.
func (a *App) statusGitSection(cfg Config) (*statusGit, error) {
	out := &statusGit{}

	if idx := cfg.ActiveGitIndex; idx >= 0 && idx < len(cfg.GitServers) {
		slot := cfg.GitServers[idx]
		if strings.TrimSpace(slot.URL) != "" {
			out.Configured = true
			name := strings.TrimSpace(slot.Name)
			if name == "" {
				name = slotRemoteName(idx)
			}
			out.Remote = &statusGitRemote{Name: name, URL: redactGitURL(slot.URL)}
		}
	}

	repo, err := a.openRepoReadOnly()
	if err != nil {
		return out, nil // no repository on disk
	}
	out.RepoExists = true

	head, err := repo.Head()
	if err != nil {
		return out, nil // a repository with no commit yet
	}
	if head.Name().IsBranch() {
		out.Branch = head.Name().Short()
	}
	out.Head = commitSummary(repo, head.Hash())

	// The remote-tracking ref: what the last pull or push wrote into this
	// repository for the branch of HEAD. This is a local read of a local
	// file. It contacts no server, so it costs nothing and it can be old.
	for _, remote := range remoteRefCandidates(cfg, out.Branch) {
		refName := plumbing.NewRemoteReferenceName(remote, out.Branch)
		ref, err := repo.Reference(refName, true)
		if err != nil || ref == nil {
			continue
		}
		out.RemoteRef = remote + "/" + out.Branch
		out.RemoteHead = commitSummary(repo, ref.Hash())
		break
	}
	return out, nil
}

// remoteRefCandidates names the git remotes to look in, most specific
// first. ensureSlotRemotes gives each configured server slot a remote of
// its own ("gitserver0"), and "origin" is the bootstrap remote that an
// older installation carries. An empty branch (a detached HEAD) has no
// remote-tracking ref at all.
func remoteRefCandidates(cfg Config, branch string) []string {
	if branch == "" {
		return nil
	}
	out := []string{}
	if idx := cfg.ActiveGitIndex; idx >= 0 && idx < len(cfg.GitServers) &&
		strings.TrimSpace(cfg.GitServers[idx].URL) != "" {
		out = append(out, slotRemoteName(idx))
	}
	return append(out, "origin")
}

// commitSummary reads one commit and reports it. A hash whose object is
// not in this repository still gives the hash, because the hash is the
// answer that the caller asked for.
func commitSummary(repo *git.Repository, h plumbing.Hash) *statusGitHead {
	hash := h.String()
	short := hash
	if len(short) > 7 {
		short = short[:7]
	}
	out := &statusGitHead{Hash: hash, Short: short}
	commit, err := repo.CommitObject(h)
	if err != nil {
		return out
	}
	out.Subject = strings.TrimSpace(strings.SplitN(commit.Message, "\n", 2)[0])
	out.Author = commit.Author.Name
	out.Date = statusTime(commit.Author.When)
	return out
}

// statusGitDirtySection is the slow half of the git answer. go-git hashes
// each tracked file to find it. One log line goes out first. The page
// then has something to show under its progress bar, because the
// /api/logs stream carries that line like it carries the sync stages.
func (a *App) statusGitDirtySection() (*statusGitDirty, error) {
	repo, err := a.openRepoReadOnly()
	if err != nil {
		return &statusGitDirty{}, nil
	}
	wTree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("open worktree: %v", err)
	}

	a.logDebugf(logStatus, "Reading the git worktree state")
	started := time.Now()
	st, err := wTree.Status()
	if err != nil {
		return nil, fmt.Errorf("worktree status: %v", err)
	}

	out := &statusGitDirty{}
	for _, fileStat := range st {
		if fileStat.Worktree == git.Untracked {
			out.Untracked++
			continue
		}
		if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			out.Changed++
		}
	}
	out.Dirty = out.Changed > 0
	a.logInfof(logStatus, "Worktree read in %s: %d changed, %d untracked",
		time.Since(started).Round(time.Millisecond), out.Changed, out.Untracked)
	return out, nil
}

func (a *App) statusSearchSection(cfg Config) *statusSearch {
	out := &statusSearch{
		Enabled: cfg.SearchEnabled,
		Scope:   cfg.SearchScope,
		Kinds:   cfg.SearchKinds,
	}
	if out.Kinds == nil {
		out.Kinds = []string{}
	}
	if a.search == nil {
		return out
	}

	a.search.mu.RLock()
	defer a.search.mu.RUnlock()

	out.Docs = len(a.search.docs)
	out.Lines = a.search.lines
	out.Bytes = a.search.bytes
	out.Dirty = a.search.dirty
	if !a.search.built.IsZero() {
		out.Built = statusTime(a.search.built)
	}
	if !a.search.checked.IsZero() {
		out.Checked = statusTime(a.search.checked)
	}

	// An ESTIMATE, and the field name says so. Go cannot report the true
	// size of a live object graph. This counts what the index holds: one
	// 8-byte mask for each indexed line, the 64-byte trigram signature,
	// and the strings. A flat allowance covers the structure and its map
	// entry.
	const perDocOverhead = 160
	var est int64
	for path, doc := range a.search.docs {
		est += int64(perDocOverhead + len(path))
		est += int64(len(doc.Path) + len(doc.Kind) + len(doc.Name) + len(doc.Title) + len(doc.URL))
		for _, t := range doc.Tags {
			est += int64(len(t) + 16)
		}
		est += int64(8 * len(doc.LineMasks))
		est += 8 + 64 // FieldMask + Tri
	}
	out.IndexBytesEstimate = est
	return out
}

func (a *App) statusRuntimeSection() *statusRuntime {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	stamp := ""
	if raw, err := os.ReadFile(filepath.Join(a.StorageDir, assetsVersionFilename)); err == nil {
		stamp = strings.TrimSpace(string(raw))
	}
	return &statusRuntime{
		GoVersion:       runtime.Version(),
		Goroutines:      runtime.NumGoroutine(),
		HeapAlloc:       mem.HeapAlloc,
		Sys:             mem.Sys,
		AssetsVersion:   stamp,
		AssetsRefreshed: AssetsRefreshed(),
	}
}

// statusStorageSection walks the storage directory ONE time and classifies
// each file by where it sits. One pass, because a walk of the note tree on
// a phone is the whole cost of this section.
func (a *App) statusStorageSection() (*statusStorage, error) {
	out := &statusStorage{Dir: a.StorageDir}
	add := func(g *statusGroup, size int64) {
		g.Files++
		g.Bytes += size
	}

	err := filepath.WalkDir(a.StorageDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable corner must not fail the whole answer
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir // the object store is git's business
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size := info.Size()
		add(&out.Total, size)

		rel, err := filepath.Rel(a.StorageDir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := strings.ToLower(path.Base(rel))
		switch {
		case strings.HasPrefix(rel, "md/") && strings.HasSuffix(name, ".md"):
			add(&out.Notes, size)
		case strings.HasPrefix(rel, "html/db_backup/"):
			add(&out.Backups, size)
		case strings.HasPrefix(rel, "html/images/"):
			add(&out.Images, size)
		case strings.HasPrefix(rel, "html/user_json/"):
			add(&out.UserJSON, size)
		case strings.HasPrefix(rel, "html/") && strings.HasSuffix(name, ".html"):
			add(&out.Pages, size)
		case strings.HasPrefix(rel, "db/") && strings.HasSuffix(name, ".sqlite"):
			add(&out.Databases, size)
		case strings.HasPrefix(rel, "asset_backups/"):
			add(&out.AssetBackups, size)
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("walk storage: %v", err)
	}
	return out, nil
}

// ----------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------

// statusTime is the one time format of this endpoint: RFC3339 in UTC, the
// same shape the backup files carry.
func statusTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// lanURLs lists the addresses that another device on the network can
// open. Loopback and link-local are dropped. The first is reachable from
// nowhere else, and the second needs a zone index that no URL carries.
//
// The address of the default route comes first, because that is the one
// a phone or a laptop on the same network uses. A desktop can carry
// several more (a docker bridge, a virtual machine bridge), and those
// follow it in sorted order.
func lanURLs(port int) []string {
	usable := func(ip net.IP) bool {
		return ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() &&
			!ip.IsLinkLocalMulticast() && ip.IsGlobalUnicast()
	}
	format := func(ip net.IP) string {
		host := ip.String()
		if ip.To4() == nil {
			host = "[" + host + "]"
		}
		return fmt.Sprintf("http://%s:%d", host, port)
	}

	out := []string{}
	seen := map[string]bool{}
	add := func(ip net.IP) {
		if !usable(ip) {
			return
		}
		url := format(ip)
		if seen[url] {
			return
		}
		seen[url] = true
		out = append(out, url)
	}

	// 1. The address of the default route, read now. A phone or a laptop
	// on the same network opens this one, so it comes first.
	add(defaultRouteIP())

	// 2. What the Android layer enumerated (see SetLANAddresses). It can
	// be older than the answer above, because ServerService writes it
	// when it builds the notification.
	for _, text := range androidLANAddresses() {
		add(net.ParseIP(text))
	}

	// 3. Each interface address that this process can read. This is the
	// desktop path, and it adds the bridges of a docker or a virtual
	// machine setup after the address of the network.
	rest := []string{}
	restSeen := map[string]bool{}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || !usable(ipNet.IP) {
				continue
			}
			url := format(ipNet.IP)
			if seen[url] || restSeen[url] {
				continue
			}
			restSeen[url] = true
			rest = append(rest, url)
		}
	}
	sort.Strings(rest)
	for _, url := range rest {
		seen[url] = true
		out = append(out, url)
	}
	return out
}

// defaultRouteIP reports the address of the interface that carries the
// default route, or nil.
//
// It exists for Android. net.InterfaceAddrs asks the kernel through
// NETLINK, and Android denies NETLINK_ROUTE to an application since
// Android 11. The call therefore fails on a phone, and the list of LAN
// addresses came back empty on the one platform where a user needs it.
//
// A UDP "connection" sends no packet. The kernel only selects the route
// and gives the local address of it, which is the address that another
// device reaches this server on.
func defaultRouteIP() net.IP {
	for _, target := range []string{"8.8.8.8:53", "192.168.1.1:9"} {
		conn, err := net.Dial("udp4", target)
		if err != nil {
			continue
		}
		addr, ok := conn.LocalAddr().(*net.UDPAddr)
		conn.Close()
		if ok && addr.IP != nil && !addr.IP.IsUnspecified() {
			return addr.IP
		}
	}
	return nil
}

// redactGitURL removes the password from a remote URL. A user name stays:
// it is part of how the remote is addressed, and it is not a secret. An
// address this function cannot parse is reported as "(hidden)" rather
// than as itself.
func redactGitURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		return raw // scp form, "git@host:path" - it carries no password
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "(hidden)"
	}
	if u.User != nil {
		if name := u.User.Username(); name != "" {
			u.User = url.User(name)
		} else {
			u.User = nil
		}
	}
	return u.String()
}

// openRepoReadOnly opens the storage repository and creates nothing. It
// uses the same filesystem wrapping as getOrInitRepo (git_helper.go), so
// both see one worktree. With no repository on disk it returns an error.
// It never initializes one.
func (a *App) openRepoReadOnly() (*git.Repository, error) {
	baseFS := osfs.New(a.StorageDir)
	wtFS := &NoLockFS{&stableMtimeFS{baseFS}}
	dotFS, err := wtFS.Chroot(".git")
	if err != nil {
		return nil, err
	}
	storer := filesystem.NewStorage(dotFS, cache.NewObjectLRUDefault())
	return git.Open(storer, wtFS)
}

// ----------------------------------------------------------------------
// Markdown rendering (?format=md)
// ----------------------------------------------------------------------

func renderStatusMarkdown(res *statusResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# OMN-Go status\n\nGenerated: %s\n", res.Generated)

	table := func(title string, rows [][2]string) {
		fmt.Fprintf(&b, "\n## %s\n\n| Field | Value |\n| --- | --- |\n", title)
		for _, row := range rows {
			fmt.Fprintf(&b, "| %s | %s |\n", row[0], row[1])
		}
	}
	yes := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}

	if s := res.Server; s != nil {
		table("Server", [][2]string{
			{"app_version", s.AppVersion},
			{"started", s.Started},
			{"uptime_s", strconv.FormatInt(s.UptimeS, 10)},
			{"bind_port", strconv.Itoa(s.BindPort)},
			{"share_lan", yes(s.ShareLAN)},
			{"lan_urls", strings.Join(s.LANURLs, ", ")},
			{"active_conns", strconv.FormatInt(s.ActiveConns, 10)},
			{"hostname", s.Hostname},
			{"goos", s.GOOS},
			{"goarch", s.GOARCH},
		})
	}
	if c := res.Config; c != nil {
		table("Config", [][2]string{
			{"internal_editor", yes(c.InternalEditor)},
			{"theme", c.Theme},
			{"max_upload_mb", strconv.Itoa(c.MaxUploadMB)},
			{"search_enabled", yes(c.SearchEnabled)},
			{"search_kinds", strings.Join(c.SearchKinds, ", ")},
			{"search_scope", c.SearchScope},
			{"search_bundled", yes(c.SearchBundled)},
			{"intent_uri", yes(c.IntentURI)},
			{"termux_intent", yes(c.TermuxIntent)},
			{"android_fullscreen", c.AndroidFullscreen},
			{"backup_prune_depth", strconv.Itoa(c.BackupPruneDepth)},
			{"hostname", c.Hostname},
			{"author", c.Author},
		})
	}
	if g := res.Git; g != nil {
		rows := [][2]string{
			{"repo_exists", yes(g.RepoExists)},
			{"configured", yes(g.Configured)},
			{"branch", g.Branch},
		}
		if g.Head != nil {
			rows = append(rows,
				[2]string{"head_hash", g.Head.Hash},
				[2]string{"head_short", g.Head.Short},
				[2]string{"head_subject", g.Head.Subject},
				[2]string{"head_author", g.Head.Author},
				[2]string{"head_date", g.Head.Date})
		}
		if g.Remote != nil {
			rows = append(rows,
				[2]string{"remote_name", g.Remote.Name},
				[2]string{"remote_url", g.Remote.URL})
		}
		if g.RemoteRef != "" {
			rows = append(rows, [2]string{"remote_ref", g.RemoteRef})
		}
		if g.RemoteHead != nil {
			rows = append(rows,
				[2]string{"remote_head_hash", g.RemoteHead.Hash},
				[2]string{"remote_head_short", g.RemoteHead.Short},
				[2]string{"remote_head_subject", g.RemoteHead.Subject},
				[2]string{"remote_head_author", g.RemoteHead.Author},
				[2]string{"remote_head_date", g.RemoteHead.Date})
		}
		table("Git", rows)
	}
	if d := res.GitDirty; d != nil {
		table("Git worktree", [][2]string{
			{"dirty", yes(d.Dirty)},
			{"changed", strconv.Itoa(d.Changed)},
			{"untracked", strconv.Itoa(d.Untracked)},
		})
	}
	if s := res.Search; s != nil {
		table("Search", [][2]string{
			{"enabled", yes(s.Enabled)},
			{"docs", strconv.Itoa(s.Docs)},
			{"lines", strconv.Itoa(s.Lines)},
			{"bytes", strconv.FormatInt(s.Bytes, 10)},
			{"index_bytes_estimate", strconv.FormatInt(s.IndexBytesEstimate, 10)},
			{"built", s.Built},
			{"checked", s.Checked},
			{"dirty", yes(s.Dirty)},
			{"kinds", strings.Join(s.Kinds, ", ")},
			{"scope", s.Scope},
		})
	}
	if rt := res.Runtime; rt != nil {
		table("Runtime", [][2]string{
			{"go_version", rt.GoVersion},
			{"goroutines", strconv.Itoa(rt.Goroutines)},
			{"heap_alloc", strconv.FormatUint(rt.HeapAlloc, 10)},
			{"sys", strconv.FormatUint(rt.Sys, 10)},
			{"assets_version", rt.AssetsVersion},
			{"assets_refreshed", yes(rt.AssetsRefreshed)},
		})
	}
	if an := res.Android; an != nil {
		table("Android", [][2]string{
			{"package", an.Package},
			{"default_port", strconv.Itoa(an.DefaultPort)},
			{"fullscreen", an.Fullscreen},
		})
	}
	if st := res.Storage; st != nil {
		fmt.Fprintf(&b, "\n## Storage\n\nDirectory: `%s`\n\n| Group | Files | Bytes |\n| --- | --- | --- |\n", st.Dir)
		for _, g := range [][2]any{
			{"notes", st.Notes}, {"pages", st.Pages}, {"images", st.Images},
			{"user_json", st.UserJSON}, {"databases", st.Databases},
			{"backups", st.Backups}, {"asset_backups", st.AssetBackups},
			{"total", st.Total},
		} {
			grp := g[1].(statusGroup)
			fmt.Fprintf(&b, "| %s | %d | %d |\n", g[0], grp.Files, grp.Bytes)
		}
	}
	if len(res.Errors) > 0 {
		rows := [][2]string{}
		names := make([]string, 0, len(res.Errors))
		for k := range res.Errors {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			rows = append(rows, [2]string{k, res.Errors[k]})
		}
		table("Errors", rows)
	}
	return b.String()
}
