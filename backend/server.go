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
	// atomic.AddInt64. It was a bare int64 until 26.08.51.
	//
	// On a 32-bit build each request panicked with "unaligned 64-bit
	// atomic operation". A 64-bit atomic needs an address on an 8-byte
	// boundary. A 32-bit struct aligns to 4 bytes alone. One Go rule
	// saves such a field: the first word of an allocated struct is on a
	// 64-bit boundary. That rule did not reach a field below Config and
	// a RWMutex.
	//
	// connectionMiddleware wraps each route, thus the first request
	// died. Nothing worked on armeabi-v7a or on x86, and F-Droid
	// publishes both. arm64 aligns to 8 bytes on its own and hid the
	// fault.
	//
	// atomic.Int64 carries its own alignment guarantee. The field can
	// therefore sit anywhere in this struct and stay correct. Do not
	// make it an int64 again to save a line. See TestNoBare64BitAtomics.
	ActiveConns atomic.Int64
	GitMutex    sync.Mutex // serializes all on-disk git repo operations
	Router      *http.ServeMux

	sqlMu  sync.Mutex         // guards sqlDBs (see sqlite.go)
	sqlDBs map[string]*sql.DB // lazily-opened user SQLite handles, by name

	// dbRestoreMu serializes each database restore and each swap. That
	// covers a manual restore and the bootstrap of a fresh device. See
	// db_backup.go. Never take it while sqlMu is held.
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

	// logFilter caches the three log switches of the configuration. It
	// holds a logFilter value, and it is empty until loadConfig runs.
	// See applyLogFilter for why a log line reads this cache and never
	// the configuration itself.
	logFilter atomic.Value

	// sessionOnce and sessionKey hold the HMAC key that signs the session
	// cookie. sessionSecret reads the key file one time and keeps the
	// bytes here. The key is NOT a field of Config, because GET
	// /api/config marshals that whole struct. See session.go.
	sessionOnce sync.Once
	sessionKey  []byte
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
// The CONFIG LOADER must apply the per-flavor default, and no code after
// it can. loadConfig writes a full default config.json on a fresh
// install. It also fills in a port for a config.json that has none.
//
// StartServer therefore resumed with a.Config.ServerPort already at a
// positive 8080. The default of the caller could not reach it. The 8080
// was on disk as well, thus the wrong port stayed for the life of the
// install. DEFAULT_SERVER_PORT=8081 of the fdroid flavor never applied.
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

// WithConfig runs fn while it holds the config write lock. Use it for an
// update that reads, changes and writes. The POST handler of handleConfig
// is one such caller.
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

// templatesFS holds the page fragments that the server renders. Examples
// are the Config dashboard and the wait page of the external editor.
//
// This tree is embedded apart from staticFS on purpose. The frontend/html
// tree of staticFS reaches StorageDir/html at the first request for each
// file. See serveLazyEmbed and serveStaticAsset. That tree is user
// content: a person opens a file with ?edit=true and writes over it.
//
// A template is not user content. It is render logic of the Go side. A
// template inside frontend/html would let a person edit it and damage it.
// Each static-file listing would also need a line to hide it.
//
//go:embed frontend/templates
var templatesFS embed.FS

