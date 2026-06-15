import os
import re

def update_application():
    # Dynamically generate backticks to completely bypass chat UI truncation!
    ticks = chr(96) * 3

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.37"', 'APP_VERSION = "1.0.38"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.37";', 'const APP_VERSION = "1.0.38";')
    ]
    
    for filepath, old, new in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            if old in content:
                with open(filepath, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old, new))

    # 2. Bump the Android Version in Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            gradle_content = f.read()
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10038', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.38"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Define File Patches
    patches = {
        "backend/frontend/index.html": [
            (
                # Remove dirty regex hack from executeScripts
                r'''        function executeScripts(container) {
            const scripts = container.querySelectorAll('script');
            scripts.forEach(oldScript => {
                const newScript = document.createElement('script');
                Array.from(oldScript.attributes).forEach(attr => newScript.setAttribute(attr.name, attr.value));
                newScript.async = false;
                if (oldScript.innerHTML) {
                    let code = oldScript.innerHTML;
                    code = code.replace(/\bconst\s+/g, 'var ').replace(/\blet\s+/g, 'var ');
                    newScript.appendChild(document.createTextNode(code));
                }
                oldScript.parentNode.replaceChild(newScript, oldScript);
            });
        }''',
                r'''        function executeScripts(container) {
            const scripts = container.querySelectorAll('script');
            scripts.forEach(oldScript => {
                const newScript = document.createElement('script');
                Array.from(oldScript.attributes).forEach(attr => newScript.setAttribute(attr.name, attr.value));
                newScript.async = false;
                if (oldScript.innerHTML) newScript.appendChild(document.createTextNode(oldScript.innerHTML));
                oldScript.parentNode.replaceChild(newScript, oldScript);
            });
        }'''
            )
        ],
        "backend/server.go": [
            (
                # Remove dirty regex hack from Bookmarker.js backend server pipeline
                r'''						js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
						js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
						js = strings.ReplaceAll(js, "const ", "var ")
						js = strings.ReplaceAll(js, "let ", "var ")
						w.Write([]byte(js))
						return''',
                r'''						js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
						js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
						w.Write([]byte(js))
						return'''
            ),
            (
                # Add ScriptRules.md generation to server startup routines and link it in Welcome.md
                r'''	welcomePath := filepath.Join(mdDir, "Welcome.md")
	if _, err := os.Stat(welcomePath); os.IsNotExist(err) {
		os.WriteFile(welcomePath, []byte("Title: Welcome to GoOMN\nDate: 2026-06-14\nCategory: System\n\n# Welcome to GoOMN\n\nThis is your offline-first, locally-hosted Markdown editor.\n\n- [Quick Notes](QuickNotes.md)\n- [Bookmarks](Bookmarks.md)\n\nNavigate using valid relative links."), 0644)
	}''',
                r'''	welcomePath := filepath.Join(mdDir, "Welcome.md")
	if _, err := os.Stat(welcomePath); os.IsNotExist(err) {
		os.WriteFile(welcomePath, []byte("Title: Welcome to GoOMN\nDate: 2026-06-14\nCategory: System\n\n# Welcome to GoOMN\n\nThis is your offline-first, locally-hosted Markdown editor.\n\n- [Quick Notes](QuickNotes.md)\n- [Bookmarks](Bookmarks.md)\n- [Scripting Rules](ScriptRules.md)\n\nNavigate using valid relative links."), 0644)
	}

	rulesPath := filepath.Join(mdDir, "ScriptRules.md")
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		os.WriteFile(rulesPath, []byte("Title: JS Scripting Rules\nDate: 2026-06-15\nCategory: System\n\n# JavaScript Guidelines for GoOMN\n\nBecause GoOMN is a Single Page Application (SPA), the global `window` scope persists between page loads. To avoid `SyntaxError: Identifier has already been declared` when scripts are re-evaluated, authors must follow these rules:\n\n### Rule 1: Isolate variables using Block Scopes or IIFEs\nNever leave `const` or `let` in the top-level global scope. Wrap the script in an Anonymous Block `{ ... }` or an Immediately Invoked Function Expression (IIFE).\n\n"''' + "\n" + ticks + '''javascript\n{\n    const myLocalVar = "Safe!";\n    let counter = 0;\n}\n''' + ticks + '''\n\n### Rule 2: Explicitly attach required globals to `window`\nIf a function is needed for an HTML `onclick` event, attach it directly to the `window` object.\n\n''' + ticks + '''javascript\nwindow.doSomething = function() {\n    alert("This works safely on reload!");\n};\n''' + ticks + '''\n\n### Rule 3: Use the OR (`||`) operator for global state\nCheck if global config objects exist before creating them so user state is preserved.\n\n''' + ticks + '''javascript\nwindow.myAppConfig = window.myAppConfig || { version: "1.0" };\n''' + ticks + '''\n\n### Rule 4: Use `var` for raw top-level variables\nIf you must declare top-level variables, use `var` because the JS engine allows `var` to be redeclared infinitely without throwing an error."), 0644)
	}'''
            )
        ]
    }

    # Execute Patches Sequentially
    for filepath_target, file_patches in patches.items():
        if os.path.exists(filepath_target):
            with open(filepath_target, 'r', encoding='utf-8') as f:
                content = f.read()
            for old, new in file_patches:
                if old in content:
                    content = content.replace(old, new)
                elif new not in content:
                    print(f"Warning: Could not find patch target in {filepath_target}:\n{old[:50]}...")
            with open(filepath_target, 'w', encoding='utf-8') as f:
                f.write(content)

    # 4. Push updates to Host files immediately so the user doesn't have to delete their storage dir to trigger the backend init
    for storage_dir in ["data/md", "android/app/media/net.basov.goomn/md"]:
        welcome_path = os.path.join(storage_dir, "Welcome.md")
        if os.path.exists(welcome_path):
            with open(welcome_path, "r", encoding="utf-8") as f:
                content = f.read()
            if "ScriptRules.md" not in content:
                content = content.replace("- [Bookmarks](Bookmarks.md)", "- [Bookmarks](Bookmarks.md)\n- [Scripting Rules](ScriptRules.md)")
                with open(welcome_path, "w", encoding="utf-8") as f:
                    f.write(content)
        
        rules_path = os.path.join(storage_dir, "ScriptRules.md")
        if os.path.exists(storage_dir) and not os.path.exists(rules_path):
            with open(rules_path, "w", encoding="utf-8") as f:
                f.write("Title: JS Scripting Rules\nDate: 2026-06-15\nCategory: System\n\n# JavaScript Guidelines for GoOMN\n\nBecause GoOMN is a Single Page Application (SPA), the global `window` scope persists between page loads. To avoid `SyntaxError: Identifier has already been declared` when scripts are re-evaluated, authors must follow these rules:\n\n### Rule 1: Isolate variables using Block Scopes or IIFEs\nNever leave `const` or `let` in the top-level global scope. Wrap the script in an Anonymous Block `{ ... }` or an Immediately Invoked Function Expression (IIFE).\n\n" + ticks + "javascript\n{\n    const myLocalVar = \"Safe!\";\n    let counter = 0;\n}\n" + ticks + "\n\n### Rule 2: Explicitly attach required globals to `window`\nIf a function is needed for an HTML `onclick` event, attach it directly to the `window` object.\n\n" + ticks + "javascript\nwindow.doSomething = function() {\n    alert(\"This works safely on reload!\");\n};\n" + ticks + "\n\n### Rule 3: Use the OR (`||`) operator for global state\nCheck if global config objects exist before creating them so user state is preserved.\n\n" + ticks + "javascript\nwindow.myAppConfig = window.myAppConfig || { version: \"1.0\" };\n" + ticks + "\n\n### Rule 4: Use `var` for raw top-level variables\nIf you must declare top-level variables, use `var` because the JS engine allows `var` to be redeclared infinitely without throwing an error.")

    # 5. Output Standardized Git Commit Message
    commit_msg = """refactor(markdown): decouple dirty js injection hacks and enforce strict authoring rules

- Stripped out all backend and frontend regex routines that forcefully converted user `const`/`let` inputs to `var`.
- Restored standard ES6 evaluation semantics inside the `executeScripts` pipeline and `Bookmarker.js` serving route.
- Intercepted backend storage initialization to dynamically bootstrap `ScriptRules.md` payload.
- Injected `[Scripting Rules]` navigation links directly into standard `Welcome.md` UI so users are natively educated on avoiding SPA global scope conflicts.
- Bumped Android versionCode to 10038

Version bumped to 1.0.38"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.38!")

if __name__ == "__main__":
    update_application()