package backend

import (
	"sync"
	"embed"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)


// App encapsulates the global state for the backend
type App struct {
	Config Config
	StorageDir  string
	ActiveConns int64
	ConnMutex   sync.Mutex
	GitMutex    sync.Mutex
	Router      *http.ServeMux
}

//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/html frontend/md
var staticFS embed.FS



func StartServer() *App {
	a := &App{
		Router: http.NewServeMux(),
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
		log.SetOutput(&JSLogger{})
		a.Router.HandleFunc("/api/logs", a.HandleLogsSSE)
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
					if !a.Config.UseInternalEd {
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
					compiled := a.compilePageWithBody(relPath, rawContent, customBody)
					scriptInjection := "<script>var IS_MARKDOWN = false; setTimeout(function(){ if(typeof toggleMode==='function') toggleMode(); }, 120);</script>"
					compiled = []byte(strings.Replace(string(compiled), "</head>", scriptInjection+"\n</head>", 1))
					w.Header().Set("Content-Type", "text/html")
					w.Write(compiled)
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

		if a.Config.ServerPort <= 0 {
			a.Config.ServerPort = 8080
		}

		bindAddr := fmt.Sprintf("0.0.0.0:%d", a.Config.ServerPort)

		log.Printf("OMN-Go Backend running on %s", bindAddr)
		err := http.ListenAndServe(bindAddr, a.connectionMiddleware(a.Router))
		if err != nil {
			log.Printf("FATAL: Server crashed: %v", err)
		}
	}()
	return a
}

// a.GetServerPort safely exposes the configured port for frontend wrappers
func (a *App) GetServerPort() int {
	return a.Config.ServerPort
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

