package backend

import (
	"sync"
	"embed"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)


// App encapsulates the global state for the backend
type App struct {
	Config      Config
	ConfigMutex sync.RWMutex // guards all reads/writes of Config
	StorageDir  string
	ActiveConns int64      // mutate only via atomic.Add/LoadInt64
	GitMutex    sync.Mutex // serializes all on-disk git repo operations
	Router      *http.ServeMux

	ready chan struct{} // closed once the HTTP listener is actually serving
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
// are neither - they're Go-side rendering logic, not user content - so
// mixing them into frontend/html would both let a user "edit" and corrupt
// them and require excluding them from every static-file listing by hand.
//
//go:embed frontend/templates
var templatesFS embed.FS



func StartServer() *App {
	a := &App{
		Router: http.NewServeMux(),
		ready:  make(chan struct{}),
	}



	a.initStorage() // Execute synchronously to ensure config is loaded instantly

	// Fallback MIME types for minimal Docker containers
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".webp", "image/webp")
	mime.AddExtensionType(".png", "image/png")
	mime.AddExtensionType(".jpg", "image/jpeg")
	mime.AddExtensionType(".jpeg", "image/jpeg")
	mime.AddExtensionType(".gif", "image/gif")
	mime.AddExtensionType(".json", "application/json")
	mime.AddExtensionType(".woff", "font/woff")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".ttf", "font/ttf")

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in server: %v", r)
			}
		}()

		// Initialize logger to stream Go logs to the frontend via SSE
		a.InitLoggerAndRoute()
		a.Router.HandleFunc("/", a.serveFrontend)

		serveLazyEmbed := func() http.Handler {
			physicalDir := filepath.Join(a.StorageDir, "html")
			fsHandler := http.FileServer(http.Dir(physicalDir))

			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Calculate physical path
				physPath := filepath.Join(physicalDir, filepath.Clean(r.URL.Path))

				// Lazy Extraction: Check if file exists on disk, if not, pull from embedFS
				if _, err := os.Stat(physPath); os.IsNotExist(err) {
					embedPath := "frontend/html" + filepath.ToSlash(filepath.Clean(r.URL.Path))
					if data, err := staticFS.ReadFile(embedPath); err == nil {
						os.MkdirAll(filepath.Dir(physPath), 0755)
						os.WriteFile(physPath, data, 0644)
					}
				}

				// Check for edit mode before serving static file
				if r.URL.Query().Get("edit") == "true" {
					relPath := strings.TrimPrefix(r.URL.Path, "/")

					// Honour external editor preference
					if !a.GetConfig().UseInternalEd {
						http.Redirect(w, r, "/api/edit-external?name="+url.QueryEscape(relPath), http.StatusSeeOther)
						return
					}

					rawContent, err := os.ReadFile(physPath)
					if err != nil {
						// File does not exist - create empty one and proceed
						os.MkdirAll(filepath.Dir(physPath), 0755)
						os.WriteFile(physPath, []byte{}, 0644)
						rawContent = []byte{}
					}
					escapedContent := a.htmlEscape(string(rawContent))
					customBody := "<pre style=\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\">" + escapedContent + "</pre>"
					// Pass raw content as mdContent so the textarea is populated
					compiled := a.compilePageWithBody(relPath, rawContent, customBody, true)
					w.Header().Set("Content-Type", "text/html")
					w.Write(a.injectRuntimeVars(compiled))
					return
				}

				// Serve the file dynamically from the physical directory
				fsHandler.ServeHTTP(w, r)
			})
		}

		a.Router.Handle("/js/", serveLazyEmbed())
		a.Router.Handle("/css/", serveLazyEmbed())
		a.Router.Handle("/json/", serveLazyEmbed())

		// Config for files handling Content-type by served directories
		serveStorageDir := func(subDir, cType string) http.Handler {
			dirPath := filepath.Join(a.StorageDir, "html", subDir)
			os.MkdirAll(dirPath, 0755)
			fsHandler := http.StripPrefix("/"+subDir+"/", http.FileServer(http.Dir(dirPath)))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if cType != "" {
					w.Header().Set("Content-Type", cType)
				}
				fsHandler.ServeHTTP(w, r)
			})
		}

		a.Router.Handle("/images/", serveStorageDir("images", ""))
		a.Router.Handle("/user_json/", serveStorageDir("user_json", "application/json"))

		a.Router.HandleFunc("/login", a.handleLogin)
		a.Router.HandleFunc("/api/quick", a.authMiddleware(a.handleQuickNote, true))
		a.Router.HandleFunc("/api/bookmark", a.authMiddleware(a.handleBookmark, true))
		a.Router.HandleFunc("/api/upload", a.authMiddleware(a.handleUpload, true))
		a.Router.HandleFunc("/api/upload_json", a.authMiddleware(a.handleUploadJSON, true))
		a.Router.HandleFunc("/api/note", a.handleGetNote)
		a.Router.HandleFunc("/api/save", a.authMiddleware(a.handleSaveNote, true))
		a.Router.HandleFunc("/api/newpage", a.authMiddleware(a.handleNewPage, true))
		a.Router.HandleFunc("/api/config", a.authMiddleware(a.handleConfig, true))
		a.Router.HandleFunc("/api/sync", a.authMiddleware(a.handleSync, true))
		a.Router.HandleFunc("/api/sync/preview", a.authMiddleware(a.handleSyncPreview, true))
		a.Router.HandleFunc("/api/edit-external", a.authMiddleware(a.handleEditExternal, true))

		// Unlocked access here is safe: this runs before net.Listen/close(a.ready),
		// i.e. before any HTTP handler can possibly be invoked concurrently.
		if a.Config.ServerPort <= 0 {
			a.Config.ServerPort = 8080
		}

		bindAddr := fmt.Sprintf("0.0.0.0:%d", a.Config.ServerPort)

		// Bind the socket first so we know the server is actually reachable
		// before signaling readiness to callers (e.g. main_desktop.go).
		listener, err := net.Listen("tcp", bindAddr)
		if err != nil {
			log.Printf("FATAL: Server failed to bind %s: %v", bindAddr, err)
			close(a.ready) // unblock any waiter rather than hang forever
			return
		}

		log.Printf("OMN-Go Backend running on %s", bindAddr)
		close(a.ready)

		if err := http.Serve(listener, a.connectionMiddleware(a.Router)); err != nil {
			log.Printf("FATAL: Server crashed: %v", err)
		}
	}()
	return a
}

// a.GetServerPort safely exposes the configured port for frontend wrappers
func (a *App) GetServerPort() int {
	return a.GetConfig().ServerPort
}

// a.autoGitIgnore safely appends extracted cache files to .gitignore
func (a *App) autoGitIgnore(cachePath string) {
	ignoreFile := ".gitignore"
	content, err := os.ReadFile(ignoreFile)
	if err != nil && !os.IsNotExist(err) {
		return // Skip if we can't read an existing file due to permissions
	}
	
	// Git requires forward slashes
	ignoreStr := filepath.ToSlash(cachePath)
	
	// Only append if the file path is not already in .gitignore
	if !strings.Contains(string(content), ignoreStr) {
		f, err := os.OpenFile(ignoreFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			f.WriteString("\n" + ignoreStr)
			f.Close()
		}
	}
}

