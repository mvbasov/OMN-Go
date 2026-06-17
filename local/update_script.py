import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.31 Path Resolution Fix...")

    # 1. Version Bumps
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.31"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.31"'),
        "android/app/build.gradle": (r'versionCode 102\d{2}', 'versionCode 10231')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            new_content = re.sub(pattern, replacement, content)
            
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.2\.\d+';", "let v = '1.2.31';", new_content)

            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.2\.\d+"', 'versionName "1.2.31"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Patch server.go
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # A. Remove "io/fs" and fSys to prevent compiler panics
        server_code = re.sub(r'\n\s*"io/fs"', '', server_code)
        server_code = re.sub(r'\n\t\tfSys,\s*_\s*:=\s*fs\.Sub\(staticFS,\s*"frontend/html"\)', '', server_code)
        server_code = re.sub(r'\n\t\tfSys,\s*_\s*:=\s*embed\.FS\(staticFS\),\s*error\(nil\)', '', server_code)
        
        # B. Fix embed directive to explicitly bundle the separate md directory
        old_embed = r'//go:embed frontend/html\nvar staticFS embed\.FS'
        new_embed = '//go:embed frontend/html frontend/md\nvar staticFS embed.FS'
        if re.search(old_embed, server_code):
            server_code = re.sub(old_embed, new_embed, server_code)
            print("  [+] Added frontend/md to staticFS embed directive.")

        # Revert any incorrect "frontend/html/md" string paths back to "frontend/md"
        if '"frontend/html/md' in server_code:
            server_code = server_code.replace('"frontend/html/md', '"frontend/md')
            print("  [+] Reverted incorrect frontend/html/md/ paths to match actual file tree.")

        # C. Replace initDefaultPage logic to extract all MD files from the correct path
        init_pattern = r'// 3\. Init Default Notes.*?\n\tinitDefaultPage := func\(fileName, defaultContent string\) \{.*?\n\t\}'
        
        new_init_logic = '''	// 3. Extract all embedded MD files first
	if entries, err := staticFS.ReadDir("frontend/md"); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				p := filepath.Join(mdDir, entry.Name())
				if _, err := os.Stat(p); os.IsNotExist(err) {
					if data, err := staticFS.ReadFile("frontend/md/" + entry.Name()); err == nil {
						os.WriteFile(p, data, 0644)
					}
				}
			}
		}
	}

	// 4. Init Default Notes fallback
	initDefaultPage := func(fileName, defaultContent string) {
		p := filepath.Join(mdDir, fileName)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			os.WriteFile(p, []byte(defaultContent), 0644)
		}
	}'''

        new_server_code = re.sub(init_pattern, new_init_logic, server_code, flags=re.DOTALL)
        if new_server_code != server_code:
            print("  [+] Overhauled initDefaultPage to dynamically extract embedded Markdown templates.")
            server_code = new_server_code
        else:
            print("  [-] Could not find initDefaultPage block to rewrite.")

        # D. Replace serveStrict with clean Lazy Extraction Router
        serve_strict_pattern = r'serveStrict := func\(ext,\s*cType\s*string\)\s*http\.Handler\s*\{.*?fsHandler\.ServeHTTP\(w,\s*r\)\s*\}\)\s*\}'
        
        lazy_router_logic = '''serveStrict := func(ext, cType string) http.Handler {
			physicalDir := filepath.Join(storageDir, "html")
			fsHandler := http.FileServer(http.Dir(physicalDir))
			
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, ext) {
					http.Error(w, "Forbidden: Invalid file extension", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", cType)
				
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
				
				// Serve the file dynamically from the physical directory
				fsHandler.ServeHTTP(w, r)
			})
		}'''
        
        new_server_code = re.sub(serve_strict_pattern, lazy_router_logic, server_code, flags=re.DOTALL)
        if new_server_code != server_code:
            print("  [+] Removed Bookmarker.js hot-patch and deployed clean Lazy Router.")
            server_code = new_server_code
        else:
            print("  [-] Could not find serveStrict block to rewrite.")

        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)

    commit_msg = """feat(core): dynamic markdown extraction and correct embedFS pathing

- Updated `go:embed` directive to explicitly bundle both `frontend/html` and `frontend/md` directories.
- Rewrote `staticFS` internal paths to correctly point to `frontend/md/` matching the physical source tree.
- Overhauled `initStorage` to dynamically iterate through all embedded Markdown files via `staticFS.ReadDir()` and extract them to the local disk.
- Simplified `initDefaultPage` to serve purely as an emergency fallback generator.
- Removed unused `io/fs` and `fSys` declarations to guarantee clean compilation.
- Bumped application to V1.2.31 (Android 10231)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()