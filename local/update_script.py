import os

def update_application():
    print("Initiating OMN-Go Phase 1 Update...")

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.1.0"', 'APP_VERSION = "1.2.0"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.1.0";', 'const APP_VERSION = "1.2.0";'),
        ("android/app/build.gradle", 'versionCode 10100', 'versionCode 10200'),
        ("android/app/build.gradle", 'versionName "1.1.0"', 'versionName "1.2.0"')
    ]
    
    for filepath, old_v, new_v in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_v in content:
                content = content.replace(old_v, new_v)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")
            else:
                print(f"  [-] Version string not found in {filepath} (Already updated?)")

    # 2. Define File Patches
    patches = {
        "backend/frontend/index.html": [
            ('<title>GoOMN Editor</title>', '<title>OMN-Go</title>'),
            ('<h2>GoOMN Login</h2>', '<h2>OMN-Go Login</h2>'),
            ('<strong>GoOMN</strong>', '<strong>OMN-Go</strong>'),
            ("'GoOMN v'", "'OMN-Go v'")
        ],
        "android/app/src/main/AndroidManifest.xml": [
            ('android:label="GoOMN"', 'android:label="OMN-Go"')
        ],
        "backend/server.go": [
            ('Welcome to GoOMN', 'Welcome to OMN-Go'),
            ('GoOMN Backend running', 'OMN-Go Backend running'),
            (
                'type Config struct {\n\tServerPort    int    `json:"server_port"`\n\tAdminPassword string `json:"admin_password"`\n\tGuestPassword string `json:"guest_password"`\n}',
                'type Config struct {\n\tServerPort    int    `json:"server_port"`\n\tAdminPassword string `json:"admin_password"`\n\tGuestPassword string `json:"guest_password"`\n\tUseInternalEd bool   `json:"use_internal_editor"`\n\tDesktopExtCmd string `json:"desktop_ext_cmd"`\n}'
            ),
            (
                'GuestPassword: "guest_secret_changeme",\n\t\t}',
                'GuestPassword: "guest_secret_changeme",\n\t\t\tUseInternalEd: true,\n\t\t\tDesktopExtCmd: "subl",\n\t\t}'
            ),
            (
                'func handleLogin(w http.ResponseWriter, r *http.Request) {',
                '''func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(appConfig)
		return
	}
	if r.Method == "POST" {
		appConfig.UseInternalEd = r.FormValue("use_internal_editor") == "true"
		appConfig.DesktopExtCmd = r.FormValue("desktop_ext_cmd")
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(filepath.Join(storageDir, "config.json"), data, 0644)
		w.Write([]byte("Config Saved"))
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {'''
            ),
            (
                'mux.HandleFunc("/login", handleLogin)',
                'mux.HandleFunc("/login", handleLogin)\n\t\tmux.HandleFunc("/api/config", authMiddleware(handleConfig, true))'
            )
        ]
    }

    # Execute patches safely
    for filepath, file_patches in patches.items():
        if not os.path.exists(filepath):
            print(f"  [!] Missing file: {filepath}")
            continue
            
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()
            
        for old_str, new_str in file_patches:
            if old_str in content:
                content = content.replace(old_str, new_str)
                print(f"  [+] Patched target in {filepath}")
            elif new_str in content:
                print(f"  [=] Target already patched in {filepath}")
            else:
                print(f"  [!] WARNING: Target string missing in {filepath}")
                
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content)

    commit_msg = """feat(core): rename GoOMN to OMN-Go and scaffold Config API

- Executed global string replacement for OMN-Go rebranding.
- Injected UseInternalEd and DesktopExtCmd into core Config struct.
- Added /api/config GET/POST endpoint for upcoming settings page.
- Bumped version to 1.2.0 (Android 10200)."""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()