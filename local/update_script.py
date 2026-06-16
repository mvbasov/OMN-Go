import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.9 EmbedFS Extraction & Manifest Fix...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.8"', 'APP_VERSION = "1.2.9"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.8";', 'const APP_VERSION = "1.2.9";'),
        ("backend/frontend/index.html", "let v = '1.2.8';", "let v = '1.2.9';"),
        ("android/app/build.gradle", "versionCode 10208", "versionCode 10209"),
        ("android/app/build.gradle", 'versionName "1.2.8"', 'versionName "1.2.9"')
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

    # 2. Patch AndroidManifest.xml (Remove deprecated package attribute)
    manifest_path = "android/app/src/main/AndroidManifest.xml"
    if os.path.exists(manifest_path):
        with open(manifest_path, "r", encoding="utf-8") as f:
            manifest_content = f.read()
        
        # Remove the attribute completely to silence the Gradle warning
        if 'package="net.basov.omngo"' in manifest_content:
            manifest_content = manifest_content.replace('package="net.basov.omngo"', '')
            with open(manifest_path, "w", encoding="utf-8") as f:
                f.write(manifest_content)
            print(f"  [+] Removed deprecated package attribute from {manifest_path}")
        else:
            print(f"  [=] Deprecated package attribute already removed from {manifest_path}")

    # 3. Patch backend/server.go for EmbedFS extraction
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        old_serve_bottom = """	// Priority 1: User's Local Storage Directory (data/css, data/js, etc)
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

        new_serve_bottom = """	// Priority 1: User's Local Storage Directory (data/css, data/js, etc)
	filePath := filepath.Join(storageDir, filepath.Clean(path))
	if stat, err := os.Stat(filePath); err == nil && !stat.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}

	// Priority 2: Embedded Fallback Template Cache - Copy to Data
	embedPath := "frontend" + filepath.Clean(path)
	if data, err := staticFS.ReadFile(embedPath); err == nil {
		if path == "/js/Bookmarker.js" {
			js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
			js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
			data = []byte(js)
		}
		
		// Copy extracted file directly to user data directory
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, data, 0644)
		
		w.Write(data)
		return
	}

	http.NotFound(w, r)
}"""

        if old_serve_bottom in server_code:
            server_code = server_code.replace(old_serve_bottom, new_serve_bottom)
            with open(server_go, "w", encoding="utf-8") as f:
                f.write(server_code)
            print("  [+] Rewrote serveFrontend to copy files from embedFS to local data directory.")
        else:
            print("  [-] Could not find the specific block to patch in server.go.")

    commit_msg = """refactor(core): copy embed assets to disk and fix manifest warning

- Removed legacy 'package' attribute from AndroidManifest.xml to comply with modern AGP namespace declarations.
- Updated the fallback static router logic to automatically extract embedded files (CSS/JS) to the physical local data directory upon first request.
- Bumped application to V1.2.9 (Android 10209)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()