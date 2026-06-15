import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.42"', 'APP_VERSION = "1.0.43"')
    ]
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "server.go": [
            # Patch 1: Inject "mime" into imports
            (
                r'''	"log"
	"net/http"''',
                r'''	"log"
	"mime"
	"net/http"'''
            ),
            
            # Patch 2: Add handleUploadJSON right under handleUpload
            (
                r'''	w.Write([]byte(fmt.Sprintf("![%s]({filename}/images/%s)", header.Filename, header.Filename)))
}''',
                r'''	w.Write([]byte(fmt.Sprintf("![%s]({filename}/images/%s)", header.Filename, header.Filename)))
}

func handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	jsonDir := filepath.Join(storageDir, "user_json")
	os.MkdirAll(jsonDir, 0755)
	
	destPath := filepath.Join(jsonDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)
	
	w.Write([]byte(fmt.Sprintf("[%s]({filename}/user_json/%s)", header.Filename, header.Filename)))
}'''
            ),
            
            # Patch 3: Force global MIME types for minimal Docker environments
            (
                r'''func StartServer() {
	initStorage() // Execute synchronously to ensure config is loaded instantly
	go func() {''',
                r'''func StartServer() {
	initStorage() // Execute synchronously to ensure config is loaded instantly
	
	// Fallback MIME types for minimal Docker containers
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".webp", "image/webp")
	mime.AddExtensionType(".png", "image/png")
	mime.AddExtensionType(".jpg", "image/jpeg")
	mime.AddExtensionType(".jpeg", "image/jpeg")
	mime.AddExtensionType(".gif", "image/gif")
	mime.AddExtensionType(".json", "application/json")

	go func() {'''
            ),
            
            # Patch 4: Add Directory-based Content-Type routing and fix /images/
            (
                r'''		mux.Handle("/js/", serveStrict(".js", "application/javascript"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))
		mux.Handle("/json/", serveStrict(".json", "application/json"))
		mux.HandleFunc("/login", handleLogin)
		mux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
		mux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
		mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
		mux.HandleFunc("/api/note", handleGetNote)
		mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))''',
                r'''		mux.Handle("/js/", serveStrict(".js", "application/javascript"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))
		mux.Handle("/json/", serveStrict(".json", "application/json"))
		
		// Config for files handling Content-type by served directories
		serveStorageDir := func(subDir, cType string) http.Handler {
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
		
		mux.Handle("/images/", serveStorageDir("images", ""))
		mux.Handle("/user_json/", serveStorageDir("user_json", "application/json"))

		mux.HandleFunc("/login", handleLogin)
		mux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
		mux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
		mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
		mux.HandleFunc("/api/upload_json", authMiddleware(handleUploadJSON, true))
		mux.HandleFunc("/api/note", handleGetNote)
		mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))'''
            )
        ]
    }

    # Execute Version Bump
    for filepath, old_v, new_v in version_replacements:
        # Fallback to backend/ directory from v1.0.22 restructuring
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if os.path.exists(actual_path):
            with open(actual_path, 'r', encoding='utf-8') as f:
                content = f.read()
            if old_v in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old_v, new_v))

    # Execute Updates Sequentially with Newline Normalization
    for filepath, file_patches in patches.items():
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if not os.path.exists(actual_path):
            print(f"Skipping {actual_path} - file not found.")
            continue

        with open(actual_path, 'r', encoding='utf-8') as f:
            content = f.read()

        # Normalize newlines for strict matching
        original_newlines = "\r\n" if "\r\n" in content else "\n"
        normalized_content = content.replace("\r\n", "\n")

        for old_str, new_str in file_patches:
            old_norm = old_str.replace("\r\n", "\n")
            new_norm = new_str.replace("\r\n", "\n")
            
            if old_norm in normalized_content:
                normalized_content = normalized_content.replace(old_norm, new_norm)
            elif new_norm in normalized_content:
                print(f"Patch already applied in {actual_path}")
            else:
                raise ValueError(f"Target string not found in {actual_path}:\n{old_norm}")

        # Restore original newlines to respect system structure
        final_content = normalized_content.replace("\n", original_newlines)

        with open(actual_path, 'w', encoding='utf-8') as f:
            f.write(final_content)
        print(f"Successfully patched {actual_path}")

    # 3. Output Standardized Git Commit Message
    commit_msg = """feat(backend): configure directory-based content types and add JSON uploads
    
- Implement `/images/` static serving explicitly routed to isolated storage
- Configure global `mime.AddExtensionType` fallbacks for minimal Docker instances
- Add `handleUploadJSON` API endpoint backing directly to `/user_json/`

Version bumped to 1.0.43"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()