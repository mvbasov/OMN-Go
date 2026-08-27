# OMN-Go (Open Markdown Notes)

**OMN-Go** is a cross-platform Markdown note application. It uses Go, HTML, and JavaScript, and it works without an internet connection.

OMN-Go replaces the original [mvbasov/OMN](https://github.com/mvbasov/OMN) project. It uses a local Go web server and a native WebView to run on the desktop (Linux) and on Android. It does not use an electron framework or an external cloud service.

<p align="center">
<!--
  <a href="https://gitlab.com/mvbasov/OMN-Go/-/releases/permalink/latest">
    <img alt="Get it on GitLab" src="https://img.shields.io/badge/Get_it_on-GitLab-FC6D26?style=for-the-badge&amp;logo=gitlab&amp;logoColor=white">
  </a><img width="14" alt="" src="https://placehold.co/14x28/transparent/transparent.png">--><a href="https://github.com/mvbasov/OMN-Go/releases/latest">
    <img alt="Get it on GitHub" src="https://img.shields.io/badge/Get_it_on-GitHub-6e5494?style=for-the-badge&amp;logo=github&amp;logoColor=white">
  </a><img width="14" alt="" src="https://placehold.co/14x28/transparent/transparent.png"><a href="https://f-droid.org/packages/net.basov.omngo.fdroid/">
    <img alt="Get it on F-Droid" src="https://img.shields.io/badge/Get_it_on-F--Droid-1976D2?style=for-the-badge&amp;logo=fdroid&amp;logoColor=white">
  </a>
</p>

> [!NOTE]
> **Not on Google Play.** OMN-Go will never be published on the Google Play Store. Google's policies do not allow some optional convenience features of OMN-Go, such as its background web service. I will not remove or restrict these features to comply.

## Features

* **Cross-Platform:** OMN-Go runs as a desktop application on Linux and as an Android application.

* **Local Web Server:** OMN-Go runs its own web server. You can open and manage your storage directory in any standard web browser.

* **Flexible Editing:** OMN-Go works with your preferred external editor (like Sublime Text, VS Code, or Kate) for long writing sessions. It also has a small built-in editor as a fallback.

* **Offline First:** OMN-Go holds all rendering dependencies (like KaTeX for math and highlight.js for code) in the binary. It does not need an internet connection.

* **Markdown Native:** OMN-Go stores notes as plain `.md` files on your local file system. You keep full ownership of your data.

* **File Storage:**

  * On Desktop: OMN-Go saves notes to `./data/md/`

  * On Android: OMN-Go saves notes to the public Media directory (`/storage/emulated/0/Android/media/net.basov.omngo/md/`), so you can create a backup of them easily.

* **Image Uploads:** Paste or drag an image into the editor. OMN-Go saves the image on the device and adds a link to it in your Markdown source.

* **Android "Share To" Integration:** OMN-Go handles Android intents. You can share a URL or text from another application into the Bookmarks page or the Quick Notes page.

* **Optional Git Sync:** OMN-Go can synchronize your whole storage directory across devices over SSH. It shows each conflict clearly. You can then do a manual merge, or a safe force pull or push.

* **Optional LAN Sharing:** LAN sharing is off by default. If you enable it, other devices on your network can read or edit your notes over HTTP. The admin password and the guest password protect this access.

* **Embedded SQL Database:** A note script (a `<script>` block in a note) can read and write a real SQL database (pure-Go, no CGO). You can create a backup of the database at any time. Git can track the backup, and you can use it to move the data to another device.

* **Automatic Tags Page:** OMN-Go reads the `Tags:` line in the header block of each note. From these lines it generates the Tags page, which indexes all of your notes by tag and works offline.

* **Android Intents & Termux (opt-in):** A note can open a system settings screen or start another application. It can also run a Termux shell command. These links use an `intent:` URI, and the function is disabled by default.

* **Theming:** OMN-Go has a light theme, a dark theme, and a theme that follows the system. You select the theme on each device.

## Architecture

OMN-Go is one Go binary that serves the frontend from a local web server. Each
platform wraps this binary in a different way:

1. **The backend (`backend/`):** A Go package that runs the whole
   application. An `http.ServeMux` (`server.go`) connects request
   authentication (`middleware.go`), the note and API handlers
   (`handlers.go`), and Markdown compilation with goldmark (`markdown.go`,
   `templates.go`). The backend writes the HTML cache to disk
   (`render_cache.go`). It also holds an embedded SQLite database (pure-Go
   `modernc.org/sqlite`, `sqlite.go` + `db_backup.go`) and runs git
   synchronization over SSH (`git_helper.go`). The build compiles all
   frontend assets into the binary with `//go:embed`, and the backend
   extracts them to the storage directory when it first needs them. This is
   why OMN-Go works without an internet connection.

2. **The frontend (`backend/frontend/`):** Pure HTML, CSS, and vanilla
   JavaScript, with no React, no Vue, and no external CDN. The page
   templates that the backend renders live in `frontend/templates/`. The
   static JavaScript and CSS assets and the bundled system notes live in
   `frontend/html/` and `frontend/md/`.

3. **The platform wrappers:**

   * **Desktop (`main_desktop.go`):** This wrapper builds the backend as a
     normal executable, starts the server, and opens your default browser.
     The release pipeline cross-compiles the Linux and the Windows binaries
     and attaches them to
     [Releases](https://github.com/mvbasov/OMN-Go/releases). Linux is the
     primary target, and it is tested. The pipeline publishes the Windows
     `.exe`, but nobody has tested it on real hardware yet. There is no
     macOS build.

   * **Android (`android/`):** A small Java application that uses a WebView.
     The build compiles the backend to a library with `gomobile bind`. A
     foreground `ServerService` starts the backend, and `MainActivity` shows
     the frontend in the WebView.

## AI-Assisted Development

AI assistants (Google Gemini, Claude, and others) do much of the development work on this project. New code arrives as atomic unified-diff patches, which you apply with `git apply`, and not as manual file edits. This keeps each change small, easy to review, and easy to revert. It also makes fast prototyping possible with little regression drift.

## Build Instructions

OMN-Go uses a Docker build environment. You do not need to install Go, Android Studio, or Gradle on your device to compile this project.

### Prerequisites

* You must install [Docker](https://docs.docker.com/get-docker/) on your device.

### 1. Fetch offline assets (first time only)

The build puts the offline rendering libraries into the binary. These libraries
are KaTeX (math), highlight.js (code), and their web fonts. The repository does
not hold these files. Download them into the frontend once after a fresh clone,
before the first build:

```
bash local/initial/offline_asset_downloader.sh
```

Run this command again only when you want to update these vendored assets.

### 2. Compile & extract

```
bash local/build.sh
```

The build runs inside Docker. It uses a cached base-toolchain image and then
the application image. After compilation finishes, the script copies the
binaries out of the container to your device. It copies the desktop
executables (Linux and Windows) and the Android APK into
`./output-binaries/`.

## Usage

**On Desktop:**
Run the binary from your extracted outputs:

```
mkdir ~/OMN-Go
cp ./output-binaries/omn-go-<VERSION>-desktop-linux-amd64 ~/OMN-Go/omn-go-desktop
cd ~/OMN-Go
./omn-go-desktop


```

At start, OMN-Go opens your default browser at `http://localhost:8080`. If you set a different port on the Config page, OMN-Go uses that port. If the browser does not open, open that address manually.

**On Android:**
Install the APK on your device. Start the "OMN-Go" application from your launcher. The backend starts in the background, and the WebView shows your notes.

## Old Android devices

OMN-Go installs on **Android 6.0 (API 23)** and newer.

The limit is not the Android version. It is the WebView, which is the browser
engine that draws the interface. **OMN-Go needs a System WebView of Chromium
85 or newer.** Android 6 ships Chromium 44, which is too old, but the "Android
System WebView" component is updatable on that release up to Chromium 106.
Chromium 106 was the last release for Android 6.0, thus the window is 85 to
106 and OMN-Go fits in it.

Update "Android System WebView" and Chrome from your application store before
you install OMN-Go on such a device. When the WebView is too old, OMN-Go shows
a red line at the top of the page that names the version it found and the
version it needs. Without that line an old WebView gives a blank page and no
explanation.

Two more things on a device of that age:

- **Install the APK for the ABI of the device.** A device of that time is
  frequently 32-bit ARM, thus `armeabi-v7a`. The universal APK also operates.
- **Git synchronization over HTTPS can fail.** The certificate store of
  Android 6 does not hold ISRG Root X1, thus a server with a Let's Encrypt
  certificate does not verify. GitHub uses a different authority and operates.
  An SSH key avoids the question.

## Versioning
Versioning in this project is informal. Numbers do not indicate stability or roadmap progress.

## Disclaimer

OMN-Go is a personal tool for one person, and not an enterprise product. It has
no separate accounts: the admin role and the guest role share one set of notes.
A note script and the SQL API operate with full rights. A script in a note can
change or delete any note and any database. LAN sharing sends plain HTTP with no
encryption. Use it only on a network that you control. Never use it on a public
network. Keep your own backup of your notes. The author gives no warranty
and takes no responsibility for lost data.

## License

[MIT License](LICENSE)
