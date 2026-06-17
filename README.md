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

* **Intelligent File Storage:** \* On Desktop: Notes are saved to `./data/md/`

  * On Android: Notes are securely saved to the public Media directory (`/storage/emulated/0/Android/media/net.basov.omngo/md/`) so they can be easily backed up.

* **Dynamic Media Uploads:** Paste or drag-and-drop images directly into the editor; they are automatically saved locally and linked in your Markdown.

* **Android "Share To" Integration:** Native Android intent handling allows you to share URLs or text directly from other apps straight into your OMN-Go Bookmarks or Quick Notes.

## Architecture & AI-Assisted Development

OMN-Go features a highly decoupled architecture:

1. **The Engine (`backend/server.go`):** A lightweight Go HTTP server that handles file I/O, session authentication, API routes, and Markdown compilation.

2. **The Frontend (`backend/frontend/`):** Pure HTML, CSS, and Vanilla JavaScript. No React, no Vue, no external CDNs.

3. **The Wrappers:**

   * **Desktop (`main_desktop.go`):** Spawns the server and optionally triggers your default web browser.

   * **Android (`android/`):** A minimal Java WebView wrapper that boots the local Go server via `gomobile` and displays the interface natively.

**AI-Assisted Build Process:**
This project is actively developed using an aggressive, AI-assisted pipeline (via Google Gemini). The entire codebase is strictly manipulated via atomic Python patching scripts rather than manual file editing. This ensures rapid prototyping, guaranteed syntax safety, and zero regression drift.

## Build Instructions

OMN-Go uses a fully containerized Docker build environment. You do not need to install Go, Android Studio, or Gradle on your host machine to compile this project.

### Prerequisites

* [Docker](https://docs.docker.com/get-docker/) must be installed on your machine.

### Compilation & Extraction

```
# Build the Docker image (caches toolchains in Stage 1, packages in Stage 2)
docker build -t omn-go-builder .

# Extract the compiled Desktop Binary and Android APK to your host
docker create --name omn-go-extract omn-go-builder
docker cp omn-go-extract:/app/bin/ ./output-binaries/
docker rm omn-go-extract


```

## Usage

**On Desktop:**
Simply execute the binary from your extracted outputs:

```
./output-binaries/omn-go-desktop


```

Then, open your web browser and navigate to `http://localhost:8080`.

**On Android:**
Install the generated APK onto your device. Launch the "OMN-Go" app from your launcher. The local server will boot automatically in the background, and the WebView will display your notes.

## License

[MIT License](LICENSE)