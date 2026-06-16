Here is the current state of the GoOMN project. We are currently at Version 1.1.0 (Android version code 10100).

Recently, we migrated the Android app to a Java WebView wrapper using a Dockerized Gradle build (eliminating the old 5MB APK limit while strictly keeping NO AppCompat). We also added offline support for KaTeX and Highlight.js, implemented a dynamic JS Console Interceptor UI with a Clear button, and fixed directory-based Content-Type routing.

Below is the complete current codebase and the master `initial_prompt.md`. Please review them and acknowledge that you are ready for my next request. Remember to strictly follow the Turn 2 Python patching output format.


### doc/initial_prompt.md START
```
You are an expert Senior Systems Engineer, Android Developer, and Go Expert. Architect and write a cross-platform Markdown note editor (replacing Open Markdown Notes) called "GoOMN".

The project must use vanilla JavaScript/Tailwind HTML for the web interface, Go for the cross-platform backend server, and a Docker environment optimized for Linux hosts to compile the Android APK without Android Studio or AndroidX/AppCompat libraries.

### 1. Storage, Package, & Initialization Constraints (F-Droid Ready)

* **Package Frameworks:** Support `net.basov.goomn` (or `net.basov.goomn.fdroid` for F-Droid builds).

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

Do NOT output full files or standard text diffs. You must respond exclusively with a runnable Python script using standard `str.replace()` mechanisms to modify the existing baseline.

The Python script must strictly adhere to this template structure:

```
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

