package backend

import (
	"embed"
	"io/fs"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const APP_VERSION = "1.0.40"

type Config struct {
	ServerPort    int    `json:"server_port"`
	AdminPassword string `json:"admin_password"`
	GuestPassword string `json:"guest_password"`
}

//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/js frontend/css frontend/json
var staticFS embed.FS

var (
	storageDir  string
	appConfig   Config
	activeConns int
)

func initStorage() {
	if runtime.GOOS == "android" {
		storageDir = "/storage/emulated/0/Android/media/net.basov.goomn"
	} else {
		storageDir = "./data"
	}

	// 1. Create Isolated Storage
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	mdDir := filepath.Join(storageDir, "md")
	os.MkdirAll(mdDir, 0755)

	// Migrate existing .md files recursively
	files, _ := filepath.Glob(filepath.Join(storageDir, "*.md"))
	for _, f := range files {
		os.Rename(f, filepath.Join(mdDir, filepath.Base(f)))
	}

	// 2. Init Config
	configPath := filepath.Join(storageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		appConfig = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
		}
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(configPath, data, 0644)
	} else {
		data, _ := os.ReadFile(configPath)
		json.Unmarshal(data, &appConfig)
	}

	// 3. Init Default Notes
	welcomePath := filepath.Join(mdDir, "Welcome.md")
	if _, err := os.Stat(welcomePath); os.IsNotExist(err) {
		welcomeContent := "Title: Welcome\nDate: 2026-06-14 12:00:00\nCategory: System\n\nWelcome to GoOMN. Start editing!\n\n- [Help](Welcome)\n- [Bookmarks](Bookmarks)\n- [Quick Notes](QuickNotes)"
		os.WriteFile(welcomePath, []byte(welcomeContent), 0644)
	}

	
	rulesPath := filepath.Join(mdDir, "ScriptRules.md")
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		os.WriteFile(rulesPath, []byte("Title: JS Scripting Rules\nDate: 2026-06-15\nCategory: System\n\n# JavaScript Guidelines for GoOMN\n\nBecause GoOMN is a Single Page Application (SPA), the global `window` scope persists between page loads. To avoid `SyntaxError: Identifier has already been declared` when scripts are re-evaluated, authors must follow these rules:\n\n### Rule 1: Isolate variables using Block Scopes or IIFEs\nNever leave `const` or `let` in the top-level global scope. Wrap the script in an Anonymous Block `{ ... }` or an Immediately Invoked Function Expression (IIFE).\n\n```javascript\n{\n    const myLocalVar = \"Safe!\";\n    let counter = 0;\n}\n```\n\n### Rule 2: Explicitly attach required globals to `window`\nIf a function is needed for an HTML `onclick` event, attach it directly to the `window` object.\n\n```javascript\nwindow.doSomething = function() {\n    alert(\"This works safely on reload!\");\n};\n```\n\n### Rule 3: Use the OR (`||`) operator for global state\nCheck if global config objects exist before creating them so user state is preserved.\n\n```javascript\nwindow.myAppConfig = window.myAppConfig || { version: \"1.0\" };\n```\n\n### Rule 4: Use `var` for raw top-level variables\nIf you must declare top-level variables, use `var` because the JS engine allows `var` to be redeclared infinitely without throwing an error."), 0644)
	}

	quickPath := filepath.Join(mdDir, "QuickNotes.md")
	if _, err := os.Stat(quickPath); os.IsNotExist(err) {
		quickContent := "Title: Quick Notes\nDate: 2026-06-14 12:00:00\nCategory: Log\n\n"
		os.WriteFile(quickPath, []byte(quickContent), 0644)
	}

	bmPath := filepath.Join(mdDir, "Bookmarks.md")
	if _, err := os.Stat(bmPath); os.IsNotExist(err) {
		bmContent := `Title: Incoming bookmarks
Date: 2023-01-13 13:59:15
Modified: 2025-11-01 11:03:46
Author: Mikhail Basov
Tags: Bookmarks

<script>bookmarks = [
<!-- Don't edit body below this line -->
  {
    "date": "2025-11-06 02:52:09",
    "url": "https://youtube.com/shorts/0pI2KHl7gCU?si=9M_DqeVBxmuyHiTC",
    "title": "Tapping 16th note rhythms🥁 #music #musiclesson #musictutorial #learnmusi...",
    "tags": [
      "Music",
      "Mathematics",
      "YouTube short",
    ],
    "notes": [
    ]
  },
  {
    "date": "2025-11-05 15:44:25",
    "url": "https://www.reddit.com/r/ErgoMechKeyboards/comments/1ol49i6/printyl_mx_keycap_optimized_for_3d_printer/",
    "title": "printyl: MX keycap optimized for 3d printer, inspired on dactyl : r/ErgoMechKeyboards",
    "tags": [
      "Reddit",
      "Keyboard",
      "3D model",
    ],
    "notes": [
    ]
  },
  {
    "date": "2023-01-22 22:22:22",
    "url": "/default/BookmarkerHelp.html",
    "title": "Help about this bookmark page",
    "tags": [
      "OMN",
      "Local pages",
      "Help"
    ],
    "notes": [
      "File format described on this page also"
    ]
  }
];
</script>
  
<!-- end of bookmarks definition -->
    
<link rel="stylesheet" type="text/css" href="/css/Bookmarker.css" />
<script type="text/javascript" src="/js/Bookmarker.js"></script>`
		os.WriteFile(bmPath, []byte(bmContent), 0644)
	}
}

