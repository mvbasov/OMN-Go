import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.8 Configurable MIME Engine Update...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.7"', 'APP_VERSION = "1.2.8"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.7";', 'const APP_VERSION = "1.2.8";'),
        ("backend/frontend/index.html", "let v = '1.2.7';", "let v = '1.2.8';"),
        ("android/app/build.gradle", "versionCode 10207", "versionCode 10208"),
        ("android/app/build.gradle", 'versionName "1.2.7"', 'versionName "1.2.8"')
    ]

    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Advanced Go Backend Replacements
    server_go = "backend/server.go"
    if not os.path.exists(server_go):
        print(f"  [!] Missing {server_go}")
        return

    with open(server_go, "r", encoding="utf-8") as f:
        server_code = f.read()

    # A. Inject MimeTypes into Config Struct
    old_config = """type Config struct {
	ServerPort    int    `json:"server_port"`
	AdminPassword string `json:"admin_password"`
	GuestPassword string `json:"guest_password"`
	UseInternalEd bool   `json:"use_internal_editor"`
	DesktopExtCmd string `json:"desktop_ext_cmd"`
}"""
    new_config = """type Config struct {
	ServerPort    int               `json:"server_port"`
	AdminPassword string            `json:"admin_password"`
	GuestPassword string            `json:"guest_password"`
	UseInternalEd bool              `json:"use_internal_editor"`
	DesktopExtCmd string            `json:"desktop_ext_cmd"`
	MimeTypes     map[string]string `json:"mime_types"`
}"""
    if old_config in server_code:
        server_code = server_code.replace(old_config, new_config)
        print("  [+] Appended MimeTypes map to Config Struct.")

    # B. Inject MimeTypes defaults into initStorage
    old_init_config = """	configPath := filepath.Join(storageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		appConfig = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
			UseInternalEd: true,
			DesktopExtCmd: "subl",
		}
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(configPath, data, 0644)
	} else {
		data, _ := os.ReadFile(configPath)
		json.Unmarshal(data, &appConfig)
	}"""
    new_init_config = """	configPath := filepath.Join(storageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		appConfig = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
			UseInternalEd: true,
			DesktopExtCmd: "subl",
			MimeTypes: map[string]string{
				".css":   "text/css",
				".js":    "application/javascript",
				".json":  "application/json",
				".html":  "text/html",
				".md":    "text/markdown",
				".svg":   "image/svg+xml",
				".png":   "image/png",
				".jpg":   "image/jpeg",
				".jpeg":  "image/jpeg",
				".woff2": "font/woff2",
			},
		}
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(configPath, data, 0644)
	} else {
		data, _ := os.ReadFile(configPath)
		json.Unmarshal(data, &appConfig)
		if appConfig.MimeTypes == nil {
			appConfig.MimeTypes = map[string]string{
				".css":   "text/css",
				".js":    "application/javascript",
				".json":  "application/json",
				".woff2": "font/woff2",
			}
			data, _ := json.MarshalIndent(appConfig, "", "  ")
			os.WriteFile(configPath, data, 0644)
		}
	}"""
    if old_init_config in server_code:
        server_code = server_code.replace(old_init_config, new_init_config)
        print("  [+] Injected MIME Defaults to config.json.")

    # C. Refactor serveFrontend bottom logic to handle extensions and priorities
    old_serve_bottom = """	filePath := filepath.Join(storageDir, filepath.Clean(path))
	if _, err := os.Stat(filePath); err == nil {
		http.ServeFile(w, r, filePath)
		return
	}

	http.NotFound(w, r)
}"""
    new_serve_bottom = """	// Unified Content-Type Resolver based strictly on extension
	ext := strings.ToLower(filepath.Ext(path))
	mimeType, exists := appConfig.MimeTypes[ext]
	if !exists {
		mimeType = mime.TypeByExtension(ext)
	}
	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}

	// Priority 1: User's Local Storage Directory (data/css, data/js, etc)
	filePath := filepath.Join(storageDir, filepath.Clean(path))
	if stat, err := os.Stat(filePath); err == nil && !stat.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}

	// Priority 2: Embedded Fallback Template Cache
	embedPath := "frontend" + filepath.Clean(path)
	if data, err := staticFS.ReadFile(embedPath); err == nil {
		if path == "/js/Bookmarker.js" {
			js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
			js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
			w.Write([]byte(js))
			return
		}
		w.Write(data)
		return
	}

	http.NotFound(w, r)
}"""
    if old_serve_bottom in server_code:
        server_code = server_code.replace(old_serve_bottom, new_serve_bottom)
        print("  [+] Rewrote serveFrontend prioritization and MIME handler.")

    # D. Completely wipe old hardcoded static routing rules from StartServer
    new_start_server_tail = """func StartServer() {
	initStorage() // Execute synchronously to ensure config is loaded instantly
	
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", serveFrontend)

		mux.HandleFunc("/login", handleLogin)
		mux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
		mux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
		mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
		mux.HandleFunc("/api/upload_json", authMiddleware(handleUploadJSON, true))
		mux.HandleFunc("/api/note", handleGetNote)
		mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))
		mux.HandleFunc("/api/config", authMiddleware(handleConfig, true))
		mux.HandleFunc("/api/edit-external", authMiddleware(handleEditExternal, true))
		
		port := fmt.Sprintf(":%d", appConfig.ServerPort)
		log.Printf("OMN-Go Backend running on %s", port)
		http.ListenAndServe(port, connectionMiddleware(mux))
	}()
}

// GetServerPort safely exposes the configured port for frontend wrappers
func GetServerPort() int {
	return appConfig.ServerPort
}
"""
    # Rip out everything from func StartServer() { to the end of the file safely
    server_code = re.sub(r'func StartServer\(\) \{.*', lambda _: new_start_server_tail, server_code, flags=re.DOTALL)
    print("  [+] Stripped redundant static directory routers from StartServer.")

    with open(server_go, "w", encoding="utf-8") as f:
        f.write(server_code)

    commit_msg = """refactor(router): unified static handlers and configurable MIME types

- Introduced customizable 'mime_types' dictionary into the Config struct / config.json.
- Removed strict directory bounds for /js/ and /css/ allowing unified traversal.
- Upgraded the static router to prioritize the local 'data/' directory before checking the embed.FS fallback, resolving custom CSS/JS 404 errors.
- Assigned Content-Type headers exclusively based on configured file extensions, eliminating false 'text/plain' 404 sniffs.
- Bumped application to V1.2.8 (Android 10208)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()