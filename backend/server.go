package backend

import (
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

//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/html frontend/md
var staticFS embed.FS

var activeConns int

func StartServer() {
	initStorage() // Execute synchronously to ensure config is loaded instantly

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

		mux := http.NewServeMux()
		// Initialize logger to stream Go logs to the frontend via SSE
		log.SetOutput(&JSLogger{})
		mux.HandleFunc("/api/logs", HandleLogsSSE)
		mux.HandleFunc("/", serveFrontend)

		serveLazyEmbed := func() http.Handler {
			physicalDir := filepath.Join(storageDir, "html")
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
					if !appConfig.UseInternalEd {
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
					escapedContent := htmlEscape(string(rawContent))
					customBody := "<pre style=\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\">" + escapedContent + "</pre>"
					// Pass raw content as mdContent so the textarea is populated
					compiled := compilePageWithBody(relPath, rawContent, customBody)
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

		mux.Handle("/js/", serveLazyEmbed())
		mux.Handle("/css/", serveLazyEmbed())
		mux.Handle("/json/", serveLazyEmbed())

		// Config for files handling Content-type by served directories
		serveStorageDir := func(subDir, cType string) http.Handler {
			dirPath := filepath.Join(storageDir, "html", subDir)
			os.MkdirAll(dirPath, 0755)
			fsHandler := http.StripPrefix("/"+subDir+"/", http.FileServer(http.Dir(dirPath)))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if cType != "" {
					w.Header().Set("Content-Type", cType)
				}
				fsHandler.ServeHTTP(w, r)
			})
		}

		mux.Handle("/images/", serveStorageDir("images", ""))
		mux.Handle("/user_json/", serveStorageDir("user_json", "application/json"))

		mux.HandleFunc("/login", handleLogin)
		mux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
		mux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
		mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
		mux.HandleFunc("/api/upload_json", authMiddleware(handleUploadJSON, true))
		mux.HandleFunc("/api/note", handleGetNote)
		mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))
		mux.HandleFunc("/api/newpage", authMiddleware(handleNewPage, true))
		mux.HandleFunc("/api/config", authMiddleware(handleConfig, true))
		mux.HandleFunc("/api/sync", authMiddleware(handleSync, true))
		mux.HandleFunc("/api/edit-external", authMiddleware(handleEditExternal, true))

		if appConfig.ServerPort <= 0 {
			appConfig.ServerPort = 8080
		}

		bindAddr := fmt.Sprintf("0.0.0.0:%d", appConfig.ServerPort)

		log.Printf("OMN-Go Backend running on %s", bindAddr)
		err := http.ListenAndServe(bindAddr, connectionMiddleware(mux))
		if err != nil {
			log.Printf("FATAL: Server crashed: %v", err)
		}
	}()
}

// GetServerPort safely exposes the configured port for frontend wrappers
func GetServerPort() int {
	return appConfig.ServerPort
}