// Simple connection tracker for the Android Canvas requirement
func connectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeConns++
		next.ServeHTTP(w, r)
		activeConns--
	})
}

func authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_role")
		if err != nil || (requireAdmin && cookie.Value != "admin") || (!requireAdmin && cookie.Value != "admin" && cookie.Value != "guest") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	pwd := r.FormValue("password")
	role := ""
	if pwd == appConfig.AdminPassword {
		role = "admin"
	} else if pwd == appConfig.GuestPassword {
		role = "guest"
	}

	if role != "" {
		http.SetCookie(w, &http.Cookie{Name: "session_role", Value: role, Path: "/"})
		w.Write([]byte("OK"))
	} else {
		http.Error(w, "Invalid", http.StatusUnauthorized)
	}
}

func handleQuickNote(w http.ResponseWriter, r *http.Request) {
	note := r.FormValue("note")
	if note == "" {
		return
	}
	path := filepath.Join(storageDir, "md", "QuickNotes.md")
	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\n")
	
	insertIdx := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" { // Find first blank line ending Pelican header
			insertIdx = i + 1
			break
		}
	}
	
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("\n---\n##### %s\n%s\n", timestamp, note)
	
	newContent := append(lines[:insertIdx], append([]string{entry}, lines[insertIdx:]...)...)
	os.WriteFile(path, []byte(strings.Join(newContent, "\n")), 0644)
	w.Write([]byte("Saved"))
}

func handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")
	
	path := filepath.Join(storageDir, "md", "Bookmarks.md")
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	tagsList := []string{}
	for _, t := range strings.Split(tags, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tagsList = append(tagsList, trimmed)
		}
	}
	notesList := []string{}
	if trimmed := strings.TrimSpace(notes); trimmed != "" {
		notesList = append(notesList, trimmed)
	}
	
	type BM struct {
		Date  string   `json:"date"`
		Url   string   `json:"url"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
		Notes []string `json:"notes"`
	}
	bm := BM{Date: timestamp, Url: url, Title: title, Tags: tagsList, Notes: notesList}
	bmJson, _ := json.MarshalIndent(bm, "  ", "  ")
	entry := "  " + string(bmJson) + ",\n"
	
	data, err := os.ReadFile(path)
	if err == nil {
		content := string(data)
		marker := "<!-- Don't edit body below this line -->"
		if strings.Contains(content, marker) {
			newContent := strings.Replace(content, marker, marker+"\n"+entry, 1)
			os.WriteFile(path, []byte(newContent), 0644)
		} else {
			f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
			defer f.Close()
			f.WriteString("\n" + entry)
		}
	}
	w.Write([]byte("Saved"))
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	imgDir := filepath.Join(storageDir, "images")
	os.MkdirAll(imgDir, 0755)
	
	destPath := filepath.Join(imgDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)
	
	w.Write([]byte(fmt.Sprintf("![%s]({filename}/images/%s)", header.Filename, header.Filename)))
}

func handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Welcome"
	}
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}
	data, err := os.ReadFile(filepath.Join(storageDir, "md", filepath.Clean(name)))
	if err != nil {
		title := strings.TrimSuffix(name, ".md")
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		newContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes\n\n", title, timestamp)
		w.Write([]byte(newContent))
		return
	}
	w.Write(data)
}

func handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		return
	}
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}
	path := filepath.Join(storageDir, "md", filepath.Clean(name))
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
	w.Write([]byte("Saved"))
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(frontendHTML)
}

func StartServer() {
	initStorage() // Execute synchronously to ensure config is loaded instantly
	go func() {
		
		mux := http.NewServeMux()
		mux.HandleFunc("/", serveFrontend)
		fSys, _ := fs.Sub(staticFS, "frontend")
		
		serveStrict := func(ext, cType string) http.Handler {
			fsHandler := http.FileServer(http.FS(fSys))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, ext) {
					http.Error(w, "Forbidden: Invalid file extension", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", cType)
				
				if r.URL.Path == "/js/Bookmarker.js" {
					data, err := fs.ReadFile(fSys, "js/Bookmarker.js")
					if err == nil {
						js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
						js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
						w.Write([]byte(js))
						return
					}
				}
				
				fsHandler.ServeHTTP(w, r)
			})
		}

		mux.Handle("/js/", serveStrict(".js", "application/javascript"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))
		mux.Handle("/json/", serveStrict(".json", "application/json"))
		mux.HandleFunc("/login", handleLogin)
		mux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
		mux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
		mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
		mux.HandleFunc("/api/note", handleGetNote)
		mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))
		
		port := fmt.Sprintf(":%d", appConfig.ServerPort)
		log.Printf("GoOMN Backend running on %s", port)
		http.ListenAndServe(port, connectionMiddleware(mux))
	}()
}

// GetServerPort safely exposes the configured port for frontend wrappers
func GetServerPort() int {
	return appConfig.ServerPort
}
