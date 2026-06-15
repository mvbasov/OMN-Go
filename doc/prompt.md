You are an expert Senior Systems Engineer, Android Developer, and Go Expert. Architect and write a cross-platform Markdown note editor (replacing Open Markdown Notes) called "GoOMN". 

The project must use vanilla JavaScript/Tailwind HTML for the web interface, Go for the cross-platform backend server, and a lightweight Docker environment optimized for Linux hosts to compile the Android APK without Android Studio or AndroidX/AppCompat libraries.

### 1. Storage, Package, & Initialization Constraints (F-Droid Ready)
- **Package Frameworks:** Support `net.basov.goomn` (or `net.basov.goomn.fdroid` for F-Droid builds).
- **Storage Isolation:** On Android, strictly target the isolated media storage directory: `/storage/emulated/0/Media/[package_name]`. This ensures the application reads and writes its own files without triggering native Android runtime permission prompts or requesting broader file system access.
- **Auto-Initialization:** On the first run, the Go backend must automatically detect if the storage directory exists. If missing, create it recursively and generate a default `Welcome.md` start page populated with application help instructions and valid Pelican CMS headers.

### 2. Configuration, Security, & Desktop UX
- **Config Management:** Read configuration settings from a local `config.json` file. If missing, initialize it with secure defaults:
  {
    "server_port": 8080,
    "admin_password": "admin_secret_changeme",
    "guest_password": "guest_secret_changeme"
  }
- **Authentication:** Enforce session-based access control. Admin has full read/write rights. Guest has Read-Only (RO) access, completely locking out editing, saving, and upload elements.
- **Desktop Launcher:** On non-Android builds, execute a system shell command (`xdg-open`, `open`, or `start`) upon successful initialization to automatically spin up the default browser targeting the local server URL.
- **Android UI Layer:** A lightweight APK containing the Go HTTP server bound to the local LAN interface. It uses a high-performance native canvas (`golang.org/x/mobile/app`) to display server IP and active connections, completely excluding AppCompat to stay under 5MB.

### 3. Specialized Fast-Capture Viewports
In addition to the default Pelican markdown viewing and full-screen editing states, the frontend must implement two specialized input mechanics:
- **Quick Notes Panel:** A dedicated popup window with a plain-text input. Submitting writes directly to `QuickNotes.md`. The backend must parse `QuickNotes.md`, keep the Pelican header intact at the top, and prepend the new note immediately below the header. Every new quick note entry must be separated by a dynamic timestamp divider formatted exactly like this:
  - - -
  #### YYYY-MM-DD HH:MM:SS

- **Bookmarks Stream Ingestion:** A dedicated link-capture form with inputs for URL, Title, Tags (comma-separated), and Notes. Submitting appends this entry to the top of a JSON array inside a file called `Bookmarks.html`. Note that the Javascript/frontend code to render this data already exists elsewhere. The Go backend only needs to cleanly inject the new structured entry right under the marker line, preserving this exact format:
  ```html
  <script>bookmarks = [
  <!-- Don't edit body below this line -->
    {
      "date": "2026-06-14 22:03:00",
      "url": "https://example.com",
      "title": "Example Title",
      "tags": ["Tag1", "Tag2"],
      "notes": []
    },
    ...
  ```

### 4. Media Handling & Rendering
- **Media Uploads:** Images sent via upload buttons or drag-and-drop are stored in an `/images` subdirectory relative to the note. The UI must instantly insert a valid Pelican reference at the cursor position: `![Description]({filename}/images/filename.jpg)`.
- **Render Engine:** By default, parse Pelican metadata headers into a styled block and convert markdown body text to interactive rich HTML via Marked.js.

### 5. Dockerized Multi-Stage Caching (Linux-Optimized)
Provide a multi-stage `Dockerfile` leveraging aggressive caching rules:
- Stage 1: Cache system environments, Linux Go toolchains, Android SDK/NDK CLI tools, and `gomobile` packages. Rebuilds only on explicit version bumps.
- Stage 2: Copy only `go.mod` and `go.sum` to lock down and cache backend module dependencies cleanly.
- Stage 3: Map application source code, compile multi-architecture desktop binaries, and pack the ultra-slim native Android APK (under 5MB, zero AppCompat/Java UI wrappers).

### 6. Code Generation & Execution Framework (CRITICAL Workflow)
You must deliver the files and all future iterations using a strict, automated system.

#### Turn 1 (Your First Output Only):
Provide a file structure description, build instructions, and a single, self-contained Python script named `setup_project.py`. Running this script must automatically generate all directories and write the full contents of all files (`main.go`, `frontend/index.html`, and `Dockerfile`) to disk. Ensure a global version variable (e.g., `APP_VERSION = "1.0.0"`) is embedded within the generated files.

#### Turn 2 and Onward (All Subsequent Modifications):
Do NOT output full files or standard text diffs. You must respond exclusively with a runnable Python script using standard `str.replace()` mechanisms to modify the existing baseline. 

The Python script must strictly adhere to this template structure:

```python
import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("main.go", 'APP_VERSION = "1.0.0"', 'APP_VERSION = "1.0.1"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.0";', 'const APP_VERSION = "1.0.1";')
    ]
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "main.go": [
            (
                '// old block of code to remove',
                '// new block of code to insert'
            )
        ]
    }

    # Execute updates sequentially...
    # (Implement safe execution that raises ValueError if old target string is missing)

    # 3. Output Standardized Git Commit Message matching your modifications
    commit_msg = """feat(core): description of the change here\n\nVersion bumped to 1.0.1"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()
```

### 7. Output Deliverables
1. **File Structure & Build Instructions** Text description block.
2. `setup_project.py` - The complete python builder script containing the full baseline definitions of `main.go`, `frontend/index.html`, and `Dockerfile`.

