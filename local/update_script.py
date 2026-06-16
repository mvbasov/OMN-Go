import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.18 Directory Routing Fix...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.17"', 'APP_VERSION = "1.2.18"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.17";', 'const APP_VERSION = "1.2.18";'),
        ("backend/frontend/index.html", "let v = '1.2.17';", "let v = '1.2.18';"),
        ("android/app/build.gradle", "versionCode 10217", "versionCode 10218"),
        ("android/app/build.gradle", 'versionName "1.2.17"', 'versionName "1.2.18"')
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

    # 2. Patch server.go to preserve directory paths
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # Fix A: Stop stripping the directory prefix in serveFrontend
        old_serve_frontend = '''	if strings.HasSuffix(path, ".html") {
		name := strings.TrimSuffix(filepath.Base(path), ".html")'''
        
        new_serve_frontend = '''	if strings.HasSuffix(path, ".html") {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".html")'''
        
        if old_serve_frontend in server_code:
            server_code = server_code.replace(old_serve_frontend, new_serve_frontend)
            print("  [+] Fixed serveFrontend to preserve full directory paths.")

        # Fix B: Upgrade precompileAllPages to use filepath.Walk for subdirectories
        old_precompile = '''func precompileAllPages() {
	mdDir := filepath.Join(storageDir, "md")
	htmlDir := filepath.Join(storageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	files, _ := filepath.Glob(filepath.Join(mdDir, "*.md"))
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err == nil {
			name := strings.TrimSuffix(filepath.Base(f), ".md")
			compiled := compilePage(name, content)
			htmlPath := filepath.Join(htmlDir, name+".html")
			os.WriteFile(htmlPath, compiled, 0644)
		}
	}
}'''

        new_precompile = '''func precompileAllPages() {
	mdDir := filepath.Join(storageDir, "md")
	htmlDir := filepath.Join(storageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	filepath.Walk(mdDir, func(f string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(f, ".md") {
			content, err := os.ReadFile(f)
			if err == nil {
				relPath, _ := filepath.Rel(mdDir, f)
				name := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
				compiled := compilePage(name, content)
				htmlPath := filepath.Join(htmlDir, filepath.Clean(name+".html"))
				os.MkdirAll(filepath.Dir(htmlPath), 0755)
				os.WriteFile(htmlPath, compiled, 0644)
			}
		}
		return nil
	})
}'''

        if old_precompile in server_code:
            server_code = server_code.replace(old_precompile, new_precompile)
            print("  [+] Upgraded precompileAllPages to dynamically traverse subdirectories.")
            
        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)

    commit_msg = """fix(routing): preserve directories for nested markdown files

- Replaced `filepath.Base()` with `strings.TrimPrefix()` in `serveFrontend` so clicking on `[Title](Dir/File)` properly targets `Dir/File.md` instead of incorrectly truncating to `File.md`.
- Replaced `filepath.Glob()` with `filepath.Walk()` in `precompileAllPages()` so nested Markdown files inside subdirectories are natively discovered and precompiled during server startup.
- Bumped application to V1.2.18 (Android 10218)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()