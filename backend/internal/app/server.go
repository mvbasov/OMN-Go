package app

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/db"
	"net.basov.omngo/backend/internal/exchange"
	"net.basov.omngo/backend/internal/gitsync"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/search"
	"net.basov.omngo/backend/internal/status"
)

// App encapsulates the global state for the backend
type App struct {
	config     config.Store
	StorageDir string
	// ActiveConns is an atomic.Int64, and not an int64. A 64-bit atomic needs
	// an 8-byte boundary, and a 32-bit build (armeabi-v7a, x86) does not give
	// one here. Each request then panics. See TestNoBare64BitAtomics.
	ActiveConns atomic.Int64
	Router      *http.ServeMux

	dbs db.Store      // the open user databases (see db_app.go)
	git gitsync.State // the lock of the repository and the host keys (see gitsync_app.go)

	// search is the global index (internal/search/index.go). It is empty until
	// global search is on.
	search *search.Index

	// onPageWritten is the hook of renderAndCache. The page cache thus does
	// not call the search index. connectGroups sets the hook, and a nil hook
	// does nothing.
	onPageWritten func(name string)

	// pages holds the values of other groups that each page shows.
	pages render.Facts

	// android holds the facts that the Android layer sets. See status.Android.
	android status.Android

	// defaultPort is the port of the flavor, for a config.json with no port.
	// 0 means 8080. Only loadConfig can apply it: see fallbackPort.
	defaultPort int

	ready chan struct{} // closed once the HTTP listener is actually serving

	// boundAddr is what the listener bound, not what the config asked for.
	// /api/status reports it with startedAt. metaMu guards boundAddr.
	metaMu    sync.RWMutex
	startedAt time.Time
	boundAddr string

	// logFilter caches the log switches of the configuration. See
	// applyLogFilter.
	logFilter atomic.Value

	// logs holds the stream clients and the history ring. See logx.Hub.
	logs logx.Hub

	// sessionKey is the HMAC key of the session cookie. It is not a field of
	// Config, because GET /api/config sends the whole Config. See session.go.
	sessionOnce sync.Once
	sessionKey  []byte
}

// waitUntilReady blocks until the HTTP server listens. It also returns
// when the bind fails, thus a caller never waits for ever.
func (a *App) waitUntilReady() {
	<-a.ready
}

// connectGroups sets the hooks between the groups. A group thus calls
// another group through the App, and not by name. See group_links_test.go.
func (a *App) connectGroups() {
	a.onPageWritten = func(string) { a.markSearchIndexDirty() }
	a.pages = render.Facts{SearchGlobal: a.globalSearchAvailable, IncomingPage: exchange.IncomingIndexName}
}

// runningApp is the App of StartServer. gomobile exports functions only, thus
// an exported setter finds the App here. earlyEnv holds a fact that arrives
// before StartServer: ServerService.java calls SetAndroidPackage first.
var (
	runningMu  sync.Mutex
	runningApp *App
	earlyEnv   status.Android
)

// setRunningApp gives a the facts of earlyEnv, and each later setter writes
// to a.
func setRunningApp(a *App) {
	runningMu.Lock()
	defer runningMu.Unlock()
	a.android.SetPackage(earlyEnv.PackageName())
	a.android.SetAddresses(earlyEnv.LANAddresses())
	runningApp = a
}

// withAndroidEnv runs fn on the facts of runningApp, or on earlyEnv before
// StartServer.
func withAndroidEnv(fn func(*status.Android)) {
	runningMu.Lock()
	defer runningMu.Unlock()
	if runningApp != nil {
		fn(&runningApp.android)
		return
	}
	fn(&earlyEnv)
}

// SetAndroidPackage records the applicationId of the Android app.
// ServerService.java calls it before Backend.startServer. gomobile exports
// it.
func SetAndroidPackage(name string) {
	withAndroidEnv(func(e *status.Android) { e.SetPackage(name) })
}

// SetLANAddresses records the addresses of this device as one list with
// commas. ServerService.java calls it with each site-local IPv4 address that
// is not loopback, each time it builds the notification. The notification and
// the Status page thus show the same addresses. The gomobile binding carries
// no slice of strings, thus the value is one string. An empty list clears it.
func SetLANAddresses(list string) {
	out := []string{}
	for _, part := range strings.Split(list, ",") {
		if text := strings.TrimSpace(part); text != "" {
			out = append(out, text)
		}
	}
	withAndroidEnv(func(e *status.Android) { e.SetAddresses(out) })
}