```

### doc/initial_prompt.md END

### Dockerfile START
```
# STAGE 1: Toolchains & Cache
FROM golang:1.25-bookworm AS builder
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    openjdk-17-jdk wget unzip cmake ninja-build \
    && rm -rf /var/lib/apt/lists/*

# Install Android CMD Line Tools
RUN wget https://dl.google.com/android/repository/commandlinetools-linux-10406996_latest.zip -O /tmp/cmd.zip && \
    mkdir -p /opt/android/cmdline-tools && \
    unzip /tmp/cmd.zip -d /opt/android/cmdline-tools && \
    mv /opt/android/cmdline-tools/cmdline-tools /opt/android/cmdline-tools/latest && \
    rm /tmp/cmd.zip

ENV ANDROID_HOME=/opt/android
ENV PATH=$PATH:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools

# Accept licenses and install platform dependencies
RUN yes | sdkmanager --licenses && \
    sdkmanager "platforms;android-34" "build-tools;33.0.2" "ndk;25.2.9519653"

# Install Gradle
RUN wget -q https://services.gradle.org/distributions/gradle-8.5-bin.zip -O /tmp/gradle.zip && \
    mkdir -p /opt/gradle && \
    unzip -q /tmp/gradle.zip -d /opt/gradle && \
    rm /tmp/gradle.zip
ENV PATH=$PATH:/opt/gradle/gradle-8.5/bin

# Install GoMobile
RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init



# STAGE 2: Dependency Lock
WORKDIR /app
COPY go.mod ./
RUN go mod download || true

# STAGE 3: Build & Pack
COPY . .
RUN go get golang.org/x/mobile@latest && go mod tidy

# Desktop Binary (Linux example)
RUN GOOS=linux GOARCH=amd64 go build -o bin/goomn-desktop main_desktop.go

# Android APK - Webview Wrapper via Gradle & gomobile bind
RUN go get -tool golang.org/x/mobile/cmd/gobind && go mod tidy && mkdir -p android/app/libs && gomobile bind -target=android -androidapi 24 -javapkg net.basov.goomn -o android/app/libs/goomn.aar ./backend

RUN cd android && if [ ! -f app/goomn.keystore ]; then keytool -genkey -v -keystore app/goomn.keystore -alias goomn -keyalg RSA -keysize 2048 -validity 10000 -storepass goomn123 -keypass goomn123 -dname "CN=GoOMN, O=Basov"; fi && gradle assembleRelease && cp app/build/outputs/apk/release/app-release.apk ../bin/goomn.apk

```

### Dockerfile END

### go.mod START
```
module net.basov.goomn

go 1.25



```

### go.mod END

### main_desktop.go START
```
//go:build !android

package main

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"
	"net.basov.goomn/backend"
)

func main() {
	backend.StartServer()
	
	// Wait for server to bind
	time.Sleep(500 * time.Millisecond)
	url := fmt.Sprintf("http://localhost:%d", backend.GetServerPort())
	
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	}
	
	if err != nil {
		log.Printf("Could not auto-launch browser. Please visit %s manually.", url)
	}
	
	select {} // Block main thread
}

```

### main_desktop.go END

### backend/server.go START
```
package backend

import (
	"embed"
	"io/fs"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const APP_VERSION = "1.1.0"

type Config struct {
	ServerPort    int    `json:"server_port"`
	AdminPassword string `json:"admin_password"`
	GuestPassword string `json:"guest_password"`
}

//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/js frontend/css frontend/json
var staticFS embed.FS

var (
	storageDir  string
	appConfig   Config
	activeConns int
)

func initStorage() {
	if runtime.GOOS == "android" {
		storageDir = "/storage/emulated/0/Android/media/net.basov.goomn"
	} else {
		storageDir = "./data"
	}

	// 1. Create Isolated Storage
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	mdDir := filepath.Join(storageDir, "md")
	os.MkdirAll(mdDir, 0755)

	// Migrate existing .md files recursively
	files, _ := filepath.Glob(filepath.Join(storageDir, "*.md"))
	for _, f := range files {
		os.Rename(f, filepath.Join(mdDir, filepath.Base(f)))
	}

	// 2. Init Config
	configPath := filepath.Join(storageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		appConfig = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
		}
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(configPath, data, 0644)
	} else {
		data, _ := os.ReadFile(configPath)
		json.Unmarshal(data, &appConfig)
	}

	// 3. Init Default Notes
	welcomePath := filepath.Join(mdDir, "Welcome.md")
	if _, err := os.Stat(welcomePath); os.IsNotExist(err) {
		welcomeContent := "Title: Welcome\nDate: 2026-06-14 12:00:00\nCategory: System\n\nWelcome to GoOMN. Start editing!\n\n- [Help](Welcome)\n- [Scripting Rules](ScriptRules.md)\n- [Bookmarks](Bookmarks)\n- [Quick Notes](QuickNotes)"
		os.WriteFile(welcomePath, []byte(welcomeContent), 0644)
	}

	
	rulesPath := filepath.Join(mdDir, "ScriptRules.md")
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		os.WriteFile(rulesPath, []byte("Title: JS Scripting Rules\nDate: 2026-06-15\nCategory: System\n\n# JavaScript Guidelines for GoOMN\n\nBecause GoOMN is a Single Page Application (SPA), the global `window` scope persists between page loads. To avoid `SyntaxError: Identifier has already been declared` when scripts are re-evaluated, authors must follow these rules:\n\n### Rule 1: Isolate variables using Block Scopes or IIFEs\nNever leave `const` or `let` in the top-level global scope. Wrap the script in an Anonymous Block `{ ... }` or an Immediately Invoked Function Expression (IIFE).\n\n```javascript\n{\n    const myLocalVar = \"Safe!\";\n    let counter = 0;\n}\n```\n\n### Rule 2: Explicitly attach required globals to `window`\nIf a function is needed for an HTML `onclick` event, attach it directly to the `window` object.\n\n```javascript\nwindow.doSomething = function() {\n    alert(\"This works safely on reload!\");\n};\n```\n\n### Rule 3: Use the OR (`||`) operator for global state\nCheck if global config objects exist before creating them so user state is preserved.\n\n```javascript\nwindow.myAppConfig = window.myAppConfig || { version: \"1.0\" };\n```\n\n### Rule 4: Use `var` for raw top-level variables\nIf you must declare top-level variables, use `var` because the JS engine allows `var` to be redeclared infinitely without throwing an error."), 0644)
	}

	quickPath := filepath.Join(mdDir, "QuickNotes.md")
	if _, err := os.Stat(quickPath); os.IsNotExist(err) {
		quickContent := "Title: Quick Notes\nDate: 2026-06-14 12:00:00\nCategory: Log\n\n"
		os.WriteFile(quickPath, []byte(quickContent), 0644)
	}

	bmPath := filepath.Join(mdDir, "Bookmarks.md")
	if _, err := os.Stat(bmPath); os.IsNotExist(err) {
		bmContent := `Title: Incoming bookmarks
Date: 2026-06-15 20:00:00
Author: 
Tags: Bookmarks

<script>bookmarks = [
<!-- Don't edit body below this line -->
  {
    "date": "2023-01-22 22:22:22",
    "url": "Welcome",
    "title": "The start page",
    "tags": [
      "GoOMN",
      "Local pages"
    ],
    "notes": [
      "This application start page"
    ]
  }
];
</script>
  
<!-- end of bookmarks definition -->
    
<link rel="stylesheet" type="text/css" href="/css/Bookmarker.css" />
<script type="text/javascript" src="/js/Bookmarker.js"></script>`
		os.WriteFile(bmPath, []byte(bmContent), 0644)
	}
}

// Simple connection tracker for the Android Canvas requirement
func connectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeConns++
		next.ServeHTTP(w, r)
		activeConns--
	})
}

func authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_role")
		if err != nil || (requireAdmin && cookie.Value != "admin") || (!requireAdmin && cookie.Value != "admin" && cookie.Value != "guest") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	pwd := r.FormValue("password")
	role := ""
	if pwd == appConfig.AdminPassword {
		role = "admin"
	} else if pwd == appConfig.GuestPassword {
		role = "guest"
	}

	if role != "" {
		http.SetCookie(w, &http.Cookie{Name: "session_role", Value: role, Path: "/"})
		w.Write([]byte("OK"))
	} else {
		http.Error(w, "Invalid", http.StatusUnauthorized)
	}
}

func handleQuickNote(w http.ResponseWriter, r *http.Request) {
	note := r.FormValue("note")
	if note == "" {
		return
	}
	path := filepath.Join(storageDir, "md", "QuickNotes.md")
	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\n")
	
	insertIdx := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" { // Find first blank line ending Pelican header
			insertIdx = i + 1
			break
		}
	}
	
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("\n---\n##### %s\n%s\n", timestamp, note)
	
	newContent := append(lines[:insertIdx], append([]string{entry}, lines[insertIdx:]...)...)
	os.WriteFile(path, []byte(strings.Join(newContent, "\n")), 0644)
	w.Write([]byte("Saved"))
}

func handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")
	
	path := filepath.Join(storageDir, "md", "Bookmarks.md")
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	tagsList := []string{}
	for _, t := range strings.Split(tags, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tagsList = append(tagsList, trimmed)
		}
	}
	notesList := []string{}
	if trimmed := strings.TrimSpace(notes); trimmed != "" {
		notesList = append(notesList, trimmed)
	}
	
	type BM struct {
		Date  string   `json:"date"`
		Url   string   `json:"url"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
		Notes []string `json:"notes"`
	}
	bm := BM{Date: timestamp, Url: url, Title: title, Tags: tagsList, Notes: notesList}
	bmJson, _ := json.MarshalIndent(bm, "  ", "  ")
	entry := "  " + string(bmJson) + ",\n"
	
	data, err := os.ReadFile(path)
	if err == nil {
		content := string(data)
		marker := "<!-- Don't edit body below this line -->"
		if strings.Contains(content, marker) {
			newContent := strings.Replace(content, marker, marker+"\n"+entry, 1)
			os.WriteFile(path, []byte(newContent), 0644)
		} else {
			f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
			defer f.Close()
			f.WriteString("\n" + entry)
		}
	}
	w.Write([]byte("Saved"))
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	imgDir := filepath.Join(storageDir, "images")
	os.MkdirAll(imgDir, 0755)
	
	destPath := filepath.Join(imgDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)
	
	w.Write([]byte(fmt.Sprintf("![%s]({filename}/images/%s)", header.Filename, header.Filename)))
}

func handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	jsonDir := filepath.Join(storageDir, "user_json")
	os.MkdirAll(jsonDir, 0755)
	
	destPath := filepath.Join(jsonDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)
	
	w.Write([]byte(fmt.Sprintf("[%s]({filename}/user_json/%s)", header.Filename, header.Filename)))
}



func handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Welcome"
	}
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}
	data, err := os.ReadFile(filepath.Join(storageDir, "md", filepath.Clean(name)))
	if err != nil {
		title := strings.TrimSuffix(name, ".md")
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		newContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes\n\n", title, timestamp)
		w.Write([]byte(newContent))
		return
	}
	w.Write(data)
}

func handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		return
	}
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}
	path := filepath.Join(storageDir, "md", filepath.Clean(name))
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
	w.Write([]byte("Saved"))
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(frontendHTML)
}

func StartServer() {
	initStorage() // Execute synchronously to ensure config is loaded instantly
	
	// Fallback MIME types for minimal Docker containers
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".webp", "image/webp")
	mime.AddExtensionType(".png", "image/png")
	mime.AddExtensionType(".jpg", "image/jpeg")
	mime.AddExtensionType(".jpeg", "image/jpeg")
	mime.AddExtensionType(".gif", "image/gif")
	mime.AddExtensionType(".json", "application/json")
	mime.AddExtensionType(".woff", "font/woff")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".ttf", "font/ttf")

	go func() {
		
		mux := http.NewServeMux()
		mux.HandleFunc("/", serveFrontend)
		fSys, _ := fs.Sub(staticFS, "frontend")
		
		serveStrict := func(ext, cType string) http.Handler {
			fsHandler := http.FileServer(http.FS(fSys))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, ext) {
					http.Error(w, "Forbidden: Invalid file extension", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", cType)
				
				if r.URL.Path == "/js/Bookmarker.js" {
					data, err := fs.ReadFile(fSys, "js/Bookmarker.js")
					if err == nil {
						js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
						js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
						w.Write([]byte(js))
						return
					}
				}
				
				fsHandler.ServeHTTP(w, r)
			})
		}

		mux.Handle("/js/", serveStrict(".js", "application/javascript"))
		mux.Handle("/css/fonts/", serveStrict(".woff2", "font/woff2"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))
		mux.Handle("/json/", serveStrict(".json", "application/json"))
		
		// Config for files handling Content-type by served directories
		serveStorageDir := func(subDir, cType string) http.Handler {
			dirPath := filepath.Join(storageDir, subDir)
			os.MkdirAll(dirPath, 0755)
			fsHandler := http.StripPrefix("/"+subDir+"/", http.FileServer(http.Dir(dirPath)))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if cType != "" {
					w.Header().Set("Content-Type", cType)
				}
				fsHandler.ServeHTTP(w, r)
			})
		}
		
		mux.Handle("/images/", serveStorageDir("images", ""))
		mux.Handle("/user_json/", serveStorageDir("user_json", "application/json"))

		mux.HandleFunc("/login", handleLogin)
		mux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
		mux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
		mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
		mux.HandleFunc("/api/upload_json", authMiddleware(handleUploadJSON, true))
		mux.HandleFunc("/api/note", handleGetNote)
		mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))
		
		port := fmt.Sprintf(":%d", appConfig.ServerPort)
		log.Printf("GoOMN Backend running on %s", port)
		http.ListenAndServe(port, connectionMiddleware(mux))
	}()
}

// GetServerPort safely exposes the configured port for frontend wrappers
func GetServerPort() int {
	return appConfig.ServerPort
}

```

### backend/server.go END

### backend/frontend/index.html START
```
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GoOMN Editor</title>
    <style>
        body { font-family: sans-serif; margin: 0; padding: 0; display: flex; flex-direction: column; height: 100vh; background: #f9f9f9; color: #333; }
        .overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); display: flex; align-items: center; justify-content: center; z-index: 50; }
        .modal { background: #fff; padding: 20px; border-radius: 4px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); width: 300px; }
        .modal input, .modal button, .modal textarea { width: 100%; box-sizing: border-box; margin-bottom: 10px; padding: 8px; }
        .modal button { background: #0056b3; color: white; border: none; cursor: pointer; border-radius: 4px; }
        #mainUI { display: none; flex: 1; flex-direction: column; }
        .header { background: #333; color: #fff; padding: 10px 20px; display: flex; gap: 15px; align-items: center; }
        .header a, .header button { color: #fff; text-decoration: none; cursor: pointer; background: transparent; border: 1px solid #555; padding: 5px 10px; border-radius: 4px; font-size: 14px; }
        .header a:hover, .header button:hover { background: #555; }
        .content-area { flex: 1; padding: 20px; position: relative; display: flex; flex-direction: column; }
        #editor { display: none; width: 100%; flex: 1; border: 1px solid #ccc; padding: 10px; font-family: monospace; resize: none; box-sizing: border-box; }
        #preview { width: 100%; flex: 1; background: #fff; border: 1px solid #ccc; padding: 20px; overflow-y: auto; box-sizing: border-box; line-height: 1.6; }
        .toolbar { display: flex; justify-content: flex-end; margin-bottom: 10px; gap: 10px; }
        .toolbar button { padding: 5px 15px; cursor: pointer; border: 1px solid #ccc; background: #eee; border-radius: 4px; }
        .hidden { display: none !important; }
        .panel { position: absolute; top: 50px; right: 20px; background: white; border: 1px solid #ccc; padding: 15px; box-shadow: 0 4px 8px rgba(0,0,0,0.1); width: 300px; z-index: 40; }
        .panel h3 { margin-top: 0; }
        .panel input, .panel textarea, .panel button { width: 100%; box-sizing: border-box; margin-bottom: 10px; padding: 8px; }
        .panel button { background: #28a745; color: white; border: none; cursor: pointer; border-radius: 4px; }
    </style>
    <link rel="stylesheet" href="/css/highlight.default.min.css">
    <script src="/js/marked.min.js"></script>
    <script src="/js/highlight.min.js"></script>
    <script>
        const eHlJs = 'true';
        marked.setOptions({
            gfm: true,
            xhtml: true
        });

        if ('true' == eHlJs) {
            marked.setOptions({
                highlight: function (code) {
                    return hljs.highlightAuto(code).value;
                }
            });
        }

        marked.use({
            renderer: {
                heading(text, level, raw) {
                    if (typeof text === 'object') {
                        const token = text;
                        const content = this.parser.parseInline(token.tokens);
                        const id = (token.text || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
                        return `<h${token.depth} id="${id}">${content}</h${token.depth}>\n`;
                    }
                    const id = text.toLowerCase().replace(/<[^>]*>/g, '').replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
                    return `<h${level} id="${id}">${text}</h${level}>\n`;
                }
            }
        });
    </script>
    <script>const APP_VERSION = "1.1.0";</script>
    <link rel="stylesheet" href="/css/highlight.default.min.css">
    <link rel="stylesheet" href="/css/katex.min.css">
</head>
<body>
    
    <!-- Login Overlay -->
    <div id="loginOverlay" class="overlay">
        <div class="modal">
            <h2>GoOMN Login</h2>
            <input type="password" id="pwdInput" placeholder="Admin or Guest Password">
            <button onclick="login()">Enter</button>
        </div>
    </div>

    <!-- Main UI -->
    <div id="mainUI">
        <div class="header">
            <strong>GoOMN</strong>
            <a href="#home" onclick="loadNote('Welcome')">Home</a>
            <a href="#help" onclick="loadNote('Welcome')">Help</a>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only">Quick Note</button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only">Add Bookmark</button>
            <a href="#bookmarks" onclick="loadNote('Bookmarks')">Bookmarks</a>
        </div>

        <div class="content-area">
            <div class="toolbar">
                <button id="metaToggleBtn" onclick="document.getElementById('metadataPanel').classList.toggle('hidden')" style="display: none; background: #17a2b8; color: white; border: none;">Metadata</button>
                <button id="saveBtn" onclick="saveNote()" class="admin-only" style="display: none; background: #28a745; color: white; border: none;">Save Note</button>
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only">Edit Mode</button>
            </div>
            <div id="metadataPanel" class="hidden" style="background: #e9ecef; padding: 15px; font-family: monospace; white-space: pre-wrap; border: 1px solid #ccc; margin-bottom: 10px; border-radius: 4px; font-size: 13px;"></div>
            <textarea id="editor" class="admin-only" placeholder="Markdown content... Drag images here to upload."></textarea>
            <div id="preview">Loading...</div>
        </div>
    </div>

    <!-- Quick Note Modal -->
    <div id="quickPanel" class="panel hidden">
        <h3>Quick Note</h3>
        <textarea id="quickText" rows="4"></textarea>
        <div style="display: flex; gap: 10px;">
            <button onclick="submitQuickNote()">Save</button>
            <button onclick="document.getElementById('quickPanel').classList.add('hidden')" style="background: #dc3545;">Cancel</button>
        </div>
    </div>

    <!-- Bookmark Modal -->
    <div id="bmPanel" class="panel hidden">
        <h3>Ingest Bookmark</h3>
        <input id="bmUrl" placeholder="URL">
        <input id="bmTitle" placeholder="Title">
        <input id="bmTags" placeholder="Tags (comma separated)">
        <textarea id="bmNotes" rows="2" placeholder="Notes"></textarea>
        <div style="display: flex; gap: 10px;">
            <button onclick="submitBookmark()">Save</button>
            <button onclick="document.getElementById('bmPanel').classList.add('hidden')" style="background: #dc3545;">Cancel</button>
        </div>
    </div>

    <script>
        let currentNote = 'Welcome';
        
        function executeScripts(container) {
            const scripts = container.querySelectorAll('script');
            scripts.forEach(oldScript => {
                const newScript = document.createElement('script');
                Array.from(oldScript.attributes).forEach(attr => newScript.setAttribute(attr.name, attr.value));
                newScript.async = false;
                if (oldScript.innerHTML) newScript.appendChild(document.createTextNode(oldScript.innerHTML));
                oldScript.parentNode.replaceChild(newScript, oldScript);
            });
        }

        function renderView(text) {
            let header = '';
            let body = text;
            const parts = text.split(/(?:\r?\n){2,}/);
            if (parts.length > 0 && /^[A-Za-z0-9_-]+:/.test(parts[0])) {
                header = parts[0];
                body = text.substring(header.length).replace(/^\s+/, '');
            }
            
            const metaPanel = document.getElementById('metadataPanel');
            const metaBtn = document.getElementById('metaToggleBtn');
            
            metaPanel.innerText = `[File: ${currentNote}]\n\n` + (header || 'No Metadata Found');
            metaBtn.style.display = 'block';
            if (!header) metaPanel.classList.add('hidden');
            
            const preview = document.getElementById('preview');
            preview.innerHTML = marked.parse(body || '*(Empty content)*');
            executeScripts(preview);
        }

        window.addEventListener('popstate', (e) => {
            if (e.state && e.state.note) {
                let fullHash = window.location.hash.substring(1);
                let scrollHash = fullHash.includes('#') ? fullHash.split('#')[1] : null;
                if (currentNote !== e.state.note) {
                    loadNoteInternal(e.state.note, scrollHash);
                } else if (scrollHash) {
                    let el = document.getElementById(scrollHash);
                    if (el) el.scrollIntoView();
                }
            }
        });

        async function loadNote(name, hash) {
            history.pushState({note: name}, "", "#" + name + (hash ? "#" + hash : ""));
            if (currentNote !== name) {
                await loadNoteInternal(name, hash);
            } else if (hash) {
                let el = document.getElementById(hash);
                if (el) el.scrollIntoView();
            }
        }

        function resolvePath(current, target) {
            if (target.startsWith('/')) {
                target = target.substring(1);
            } else {
                let parts = current.split('/');
                parts.pop();
                target = (parts.length > 0 ? parts.join('/') + '/' : '') + target;
            }
            let segments = target.split('/');
            let result = [];
            for (let seg of segments) {
                if (seg === '.' || seg === '') continue;
                if (seg === '..') {
                    if (result.length > 0) result.pop();
                } else {
                    result.push(seg);
                }
            }
            return result.join('/');
        }

        async function loadNoteInternal(name, hash) {
            currentNote = name;
            if (currentMode === 'edit') {
                document.getElementById('editor').style.display = 'none';
                document.getElementById('preview').style.display = 'block';
                document.getElementById('toggleBtn').innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                document.getElementById('metaToggleBtn').style.display = 'none';
                document.getElementById('metadataPanel').classList.add('hidden');
                currentMode = 'view';
            }
            const res = await fetch('/api/note?name=' + encodeURIComponent(name));
            const text = await res.text();
            document.getElementById('editor').value = text;
            renderView(text);
            if (hash) {
                setTimeout(() => {
                    let el = document.getElementById(hash);
                    if (el) el.scrollIntoView();
                }, 100);
            }
        }

        window.onload = () => {
            let fullHash = window.location.hash.substring(1);
            let startNote = fullHash.split('#')[0] || 'Welcome';
            let scrollHash = fullHash.includes('#') ? fullHash.split('#')[1] : null;
            history.replaceState({note: startNote}, "", "#" + (fullHash || 'Welcome'));
            loadNoteInternal(startNote, scrollHash);
        };

        // Intercept Markdown links to load internally
        document.getElementById('preview').addEventListener('click', (e) => {
            let target = e.target.closest('a');
            if(target) {
                const href = target.getAttribute('href');
                if(href && !href.startsWith('http') && !href.startsWith('javascript:')) {
                    e.preventDefault();
                    let pathAndQuery = href.split('#')[0];
                    let file = pathAndQuery.split('?')[0]; 
                    let hash = href.includes('#') ? href.substring(href.indexOf('#') + 1) : null;
                    if (!file && hash) {
                        let el = document.getElementById(hash);
                        if(el) el.scrollIntoView();
                        return;
                    }
                    if (!file) {
                        file = currentNote;
                    } else {
                        file = resolvePath(currentNote, file);
                    }
                    loadNote(file, hash);
                } else if (href && href.startsWith('http')) {
                    e.preventDefault();
                    window.open(href, '_blank');
                }
            }
        });

        let currentMode = 'view';
        function toggleMode() {
            const editor = document.getElementById('editor');
            const preview = document.getElementById('preview');
            const btn = document.getElementById('toggleBtn');
            
            if(currentMode === 'view') {
                editor.style.display = 'block';
                preview.style.display = 'none';
                btn.innerText = 'View Mode';
                document.getElementById('saveBtn').style.display = 'block';
                document.getElementById('metaToggleBtn').style.display = 'none';
                document.getElementById('metadataPanel').classList.add('hidden');
                currentMode = 'edit';
            } else {
                editor.style.display = 'none';
                preview.style.display = 'block';
                renderView(editor.value || '*(Empty content)*');
                btn.innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                currentMode = 'view';
            }
        }

        // Image Drag & Drop
        const editor = document.getElementById('editor');
        editor.addEventListener('dragover', e => e.preventDefault());
        editor.addEventListener('drop', async e => {
            e.preventDefault();
            if(e.dataTransfer.files.length > 0) {
                const fd = new FormData();
                fd.append('image', e.dataTransfer.files[0]);
                const res = await fetch('/api/upload', { method: 'POST', body: fd });
                if(res.ok) {
                    const text = await res.text();
                    const cursor = editor.selectionStart;
                    editor.value = editor.value.substring(0, cursor) + text + editor.value.substring(cursor);
                    editor.dispatchEvent(new Event('input'));
                }
            }
        });

        async function login() {
            const pwd = document.getElementById('pwdInput').value;
            const res = await fetch('/login', {
                method: 'POST',
                headers: {'Content-Type': 'application/x-www-form-urlencoded'},
                body: 'password=' + encodeURIComponent(pwd)
            });
            if(res.ok) {
                document.getElementById('loginOverlay').style.display = 'none';
                document.getElementById('mainUI').style.display = 'flex';
                checkRole();
            } else {
                alert('Invalid Password');
            }
        }

        function checkRole() {
            if(document.cookie.includes('session_role=guest')) {
                document.querySelectorAll('.admin-only').forEach(el => {
                    if(el.tagName === 'BUTTON' || el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') el.disabled = true;
                    if(el.id === 'toggleBtn' || el.id === 'editor' || el.id === 'saveBtn') el.style.display = 'none';
                });
            }
        }

        async function saveNote() {
            let content = document.getElementById('editor').value;
            const parts = content.split(/(?:\r?\n){2,}/);
            if (parts.length > 0 && /^[A-Za-z0-9_-]+:/.test(parts[0])) {
                let headerLines = parts[0].split(/\r?\n/);
                let modIdx = headerLines.findIndex(l => l.startsWith('Modified:'));
                let now = new Date().toISOString().replace('T', ' ').substring(0, 19);
                if (modIdx !== -1) {
                    headerLines[modIdx] = `Modified: ${now}`;
                } else {
                    headerLines.push(`Modified: ${now}`);
                }
                parts[0] = headerLines.join('\n');
                content = parts.join('\n\n');
                document.getElementById('editor').value = content;
            }

            const fd = new URLSearchParams();
            fd.append('name', currentNote);
            fd.append('content', content);
            const res = await fetch('/api/save', { method: 'POST', body: fd });
            if(res.ok) {
                alert('Note saved!');
                toggleMode();
            } else {
                alert('Failed to save!');
            }
        }

        async function submitQuickNote() {
            const fd = new URLSearchParams();
            fd.append('note', document.getElementById('quickText').value);
            const res = await fetch('/api/quick', { method: 'POST', body: fd });
            if(res.ok) {
                document.getElementById('quickText').value = '';
                document.getElementById('quickPanel').classList.add('hidden');
                alert('Saved!');
            }
        }

        async function submitBookmark() {
            const fd = new URLSearchParams();
            fd.append('url', document.getElementById('bmUrl').value);
            fd.append('title', document.getElementById('bmTitle').value);
            fd.append('tags', document.getElementById('bmTags').value);
            fd.append('notes', document.getElementById('bmNotes').value);
            const res = await fetch('/api/bookmark', { method: 'POST', body: fd });
            if(res.ok) {
                document.getElementById('bmPanel').classList.add('hidden');
                document.querySelectorAll('#bmPanel input, #bmPanel textarea').forEach(el => el.value = '');
                alert('Saved!');
            }
        }
    </script>

    <!-- Code & Math Formatting Assets -->
    <script src="/js/highlight.min.js"></script>
    <script src="/js/katex.min.js"></script>
    <script src="/js/auto-render.min.js"></script>
    <script>
        document.addEventListener("DOMContentLoaded", () => {
            // 1. Hook Highlight.js into Marked parser globally
            if (window.marked && window.hljs) {
                window.marked.setOptions({
                    highlight: function(code, lang) {
                        const language = window.hljs.getLanguage(lang) ? lang : 'plaintext';
                        return window.hljs.highlight(code, { language }).value;
                    },
                    langPrefix: 'hljs language-'
                });
            }
            
            // 2. Setup Auto-Rendering for KaTeX via MutationObserver
            // This guarantees Math renders regardless of how you inject the Markdown!
            const previewNode = document.getElementById('preview') || document.body;
            let renderTimeout;
            const observer = new MutationObserver(() => {
                clearTimeout(renderTimeout);
                renderTimeout = setTimeout(() => {
                    if (window.renderMathInElement) {
                        renderMathInElement(previewNode, {
                            delimiters: [
                                {left: '$$', right: '$$', display: true},
                                {left: '$', right: '$', display: false},
                                {left: '\(', right: '\)', display: false},
                                {left: '\[', right: '\]', display: true}
                            ],
                            throwOnError: false
                        });
                    }
                }, 50);
            });
            observer.observe(previewNode, { childList: true, subtree: true });
        });
    </script>

    <!-- Small Version Footer -->
    <div id="goomn-version-footer" style="position: fixed; bottom: 4px; right: 8px; font-size: 0.75rem; color: #888; z-index: 9999; opacity: 0.7; pointer-events: none;"></div>
    <script>
        document.addEventListener("DOMContentLoaded", () => {
            const footer = document.getElementById('goomn-version-footer');
            let v = '1.1.0';
            try { if (APP_VERSION) v = APP_VERSION; } catch(e) {}
            if (footer) footer.innerText = 'GoOMN v' + v;
        });
    </script>

    <!-- JS Console Interceptor & UI -->
    <script>
        (function() {
            const originalLog = console.log;
            const originalError = console.error;
            const originalWarn = console.warn;
            const originalInfo = console.info;

            let logs = [];
            let consoleBtn = null;
            let consoleModal = null;
            let logsContainer = null;

            function initConsoleUI() {
                if (consoleBtn) return;

                // 1. Create Scrollable Modal
                consoleModal = document.createElement('div');
                consoleModal.id = 'goomn-console-modal';
                consoleModal.style.cssText = 'display:none; position:fixed; top:10%; left:10%; width:80%; height:80%; background:#1e1e1e; color:#00ff00; z-index:10000; border:2px solid #555; border-radius:8px; flex-direction:column; font-family:monospace; box-shadow: 0 4px 12px rgba(0,0,0,0.5);';

                const header = document.createElement('div');
                header.style.cssText = 'padding:10px; background:#333; color:#fff; display:flex; justify-content:space-between; align-items:center; border-bottom:1px solid #555; font-weight:bold;';
                header.innerHTML = '<span>JS Console Output</span><div><button id="goomn-console-clear" style="background:#888; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer; margin-right:8px;">Clear</button><button id="goomn-console-close" style="background:#ff5555; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer;">Close</button></div>';

                logsContainer = document.createElement('div');
                logsContainer.style.cssText = 'flex:1; overflow-y:auto; padding:10px; white-space:pre-wrap; word-break:break-all; font-size:12px; line-height:1.4;';

                consoleModal.appendChild(header);
                consoleModal.appendChild(logsContainer);
                document.body.appendChild(consoleModal);

                document.getElementById('goomn-console-close').onclick = () => {
                    consoleModal.style.display = 'none';
                };
                let clrBtn = document.getElementById('goomn-console-clear');
                if (clrBtn) {
                    clrBtn.onclick = () => {
                        logs = [];
                        if (logsContainer) logsContainer.innerHTML = '';
                        if (consoleBtn) consoleBtn.innerText = 'Console (0)';
                    };
                }

                // 2. Create Activation Button
                consoleBtn = document.createElement('button');
                consoleBtn.id = 'goomn-console-btn';
                consoleBtn.innerText = 'Console (0)';
                consoleBtn.style.cssText = 'margin-left:8px; padding:4px 8px; background:#ff9800; color:#fff; border:none; border-radius:4px; cursor:pointer; font-size:0.8rem; font-weight:bold;';
                consoleBtn.onclick = () => {
                    consoleModal.style.display = 'flex';
                };

                // 3. Intelligently locate the "metadata" element to snap next to it
                let metadataEl = Array.from(document.querySelectorAll('*')).find(el => {
                    if (el.children.length > 0) return false; // Focus on leaf nodes only
                    const text = (el.textContent || '').toLowerCase();
                    const id = (el.id || '').toLowerCase();
                    const cls = (el.className || '').toLowerCase();
                    return text.includes('metadata') || id.includes('metadata') || cls.includes('metadata');
                });

                if (metadataEl && metadataEl.parentNode) {
                    metadataEl.parentNode.insertBefore(consoleBtn, metadataEl.nextSibling);
                } else {
                    // Fallback: Drop it floating in the bottom-left if metadata isn't found
                    consoleBtn.style.position = 'fixed';
                    consoleBtn.style.bottom = '4px';
                    consoleBtn.style.left = '8px';
                    consoleBtn.style.zIndex = '9999';
                    document.body.appendChild(consoleBtn);
                }
            }

            function appendLog(type, args) {
                logs.push({type, args});
                
                // Ensure the DOM is ready before trying to append UI elements
                if (!document.body) {
                    window.addEventListener('DOMContentLoaded', () => appendLog(type, args));
                    return;
                }

                if (!consoleBtn) initConsoleUI();

                consoleBtn.innerText = `Console (${logs.length})`;

                if (logsContainer) {
                    const msg = document.createElement('div');
                    msg.style.marginBottom = '4px';
                    msg.style.paddingBottom = '4px';
                    msg.style.borderBottom = '1px solid #333';
                    const color = type === 'error' ? '#ff5555' : type === 'warn' ? '#ffb86c' : '#f8f8f2';
                    msg.style.color = color;

                    const text = Array.from(args).map(a => {
                        try { return typeof a === 'object' ? JSON.stringify(a) : String(a); }
                        catch(e) { return String(a); }
                    }).join(' ');
                    
                    msg.textContent = `[${type.toUpperCase()}] ${text}`;
                    logsContainer.appendChild(msg);
                    logsContainer.scrollTop = logsContainer.scrollHeight;
                }
            }

            // 4. Override Native Console Functions
            console.log = function(...args) {
                originalLog.apply(console, args);
                appendLog('log', args);
            };
            console.error = function(...args) {
                originalError.apply(console, args);
                appendLog('error', args);
            };
            console.warn = function(...args) {
                originalWarn.apply(console, args);
                appendLog('warn', args);
            };
            console.info = function(...args) {
                originalInfo.apply(console, args);
                appendLog('info', args);
            };
            
            // 5. Catch fatal uncaught errors automatically
            window.addEventListener('error', function(e) {
                console.error('Uncaught Error:', e.message, 'at', e.filename, ':', e.lineno);
            });
        })();
    </script>
</body>
</html>

```

### backend/frontend/index.html END

### android/build.gradle START
```
buildscript {
    repositories {
        google()
        mavenCentral()
    }
    dependencies {
        classpath 'com.android.tools.build:gradle:8.1.2'
    }
}
allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

```

### android/build.gradle END

### android/settings.gradle START
```
rootProject.name = "GoOMN"
include ':app'

```

### android/settings.gradle END

### android/app/build.gradle START
```
plugins {
    id 'com.android.application'
}

android {
    namespace 'net.basov.goomn'
    compileSdk 34

    defaultConfig {
        applicationId "net.basov.goomn"
        minSdk 24
        targetSdk 34
        versionCode 10100
        versionName "1.1.0"
    }

    signingConfigs {
        release {
            storeFile file('goomn.keystore')
            storePassword 'goomn123'
            keyAlias 'goomn'
            keyPassword 'goomn123'
        }
    }
    buildTypes {
        release {
            signingConfig signingConfigs.release
            minifyEnabled false
            proguardFiles getDefaultProguardFile('proguard-android-optimize.txt'), 'proguard-rules.pro'
        }
    }
    compileOptions {
        sourceCompatibility JavaVersion.VERSION_17
        targetCompatibility JavaVersion.VERSION_17
    }
}

dependencies {
    implementation fileTree(dir: 'libs', include: ['*.jar', '*.aar'])
}

```

### android/app/build.gradle END

### android/app/src/main/java/net/basov/goomn/MainActivity.java START
```
package net.basov.goomn;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import net.basov.goomn.backend.Backend;
import android.os.Handler;
import android.os.Looper;

public class MainActivity extends Activity {
    private WebView webView;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // Start the Go Backend Server from the gomobile .aar
        Backend.startServer();

        // Initialize WebView
        webView = new WebView(this);
        WebSettings webSettings = webView.getSettings();
        webSettings.setJavaScriptEnabled(true);
        webSettings.setDomStorageEnabled(true);
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                if (url != null && (url.startsWith("http://") || url.startsWith("https://"))) {
                    if (!url.contains("localhost")) {
                        view.getContext().startActivity(
                            new android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url))
                        );
                        return true;
                    }
                }
                return false;
            }
        });

        setContentView(webView);

        // Wait for the Go server to bind before loading
        new Handler(Looper.getMainLooper()).postDelayed(new Runnable() {
            @Override
            public void run() {
                webView.loadUrl("http://localhost:8080");
            }
        }, 1000); // 1 second delay
    }

    @Override
    public void onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack();
        } else {
            super.onBackPressed();
        }
    }
}

```

### android/app/src/main/java/net/basov/goomn/MainActivity.java END

### android/app/src/main/AndroidManifest.xml START
```
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="net.basov.goomn">
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" />
    <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" />
    
    <application
        android:allowBackup="true"
        android:label="GoOMN"
        android:usesCleartextTraffic="true"
        android:hardwareAccelerated="true"
        android:theme="@android:style/Theme.NoTitleBar.Fullscreen">
        <activity
            android:name=".MainActivity"
            android:exported="true"
            android:configChanges="orientation|keyboardHidden|screenSize">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>

```

### android/app/src/main/AndroidManifest.xml END