// StartServer starts the Go backend.
//
// A storageDir that is not empty replaces the default that initStorage
// computes from runtime.GOOS. See backend/storage.go. Android passes its
// own per-flavor external media directory here. See
// ServerService.storageDir in android/.../ServerService.java. The Go
// runtime cannot learn the applicationId of the running application,
// which is net.basov.omngo or net.basov.omngo.fdroid. Each other caller
// passes "" and keeps the default. main_desktop.go is one such caller.
//
// A defaultPort above 0 becomes the server port when config.json carries
// no positive server_port of its own. The reason is the same as the
// reason for storageDir: the flavor knows, and this package cannot. A
// person can install the standard flavor and the fdroid flavor side by
// side, thus the two must not compete for one loopback port. See
// DEFAULT_SERVER_PORT in android/app/build.gradle. Pass 0 to keep the
// historic default of 8080, which the desktop does.
func StartServer(storageDir string, defaultPort int) *App {
	a := &App{
		Router:    http.NewServeMux(),
		ready:     make(chan struct{}),
		startedAt: time.Now(),
	}

	// Set this BEFORE initStorage. That function loads config.json, and
	// on a fresh install it writes the file. It is the only place that
	// can still apply the per-flavor default.
	a.defaultPort = defaultPort

	a.initStorage(storageDir) // Execute synchronously to ensure config is loaded instantly

	// One function resolves each content type now. It is
	// resolveContentType in serving.go, and it carries the canonical
	// table. The startup mime.AddExtensionType calls seeded that table,
	// thus they are gone.

	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logErrf(logServer, "Recovered from panic in server: %v", r)
			}
		}()

		// Initialize logger to stream Go logs to the frontend via SSE
		a.InitLoggerAndRoute()
		a.Router.HandleFunc("/", a.serveFrontend)

		// The /js, /css and /json trees hold embedded assets. One shared
		// handler in serving.go extracts each file at its first request
		// and serves it, and ?edit=true opens it. The root catch-all
		// reaches the same serveEmbeddableAsset through serveFrontend
		// and serveStaticAsset.
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
		// Registered WITHOUT authMiddleware, the same as /api/note above
		// and the same as each page and each static route. Search
		// collects nothing that a guest on the LAN cannot already read
		// file by file. A gate here would add no confidentiality and
		// would break the guest.
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
		// A PAGE with its own route, and not an arm of serveHTMLPage.
		// That switch sits behind the catch-all, which needs no
		// authentication, and this listing is admin only.
		//
		// Registered WITHOUT authMiddleware on purpose. The handler asks
		// hasRole itself. It can therefore answer a refusal with a page,
		// and not with a line of plain text. An exact pattern wins
		// against "/".
		a.Router.HandleFunc("/OMNGoFiles.html", a.serveFilesPage)
		a.Router.HandleFunc("/api/sync", a.authMiddleware(a.handleSync, true))
		a.Router.HandleFunc("/api/sync/preview", a.authMiddleware(a.handleSyncPreview, true))
		a.Router.HandleFunc("/api/edit-external", a.authMiddleware(a.handleEditExternal, true))
		// Note exchange. See note_exchange.go. Both routes are admin
		// only. Import writes files, which is reason enough. Export is
		// locked by decision, because it is a new way out of the note
		// tree and a guest on the LAN needs none. A local connection
		// passes authMiddleware, thus the device itself keeps both
		// routes. On Android, where a person uses this feature, the
		// caller IS the device.
		a.Router.HandleFunc("/api/export/note", a.authMiddleware(a.handleExportNote, true))
		a.Router.HandleFunc("/api/import/note", a.authMiddleware(a.handleImportNote, true))
		// Admin only: the answer carries LAN addresses, absolute paths and
		// a commit subject (see status.go).
		a.Router.HandleFunc("/api/status", a.authMiddleware(a.handleStatus, true))
		// The Status page. Registered WITHOUT authMiddleware for the same
		// reason as /OMNGoFiles.html above. The handler asks hasRole
		// itself, thus a guest gets a page and not a line of plain text.
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

		// BEHAVIOR CHANGE against each version before ShareLAN. The
		// server bound 0.0.0.0 at each start. It now binds the loopback
		// address alone while the "Share on LAN" option is off.
		//
		// The socket is the enforcement. With sharing off, another
		// device cannot complete a TCP handshake, whatever the
		// authorization code says. With sharing on, authMiddleware
		// guards each client that is not local with the admin password
		// and the guest password.
		//
		// The listener binds one time. A change of this option on the
		// Config page therefore applies at the next start.
		bindHost := "127.0.0.1"
		if a.Config.ShareLAN {
			bindHost = "0.0.0.0"
		}
		bindAddr := fmt.Sprintf("%s:%d", bindHost, a.Config.ServerPort)

		// Bind the socket first. A caller such as main_desktop.go then
		// learns that the server is reachable, and not that it is about
		// to be.
		//
		// The bind retries for a short time. At a self-restart through
		// /api/restart, the new process can reach this line before the
		// old process closes its socket. A stop at the first EADDRINUSE
		// would make each restart a matter of chance. Ten attempts of
		// 300 ms cover that window, which is about 3 seconds. A port
		// that another program holds still fails fast.
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
