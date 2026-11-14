package status

import (
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/render"
)

// ----------------------------------------------------------------------
// GET /api/status: what this build does now
// ----------------------------------------------------------------------
//
// One endpoint answers the questions of a fault report. Which address does
// the server listen on? Which commit are the notes at? How large is the
// search index? Which Android package runs? The Status page reads this
// endpoint, and it has no second source.
//
// ADMIN ONLY. The answer holds LAN addresses, absolute paths and a commit
// subject. hasRole treats a local connection as the owner, thus the Android
// WebView and a desktop browser need no login.
//
// Two sections cost real work, and the default answer NEVER holds them.
// "storage" walks the storage directory, and "git_dirty" walks the worktree.
// A caller names them. The page thus shows the cheap facts at once, with a
// progress bar for the slow request.
//
// Nothing here opens a network connection or writes a file. The git section
// opens the repository read-only, because gitsync.Service.GetOrInitRepo would
// CREATE one.

// statusCheapSections is the default answer. A caller must name each of
// statusSlowSections in "sections", or ask for "all".
var (
	statusCheapSections = []string{"server", "config", "git", "search", "runtime", "android"}
	statusSlowSections  = []string{"storage", "git_dirty"}
)

// Android holds the facts that only the Android layer knows. The Go
// runtime cannot ask Android for them.
//
//   - pkg is the applicationId, net.basov.omngo or net.basov.omngo.fdroid.
//   - addresses are the LAN addresses of the device.
//     java.net.NetworkInterface uses getifaddrs(), which an app can call. Go
//     asks the kernel over a NETLINK_ROUTE socket, and Android 11 and later
//     deny that to an app.
//
// SetAndroidPackage and SetLANAddresses in server.go write these facts.
type Android struct {
	mu        sync.RWMutex
	pkg       string
	addresses []string
}

func (e *Android) SetPackage(name string) {
	e.mu.Lock()
	e.pkg = strings.TrimSpace(name)
	e.mu.Unlock()
}

func (e *Android) SetAddresses(list []string) {
	e.mu.Lock()
	e.addresses = append([]string(nil), list...)
	e.mu.Unlock()
}

func (e *Android) PackageName() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.pkg
}

// LANAddresses answers a copy, thus a caller can keep it.
func (e *Android) LANAddresses() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]string(nil), e.addresses...)
}