// StartServer starts the Go backend.
//
// A storageDir that is not empty replaces the default of initStorage. Android
// passes the external media directory of its flavor, because the Go runtime
// cannot learn the applicationId. The desktop passes "".
//
// A defaultPort above 0 is the port when config.json has none. The standard
// and the fdroid flavor can run side by side, thus they need two ports. See
// DEFAULT_SERVER_PORT in android/app/build.gradle. The desktop passes 0 for
// 8080.
func StartServer(storageDir string, defaultPort int) {
	startServer(storageDir, defaultPort)
}

// WaitUntilReady blocks until the server of StartServer listens. It also
// returns when the bind fails, and at once when StartServer did not run.
// main_desktop.go calls it before it opens the browser. gomobile exports it.
func WaitUntilReady() {
	if a := currentApp(); a != nil {
		a.waitUntilReady()
	}
}

// ServerPort answers the port of the server of StartServer, or 0 when
// StartServer did not run. main_desktop.go builds the address of the browser
// from it. gomobile exports it.
func ServerPort() int {
	if a := currentApp(); a != nil {
		return a.serverPort()
	}
	return 0
}

// currentApp answers runningApp under its lock.
func currentApp() *App {
	runningMu.Lock()
	defer runningMu.Unlock()
	return runningApp
}

// startServer is StartServer. It answers the App for the tests.
func startServer(storageDir string, defaultPort int) *App {
	a := &App{
		Router:    http.NewServeMux(),
		ready:     make(chan struct{}),
		startedAt: time.Now(),
	}

	// Set it before initStorage, which writes config.json on a fresh install.
	a.defaultPort = defaultPort
	setRunningApp(a)
	setStartStage("Preparing the storage")

	a.initStorage(storageDir) // Execute synchronously to ensure config is loaded instantly

	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.log(logx.Server).Errf("Recovered from panic in server: %v", r)
			}
		}()

		a.initLogger()
		a.registerRoutes(a.Router)

		// loadConfig already set the port, thus this test is a guard only.
		a.config.Update(func(c *config.Config) {
			if c.ServerPort <= 0 {
				c.ServerPort = a.fallbackPort()
			}
		})
		cfg := a.config.Get()
		setStartStage("Starting the server")

		// The socket decides who can connect. With "Share on LAN" off, the
		// listener binds the loopback address alone. See
		// doc/decisions/0002-bind-the-loopback-address-when-lan-sharing-is-off.md.
		// The listener binds one time, thus a change applies at the next
		// start.
		bindHost := "127.0.0.1"
		if cfg.ShareLAN {
			bindHost = "0.0.0.0"
		}
		bindAddr := fmt.Sprintf("%s:%d", bindHost, cfg.ServerPort)

		// Bind first, thus WaitUntilReady means "reachable". The bind retries
		// for about 3 seconds: after /api/restart, the new process can reach
		// this line before the old one closes its socket. A port that another
		// program holds still fails.
		var listener net.Listener
		var err error
		for attempt := 1; attempt <= 10; attempt++ {
			listener, err = net.Listen("tcp", bindAddr)
			if err == nil {
				break
			}
			a.log(logx.Server).Debugf("bind %s failed (attempt %d/10), retrying: %v", bindAddr, attempt, err)
			time.Sleep(300 * time.Millisecond)
		}
		if err != nil {
			a.log(logx.Server).Errf("Server failed to bind %s: %v", bindAddr, err)
			setStartStage("")
			close(a.ready) // unblock any waiter rather than hang forever
			return
		}

		// The listener knows the real port, for example after a port of 0.
		a.setBoundAddress(listener.Addr().String())

		a.log(logx.Server).Infof("OMN-Go Backend running on %s", bindAddr)
		setStartStage("")
		close(a.ready)

		if err := http.Serve(listener, a.connectionMiddleware(a.Router)); err != nil {
			a.log(logx.Server).Errf("Server crashed: %v", err)
		}
	}()
	return a
}

// serverPort answers the configured port. See ServerPort.
func (a *App) serverPort() int {
	return a.config.Get().ServerPort
}

