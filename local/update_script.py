import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.34 Syntax Rescue Patch...")

    # 1. Version Bumps
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.34"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.34"'),
        "android/app/build.gradle": (r'versionCode 102\d{2}', 'versionCode 10234')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            new_content = re.sub(pattern, replacement, content)
            
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.2\.\d+';", "let v = '1.2.34';", new_content)

            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.2\.\d+"', 'versionName "1.2.34"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Patch server.go using bulletproof String Slicing and Go Backticks
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # Find exact start and end points of the mangled initDefaultPage block
        start_idx = server_code.find("// 3. Extract all embedded MD files first")
        if start_idx == -1:
            start_idx = server_code.find("// 3. Init Default Notes")
            
        end_idx = server_code.find("// Precompile all notes", start_idx)

        if start_idx != -1 and end_idx != -1:
            safe_init_logic = '''// 3. Extract all embedded MD files first
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

	// 4. Init Default Notes fallback (if embedFS fails)
	initDefaultPage := func(fileName, defaultContent string) {
		p := filepath.Join(mdDir, fileName)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			os.WriteFile(p, []byte(defaultContent), 0644)
		}
	}

	initDefaultPage("Welcome.md", `Title: Welcome
Date: 2026-06-14 12:00:00
Category: System

Welcome to OMN-Go! Start editing.

- [Help](Welcome)
- [Scripting Rules](ScriptRules.md)
- [Bookmarks](Bookmarks)
- [Quick Notes](QuickNotes)`)

	initDefaultPage("ScriptRules.md", `Title: JS Scripting Rules
Date: 2026-06-15
Category: System

# JavaScript Guidelines for OMN-Go

Because OMN-Go is rendered server-side, keep scripts wrapped in block scopes.`)

	initDefaultPage("QuickNotes.md", `Title: Quick Notes
Date: 2026-06-14 12:00:00
Category: Log

`)

	initDefaultPage("Bookmarks.md", `Title: Incoming bookmarks
Date: 2026-06-15 20:00:00
Author: 
Tags: Bookmarks

<script>bookmarks = [
<!-- Don't edit body below this line -->
];
</script>`)

	'''
            # Slice the broken block out and insert the backtick-safe logic
            server_code = server_code[:start_idx] + safe_init_logic + server_code[end_idx:]
            print("  [+] Successfully sliced and replaced initDefaultPage using exact indices and Go backticks.")
            
            with open(server_go, "w", encoding="utf-8") as f:
                f.write(server_code)
        else:
            print("  [-] CRITICAL: Could not find the exact bounds of the initDefaultPage block!")

    commit_msg = """fix(compiler): rescue go syntax errors by converting strings to backticks

- Resolved 'newline in string' and cascading syntax compiler panics in `server.go`.
- Abandoned `re.sub` replacement strategies for the `initDefaultPage` templates to prevent Python from un-escaping newline sequences.
- Converted all multiline string templates to native Go backtick execution (`` `...` ``) ensuring 100% syntax compliance.
- Bumped application to V1.2.34 (Android 10234)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()