// AndroidPackage answers the applicationId. Without SetAndroidPackage,
// it takes the last element of the storage directory, which IS the package
// name on Android.
func (svc Service) AndroidPackage() string {
	if name := svc.Android.PackageName(); name != "" {
		return name
	}
	base := filepath.Base(filepath.Clean(string(svc.Layout)))
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
	Config    statusConfig      `json:"config,omitempty"`
	Git       *statusGit        `json:"git,omitempty"`
	Search    *statusSearch     `json:"search,omitempty"`
	Runtime   *statusRuntime    `json:"runtime,omitempty"`
	Android   *statusAndroid    `json:"android,omitempty"`
	Storage   *statusStorage    `json:"storage,omitempty"`
	GitDirty  *statusGitDirty   `json:"git_dirty,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
}

// statusServer holds no listen ADDRESS. The listener binds "::" or "0.0.0.0",
// and "[::]:8080" answers no question. The port is the useful half, and
// share_lan with lan_urls tells the rest.
// build_commit is the commit of the build. The Git section is the git sync.
type statusServer struct {
	AppVersion      string   `json:"app_version"`
	BuildCommit     string   `json:"build_commit"`
	BuildCommitTime string   `json:"build_commit_time"`
	BuildCommitURL  string   `json:"build_commit_url,omitempty"`
	Started         string   `json:"started"`
	UptimeS         int64    `json:"uptime_s"`
	BindPort        int      `json:"bind_port"`
	ShareLAN        bool     `json:"share_lan"`
	LANURLs         []string `json:"lan_urls"`
	ActiveConns     int64    `json:"active_conns"`
	Hostname        string   `json:"hostname"`
	GOOS            string   `json:"goos"`
	GOARCH          string   `json:"goarch"`
}

// statusConfig is the config section, in the order of the config table.
type statusConfig []config.StatusValue

func (c statusConfig) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, v := range c {
		if i > 0 {
			buf = append(buf, ',')
		}
		k, err := json.Marshal(v.Key)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(v.Value)
		if err != nil {
			return nil, err
		}
		buf = append(append(append(buf, k...), ':'), val...)
	}
	return append(buf, '}'), nil
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
	// repository, for example "gitserver0/master". Nothing asks the remote
	// server. Compare head.hash with remote_head.hash to see whether the two
	// ends agree.
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
	// AssetsRefreshed tells whether this start wrote a version-dependent
	// file. It is true one time after an update.
	AssetsRefreshed bool `json:"assets_refreshed"`
}

type statusAndroid struct {
	Package     string `json:"package"`
	DefaultPort int    `json:"default_port"`
	Fullscreen  string `json:"fullscreen"`
}

// statusGroup is one counted group of files in the storage directory.
type statusGroup struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

type statusStorage struct {
	Dir           string      `json:"dir"`
	Notes         statusGroup `json:"notes"`
	Pages         statusGroup `json:"pages"`
	Images        statusGroup `json:"images"`
	UserJSON      statusGroup `json:"user_json"`
	UserContacts  statusGroup `json:"user_contacts"`
	UserCalendars statusGroup `json:"user_calendars"`
	Databases     statusGroup `json:"databases"`
	Backups       statusGroup `json:"backups"`
	AssetBackups  statusGroup `json:"asset_backups"`
	Total         statusGroup `json:"total"`
}

// ----------------------------------------------------------------------
// The handler
// ----------------------------------------------------------------------

func (svc Service) HandleStatus(w http.ResponseWriter, r *http.Request) {

	want, unknown := parseStatusSections(r.URL.Query().Get("sections"))
	if len(unknown) > 0 {
		svc.writeJSON(w, http.StatusBadRequest, render.JSONStatus{
			Status:  "error",
			Message: "unknown section: " + strings.Join(unknown, ", "),
		})
		return
	}

	res := svc.buildStatus(want)

	if strings.EqualFold(r.URL.Query().Get("format"), "md") {
		// Answer text/plain and not text/markdown, because the Android
		// WebView shows only text/plain. For the same reason, config.BuiltinMIME
		// serves .jsonl as text/plain.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(renderStatusMarkdown(res)))
		return
	}

	svc.writeJSON(w, http.StatusOK, res)
}

var statusPageTmpl = render.LoadTemplate("status_page.html")

// ServeStatusPage answers /OMNGoStatus.html. The page reads /api/status and
// shows the answer. It holds no facts of its own.
func (svc Service) ServeStatusPage(w http.ResponseWriter, r *http.Request) {
	svc.RenderPage(w, http.StatusOK, "Status", render.PageHeader("Status", "System", "css/OMN-Go/omn-go-status.css"), statusPageTmpl)
}

// parseStatusSections changes the "sections" parameter into a set. Empty
// means the cheap sections, and "all" means each section. An unknown name is
// an error, thus a caller who asks for "sarch" knows it.
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

func (svc Service) buildStatus(want map[string]bool) *statusResponse {
	cfg := svc.Config
	res := &statusResponse{Generated: statusTime(time.Now())}
	fail := func(section string, err error) {
		if res.Errors == nil {
			res.Errors = map[string]string{}
		}
		res.Errors[section] = err.Error()
	}

	if want["server"] {
		res.Server = svc.statusServerSection(cfg)
	}
	if want["config"] {
		res.Config = statusConfigSection(cfg)
	}
	if want["git"] {
		// The name is not "git", because this file imports that package.
		section, err := svc.statusGitSection(cfg)
		res.Git = section
		if err != nil {
			fail("git", err)
		}
	}
	if want["search"] {
		res.Search = svc.statusSearchSection(cfg)
	}
	if want["runtime"] {
		res.Runtime = svc.statusRuntimeSection()
	}
	if want["android"] && runtime.GOOS == "android" {
		res.Android = &statusAndroid{
			Package:     svc.AndroidPackage(),
			DefaultPort: svc.FallbackPort,
			Fullscreen:  cfg.AndroidFullscreen,
		}
	}
	if want["storage"] {
		storage, err := svc.statusStorageSection()
		res.Storage = storage
		if err != nil {
			fail("storage", err)
		}
	}
	if want["git_dirty"] {
		dirty, err := svc.statusGitDirtySection()
		res.GitDirty = dirty
		if err != nil {
			fail("git_dirty", err)
		}
	}
	return res
}

// boundAddress reports the address of the listener as host, port and the
// joined form. Each value is empty before the bind.
func (svc Service) boundAddress() (host, port, addr string) {
	addr = svc.BoundAddr
	if addr == "" {
		return "", "", ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", addr
	}
	return host, port, addr
}
