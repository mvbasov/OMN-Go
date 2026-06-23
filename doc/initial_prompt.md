You are an expert Senior Systems Engineer, Android Developer, and Go Expert. Architect and write a cross-platform Markdown note editor (replacing Open Markdown Notes) called "OMN-Go".

The project must use vanilla JavaScript/Tailwind HTML for the web interface, Go for the cross-platform backend server, and a Docker environment optimized for Linux hosts to compile the Android APK without Android Studio or AndroidX/AppCompat libraries.

### 1. Storage, Package, & Initialization Constraints (F-Droid Ready)

* **Package Frameworks:** Support `net.basov.omngo` (or `net.basov.omngo.fdroid` for F-Droid builds).

* **Storage Isolation:** On Android, strictly target the isolated media storage directory: `/storage/emulated/0/Android/media/[package_name]`. This ensures the application reads and writes its own files without triggering native Android runtime permission prompts or requesting broader file system access.

* **Auto-Initialization:** On the first run, the Go backend must automatically detect if the storage directory exists. If missing, create it recursively and generate a default `Welcome.md` start page populated with application help instructions and valid Pelican CMS headers.

### 2. Configuration, Security, & Desktop UX

* **Config Management:** Read configuration settings from a local `config.json` file. If missing, initialize it with secure defaults:
  {
  "server_port": 8080,
  "admin_password": "admin_secret_changeme",
  "guest_password": "guest_secret_changeme"
  }

* **Authentication:** Enforce session-based access control. Admin has full read/write rights. Guest has Read-Only (RO) access, completely locking out editing, saving, and upload elements.

* **Desktop Launcher:** On non-Android builds, execute a system shell command (`xdg-open`, `open`, or `start`) upon successful initialization to automatically spin up the default browser targeting the local server URL.

* **Android UI Layer:** An Android APK containing the Go HTTP server bound to the local LAN interface. It wraps the Go backend in a native Java WebView. To maintain simplicity, it must completely exclude AndroidX/AppCompat libraries, using only standard built-in Android UI classes (e.g., `android.app.Activity` and `android.webkit.WebView`).

### 3. Specialized Fast-Capture Viewports

In addition to the default Pelican markdown viewing and full-screen editing states, the frontend must implement two specialized input mechanics:

* **Quick Notes Panel:** A dedicated popup window with a plain-text input. Submitting writes directly to `QuickNotes.md`. The backend must parse `QuickNotes.md`, keep the Pelican header intact at the top, and prepend the new note immediately below the header. Every new quick note entry must be separated by a dynamic timestamp divider formatted exactly like this:

```
- - -
#### YYYY-MM-DD HH:MM:SS
[ EMPTY STRING ]
[ NOTE TEXT ]
```
* **Bookmarks Stream Ingestion:** A dedicated link-capture form with inputs for URL, Title, Tags (comma-separated), and Notes. Submitting appends this entry to the top of a JSON array inside a file called `Bookmarks.html`. Note that the Javascript/frontend code to render this data already exists elsewhere. The Go backend only needs to cleanly inject the new structured entry right under the marker line, preserving this exact format:

  ```
  <script>bookmarks = [
  <!-- Don't edit body below this line -->
    {
      "date": "2026-06-14 22:03:00",
      "url": "[https://example.com](https://example.com)",
      "title": "Example Title",
      "tags": ["Tag1", "Tag2"],
      "notes": []
    },
    ...
  
  ```

### 4. Media Handling & Rendering

* **Media Uploads:** Images sent via upload buttons or drag-and-drop are stored in an `/images` subdirectory relative to the note. The UI must instantly insert a valid Pelican reference at the cursor position: `![Description]({filename}/images/filename.jpg)`.

* **Render Engine:** By default, parse Pelican metadata headers into a styled block and convert markdown body text to interactive rich HTML via Marked.js.

### 5. Dockerized Multi-Stage Caching (Linux-Optimized)

Provide a multi-stage `Dockerfile` leveraging aggressive caching rules:

* Stage 1: Cache system environments, Linux Go toolchains, Android SDK/NDK CLI tools, Gradle, and `gomobile` packages. Rebuilds only on explicit version bumps.

* Stage 2: Copy only `go.mod` and `go.sum` to lock down and cache backend module dependencies cleanly.

* Stage 3: Map application source code, compile multi-architecture desktop binaries, and pack the Android APK using `gomobile bind` and a Dockerized Gradle build (strictly zero AndroidX/AppCompat libraries).

### 6. Code Generation & Execution Framework (CRITICAL Workflow)

You must deliver the files and all future iterations using a strict, automated system.

#### Turn 1 (Your First Output Only):

Provide a file structure description, build instructions, and a single, self-contained Python script named `setup_project.py`. Running this script must automatically generate all directories and write the full contents of all files (`main.go`, `frontend/index.html`, and `Dockerfile`) to disk. Ensure a global version variable (e.g., `APP_VERSION = "1.0.0"`) is embedded within the generated files.

#### Turn 2 and Onward (All Subsequent Modifications):

Do **NOT** output full files or standard text diffs.  Respond exclusively with a
runnable, **idempotent** Python script that reads the current state of the
codebase, determines the current version automatically, and applies precise
`str.replace()` patches.  The script must be safe to run multiple times.

**Required structure (evolved, stable pattern):**

```python
#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    """Replace *old* with *new* in *path*.  Raise ValueError if *old* missing."""
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def increment_version(ver_str):
    """'1.3.35' → '1.3.36'"""
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1.  Auto‑detect current version from backend/version.go and bump it
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)

    # Update version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    # Update android/app/build.gradle (versionCode and versionName)
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}',
                            f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"',
                            f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2.  Apply feature patches using patch_file()
    patch_file("backend/server.go",
               "// old exact code block",
               "// new exact code block")

    # 3.  Print the standardised Git commit message
    commit_msg = (
        "feat(core): concise description of what changed\n\n"
        "- Bullet point details\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()
```

**Key rules (distilled from many iterations):**

- **Auto‑increment**: read `APP_VERSION` from `backend/version.go` with a regex;
  never hard‑code the old version number.
- **Idempotent**: the script must be safe to run twice — it should either succeed
  both times or fail cleanly on the second run because the old target string is
  already gone.
- **`patch_file()`**: small helper that raises a loud `ValueError` if the
  expected old string does not exist.  This catches incomplete previous patches
  immediately.
- **Multiple file types**: version bumps touch `backend/version.go` and
  `android/app/build.gradle` together.
- **Commit message**: exactly one `[GIT_COMMIT_MESSAGE]...[/GIT_COMMIT_MESSAGE]`
  block printed to stdout, containing the version bump note.
- **No unused code**: do not include scaffolding you do not need; the helper
  functions above are the minimum that has proven reliable.
### 7. Output Deliverables

1. **File Structure & Build Instructions** Text description block.

2. `setup_project.py` - The complete python builder script containing the full baseline definitions of `main.go`, `frontend/index.html`, and `Dockerfile`.
