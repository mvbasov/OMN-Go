package backend

import (
	"database/sql"
	"embed"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// App encapsulates the global state for the backend
type App struct {
	Config      Config
	ConfigMutex sync.RWMutex // guards all reads/writes of Config
	StorageDir  string
	// ActiveConns is atomic.Int64 and NOT a bare int64 behind
	// atomic.AddInt64, which is what it was until 26.08.51. On a 32-bit
	// build every single request panicked with "unaligned 64-bit atomic
	// operation" - a 64-bit atomic needs an 8-byte-aligned address, a
	// 32-bit struct only aligns to 4, and the Go rule that saves you
	// ("the first word of an allocated struct is 64-bit aligned") did
	// not apply to a field sitting after Config and a RWMutex.
	//
	// connectionMiddleware wraps every route, so the very first request
	// died: nothing ever worked on armeabi-v7a or x86, which is the APK
	// F-Droid publishes. arm64 has natural 8-byte alignment and hid it.
	//
	// atomic.Int64 carries its own alignment guarantee, so the field can
	// sit anywhere in this struct and stay correct. Do not turn it back
	// into an int64 to save a line - see TestNoBare64BitAtomics.
	ActiveConns atomic.Int64
	GitMutex    sync.Mutex // serializes all on-disk git repo operations
	Router      *http.ServeMux

	sqlMu  sync.Mutex         // guards sqlDBs (see sqlite.go)
	sqlDBs map[string]*sql.DB // lazily-opened user SQLite handles, by name

	// serializes database restore/swap operations (manual restores and
	// the fresh-device bootstrap; see db_backup.go). Never taken while
	// sqlMu is held.
	dbRestoreMu sync.Mutex

	// search is the global search index (see search_index.go). Non-nil from
	// startup, but empty until global search is switched on - the memory
	// belongs to the documents, not to the struct.
	search *searchIndex

	// defaultPort is the per-flavor fallback StartServer was given: the port
	// to use when config.json does not name one. 0 means "the historical
	// 8080". It is read by loadConfig, which is the only place that can
	// honour it - see fallbackPort.
	defaultPort int

	ready chan struct{} // closed once the HTTP listener is actually serving

	// startedAt is when StartServer ran. boundAddr is what the listener
	// bound, not what the config asked for. /api/status reports both
	// (status.go). Until now only a log line carried the address, and the
	// retry loop below can end on another port than the configured one.
	// metaMu guards boundAddr. The goroutine that binds writes it, and
	// request goroutines read it.
	metaMu    sync.RWMutex
	startedAt time.Time
	boundAddr string
}

// boundAddress reports the address the HTTP listener is actually on, as
// host, port and the joined form. Every value is empty before the bind
// (see statusServerSection, which falls back to the config then).
func (a *App) boundAddress() (host, port, addr string) {
	a.metaMu.RLock()
	addr = a.boundAddr
	a.metaMu.RUnlock()
	if addr == "" {
		return "", "", ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", addr
	}
	return host, port, addr
}

func (a *App) setBoundAddress(addr string) {
	a.metaMu.Lock()
	a.boundAddr = addr
	a.metaMu.Unlock()
}

// fallbackPort is the port to use when config.json has nothing usable to say.
//
// This exists because the per-flavor default has to be applied by the CONFIG
// LOADER, not after it. loadConfig writes a full default config.json on a fresh
// install and fills in a port for one that lacks it; by the time StartServer
// resumed, a.Config.ServerPort was already a positive 8080 and any
// caller-supplied default was unreachable - and, worse, the 8080 had been
// persisted, so the wrong port stuck for the life of the install. That was the
// fdroid flavor's DEFAULT_SERVER_PORT=8081 never taking effect.
func (a *App) fallbackPort() int {
	if a.defaultPort > 0 {
		return a.defaultPort
	}
	return 8080
}

// GetConfig returns a copy of the current config, safe for concurrent reads.
func (a *App) GetConfig() Config {
	a.ConfigMutex.RLock()
	defer a.ConfigMutex.RUnlock()
	return a.Config
}

// WithConfig runs fn while holding the config write lock. Use for
// read-modify-write updates (e.g. handleConfig's POST handler).
func (a *App) WithConfig(fn func(c *Config)) {
	a.ConfigMutex.Lock()
	defer a.ConfigMutex.Unlock()
	fn(&a.Config)
}

// WaitUntilReady blocks until the HTTP server has actually started
// listening. Replaces the previous fixed time.Sleep(500ms) hack that used
// to live in main_desktop.go.
func (a *App) WaitUntilReady() {
	<-a.ready
}

//go:embed frontend/html frontend/md
var staticFS embed.FS

// templatesFS holds internal server-rendered page fragments (the Config
// dashboard, the "editing externally" wait page, ...). These are
// deliberately embedded separately from staticFS: staticFS's frontend/html
// tree is lazily extracted to StorageDir/html on first request (see
// serveLazyEmbed / serveStaticAsset) and is treated as user-editable
// content that a person can open with ?edit=true and overwrite. Templates
// are neither - they are Go-side rendering logic, not user content - so
// mixing them into frontend/html would both let a user "edit" and corrupt
// them and require excluding them from every static-file listing by hand.
//
//go:embed frontend/templates
var templatesFS embed.FS

// StartServer boots the Go backend. storageDir, when non-empty, overrides
// the runtime.GOOS-based default initStorage would otherwise compute (see
// backend/storage.go) - Android passes its actual per-flavor external
// media directory here (see ServerService.storageDir in
// android/.../ServerService.java), since the Go runtime has no way to
// learn the running app's applicationId (net.basov.omngo vs
// net.basov.omngo.fdroid) on its own. Every other caller (desktop's
// main_desktop.go) passes "" and gets the existing default unchanged.
//
// defaultPort, when > 0, is used as the server port whenever config.json
// does not carry a (positive) server_port of its own - the same
// "flavor knows, this package cannot" reasoning as storageDir: the
// standard and fdroid Android flavors are installable side by side, so
// they must not compete for the same default loopback port (see
// DEFAULT_SERVER_PORT in android/app/build.gradle). Pass 0 to keep the
// historical default of 8080 (desktop does).
func StartServer(storageDir string, defaultPort int) *App {
	a := &App{
		Router:    http.NewServeMux(),
		ready:     make(chan struct{}),
		startedAt: time.Now(),
	}

	// Set BEFORE initStorage: that is what loads (and, on a fresh install,
	// writes) config.json, and it is the only place the per-flavor default can
	// still be applied.
	a.defaultPort = defaultPort

	a.initStorage(storageDir) // Execute synchronously to ensure config is loaded instantly

	// Content-type resolution now lives in one place, resolveContentType
	// (serving.go), which carries the canonical table these startup
	// mime.AddExtensionType(...) calls used to seed - so they are gone.

	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logErrf(logServer, "Recovered from panic in server: %v", r)
			}
		}()

		// Initialize logger to stream Go logs to the frontend via SSE
		a.InitLoggerAndRoute()
		a.Router.HandleFunc("/", a.serveFrontend)

		// The /js|/css|/json trees are embedded assets, lazily extracted and
		// served (and ?edit-able) by the one shared asset handler in
		// serving.go; the root catch-all (serveFrontend -> serveStaticAsset)
		// funnels into the same serveEmbeddableAsset.
		assetTree := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a.serveEmbeddableAsset(w, r, r.URL.Path)
		})
		a.Router.Handle("/js/", assetTree)
		a.Router.Handle("/css/", assetTree)
		a.Router.Handle("/json/", assetTree)

		// /images and /user_json are pure user content (never embedded),
		// served straight from their storage subdirectory. Both resolve the
		// content-type per file so /user_json serves .json as application/json
		// and .jsonl as text/plain (see resolveContentType).
		a.Router.Handle("/images/", a.serveStorageSubdir("images", ""))
		a.Router.Handle("/user_json/", a.serveStorageSubdir("user_json", ""))

		a.Router.HandleFunc("/login", a.handleLogin)
		a.Router.HandleFunc("/api/quick", a.authMiddleware(a.handleQuickNote, true))
		a.Router.HandleFunc("/api/bookmark", a.authMiddleware(a.handleBookmark, true))
		a.Router.HandleFunc("/api/upload", a.authMiddleware(a.handleUpload, true))
		a.Router.HandleFunc("/api/upload_json", a.authMiddleware(a.handleUploadJSON, true))
		a.Router.HandleFunc("/api/note", a.handleGetNote)
		// Registered WITHOUT authMiddleware, like /api/note directly above and
		// every page and static route: search aggregates nothing a LAN guest
		// could not already fetch file by file, so gating it would buy no
		// confidentiality while breaking the guest experience.
		a.Router.HandleFunc("/api/search", a.handleSearch)
		a.Router.HandleFunc("/api/save", a.authMiddleware(a.handleSaveNote, true))
		a.Router.HandleFunc("/api/newpage", a.authMiddleware(a.handleNewPage, true))
		a.Router.HandleFunc("/api/config", a.authMiddleware(a.handleConfigExt, true))
		a.Router.HandleFunc("/api/restart", a.authMiddleware(a.handleRestart, true))
		a.Router.HandleFunc("/api/sql", a.authMiddleware(a.handleSQL, true))
		a.Router.HandleFunc("/api/db/backup", a.authMiddleware(a.handleDBBackupCreate, true))
		a.Router.HandleFunc("/api/db/backups", a.authMiddleware(a.handleDBBackupList, true))
		a.Router.HandleFunc("/api/db/restore", a.authMiddleware(a.handleDBRestore, true))
		a.Router.HandleFunc("/db_backups", a.authMiddleware(a.serveDBBackupsPage, true))
		// A PAGE with its own route rather than an arm of serveHTMLPage: that
		// switch sits behind the unauthenticated catch-all, and this listing is
		// admin-only. Registered WITHOUT authMiddleware on purpose - the
		// handler asks hasRole itself so it can answer a refusal with a page
		// instead of a line of plain text. An exact pattern beats "/".
		a.Router.HandleFunc("/OMNGoFiles.html", a.serveFilesPage)
		a.Router.HandleFunc("/api/sync", a.authMiddleware(a.handleSync, true))
		a.Router.HandleFunc("/api/sync/preview", a.authMiddleware(a.handleSyncPreview, true))
		a.Router.HandleFunc("/api/edit-external", a.authMiddleware(a.handleEditExternal, true))
		// Note exchange (note_exchange.go). Both admin only. Import writes
		// files, which is reason enough; export is locked as well by
		// decision, because it is a new way out of the note tree and a LAN
		// guest has no business with one. A local connection bypasses
		// authMiddleware, so the device itself is unaffected - and on
		// Android, where this feature is used, the caller IS the device.
		a.Router.HandleFunc("/api/export/note", a.authMiddleware(a.handleExportNote, true))
		a.Router.HandleFunc("/api/import/note", a.authMiddleware(a.handleImportNote, true))
		// Admin only: the answer carries LAN addresses, absolute paths and
		// a commit subject (see status.go).
		a.Router.HandleFunc("/api/status", a.authMiddleware(a.handleStatus, true))
		// The Status page. Registered WITHOUT authMiddleware for the same
		// reason as /OMNGoFiles.html above: the handler asks hasRole
		// itself, so a guest gets a page instead of a line of plain text.
		a.Router.HandleFunc("/OMNGoStatus.html", a.serveStatusPage)

		// Unlocked access here is safe: this runs before net.Listen/close(a.ready),
		// i.e. before any HTTP handler can possibly be invoked concurrently.
		//
		// loadConfig has already resolved the port - a configured (positive)
		// server_port wins, otherwise fallbackPort(). This is only a guard
		// against a caller that reached here without going through it.
		if a.Config.ServerPort <= 0 {
			a.Config.ServerPort = a.fallbackPort()
		}

		// BEHAVIOR CHANGE vs pre-ShareLAN versions: the server used to
		// always bind 0.0.0.0. It now binds loopback-only unless the
		// "Share on LAN" config option is enabled - the socket itself is
		// the enforcement, so with sharing off there is no way for
		// another device to even complete a TCP handshake, regardless of
		// any auth logic. (authMiddleware still guards non-local clients
		// with the admin/guest passwords when sharing is on.) Since the
		// listener is bound exactly once, toggling this option in the
		// Config page takes effect on the next application start.
		bindHost := "127.0.0.1"
		if a.Config.ShareLAN {
			bindHost = "0.0.0.0"
		}
		bindAddr := fmt.Sprintf("%s:%d", bindHost, a.Config.ServerPort)

		// Bind the socket first so we know the server is actually reachable
		// before signaling readiness to callers (e.g. main_desktop.go).
		// Retried briefly: during a self-restart (/api/restart) the
		// replacement process can race the old one's socket teardown, and
		// dying on the first EADDRINUSE would turn every restart into a
		// coin flip. Ten 300ms attempts (~3s) comfortably covers that
		// window while still failing fast on a genuinely occupied port.
		var listener net.Listener
		var err error
		for attempt := 1; attempt <= 10; attempt++ {
			listener, err = net.Listen("tcp", bindAddr)
			if err == nil {
				break
			}
			a.logDebugf(logServer, "bind %s failed (attempt %d/10), retrying: %v", bindAddr, attempt, err)
			time.Sleep(300 * time.Millisecond)
		}
		if err != nil {
			a.logErrf(logServer, "Server failed to bind %s: %v", bindAddr, err)
			close(a.ready) // unblock any waiter rather than hang forever
			return
		}

		// The listener knows better than bindAddr does: a port of 0, or a
		// retry that landed elsewhere, resolves here (see boundAddress).
		a.setBoundAddress(listener.Addr().String())

		a.logInfof(logServer, "OMN-Go Backend running on %s", bindAddr)
		close(a.ready)

		if err := http.Serve(listener, a.connectionMiddleware(a.Router)); err != nil {
			a.logErrf(logServer, "Server crashed: %v", err)
		}
	}()
	return a
}

// a.GetServerPort safely exposes the configured port for frontend wrappers
func (a *App) GetServerPort() int {
	return a.GetConfig().ServerPort
}
