package main

import (
	_ "embed"
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

const APP_VERSION = "1.0.18"

type Config struct {
	ServerPort    int    `json:"server_port"`
	AdminPassword string `json:"admin_password"`
	GuestPassword string `json:"guest_password"`
}

//go:embed frontend/index.html
var frontendHTML []byte

var (
	storageDir  string
	appConfig   Config
	activeConns int
)

func initStorage() {
	if runtime.GOOS == "android" {
		storageDir = "/storage/emulated/0/Media/net.basov.goomn"
	} else {
		storageDir = "./data"
	}

	// 1. Create Isolated Storage
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage: %v", err)
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
	welcomePath := filepath.Join(storageDir, "Welcome.md")
	if _, err := os.Stat(welcomePath); os.IsNotExist(err) {
		welcomeContent := "Title: Welcome\nDate: 2026-06-14 12:00:00\nCategory: System\n\nWelcome to GoOMN. Start editing!\n\n- [Help](Welcome)\n- [Bookmarks](Bookmarks)\n- [Quick Notes](QuickNotes)"
		os.WriteFile(welcomePath, []byte(welcomeContent), 0644)
	}

	quickPath := filepath.Join(storageDir, "QuickNotes.md")
	if _, err := os.Stat(quickPath); os.IsNotExist(err) {
		quickContent := "Title: Quick Notes\nDate: 2026-06-14 12:00:00\nCategory: Log\n\n"
		os.WriteFile(quickPath, []byte(quickContent), 0644)
	}

	bmPath := filepath.Join(storageDir, "Bookmarks.md")
	if _, err := os.Stat(bmPath); os.IsNotExist(err) {
		bmContent := "Title: Bookmarks\nDate: 2026-06-14 12:00:00\nCategory: Links\n\n"
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
	path := filepath.Join(storageDir, "QuickNotes.md")
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
	entry := fmt.Sprintf("\n---\n#### %s\n%s\n", timestamp, note)
	
	newContent := append(lines[:insertIdx], append([]string{entry}, lines[insertIdx:]...)...)
	os.WriteFile(path, []byte(strings.Join(newContent, "\n")), 0644)
	w.Write([]byte("Saved"))
}

func handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")
	
	path := filepath.Join(storageDir, "Bookmarks.md")
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	entry := fmt.Sprintf("\n- [%s](%s) | Tags: %s | Notes: %s | Added: %s\n", title, url, tags, notes, timestamp)
	
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(entry)
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
	data, err := os.ReadFile(filepath.Join(storageDir, filepath.Clean(name)))
	if err != nil {
		w.Write([]byte("*(File not found)*"))
		return
	}
	w.Write(data)
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(frontendHTML)
}

func runServer() {
	initStorage()
	
	mux := http.NewServeMux()
	mux.HandleFunc("/", serveFrontend)
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
	mux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
	mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
	mux.HandleFunc("/api/note", handleGetNote)
	
	port := fmt.Sprintf(":%d", appConfig.ServerPort)
	log.Printf("GoOMN Backend running on %s", port)
	http.ListenAndServe(port, connectionMiddleware(mux))
}
