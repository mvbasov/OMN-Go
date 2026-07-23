# OMN-Go (Open Markdown Notes)

**OMN-Go** is a fast, offline-first, cross-platform Markdown note-taking application built with Go, HTML, and JavaScript.

It is designed to be the modern successor to the original [mvbasov/OMN](https://github.com/mvbasov/OMN) project. By leveraging a local Go web server and a native WebView, OMN-Go delivers a seamless experience across both Desktop (Linux) and Mobile (Android) environments without relying on bloated electron frameworks or external cloud services.

> **Note:** This project is currently in an **early development state**. Features and storage structures are subject to change.

## Features

* **Cross-Platform:** Runs natively as a desktop application on Linux and as an Android app.

* **Local Web Server:** Acts as a standalone web server, allowing you to access and manage your workspace directly through any standard web browser.

* **Flexible Editing:** Seamlessly integrates with your preferred external editor (like Sublime Text, VS Code, or Nano) for heavy writing, while providing a tiny, built-in embedded editor as a quick and reliable fallback.

* **Offline First:** All rendering dependencies (like KaTeX for math and highlight.js for code) are bundled directly into the binary. No internet connection is required.

* **Markdown Native:** Notes are stored as plain `.md` files on your local file system, ensuring you completely own your data.

* **Intelligent File Storage:**

  * On Desktop: Notes are saved to `./data/md/`

  * On Android: Notes are securely saved to the public Media directory (`/storage/emulated/0/Android/media/net.basov.omngo/md/`) so they can be easily backed up.

* **Dynamic Media Uploads:** Paste or drag-and-drop images directly into the editor; they are automatically saved locally and linked in your Markdown.

* **Android "Share To" Integration:** Native Android intent handling allows you to share URLs or text directly from other apps straight into your OMN-Go Bookmarks or Quick Notes.

* **Optional Git Sync:** Synchronize your entire notes directory across devices over SSH, with clear conflict handling (manual-merge, or safe force pull/push).

* **Optional LAN Sharing:** Off by default — when enabled, other devices on your network can read or edit your notes over HTTP, protected by admin and guest passwords.

* **Embedded SQL Database:** Note `<script>` blocks can read and write a real SQL database (pure-Go, no CGO), with on-demand, git-trackable snapshots for backup and cross-device transfer.

* **Automatic Tags Page:** Every note's `Tags:` header feeds an auto-generated, offline-friendly overview page that indexes all of your notes by tag.

* **Android Intents & Termux (opt-in):** Notes can open system settings screens, launch other apps, or run Termux shell commands via `intent:` links — disabled by default.

* **Theming:** Light, dark, and system-following themes, selectable per device.

## Architecture

OMN-Go is a single Go binary that serves a local web UI, wrapped differently
per platform:

1. **The backend (`backend/`):** A Go package that runs the whole app — an
   `http.ServeMux` (`server.go`) wiring request auth (`middleware.go`),
   note and API handlers (`handlers.go`), Markdown compilation via goldmark
   (`markdown.go`, `templates.go`) cached to disk (`render_cache.go`), an
   embedded SQLite database (pure-Go `modernc.org/sqlite`, `sqlite.go` +
   `db_backup.go`), and git-over-SSH sync (`git_helper.go`). All frontend
   assets are compiled into the binary with `//go:embed` and lazily extracted
   to the storage directory on first use — this is what makes OMN-Go
   offline-first.

2. **The frontend (`backend/frontend/`):** Pure HTML, CSS, and vanilla
   JavaScript — no React, no Vue, no external CDNs. Server-rendered page
   templates live in `frontend/templates/`; static JS/CSS and the bundled
   system notes live in `frontend/html/` and `frontend/md/`.

3. **The platform wrappers:**

   * **Desktop (`main_desktop.go`):** Builds the backend as a normal
     executable, starts the server, and opens your default browser. The
     release pipeline cross-compiles both Linux and Windows binaries and
     attaches them to [Releases](https://github.com/mvbasov/OMN-Go/releases);
     Linux is the primary, tested target, while the Windows `.exe` is
     published but not yet tested on real hardware. macOS is not built.

   * **Android (`android/`):** A minimal Java WebView app. The backend is
     compiled to a library with `gomobile bind`; a foreground `ServerService`
     boots it and `MainActivity` displays the local UI in a WebView.

## AI-Assisted Development

This project is actively developed using an aggressive, AI-assisted pipeline (via Google Gemini, Claude, etc.). New code is delivered as atomic unified-diff patches (applied with `git apply`) rather than manual file editing. This keeps every change small, reviewable, and easy to roll back, for rapid prototyping with minimal regression drift.

## Build Instructions

OMN-Go uses a fully containerized Docker build environment. You do not need to install Go, Android Studio, or Gradle on your host machine to compile this project.

### Prerequisites

* [Docker](https://docs.docker.com/get-docker/) must be installed on your machine.

### 1. Fetch offline assets (first time only)

The offline rendering libraries that get bundled into the binary — KaTeX (math),
highlight.js (code), and their web fonts — are downloaded into the frontend
rather than committed to the repository, so a fresh clone must fetch them once
before the first build:

```
bash local/initial/offline_asset_downloader.sh
```

Re-run it only when you want to refresh those vendored assets.

### 2. Compile & extract

```
bash local/build.sh
```

The build itself runs entirely inside Docker (a cached base-toolchain image,
then the app image). Once compilation finishes, the script *extracts* the
resulting binaries out of the container onto your host — the desktop
executables (Linux and Windows) and the Android APK are copied into
`./output-binaries/`.

## Usage

**On Desktop:**
Simply execute the binary from your extracted outputs:

```
mkdir ~/OMN-Go
cp ./output-binaries/omn-go-<VERSION>-desktop-linux-amd64 ~/OMN-Go/omn-go-desktop
cd ~/OMN-Go
./omn-go-desktop


```

On launch it automatically opens your default browser at `http://localhost:8080` (or the port you set on the Config page). If the browser does not open on its own, visit that address manually.

**On Android:**
Install the generated APK onto your device. Launch the "OMN-Go" app from your launcher. The local server will boot automatically in the background, and the WebView will display your notes.

## Versioning
Versioning in this project is informal. Numbers do not indicate stability or roadmap progress.

## License

[MIT License](LICENSE)