// registerRoutes writes each route of the application into mux. Section 3 of
// CLAUDE.md asks for one block.
//
// It takes an interface and not a *http.ServeMux. StartServer passes
// a.Router, and TestBaseline_RouteSet passes a recorder to read the real
// patterns.
type routeTable interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

func (a *App) registerRoutes(mux routeTable) {
	// /api/logs and /api/logs/history are admin only. A remote caller
	// reads no log line, live or held. See handleLogHistory.
	a.route(mux, "/api/logs", admin, "SSE", get(a.handleLogsSSE))
	a.route(mux, "/api/logs/history", admin, "JSON", get(a.handleLogHistory))

	// The catch-all and the asset trees take each method.
	mux.HandleFunc("/", a.serveFrontend)

	// The /js, /css and /json trees hold embedded assets.
	// serveEmbeddableAsset extracts a file at its first request and serves
	// it. ?edit=true opens the editor.
	assetTree := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.serveEmbeddableAsset(w, r, r.URL.Path)
	})
	mux.Handle("/js/", assetTree)
	mux.Handle("/css/", assetTree)
	mux.Handle("/json/", assetTree)

	// /images and each tree of config.UserFileTrees are user content, and
	// the binary embeds none of it. resolveContentType gives each file its
	// type.
	mux.Handle("/images/", a.serveStorageSubdir("images", ""))
	for _, tree := range config.UserFileTrees {
		mux.Handle("/"+tree.Dir+"/", a.serveStorageSubdir(tree.Dir, ""))
	}

	a.route(mux, "/login", open, "text", post(a.handleLogin))
	a.route(mux, "/api/quick", admin, "text", post(a.handleQuickNote))
	a.route(mux, "/api/bookmark", admin, "text", post(a.handleBookmark))
	a.route(mux, "/api/upload", admin, "text (HTML fragment)", post(a.handleUpload))
	for _, tree := range config.UserFileTrees {
		a.route(mux, tree.Upload, admin, "text (Markdown fragment)", post(a.handleUploadUserFile(tree)))
	}
	a.route(mux, "/api/note", open, "raw file", get(a.handleGetNote))
	// This route is open, the same as /api/note and each page.
	// Search collects nothing that a remote caller cannot read file by file.
	a.route(mux, "/api/search", open, "JSON", get(a.handleSearch))
	a.route(mux, "/api/save", admin, "text", post(a.handleSaveNote))
	a.route(mux, "/api/newpage", admin, "text", post(a.handleNewPage))
	a.route(mux, "/api/config", admin, "JSON / text", get(a.handleConfigGet), post(a.handleConfigPost))
	a.route(mux, "/api/restart", admin, "text", post(a.handleRestart))
	a.route(mux, "/api/sql", admin, "JSON", post(a.handleSQL))
	a.route(mux, "/api/db/backup", admin, "JSON", post(a.handleDBBackupCreate))
	a.route(mux, "/api/db/backups", admin, "JSON", get(a.handleDBBackupList))
	a.route(mux, "/api/db/restore", admin, "JSON", post(a.handleDBRestore))
	a.route(mux, "/api/sync", admin, "JSON", post(a.handleSync))
	a.route(mux, "/api/sync/preview", admin, "JSON", get(a.handleSyncPreview))
	a.route(mux, "/api/sync/trust-host-key", admin, "JSON", post(a.handleTrustHostKey))
	a.route(mux, "/api/edit-external", admin, "HTML or 303", get(a.handleEditExternal))
	// Note exchange. Both routes are admin only: import writes files, and
	// export is a way out of the note tree. The device itself is always
	// admin, and on Android the device is the caller.
	a.route(mux, "/api/export/note", admin, "Markdown download", get(a.handleExportNote))
	a.route(mux, "/api/import/note", admin, "JSON", post(a.handleImportNote))
	// This route is admin only, because the answer holds LAN addresses,
	// absolute paths and a commit subject.
	a.route(mux, "/api/status", admin, "JSON / Markdown", get(a.handleStatus))
	// The loop registers each system page. See page_access.go.
	for _, p := range a.systemPages() {
		who := open
		if p.admin {
			who = adminPage
		}
		a.route(mux, p.path, who, "HTML", get(a.pageHandler(p)))
	}
}
