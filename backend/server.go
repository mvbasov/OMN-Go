package backend

import (
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if requireAdmin && cookie.Value != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func serveStorageDir(subDir string, cType string) http.Handler {
	dirPath := filepath.Join(storageDir, subDir)
	os.MkdirAll(dirPath, 0755)
	fsHandler := http.StripPrefix("/"+subDir+"/", http.FileServer(http.Dir(dirPath)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cType != "" {
			w.Header().Set("Content-Type", cType)
		}
		fsHandler.ServeHTTP(w, r)
	})
}

func GetServerPort() int {
	return appConfig.ServerPort
}

func StartServer() {
	storageDir = GetStorageDir()
	log.Printf("Using storage directory: %s", storageDir)

	os.MkdirAll(filepath.Join(storageDir, "md"), 0755)
	os.MkdirAll(filepath.Join(storageDir, "html"), 0755)

	cfgPath := filepath.Join(storageDir, "config.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		json.Unmarshal(data, &appConfig)
	} else {
		appConfig = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
			Author:        "Anonymous",
			UseInternalEd: true,
		}
		data, _ = json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(cfgPath, data, 0644)
	}

	if appConfig.MimeTypes != nil {
		for ext, typ := range appConfig.MimeTypes {
			mime.AddExtensionType(ext, typ)
		}
	}

	ExtractDefaultMarkdown()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveFrontend)
	
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
	mux.HandleFunc("/api/edit-external", authMiddleware(handleEditExternal, true))

	if appConfig.ServerPort <= 0 {
		appConfig.ServerPort = 8080
	}

	bindAddr := fmt.Sprintf("0.0.0.0:%d", appConfig.ServerPort)

	log.Printf("OMN-Go Backend running on %s", bindAddr)
	err := http.ListenAndServe(bindAddr, mux)
	if err != nil {
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "listen" {
			log.Println("Port binding to 0.0.0.0 failed, falling back to 127.0.0.1 (IPv6 Loopback workaround)")
			bindAddr = fmt.Sprintf("127.0.0.1:%d", appConfig.ServerPort)
			log.Printf("OMN-Go Backend running on %s", bindAddr)
			if errFallback := http.ListenAndServe(bindAddr, mux); errFallback != nil {
				log.Fatalf("Server fallback failed: %v", errFallback)
			}
		} else {
			log.Fatalf("Server failed: %v", err)
		}
	}
}
