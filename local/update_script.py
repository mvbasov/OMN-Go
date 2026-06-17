import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.27 Force Compiler Fix...")

    # 1. Robust Regex Version Bumps (catches ANY 1.2.x version)
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.27"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.27"'),
        "android/app/build.gradle": (r'versionCode 102\d{2}', 'versionCode 10227')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            new_content = re.sub(pattern, replacement, content)
            
            # Special second pass for index.html 'let v'
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.2\.\d+';", "let v = '1.2.27';", new_content)

            # Special second pass for build.gradle versionName
            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.2\.\d+"', 'versionName "1.2.27"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Force-bumped version in {filepath}")
            else:
                print(f"  [=] Version already updated in {filepath}")

    # 2. Forcefully rewrite the StartServer block in server.go
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # A. Ensure 'io/fs' is imported
        if '"io/fs"' not in server_code:
            server_code = server_code.replace('"io"', '"io"\n\t"io/fs"')
            print("  [+] Imported 'io/fs' package.")

        # B. Rewrite StartServer completely to avoid regex misses
        start_server_pattern = r'func StartServer\(\) \{.*?\n\t\}\(\)\n\}'
        
        new_start_server = '''func StartServer() {
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
		mux.HandleFunc("/", serveFrontend)
		fSys, _ := fs.Sub(staticFS, "frontend/html")
		
		serveStrict := func(ext, cType string) http.Handler {
			fsHandler := http.FileServer(http.FS(fSys))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, ext) {
					http.Error(w, "Forbidden: Invalid file extension", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", cType)
				
				if r.URL.Path == "/js/Bookmarker.js" {
					data, err := staticFS.ReadFile("frontend/html/js/Bookmarker.js")
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
		mux.Handle("/css/fonts/", serveStrict(".woff2", "font/woff2"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))
		mux.Handle("/json/", serveStrict(".json", "application/json"))
		
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
		mux.HandleFunc("/api/config", authMiddleware(handleConfig, true))
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
}'''

        new_server_code = re.sub(start_server_pattern, new_start_server, server_code, flags=re.DOTALL)
        
        if new_server_code != server_code:
            with open(server_go, "w", encoding="utf-8") as f:
                f.write(new_server_code)
            print("  [+] Forcefully rewrote StartServer() to guarantee fs.Sub injection.")
        else:
            print("  [-] Could not find StartServer block. Manual review may be needed.")

    commit_msg = """fix(compiler): rewrite server routing block to guarantee fs.Sub application

- Used wildcard Regular Expressions to bump version strings, catching edge cases where previous script runs failed or were skipped.
- Rewrote the entire `StartServer` block in `server.go` to explicitly enforce `fs.Sub(staticFS, "frontend/html")` bypassing code formatting inconsistencies that broke previous regex attempts.
- Bumped application to V1.2.27 (Android 10227)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()