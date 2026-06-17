import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.35 Fonts Extraction Fix...")

    # 1. Version Bumps
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.35"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.35"'),
        "android/app/build.gradle": (r'versionCode 102\d{2}', 'versionCode 10235')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            new_content = re.sub(pattern, replacement, content)
            
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.2\.\d+';", "let v = '1.2.35';", new_content)

            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.2\.\d+"', 'versionName "1.2.35"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Patch server.go to simplify the router and fix font extraction
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # Find the start of the strict router and the end of its mux registrations
        start_str = "serveStrict := func("
        end_search = 'mux.Handle("/json/",'

        start_idx = server_code.find(start_str)
        end_line_idx = server_code.find(end_search)

        if start_idx != -1 and end_line_idx != -1:
            # Find the end of the line containing the last mux.Handle mapping
            end_idx = server_code.find('\n', end_line_idx)
            if end_idx == -1:
                end_idx = len(server_code)

            lazy_router_logic = '''serveLazyEmbed := func() http.Handler {
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
				
				// Serve the file dynamically from the physical directory
				fsHandler.ServeHTTP(w, r)
			})
		}

		mux.Handle("/js/", serveLazyEmbed())
		mux.Handle("/css/", serveLazyEmbed())
		mux.Handle("/json/", serveLazyEmbed())'''

            # Slice out the overly strict router and replace it with the clean proxy
            server_code = server_code[:start_idx] + lazy_router_logic + server_code[end_idx:]
            print("  [+] Successfully replaced strict router with unified serveLazyEmbed proxy.")
            
            with open(server_go, "w", encoding="utf-8") as f:
                f.write(server_code)
        else:
            print("  [-] CRITICAL: Could not find the serveStrict block bounds!")

    commit_msg = """fix(routing): remove strict extension gatekeeping for lazy extraction

- Replaced `serveStrict` with `serveLazyEmbed`. The previous implementation was blocking `.woff` files because the `/css/fonts/` route was hardcoded to only accept `.woff2` extensions.
- Removed explicit MIME type parameters, relying instead on Go's native `http.FileServer` to negotiate Content-Type mappings (which were previously initialized via `mime.AddExtensionType`).
- Consolidated `/css/fonts/` sub-routing recursively into `/css/`, allowing all embedded nested directories to lazily extract to the physical disk flawlessly upon request.
- Bumped application to V1.2.35 (Android 10235)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()