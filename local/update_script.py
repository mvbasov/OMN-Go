import os

def update_application():
    print("[*] Applying OMN-Go V1.2.1 External Editor Fix...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.0"', 'APP_VERSION = "1.2.1"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.0";', 'const APP_VERSION = "1.2.1";'),
        ("backend/frontend/index.html", "let v = '1.2.0';", "let v = '1.2.1';"),
        ("android/app/build.gradle", 'versionCode 10200', 'versionCode 10201'),
        ("android/app/build.gradle", 'versionName "1.2.0"', 'versionName "1.2.1"')
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

    # 2. Patch handleEditExternal in server.go
    server_go = "backend/server.go"
    
    old_cmd_block = """\tcmd := exec.Command(appConfig.DesktopExtCmd, filePath)
	err := cmd.Start()
	if err != nil {
		log.Printf("Failed to run external editor: %v", err)
	}"""

    new_cmd_block = """\tvar cmd *exec.Cmd
	cmdStr := strings.TrimSpace(appConfig.DesktopExtCmd)
	
	if cmdStr == "" {
		switch runtime.GOOS {
		case "linux":
			cmd = exec.Command("xdg-open", filePath)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
		case "darwin":
			cmd = exec.Command("open", filePath)
		}
	} else {
		parts := strings.Fields(cmdStr)
		if len(parts) > 0 {
			args := append(parts[1:], filePath)
			cmd = exec.Command(parts[0], args...)
		}
	}

	if cmd != nil {
		err := cmd.Start()
		if err != nil {
			log.Printf("Failed to run external editor: %v", err)
		}
	} else {
		log.Printf("Failed to run external editor: no command configured")
	}"""

    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            content = f.read()
            
        if old_cmd_block in content:
            content = content.replace(old_cmd_block, new_cmd_block)
            with open(server_go, "w", encoding="utf-8") as f:
                f.write(content)
            print("  [+] Patched external editor command logic in server.go")
        elif new_cmd_block in content:
            print("  [=] External editor logic already patched.")
        else:
            print("  [!] WARNING: Target block not found in server.go")

    commit_msg = """fix(editor): handle empty command and command arguments for external editor

- Resolved 'exec: no command' error when DesktopExtCmd is empty by falling back to OS defaults (xdg-open/open).
- Implemented argument splitting via strings.Fields to support editor commands with arguments (e.g. 'code -n').
- Bumped application version to 1.2.1 (Android 10201)."""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()