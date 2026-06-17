import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.29 Lazy Asset Extraction (Fixed)...")

    # 1. Version Bumps
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.29"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.29"'),
        "android/app/build.gradle": (r'versionCode 102\d{2}', 'versionCode 10229')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            new_content = re.sub(pattern, replacement, content)
            
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.2\.\d+';", "let v = '1.2.29';", new_content)

            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.2\.\d+"', 'versionName "1.2.29"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Patch server.go to implement Lazy Extraction Strategy
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # A. Remove "io/fs" from imports to prevent compiler panics
        server_code = re.sub(r'\n\s*"io/fs"', '', server_code)
        print("  [+] Removed 'io/fs' import to keep compiler clean.")

        # B. Remove fSys declaration to prevent declared-and-not-used compiler panic
        server_code = re.sub(r'\n\t\tfSys,\s*_\s*:=\s*fs\.Sub\(staticFS,\s*"frontend/html"\)', '', server_code)
        server_code = re.sub(r'\n\t\tfSys,\s*_\s*:=\s*embed\.FS\(staticFS\),\s*error\(nil\)', '', server_code)

        # C. Fix legacy "frontend/md/" paths so EmbedFS priority works
        if '"frontend/md/' in server_code:
            server_code = server_code.replace('"frontend/md/', '"frontend/html/md/')
            print("  [+] Restored EmbedFS priority by fixing the 'html' subdirectory path in fallback routers.")

        # D. Replace serveStrict with the new Lazy Extraction Router (No Bookmarker JS hack)
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
            print("  [+] Rewired serveStrict router with clean Lazy Extraction logic.")
        else:
            print("  [-] Could not find serveStrict block to rewrite.")
            
        with open(server_go, "w", encoding="utf-8") as f:
            f.write(new_server_code)

    commit_msg = """feat(core): implement lazy extraction proxy and embedfs path resolution

- Redesigned the `serveStrict` HTTP router to act as a clean, lazy-loading proxy. 
- Removed legacy hot-patch interception for `Bookmarker.js` as the redesigned JS logic natively resolves its targets.
- Updated all legacy `"frontend/md/"` paths inside `initDefaultPage`, `handleGetNote`, and `serveFrontend` to correctly target `"frontend/html/md/"`. This guarantees `embedFS` extraction takes priority over generating empty `.md` files.
- Removed unused `io/fs` and `fSys` logic entirely, preventing compiler errors.
- Bumped application to V1.2.29 (Android 10229)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()