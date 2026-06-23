Here is the current state of the OMN-Go project. We are currently at Version ${CUR_OMN_GO_VER} (Android version code ${CUR_OMN_GO_VER_A}).

Below is the complete current codebase and the master `initial_prompt.md`. Please review them and acknowledge that you are ready for my next request. Remember to strictly follow the Turn 2 Python patching output format. Application version need to be updated on every changes.

-------- doc/initial_prompt.md START --------

```
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

-------- doc/initial_prompt.md END --------


-------- Dockerfile START --------

```
# STAGE 1: Toolchains & Cache
FROM golang:1.26-bookworm AS builder
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
RUN go get github.com/yuin/goldmark@latest && go get golang.org/x/mobile@latest && go mod tidy

# Desktop Binary (OMN-Go naming convention)
RUN VERSION=$(awk -F'"' '/APP_VERSION =/ {print $2}' backend/version.go) && \
    GOOS=linux GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-linux-amd64" main_desktop.go && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-windows-amd64.exe" main_desktop.go

# Android APK - Webview Wrapper via Gradle & gomobile bind (strictly zero AndroidX/AppCompat)
RUN go get -tool golang.org/x/mobile/cmd/gobind && \
    go mod tidy && \
    mkdir -p android/app/libs && \
    gomobile bind -target=android -androidapi 24 -javapkg net.basov.omngo -o android/app/libs/omngo.aar ./backend

RUN cd android && \
    if [ ! -f app/omn-go.keystore ]; then \
      keytool -genkey -v -keystore app/omn-go.keystore \
              -alias omn-go -keyalg RSA -keysize 2048 \
              -validity 10000 -storepass omn-go123 -keypass omn-go123 \
              -dname "CN=OMN-Go, O=Basov"; \
    fi && \
    gradle assembleRelease && \
    cp app/build/outputs/apk/release/*.apk ../bin/ #&& \
    #cp app/omn-go.keystore ../bin/omn-go.keystore

```

-------- Dockerfile END --------


-------- go.mod START --------

```
module net.basov.omngo

go 1.26

require github.com/yuin/goldmark v1.8.2

```

-------- go.mod END --------


-------- main_desktop.go START --------

```
//go:build !android

package main

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"
	"net.basov.omngo/backend"
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

-------- main_desktop.go END --------


-------- backend/server.go START --------

```
package backend

import (
	"embed"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/html frontend/md
var staticFS embed.FS

var activeConns int

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
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in server: %v", r)
			}
		}()

		mux := http.NewServeMux()
		mux.HandleFunc("/", serveFrontend)

		serveLazyEmbed := func() http.Handler {
			physicalDir := filepath.Join(storageDir, "html")
			fsHandler := http.FileServer(http.Dir(physicalDir))

			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Calculate physical path
				physPath := filepath.Join(physicalDir, filepath.Clean(r.URL.Path))

				// Lazy Extraction: Check if file exists on disk, if not, pull from embedFS
				if _, err := os.Stat(physPath); os.IsNotExist(err) {
					embedPath := "frontend/html" + filepath.ToSlash(filepath.Clean(r.URL.Path))
					if data, err := staticFS.ReadFile(embedPath); err == nil {
						os.MkdirAll(filepath.Dir(physPath), 0755)
						os.WriteFile(physPath, data, 0644)
					}
				}

				// Serve the file dynamically from the physical directory
				fsHandler.ServeHTTP(w, r)
			})
		}

		mux.Handle("/js/", serveLazyEmbed())
		mux.Handle("/css/", serveLazyEmbed())
		mux.Handle("/json/", serveLazyEmbed())

		// Config for files handling Content-type by served directories
		serveStorageDir := func(subDir, cType string) http.Handler {
			dirPath := filepath.Join(storageDir, "html", subDir)
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
		mux.HandleFunc("/api/newpage", authMiddleware(handleNewPage, true))
		mux.HandleFunc("/api/config", authMiddleware(handleConfig, true))
		mux.HandleFunc("/api/edit-external", authMiddleware(handleEditExternal, true))

		if appConfig.ServerPort <= 0 {
			appConfig.ServerPort = 8080
		}

		bindAddr := fmt.Sprintf("0.0.0.0:%d", appConfig.ServerPort)

		log.Printf("OMN-Go Backend running on %s", bindAddr)
		err := http.ListenAndServe(bindAddr, connectionMiddleware(mux))
		if err != nil {
			log.Printf("FATAL: Server crashed: %v", err)
		}
	}()
}

// GetServerPort safely exposes the configured port for frontend wrappers
func GetServerPort() int {
	return appConfig.ServerPort
}

```

-------- backend/server.go END --------


-------- backend/config.go START --------

```
package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	ServerPort    int               `json:"server_port"`
	AdminPassword string            `json:"admin_password"`
	GuestPassword string            `json:"guest_password"`
	Author        string            `json:"author"`
	UseInternalEd bool              `json:"use_internal_editor"`
	DesktopExtCmd string            `json:"desktop_ext_cmd"`
	MimeTypes     map[string]string `json:"mime_types"`
}

var appConfig Config

func loadConfig(storageDir string) {
	configPath := filepath.Join(storageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		appConfig = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
			Author:        "Anonymous",
			UseInternalEd: true,
			DesktopExtCmd: "subl",
			MimeTypes: map[string]string{
				".css":   "text/css",
				".js":    "application/javascript",
				".json":  "application/json",
				".html":  "text/html",
				".md":    "text/markdown",
				".svg":   "image/svg+xml",
				".png":   "image/png",
				".jpg":   "image/jpeg",
				".jpeg":  "image/jpeg",
				".woff2": "font/woff2",
			},
		}
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(configPath, data, 0644)
	} else {
		data, _ := os.ReadFile(configPath)
		json.Unmarshal(data, &appConfig)
		if appConfig.ServerPort == 0 {
			appConfig.ServerPort = 8080
		}
		if appConfig.MimeTypes == nil {
			appConfig.MimeTypes = map[string]string{
				".css":   "text/css",
				".js":    "application/javascript",
				".json":  "application/json",
				".woff2": "font/woff2",
			}
			data, _ := json.MarshalIndent(appConfig, "", "  ")
			os.WriteFile(configPath, data, 0644)
		}
	}
}

```

-------- backend/config.go END --------


-------- backend/handlers.go START --------

```
package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func getConfigPageBody() string {
	return fmt.Sprintf(`
<div style="max-width: 600px; margin: 0 auto; background: #ffffff; padding: 30px; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); border: 1px solid #e1e4e8;">
    <h2 style="margin-top: 0; color: #1a1a1a; font-size: 24px; font-weight: 700; border-bottom: 2px solid #eaecef; padding-bottom: 10px;">Configuration Dashboard</h2>
    <form id="configForm" onsubmit="saveConfig(event)" style="margin-top: 20px;">
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Server Port</label>
            <input type="number" id="cfgPort" value="%d" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" required />
        </div>
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Admin Password</label>
            <input type="password" id="cfgAdminPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" required />
        </div>
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Guest Password</label>
            <input type="password" id="cfgGuestPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" required />
        </div>
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Author Name</label>
            <input type="text" id="cfgAuthor" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
        </div>
        <div style="margin-bottom: 20px; display: flex; align-items: center; gap: 10px;">
            <input type="checkbox" id="cfgUseInternal" %s style="width: 20px; height: 20px; cursor: pointer;" />
            <label for="cfgUseInternal" style="font-weight: 600; color: #444; cursor: pointer;">Use HTML Internal Editor</label>
        </div>
        <div style="margin-bottom: 25px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Desktop External Editor Command</label>
            <input type="text" id="cfgExtCmd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
            <small style="color: #666; display: block; margin-top: 5px;">Example: <code>subl</code> or <code>code</code> or <code>nano</code></small>
        </div>
        <button type="submit" style="background: #28a745; color: white; border: none; padding: 12px 20px; border-radius: 4px; font-weight: bold; cursor: pointer; width: 100%%; font-size: 16px; transition: background 0.2s;">Save Configuration</button>
    </form>
</div>
<script>
    async function saveConfig(event) {
        event.preventDefault();
        const params = new URLSearchParams();
        params.append("server_port", document.getElementById("cfgPort").value);
        params.append("admin_password", document.getElementById("cfgAdminPwd").value);
        params.append("guest_password", document.getElementById("cfgGuestPwd").value);
        params.append("author", document.getElementById("cfgAuthor").value);
        params.append("use_internal_editor", document.getElementById("cfgUseInternal").checked ? "true" : "false");
        params.append("desktop_ext_cmd", document.getElementById("cfgExtCmd").value);

        const res = await fetch("/api/config", { method: "POST", body: params });
        if (res.ok) {
            alert("Configuration saved successfully! Server port changes will take effect after restarting the application.");
            window.location.reload();
        } else {
            alert("Failed to save configuration.");
        }
    }
</script>
`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author,
		func() string {
			if appConfig.UseInternalEd {
				return "checked"
			}
			return ""
		}(),
		appConfig.DesktopExtCmd)
}

func getExternalEditPageBody(name string) string {
	return fmt.Sprintf(`
<div style="max-width: 600px; margin: 40px auto; background: #ffffff; padding: 40px; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); border: 1px solid #e1e4e8; text-align: center;">
    <div style="font-size: 48px; margin-bottom: 20px;">📝</div>
    <h2 style="margin-top: 0; color: #1a1a1a; font-size: 24px; font-weight: 700;">Editing Externally</h2>
    <p style="color: #555; font-size: 16px; margin-bottom: 30px; line-height: 1.5;">
        We have launched <strong>%s</strong> to edit <code>%s.md</code>. Please complete your changes in your editor, save the file, and click the button below to view the updated page.
    </p>
    <button onclick="window.location.replace('/%s.html')" style="background: #0056b3; color: white; border: none; padding: 15px 30px; border-radius: 6px; font-weight: bold; cursor: pointer; font-size: 18px; transition: background 0.2s; box-shadow: 0 2px 5px rgba(0,0,0,0.2);">
        Press after edit to refresh view
    </button>
</div>
`, appConfig.DesktopExtCmd, name, name)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(appConfig)
		return
	}
	if r.Method == "POST" {
		portStr := r.FormValue("server_port")
		var port int
		fmt.Sscanf(portStr, "%d", &port)
		if port > 0 {
			appConfig.ServerPort = port
		}
		appConfig.AdminPassword = r.FormValue("admin_password")
		appConfig.GuestPassword = r.FormValue("guest_password")
		appConfig.Author = r.FormValue("author")
		appConfig.UseInternalEd = r.FormValue("use_internal_editor") == "true"
		appConfig.DesktopExtCmd = r.FormValue("desktop_ext_cmd")

		data, _ := json.MarshalIndent(appConfig, "", "  ")
		configPath := filepath.Join(storageDir, "config.json")
		os.WriteFile(configPath, data, 0644)
		w.Write([]byte("Saved"))
		return
	}
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

func handleEditExternal(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Missing name", http.StatusBadRequest)
		return
	}

	if runtime.GOOS == "android" {
		w.Header().Set("Location", "omngo://edit?name="+name)
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	cleanName := strings.TrimSuffix(name, ".html")
	if !strings.HasSuffix(cleanName, ".md") {
		cleanName += ".md"
	}
	filePath := filepath.Join(storageDir, "md", cleanName)

	var cmd *exec.Cmd
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
	}

	w.Header().Set("Content-Type", "text/html")
	pageName := strings.TrimSuffix(cleanName, ".md")
	waitBody := getExternalEditPageBody(pageName)
	compiledWait := compilePageWithBody(pageName, fmt.Appendf(nil, "Title: Refresh %s\nDate: %s\nCategory: Action\n\n", pageName, time.Now().Format("2006-01-02 15:04:05")), waitBody)
	w.Write(compiledWait)
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
	fullMarkdown := strings.Join(newContent, "\n")
	fullMarkdown = ensureHeaderModified(fullMarkdown, "Quick Notes")
	os.WriteFile(path, []byte(fullMarkdown), 0644)

	// Update Dynamic Precompile instantly
	compiled := compilePage("QuickNotes", []byte(fullMarkdown))
	os.WriteFile(filepath.Join(storageDir, "html", "QuickNotes.html"), compiled, 0644)

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
	for t := range strings.SplitSeq(tags, ",") {
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
			newContent = ensureHeaderModified(newContent, "Incoming bookmarks")
			os.WriteFile(path, []byte(newContent), 0644)
			// Update Dynamic Precompile instantly
			compiled := compilePage("Bookmarks", []byte(newContent))
			os.WriteFile(filepath.Join(storageDir, "html", "Bookmarks.html"), compiled, 0644)
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

	imgDir := filepath.Join(storageDir, "html", "images")
	os.MkdirAll(imgDir, 0755)

	destPath := filepath.Join(imgDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)

	w.Write(fmt.Appendf(nil, "![%s]({filename}/images/%s)", header.Filename, header.Filename))
}

func handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	jsonDir := filepath.Join(storageDir, "html", "user_json")
	os.MkdirAll(jsonDir, 0755)

	destPath := filepath.Join(jsonDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)

	w.Write(fmt.Appendf(nil, "[%s]({filename}/user_json/%s)", header.Filename, header.Filename))
}

func handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Welcome"
	}

	var path string
	var data []byte
	var err error

	if !strings.Contains(name, ".") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {
		cleanName := strings.TrimSuffix(name, ".html")
		if !strings.HasSuffix(cleanName, ".md") {
			cleanName += ".md"
		}
		path = filepath.Join(storageDir, "md", filepath.Clean(cleanName))
		data, err = os.ReadFile(path)
		if err != nil {
			embedPath := "frontend/md/" + cleanName
			data, err = staticFS.ReadFile(embedPath)
			if err != nil {
				title := strings.TrimSuffix(cleanName, ".md")
				timestamp := time.Now().Format("2006-01-02 15:04:05")
				authorLine := ""
				if appConfig.Author != "" {
					authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
				}
				newContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n", title, timestamp, authorLine)
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte(newContent), 0644)
				data = []byte(newContent)
			} else {
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, data, 0644)
			}
		}
	} else {
		path = filepath.Join(storageDir, "html", filepath.Clean(name))
		data, err = os.ReadFile(path)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
	}
	w.Write(data)
}

func handleNewPage(w http.ResponseWriter, r *http.Request) {
	source := r.FormValue("source")
	target := r.FormValue("target")
	title := r.FormValue("title")

	if target == "" || title == "" {
		http.Error(w, "Missing fields", http.StatusBadRequest)
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	targetMdPath := filepath.Join(storageDir, "md", target+".md")
	if _, err := os.Stat(targetMdPath); os.IsNotExist(err) {
		authorLine := ""
		if appConfig.Author != "" {
			authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
		}
		defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nModified: %s\nCategory: Notes%s\n\n", title, now, now, authorLine)
		os.MkdirAll(filepath.Dir(targetMdPath), 0755)
		os.WriteFile(targetMdPath, []byte(defaultContent), 0644)
	}

	if source != "" {
		sourceMdPath := filepath.Join(storageDir, "md", source+".md")
		sourceData, err := os.ReadFile(sourceMdPath)
		if err == nil {
			content := string(sourceData)
			linkStr := fmt.Sprintf("* [%s](%s)", title, target)
			parts := strings.SplitN(content, "\n\n", 2)

			isHeader := false
			if len(parts) > 0 && strings.Contains(parts[0], ":") {
				firstLine := strings.Split(parts[0], "\n")[0]
				if strings.Contains(firstLine, ":") && !strings.HasPrefix(firstLine, " ") && !strings.HasPrefix(firstLine, "#") {
					isHeader = true
				}
			}

			if isHeader {
				if len(parts) > 1 {
					content = parts[0] + "\n\n" + linkStr + "\n" + parts[1]
				} else {
					content = parts[0] + "\n\n" + linkStr + "\n"
				}
			} else {
				content = linkStr + "\n\n" + content
			}

			content = ensureHeaderModified(content, source)
			os.WriteFile(sourceMdPath, []byte(content), 0644)

			// Recompile Source HTML immediately to prevent caching delays
			htmlPath := filepath.Join(storageDir, "html", source+".html")
			compiled := compilePage(source, []byte(content))
			os.MkdirAll(filepath.Dir(htmlPath), 0755)
			os.WriteFile(htmlPath, compiled, 0644)
		}
	}

	w.Write([]byte("Created"))
}

func handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		return
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")

	var path string
	if !strings.Contains(name, ".") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {
		cleanName := strings.TrimSuffix(name, ".html")
		if !strings.HasSuffix(cleanName, ".md") {
			cleanName += ".md"
		}
		path = filepath.Join(storageDir, "md", filepath.Clean(cleanName))

		content = ensureHeaderModified(content, strings.TrimSuffix(cleanName, ".md"))

		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)

		htmlPath := filepath.Join(storageDir, "html", strings.TrimSuffix(cleanName, ".md")+".html")
		os.MkdirAll(filepath.Dir(htmlPath), 0755)
		compiled := compilePage(strings.TrimSuffix(cleanName, ".md"), []byte(content))
		os.WriteFile(htmlPath, compiled, 0644)

	} else {
		path = filepath.Join(storageDir, "html", filepath.Clean(name))
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)
	}

	w.Write([]byte("Saved"))
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" || path == "/index.html" {
		http.Redirect(w, r, "/Welcome.html", http.StatusSeeOther)
		return
	}

	if strings.HasSuffix(path, ".html") {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".html")

		if name == "Config" {
			w.Header().Set("Content-Type", "text/html")
			body := getConfigPageBody()
			compiled := compilePageWithBody("Config", []byte("Title: Config\nCategory: Settings\n\n"), body)
			injected := strings.Replace(string(compiled), "</head>", fmt.Sprintf("<script>var APP_VERSION = \"%s\"; var USE_INTERNAL_ED = %t;</script></head>", APP_VERSION, appConfig.UseInternalEd), 1)
			w.Write([]byte(injected))
			return
		}

		htmlPath := filepath.Join(storageDir, "html", filepath.Clean(name+".html"))
		mdPath := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))

		htmlStat, errHtml := os.Stat(htmlPath)
		mdStat, errMd := os.Stat(mdPath)

		// Recompile if HTML is missing, OR if Markdown was modified more recently than HTML
		forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
		if forceRefresh || os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
			if os.IsNotExist(errMd) {
				embedData, err := staticFS.ReadFile("frontend/md/" + name + ".md")
				if err == nil {
					os.MkdirAll(filepath.Dir(mdPath), 0755)
					os.WriteFile(mdPath, embedData, 0644)
				} else {
					timestamp := time.Now().Format("2006-01-02 15:04:05")
					authorLine := ""
					if appConfig.Author != "" {
						authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
					}
					defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n", name, timestamp, authorLine)
					os.MkdirAll(filepath.Dir(mdPath), 0755)
					os.WriteFile(mdPath, []byte(defaultContent), 0644)
				}
			}

			mdContent, err := os.ReadFile(mdPath)
			if err == nil {
				if errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime()) {
					updatedContent := ensureHeaderModified(string(mdContent), name)
					if updatedContent != string(mdContent) {
						os.WriteFile(mdPath, []byte(updatedContent), 0644)
						mdContent = []byte(updatedContent)
					}
				}
				compiled := compilePage(name, mdContent)
				os.MkdirAll(filepath.Dir(htmlPath), 0755)
				os.WriteFile(htmlPath, compiled, 0644)
			}
		}

		w.Header().Set("Content-Type", "text/html")
		data, err := os.ReadFile(htmlPath)
		if err == nil {
			injected := strings.Replace(string(data), "</head>", fmt.Sprintf("<script>var APP_VERSION = \"%s\"; var USE_INTERNAL_ED = %t;</script></head>", APP_VERSION, appConfig.UseInternalEd), 1)
			w.Write([]byte(injected))
		} else {
			http.ServeFile(w, r, htmlPath)
		}
		return
	}

	// Unified Content-Type Resolver based strictly on extension
	ext := strings.ToLower(filepath.Ext(path))
	mimeType, exists := appConfig.MimeTypes[ext]
	if !exists {
		mimeType = mime.TypeByExtension(ext)
	}
	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}

	// Priority 1: User's Local Storage Directory (data/html/css, data/html/js, etc)
	filePath := filepath.Join(storageDir, "html", filepath.Clean(path))
	if stat, err := os.Stat(filePath); err == nil && !stat.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}

	// Priority 2: Embedded Fallback Template Cache - Copy to Data
	embedPath := "frontend" + filepath.Clean(path)
	if data, err := staticFS.ReadFile(embedPath); err == nil {
		// Copy extracted file directly to user data directory
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, data, 0644)

		w.Write(data)
		return
	}

	http.NotFound(w, r)
}

```

-------- backend/handlers.go END --------


-------- backend/markdown.go START --------

```
package backend

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var mdParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		html.WithHardWraps(),
		html.WithUnsafe(), // CRITICAL: Allows raw Bookmarks.md scripts to execute
	),
)

func renderMarkdownToHTML(mdContent []byte) string {
	contentStr := string(mdContent)
	mathBlocks := make(map[string]string)
	counter := 0

	// Protect complex KaTeX Math blocks from markdown emphasis corruption
	contentStr = regexp.MustCompile(`(?s)\$\$.*?\$\$`).ReplaceAllStringFunc(contentStr, func(m string) string {
		placeholder := fmt.Sprintf("OMN_MATH_BLOCK_%d", counter)
		mathBlocks[placeholder] = m
		counter++
		return placeholder
	})
	contentStr = regexp.MustCompile(`\$[^\$]+\$`).ReplaceAllStringFunc(contentStr, func(m string) string {
		placeholder := fmt.Sprintf("OMN_MATH_INLINE_%d", counter)
		mathBlocks[placeholder] = m
		counter++
		return placeholder
	})

	var buf bytes.Buffer
	if err := mdParser.Convert([]byte(contentStr), &buf); err != nil {
		return string(mdContent)
	}
	htmlStr := buf.String()

	// Restore math blocks natively for the offline KaTeX frontend
	for placeholder, original := range mathBlocks {
		htmlStr = strings.ReplaceAll(htmlStr, placeholder, original)
	}

	// Remap static browsing links natively
	htmlStr = regexp.MustCompile(`href="([^"http#:]+)\.md"`).ReplaceAllString(htmlStr, `href="$1.html"`)
	htmlStr = regexp.MustCompile(`href="([^"\.#:]+)"`).ReplaceAllString(htmlStr, `href="$1.html"`)
	return htmlStr
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func compilePage(name string, mdContent []byte) []byte {
	return compilePageWithBody(name, mdContent, "")
}

func compilePageWithBody(name string, mdContent []byte, customBody string) []byte {
	var headers []string
	var bodyLines []string
	inHeader := true

	lines := strings.SplitSeq(string(mdContent), "\n")
	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if inHeader {
			if trimmed == "" {
				inHeader = false
				continue
			}
			if strings.Contains(line, ":") {
				headers = append(headers, line)
			} else {
				inHeader = false
				bodyLines = append(bodyLines, line)
			}
		} else {
			bodyLines = append(bodyLines, line)
		}
	}

	renderedBody := customBody
	if renderedBody == "" {
		renderedBody = renderMarkdownToHTML([]byte(strings.Join(bodyLines, "\n")))
	}

	layout := string(frontendHTML)

	title := "OMN-Go - " + name
	var metaTags []string
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			k := strings.ToLower(strings.TrimSpace(parts[0]))
			v := htmlEscape(strings.TrimSpace(parts[1]))
			metaTags = append(metaTags, fmt.Sprintf(`    <meta name="%s" content="%s" />`, k, v))
			if k == "title" {
				title = strings.TrimSpace(parts[1])
			}
		}
	}
	metaTags = append(metaTags, fmt.Sprintf(`    <meta name="generator" content="OMN-Go %s" />`, APP_VERSION))

	metaScript := fmt.Sprintf(`    <script>
      var PackageName = 'net.basov.omngo';
      var PageName = '%s';
      var Title = '%s';
    </script>`, name, title)

	metaBlock := strings.Join(metaTags, "\n") + "\n" + metaScript

	layout = strings.ReplaceAll(layout, "</head>", metaBlock+"\n</head>")
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PAGE_TITLE -->", title)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PREVIEW_BODY -->", renderedBody)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", htmlEscape(string(mdContent)))
	layout = strings.ReplaceAll(layout, "/* OMN_GO_PAGE_NAME_JS */", fmt.Sprintf(`let currentNote = "%s";`, name))
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", "")

	return []byte(layout)
}

func ensureHeaderModified(content string, defaultTitle string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	parts := strings.SplitN(content, "\n\n", 2)
	now := time.Now().Format("2006-01-02 15:04:05")

	isHeader := false
	if len(parts) > 0 && strings.Contains(parts[0], ":") {
		lines := strings.Split(parts[0], "\n")
		if len(lines) > 0 && strings.Contains(lines[0], ":") && !strings.HasPrefix(lines[0], " ") && !strings.HasPrefix(lines[0], "#") && !strings.HasPrefix(lines[0], "<") {
			isHeader = true
		}
	}

	if isHeader {
		headerLines := strings.Split(parts[0], "\n")
		modIdx := -1
		for i, l := range headerLines {
			if strings.HasPrefix(strings.ToLower(l), "modified:") {
				modIdx = i
				break
			}
		}
		if modIdx != -1 {
			headerLines[modIdx] = fmt.Sprintf("Modified: %s", now)
		} else {
			headerLines = append(headerLines, fmt.Sprintf("Modified: %s", now))
		}
		parts[0] = strings.Join(headerLines, "\n")
		if len(parts) > 1 {
			return parts[0] + "\n\n" + parts[1]
		}
		return parts[0] + "\n\n"
	}

	authorLine := ""
	if appConfig.Author != "" {
		authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
	}
	return fmt.Sprintf("Title: %s\nDate: %s\nModified: %s%s\n\n%s", defaultTitle, now, now, authorLine, content)
}

```

-------- backend/markdown.go END --------


-------- backend/middleware.go START --------

```
package backend

import (
	"net"
	"net/http"
)

func isLocalConnection(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func connectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeConns++
		next.ServeHTTP(w, r)
		activeConns--
	})
}

func authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Automatically bypass authorization for internal OS/WebView connections
		if isLocalConnection(r) {
			next(w, r)
			return
		}

		cookie, err := r.Cookie("session_role")
		if err != nil || (requireAdmin && cookie.Value != "admin") || (!requireAdmin && cookie.Value != "admin" && cookie.Value != "guest") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

```

-------- backend/middleware.go END --------


-------- backend/storage.go START --------

```
package backend

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var storageDir string

func initStorage() {
	if runtime.GOOS == "android" {
		storageDir = "/storage/emulated/0/Android/media/net.basov.omngo"
	} else {
		storageDir = "./data"
	}

	// 1. Create Isolated Storage
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Printf("Failed to create storage: %v", err)
	}

	mdDir := filepath.Join(storageDir, "md")
	os.MkdirAll(mdDir, 0755)

	htmlDir := filepath.Join(storageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	// Migrate legacy root md files recursively
	files, _ := filepath.Glob(filepath.Join(storageDir, "*.md"))
	for _, f := range files {
		os.Rename(f, filepath.Join(mdDir, filepath.Base(f)))
	}

	// Migrate static directories inside html/
	dirsToMove := []string{"images", "user_json", "css", "js", "json", "fonts"}
	for _, d := range dirsToMove {
		oldPath := filepath.Join(storageDir, d)
		newPath := filepath.Join(htmlDir, d)
		if stat, err := os.Stat(oldPath); err == nil && stat.IsDir() {
			os.Rename(oldPath, newPath)
		}
	}

	// 2. Init Config
	loadConfig(storageDir)

	// 3. Extract all embedded MD files first
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

Yo! Welcome to OMN-Go! Start editing.

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
	// Precompile all notes to data/html/ at startup in the background
	go precompileAllPages()
}

func precompileAllPages() {
	mdDir := filepath.Join(storageDir, "md")
	htmlDir := filepath.Join(storageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	filepath.Walk(mdDir, func(f string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(f, ".md") {
			content, err := os.ReadFile(f)
			if err == nil {
				relPath, _ := filepath.Rel(mdDir, f)
				name := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
				compiled := compilePage(name, content)
				htmlPath := filepath.Join(htmlDir, filepath.Clean(name+".html"))
				os.MkdirAll(filepath.Dir(htmlPath), 0755)
				os.WriteFile(htmlPath, compiled, 0644)
			}
		}
		return nil
	})
}

```

-------- backend/storage.go END --------


-------- backend/version.go START --------

```
package backend

const APP_VERSION = "1.3.25"

```

-------- backend/version.go END --------


-------- backend/frontend/index.html START --------

```
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title><!-- OMN_GO_PAGE_TITLE --></title>
    <link rel="stylesheet" href="/css/omn-go-core.css">
    <link rel="stylesheet" href="/css/highlight.default.min.css">
    <link rel="stylesheet" href="/css/katex.min.css">
</head>
<body>
    
    <!-- Login Overlay -->
    <div id="loginOverlay" class="overlay" style="display: none;">
        <div class="modal">
            <h2>OMN-Go Login</h2>
            <input type="password" id="pwdInput" placeholder="Admin or Guest Password">
            <button onclick="login()">Enter</button>
        </div>
    </div>

    <!-- Main UI -->
    <div id="mainUI">
        <div class="header">
            <strong><!-- OMN_GO_PAGE_TITLE --></strong>
            <a href="/Welcome.html"><i class="material-icons">home</i></a>
            <a href="/Welcome.html#help"><i class="material-icons">help</i></a>
            <button onclick="createNewPage()" class="admin-only" style="background: #17a2b8; border-color: #17a2b8;"><i class="material-icons">note_add</i></button>
            <button onclick="window.location.href = window.location.pathname + '?refresh=1'" class="admin-only" style="background: #6c757d; border-color: #6c757d;"><i class="material-icons">refresh</i></button>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only"><i class="material-icons">bolt</i></button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only"><i class="material-icons">bookmark_add</i></button>
            <a href="/Bookmarks.html"><i class="material-icons">bookmarks</i></a>
            <a href="#" onclick="window.location.replace('/Config.html'); return false;" style="background: #444; border-color: #666;"><i class="material-icons">settings</i></a>
        </div>

        <div class="content-area">
            <div class="toolbar">
                <button id="metaToggleBtn" onclick="document.getElementById('metadataPanel').classList.toggle('hidden')" style="display: block; background: #17a2b8; color: white; border: none;"><i class="material-icons">info</i></button>
                <button id="saveBtn" onclick="saveNote()" class="admin-only" style="display: none; background: #28a745; color: white; border: none;"><i class="material-icons">save</i></button>
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only"><i class="material-icons">edit</i></button>
            </div>
            <div id="metadataPanel" class="hidden" style="background: #e9ecef; padding: 15px; font-family: monospace; white-space: pre-wrap; border: 1px solid #ccc; margin-bottom: 10px; border-radius: 4px; font-size: 13px;"><!-- OMN_GO_METADATA_PANEL --></div>
            <textarea id="editor" class="admin-only" placeholder="Markdown/Code content... Drag images here to upload."><!-- OMN_GO_RAW_MD --></textarea>
            <div id="preview"><!-- OMN_GO_PREVIEW_BODY --></div>
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
        /* OMN_GO_PAGE_NAME_JS */
    </script>
    <script src="/js/omn-go-core.js"></script>

    <!-- Code & Math Formatting Assets -->
    <script src="/js/highlight.min.js"></script>
    <script src="/js/katex.min.js"></script>
    <script src="/js/auto-render.min.js"></script>

    <!-- Small Version Footer -->
    <div id="omn-go-version-footer" style="position: fixed; bottom: 4px; right: 8px; font-size: 0.75rem; color: #888; z-index: 9999; opacity: 0.7; pointer-events: none;"></div>

</body>
</html>

```

-------- backend/frontend/index.html END --------


-------- backend/frontend/html/css/omn-go-core.css START --------

```
@font-face {
  font-family: 'Material Icons';
  font-style: normal;
  font-weight: 400;
  src: url('/css/fonts/material-icons.woff2') format('woff2');
}
.material-icons {
  font-family: 'Material Icons';
  font-weight: normal;
  font-style: normal;
  font-size: 24px;
  line-height: 1;
  letter-spacing: normal;
  text-transform: none;
  display: inline-block;
  white-space: nowrap;
  word-wrap: normal;
  direction: ltr;
  -webkit-font-feature-settings: 'liga';
  -webkit-font-smoothing: antialiased;
}

body { font-family: sans-serif; margin: 0; padding: 0; display: flex; flex-direction: column; height: 100vh; background: #f9f9f9; color: #333; }
.overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); display: flex; align-items: center; justify-content: center; z-index: 50; }
.modal { background: #fff; padding: 20px; border-radius: 4px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); width: 300px; }
.modal input, .modal button, .modal textarea { width: 100%; box-sizing: border-box; margin-bottom: 10px; padding: 8px; }
.modal button { background: #0056b3; color: white; border: none; cursor: pointer; border-radius: 4px; }
#mainUI { display: flex; flex: 1; flex-direction: column; }
.header { background: #333; color: #fff; padding: 10px 20px; display: flex; gap: 15px; align-items: center; }
.header a, .header button { color: #fff; text-decoration: none; cursor: pointer; background: transparent; border: 1px solid #555; padding: 5px 10px; border-radius: 4px; font-size: 14px; display: flex; align-items: center; }
.header a:hover, .header button:hover { background: #555; }
.content-area { flex: 1; padding: 20px; position: relative; display: flex; flex-direction: column; }
#editor { display: none; width: 100%; flex: 1; border: 1px solid #ccc; padding: 10px; font-family: monospace; resize: none; box-sizing: border-box; }
#preview { width: 100%; flex: 1; background: #fff; border: 1px solid #ccc; padding: 20px; overflow-y: auto; box-sizing: border-box; line-height: 1.6; }
.toolbar { display: flex; justify-content: flex-end; margin-bottom: 10px; gap: 10px; }
.toolbar button { padding: 5px 15px; cursor: pointer; border: 1px solid #ccc; border-radius: 4px; display: flex; align-items: center; }
.hidden { display: none !important; }
.panel { position: absolute; top: 50px; right: 20px; background: white; border: 1px solid #ccc; padding: 15px; box-shadow: 0 4px 8px rgba(0,0,0,0.1); width: 300px; z-index: 40; }
.panel h3 { margin-top: 0; }
.panel input, .panel textarea, .panel button { width: 100%; box-sizing: border-box; margin-bottom: 10px; padding: 8px; }
.panel button { background: #28a745; color: white; border: none; cursor: pointer; border-radius: 4px; }

/* JS Console UI */
.console-modal { display: none; position: fixed; top: 10%; left: 10%; width: 80%; height: 80%; background: #1e1e1e; color: #00ff00; z-index: 10000; border: 2px solid #555; border-radius: 8px; flex-direction: column; font-family: monospace; box-shadow: 0 4px 12px rgba(0,0,0,0.5); }
.console-header { padding: 10px; background: #333; color: #fff; display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #555; font-weight: bold; }
.console-actions { display: flex; gap: 8px; }
.console-logs { flex: 1; overflow-y: auto; padding: 10px; white-space: pre-wrap; word-break: break-all; font-size: 12px; line-height: 1.4; }
.btn-console { color: white; border: none; border-radius: 4px; padding: 4px 8px; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.btn-console-clear { background: #888; }
.btn-console-close { background: #ff5555; }
.btn-console-main { margin-left: 8px; padding: 4px 8px; background: #ff9800; color: #fff; border: none; border-radius: 4px; cursor: pointer; font-size: 0.8rem; font-weight: bold; display: flex; align-items: center; }
.btn-console-main-fixed { position: fixed; bottom: 4px; left: 8px; z-index: 9999; margin-left: 0; }
.icon-sm { font-size: 18px; }
.icon-xs { font-size: 16px; margin-right: 4px; }

/* Responsive Design */
@media (max-width: 600px) {
    .header { 
        flex-wrap: wrap; 
        justify-content: center; 
        gap: 5px; 
        padding: 5px;
    }
    .header strong { width: 100%; text-align: center; margin-bottom: 5px; }
    .header a, .header button { padding: 5px; }
    .content-area { padding: 10px; }
    .panel { right: 10px; left: 10px; width: auto; box-sizing: border-box; }
    .modal { width: 90%; }
}

```

-------- backend/frontend/html/css/omn-go-core.css END --------


-------- backend/frontend/html/js/omn-go-core.js START --------

```
if (typeof currentNote === 'undefined') {
    currentNote = (window.location.pathname.split('/').pop() || 'Welcome').replace(/\.html$/, '').replace(/\.md$/, '');
}

// Try to load console interceptor as early as possible
(function() {
            const originalLog = console.log;
            const originalError = console.error;
            const originalWarn = console.warn;
            const originalInfo = console.info;
	    const originalDebug = console.debug;
            const originalTrace = console.trace;
            const originalTable = console.table;
            const originalDir = console.dir;
            const originalTime = console.time;
            const originalTimeEnd = console.timeEnd;

            let logs = [];
            let consoleBtn = null;
            let consoleModal = null;
            let logsContainer = null;

            function initConsoleUI() {
                if (consoleBtn) return;

                consoleModal = document.createElement('div');
                consoleModal.id = 'omn-go-console-modal';
                consoleModal.className = 'console-modal';

                const header = document.createElement('div');
                header.className = 'console-header';
                header.innerHTML = '<span>JS Console Output</span><div class="console-actions"><button id="omn-go-console-clear" class="btn-console btn-console-clear" title="Clear Console"><i class="material-icons icon-sm">delete_sweep</i></button><button id="omn-go-console-close" class="btn-console btn-console-close" title="Close Console"><i class="material-icons icon-sm">close</i></button></div>';

                logsContainer = document.createElement('div');
                logsContainer.className = 'console-logs';

                consoleModal.appendChild(header);
                consoleModal.appendChild(logsContainer);
                document.body.appendChild(consoleModal);

                document.getElementById('omn-go-console-close').onclick = () => {
                    consoleModal.style.display = 'none';
                };
                let clrBtn = document.getElementById('omn-go-console-clear');
                if (clrBtn) {
                    clrBtn.onclick = () => {
                        logs = [];
                        if (logsContainer) logsContainer.innerHTML = '';
                        if (consoleBtn) consoleBtn.innerHTML = '<i class="material-icons icon-xs">terminal</i><span>0</span>';
                    };
                }

                consoleBtn = document.createElement('button');
                consoleBtn.id = 'omn-go-console-btn';
                consoleBtn.className = 'btn-console-main';
                consoleBtn.innerHTML = '<i class="material-icons icon-xs">terminal</i><span>0</span>';
                consoleBtn.onclick = () => {
                    consoleModal.style.display = 'flex';
                };

                let metadataEl = Array.from(document.querySelectorAll('*')).find(el => {
                    if (el.children.length > 0) return false;
                    const text = (el.textContent || '').toLowerCase();
                    const id = (el.id || '').toLowerCase();
                    const cls = (el.className || '').toLowerCase();
                    return text.includes('metadata') || id.includes('metadata') || cls.includes('metadata');
                });

                //if (metadataEl && metadataEl.parentNode) {
                //    metadataEl.parentNode.insertBefore(consoleBtn, metadataEl.nextSibling);
                //} else {
                //    consoleBtn.classList.add('btn-console-main-fixed');
                //    document.body.appendChild(consoleBtn);
                //}
                document.querySelector('.toolbar').appendChild(consoleBtn);
            }

            function appendLog(type, args) {
                logs.push({type, args});
                if (!document.body) {
                    window.addEventListener('DOMContentLoaded', () => appendLog(type, args));
                    return;
                }
                if (!consoleBtn) initConsoleUI();
                consoleBtn.innerHTML = `<i class="material-icons icon-xs">terminal</i><span>${logs.length}</span>`;

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
	    // Wrapper function creator
            function wrapConsole(methodName, originalMethod, level) {
                console[methodName] = function(...args) {
                    // Call original first (or after, depending on your needs)
                    try {
                        // Use .apply with the array directly
                        originalMethod.apply(console, args);
                    } catch (e) {
                        // Fallback if native apply fails
                        originalMethod(...args);
                    }
            
                    // Capture
                    appendLog(level, args);
               };
            }   

            // Override all major methods
            wrapConsole('log', originalLog, 'log');
            wrapConsole('error', originalError, 'error');
            wrapConsole('warn', originalWarn, 'warn');
            wrapConsole('info', originalInfo, 'info');
            wrapConsole('debug', originalDebug, 'debug');
            wrapConsole('trace', originalTrace, 'trace');
            wrapConsole('table', originalTable, 'table');
            wrapConsole('dir', originalDir, 'dir');
            wrapConsole('time', originalTime, 'time');
            wrapConsole('timeEnd', originalTimeEnd, 'timeEnd');
            window.addEventListener('error', function(e) {
                console.error('Uncaught Error:', e.message, 'at', e.filename, ':', e.lineno);
            });
})();

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

        // Intercept Markdown links for standard browser-side redirects
        document.getElementById('preview').addEventListener('click', (e) => {
            let target = e.target.closest('a');
            if(target) {
                const href = target.getAttribute('href');
                if (href) {
                    if (href.startsWith('http')) {
                        e.preventDefault();
                        window.open(href, '_blank');
                    } else if (!href.startsWith('javascript:') && !href.startsWith('#')) {
                        e.preventDefault();
                        let cleanHref = href;
                        if (cleanHref.endsWith('.md')) {
                            cleanHref = cleanHref.substring(0, cleanHref.length - 3) + '.html';
                        } else if (!cleanHref.includes('.')) {
                            cleanHref = cleanHref + '.html';
                        }
                        window.location.href = cleanHref;
                    }
                }
            }
        });

        async function loadNoteIntoEditor() {
            const res = await fetch('/api/getnote?name=' + encodeURIComponent(currentNote));
            if (res.ok) {
                document.getElementById('editor').value = await res.text();
            }
        }

        let currentMode = 'view';
        async function toggleMode() {
            if (currentMode === 'view') {
                if (typeof USE_INTERNAL_ED !== 'undefined' && !USE_INTERNAL_ED) {
                    window.location.replace('/api/edit-external?name=' + encodeURIComponent(currentNote));
                    return;
                }
                
                await loadNoteIntoEditor();
                
                const editor = document.getElementById('editor');
                const preview = document.getElementById('preview');
                const btn = document.getElementById('toggleBtn');
                
                editor.style.display = 'block';
                preview.style.display = 'none';
                btn.innerHTML = '<i class="material-icons" title="Switch to View Mode">visibility</i>';
                document.getElementById('saveBtn').style.display = 'block';
                document.getElementById('metaToggleBtn').style.display = 'none';
                document.getElementById('metadataPanel').classList.add('hidden');
                currentMode = 'edit';
            } else {
                const editor = document.getElementById('editor');
                const preview = document.getElementById('preview');
                const btn = document.getElementById('toggleBtn');
                
                editor.style.display = 'none';
                preview.style.display = 'block';
                btn.innerHTML = '<i class="material-icons" title="Switch to Edit Mode">edit</i>';
                document.getElementById('saveBtn').style.display = 'none';
                document.getElementById('metaToggleBtn').style.display = 'block';
                currentMode = 'view';
            }
        }

        // Global Drag & Drop for URLs (Bookmarks)
        document.body.addEventListener('dragover', e => {
            if (!e.target.closest('#editor')) e.preventDefault();
        });
        document.body.addEventListener('drop', e => {
            if (e.target.closest('#editor')) return;
            const url = e.dataTransfer.getData('text/uri-list') || e.dataTransfer.getData('text/plain');
            if (url && (url.startsWith('http://') || url.startsWith('https://'))) {
                e.preventDefault();
                document.getElementById('bmUrl').value = url;
                document.getElementById('bmTitle').value = '';
                const html = e.dataTransfer.getData('text/html');
                if (html) {
                    const match = html.match(/<a[^>]*>(.*?)<\/a>/i);
                    if (match && match[1]) {
                        document.getElementById('bmTitle').value = match[1].replace(/<[^>]+>/g, '').trim();
                    }
                }
                document.getElementById('bmPanel').classList.remove('hidden');
            }
        });

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

        function toCamelCase(str) {
            let words = str.split(/[-_\s]+/);
            return words.map(w => w ? w.charAt(0).toUpperCase() + w.slice(1) : '').join('');
        }

        async function createNewPage() {
            let title = prompt("Enter New Page Title:");
            if (!title) return;
            let camel = toCamelCase(title);
            let safeName = camel.replace(/[^a-zA-Z0-9-]/g, '-');
            let fileName = prompt("Confirm File Name:", safeName);
            if (!fileName) return;

            let src = typeof currentNote !== 'undefined' ? currentNote : 'Welcome';
            const fd = new URLSearchParams();
            fd.append('source', src);
            fd.append('target', fileName);
            fd.append('title', title);

            const res = await fetch('/api/newpage', { method: 'POST', body: fd });
            if (res.ok) {
                window.location.href = '/' + fileName + '.html?edit=true';
            } else {
                alert("Failed to create new page!");
            }
        }

        async function saveNote() {
            let content = document.getElementById('editor').value;
            const fd = new URLSearchParams();
            fd.append('name', currentNote);
            fd.append('content', content);
            const res = await fetch('/api/save', { method: 'POST', body: fd });
            if(res.ok) {
                alert('Note saved!');
                window.location.reload();
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
                window.location.reload();
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
                window.location.reload();
            }
        }

        async function checkSession() {
            // Unhide UI if role cookies exist
            if (document.cookie.includes('session_role=')) {
                document.getElementById('loginOverlay').style.display = 'none';
                document.getElementById('mainUI').style.display = 'flex';
                checkRole();
            } else {
                // Check if server is configured with public role or check backend
                const test = await fetch('/api/config');
                if (test.status === 401) {
                    document.getElementById('loginOverlay').style.display = 'flex';
                    document.getElementById('mainUI').style.display = 'none';
                } else {
                    document.getElementById('loginOverlay').style.display = 'none';
                    document.getElementById('mainUI').style.display = 'flex';
                }
            }
        }

        window.handleShare = function(text, subject) {
            text = text || '';
            subject = subject || '';
            
            // Regex to find the first valid URL
            const urlMatch = text.match(/(https?:\/\/[^\s]+)/) || subject.match(/(https?:\/\/[^\s]+)/);
            
            if (urlMatch) {
                // URL Found -> Route to Bookmark Panel
                const url = urlMatch[0];
                document.getElementById('bmUrl').value = url;
                
                let title = subject;
                if (!title || title.includes(url)) {
                    title = text.replace(url, '').trim();
                }
                if (!title) title = "Shared Link";
                
                document.getElementById('bmTitle').value = title;
                document.getElementById('bmPanel').classList.remove('hidden');
                document.getElementById('quickPanel').classList.add('hidden');
            } else {
                // No URL -> Route to Quick Note Panel
                let content = '';
                if (subject) content += subject + "\n\n";
                if (text) content += text;
                
                document.getElementById('quickText').value = content.trim();
                document.getElementById('quickPanel').classList.remove('hidden');
                document.getElementById('bmPanel').classList.add('hidden');
            }
        };

        window.onload = () => {
            checkSession();
            
            const params = new URLSearchParams(window.location.search);
            if (params.has('share_text') || params.has('share_subject')) {
                window.handleShare(params.get('share_text'), params.get('share_subject'));
                window.history.replaceState({}, document.title, window.location.pathname + window.location.hash);
            }
            if (window.hljs) {
                document.querySelectorAll('#preview pre code').forEach((block) => {
                    hljs.highlightElement(block);
                });
            }
            if (typeof OMN_GO_KATEX !== 'undefined' && OMN_GO_KATEX && window.renderMathInElement) {
                renderMathInElement(document.getElementById('preview') || document.body, {
                    delimiters: [
                        {left: '$$', right: '$$', display: true},
                        {left: '$', right: '$', display: false},
                        {left: '\\(', right: '\\)', display: false},
                        {left: '\\[', right: '\\]', display: true}
                    ],
                    throwOnError: false
                });
            }
            if (typeof currentNote !== 'undefined' && currentNote === 'Config') {
                const tb = document.getElementById('toggleBtn');
                if (tb) tb.style.display = 'none';
            }
            if (window.location.search.includes('edit=true')) {
                setTimeout(() => {
                    if (typeof currentMode !== 'undefined' && currentMode === 'view' && typeof toggleMode === 'function') toggleMode();
                }, 100);
            }
            let hash = window.location.hash;
            if (hash) {
                let el = document.getElementById(hash.substring(1));
                if (el) el.scrollIntoView();
            }
        };

document.addEventListener("DOMContentLoaded", () => {
            // Setup Auto-Rendering for KaTeX via MutationObserver
            const previewNode = document.getElementById('preview') || document.body;
            let renderTimeout;
            const observer = new MutationObserver(() => {
                clearTimeout(renderTimeout);
                renderTimeout = setTimeout(() => {
                    if (typeof OMN_GO_KATEX !== 'undefined' && OMN_GO_KATEX && window.renderMathInElement) {
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

document.addEventListener("DOMContentLoaded", () => {
            const footer = document.getElementById('omn-go-version-footer');
            let v = 'xx.xx.xx';
            try { if (APP_VERSION) v = APP_VERSION; } catch(e) {}
            if (footer) footer.innerText = 'OMN-Go v' + v;
        });


// --- Dynamic Metadata Panel Extractor ---
document.addEventListener("DOMContentLoaded", () => {
    const panel = document.getElementById('metadataPanel');
    if (panel) {
        let metaHtml = `<div style="margin-bottom: 8px; color: #0056b3; font-weight: bold; border-bottom: 1px solid #ccc; padding-bottom: 4px;">File: ${typeof PageName !== 'undefined' ? PageName + '.md' : ''}</div>`;
        document.querySelectorAll('meta').forEach(m => {
            const name = m.getAttribute('name');
            const content = m.getAttribute('content');
            if (name && content && !['viewport', 'charset'].includes(name.toLowerCase())) {
                metaHtml += `<div style="margin-bottom: 4px;"><strong>${name.charAt(0).toUpperCase() + name.slice(1)}:</strong> ${content}</div>`;
            }
        });
        panel.innerHTML = metaHtml;
    }
});

window.addEventListener('pageshow', function(event) {
    if (event.persisted) {
        window.location.reload();
    }
});

```

-------- backend/frontend/html/js/omn-go-core.js END --------


-------- android/build.gradle START --------

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

-------- android/build.gradle END --------


-------- android/settings.gradle START --------

```
rootProject.name = "OMN-Go"
include ':app'

```

-------- android/settings.gradle END --------


-------- android/app/build.gradle START --------

```
plugins {
    id 'com.android.application'
}

android {
    namespace 'net.basov.omngo'
    compileSdk 34

    defaultConfig {
        applicationId "net.basov.omngo"
        minSdk 24
        targetSdk 34
        versionCode 10325
        versionName "1.3.25"
    }

    signingConfigs {
        release {
            storeFile file('omn-go.keystore')
            storePassword 'omn-go123'
            keyAlias 'omn-go'
            keyPassword 'omn-go123'
        }
    }
    splits {
        abi {
            enable true
            reset()
            include "armeabi-v7a", "arm64-v8a", "x86", "x86_64"
            universalApk true // Set to false if you want ONLY the split APKs
        }
    }

    buildTypes {
        release {
            signingConfig signingConfigs.release
            minifyEnabled false
            proguardFiles getDefaultProguardFile('proguard-android-optimize.txt'), 'proguard-rules.pro'
        }
    }
    applicationVariants.all { variant ->
        variant.outputs.all { output ->
            outputFileName = "omn-go-v${variant.versionName}-${output.baseName}.apk"
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

// Suppress deprecated API usage notes during compilation (e.g. WebViewClient URL loading)
tasks.withType(JavaCompile).configureEach {
    options.compilerArgs += ["-Xlint:-deprecation"]
}

```

-------- android/app/build.gradle END --------


-------- android/app/src/main/java/net/basov/omngo/MainActivity.java START --------

```
package net.basov.omngo;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import net.basov.omngo.backend.Backend;
import android.os.Handler;
import android.os.Looper;

public class MainActivity extends Activity {
    private WebView webView;
    private String currentEditingName;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // Ensure Android OS mounts scoped storage directories for native C/Go access
        java.io.File[] mediaDirs = getExternalMediaDirs();
        if (mediaDirs != null && mediaDirs.length > 0 && mediaDirs[0] != null) {
            mediaDirs[0].mkdirs();
        }

        // Start the Go Backend Server from the gomobile .aar
        Backend.startServer();

        // Acquire partial wake lock to keep the Go server alive in the background
        try {
            android.os.PowerManager pm = (android.os.PowerManager) getSystemService(android.content.Context.POWER_SERVICE);
            android.os.PowerManager.WakeLock wl = pm.newWakeLock(android.os.PowerManager.PARTIAL_WAKE_LOCK, "OMNGo::ServerWakeLock");
            wl.acquire();
        } catch (Exception e) {
            e.printStackTrace();
        }
        // Create Native Loading Layout
        android.widget.FrameLayout rootLayout = new android.widget.FrameLayout(this);
        rootLayout.setBackgroundColor(android.graphics.Color.parseColor("#f9f9f9"));
        
        final android.widget.ProgressBar progressBar = new android.widget.ProgressBar(this);
        android.widget.FrameLayout.LayoutParams pbParams = new android.widget.FrameLayout.LayoutParams(
            android.view.ViewGroup.LayoutParams.WRAP_CONTENT,
            android.view.ViewGroup.LayoutParams.WRAP_CONTENT);
        pbParams.gravity = android.view.Gravity.CENTER;
        progressBar.setLayoutParams(pbParams);

        // Initialize WebView
        webView = new WebView(this);
        webView.setLayoutParams(new android.widget.FrameLayout.LayoutParams(
            android.view.ViewGroup.LayoutParams.MATCH_PARENT,
            android.view.ViewGroup.LayoutParams.MATCH_PARENT));

        WebSettings webSettings = webView.getSettings();
        webSettings.setJavaScriptEnabled(true);
        webSettings.setDomStorageEnabled(true);

        webView.setWebChromeClient(new android.webkit.WebChromeClient() {
            @Override
            public boolean onJsAlert(android.webkit.WebView view, String url, String message, android.webkit.JsResult result) {
                new android.app.AlertDialog.Builder(view.getContext())
                    .setMessage(message)
                    .setPositiveButton("OK", (d, w) -> result.confirm())
                    .setOnCancelListener(d -> result.cancel())
                    .show();
                return true;
            }

            @Override
            public boolean onJsConfirm(android.webkit.WebView view, String url, String message, android.webkit.JsResult result) {
                new android.app.AlertDialog.Builder(view.getContext())
                    .setMessage(message)
                    .setPositiveButton("OK", (d, w) -> result.confirm())
                    .setNegativeButton("Cancel", (d, w) -> result.cancel())
                    .setOnCancelListener(d -> result.cancel())
                    .show();
                return true;
            }

            @Override
            public boolean onJsPrompt(android.webkit.WebView view, String url, String message, String defaultValue, android.webkit.JsPromptResult result) {
                android.widget.EditText input = new android.widget.EditText(view.getContext());
                input.setText(defaultValue);
                new android.app.AlertDialog.Builder(view.getContext())
                    .setMessage(message)
                    .setView(input)
                    .setPositiveButton("OK", (d, w) -> result.confirm(input.getText().toString()))
                    .setNegativeButton("Cancel", (d, w) -> result.cancel())
                    .setOnCancelListener(d -> result.cancel())
                    .show();
                return true;
            }
        });

        webView.setWebViewClient(new WebViewClient() {
            @Override
            public void onPageFinished(WebView view, String url) {
                progressBar.setVisibility(android.view.View.GONE);
                super.onPageFinished(view, url);
            }
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                if (url != null && url.startsWith("omngo://edit")) {
                    try {
                        String name = url.substring(url.indexOf("?name=") + 6);
                        if (name.contains("&")) {
                            name = name.split("&")[0];
                        }
                        name = android.net.Uri.decode(name);
                        currentEditingName = name;
                        
                        // Disable strict mode exposed file exceptions
                        android.os.StrictMode.VmPolicy.Builder builder = new android.os.StrictMode.VmPolicy.Builder();
                        android.os.StrictMode.setVmPolicy(builder.build());

                        java.io.File file = new java.io.File("/storage/emulated/0/Android/media/net.basov.omngo/md/" + name + ".md");
                        if (!file.exists()) {
                            file.getParentFile().mkdirs();
                            file.createNewFile();
                        }

                        android.content.Intent intent = new android.content.Intent(android.content.Intent.ACTION_EDIT);
                        intent.setDataAndType(android.net.Uri.fromFile(file), "text/plain");
                        intent.addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION | android.content.Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
                        
                        MainActivity.this.startActivityForResult(android.content.Intent.createChooser(intent, "Edit Markdown File"), 1001);
                    } catch (Exception e) {
                        e.printStackTrace();
                    }
                    return true;
                }

                if (url != null && (url.startsWith("http://") || url.startsWith("https://"))) {
                    if (!url.contains("localhost") && !url.contains("127.0.0.1")) {
                        view.getContext().startActivity(
                            new android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url))
                        );
                        return true;
                    }
                }
                return false;
            }
        });
        rootLayout.addView(webView);
        rootLayout.addView(progressBar);
        setContentView(rootLayout);

        // Wait for the Go server to bind before loading
        new Handler(Looper.getMainLooper()).postDelayed(new Runnable() {
            @Override
            public void run() {
                String startUrl = "http://127.0.0.1:8080/Welcome.html";
                android.content.Intent intent = getIntent();
                if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())) {
                    String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
                    String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
                    startUrl += "?share_text=" + (sharedText != null ? android.net.Uri.encode(sharedText) : "") + 
                                "&share_subject=" + (sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "");
                }
                webView.loadUrl(startUrl);
            }
        }, 1000); // 1 second delay
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, android.content.Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == 1001 && webView != null) {
            if (currentEditingName != null && !currentEditingName.isEmpty()) {
                webView.loadUrl("http://127.0.0.1:8080/" + android.net.Uri.encode(currentEditingName) + ".html");
                currentEditingName = null;
            } else {
                webView.reload(); // Refresh view when returning from external editor
            }
        }
    }

    @Override
    protected void onNewIntent(android.content.Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())) {
            String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
            String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
            if (webView != null) {
                String tText = sharedText != null ? android.net.Uri.encode(sharedText) : "";
                String tSubj = sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "";
                String js = "javascript:(function(){ if(window.handleShare) window.handleShare(decodeURIComponent('" + tText + "'), decodeURIComponent('" + tSubj + "')); })();";
                webView.evaluateJavascript(js, null);
            }
        }
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

-------- android/app/src/main/java/net/basov/omngo/MainActivity.java END --------


-------- android/app/src/main/AndroidManifest.xml START --------

```
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" >
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.WAKE_LOCK" />
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" />
    <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" />
    
    <application
        android:icon="@mipmap/ic_launcher"
        android:roundIcon="@mipmap/ic_launcher_round"
        android:allowBackup="true"
        android:label="OMN-Go"
        android:usesCleartextTraffic="true"
        android:hardwareAccelerated="true"
        android:theme="@android:style/Theme.NoTitleBar.Fullscreen">
        <activity
            android:name=".MainActivity"
            android:exported="true"
            android:launchMode="singleTask"
            android:configChanges="orientation|keyboardHidden|screenSize">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
                    <intent-filter>
                <action android:name="android.intent.action.SEND" />
                <category android:name="android.intent.category.DEFAULT" />
                <data android:mimeType="text/plain" />
            </intent-filter>
        </activity>
    </application>
</manifest>

```

-------- android/app/src/main/AndroidManifest.xml END --------


### FULL PR0JECT DIRECTORY TREE START

.
├── android
│   ├── app
│   │   ├── build.gradle
│   │   ├── omn-go.keystore
│   │   └── src
│   │       ├── fdroid
│   │       │   └── res
│   │       │       └── drawable
│   │       │           └── ic_launcher_foreground.xml
│   │       └── main
│   │           ├── AndroidManifest.xml
│   │           ├── java
│   │           │   └── net
│   │           │       └── basov
│   │           │           └── omngo
│   │           │               └── MainActivity.java
│   │           └── res
│   │               ├── drawable
│   │               │   ├── ic_launcher_background.xml
│   │               │   └── ic_launcher_foreground.xml
│   │               └── mipmap-anydpi-v26
│   │                   ├── ic_launcher_round.xml
│   │                   └── ic_launcher.xml
│   ├── build.gradle
│   └── settings.gradle
├── backend
│   ├── config.go
│   ├── frontend
│   │   ├── html
│   │   │   ├── css
│   │   │   │   ├── Bookmarker.css
│   │   │   │   ├── fonts
│   │   │   │   │   ├── DSEG14Modern-Italic.woff
│   │   │   │   │   ├── DSEG7Modern-BoldItalic.woff
│   │   │   │   │   ├── DSEG7Modern-Italic.woff
│   │   │   │   │   ├── KaTeX_AMS-Regular.woff2
│   │   │   │   │   ├── KaTeX_Caligraphic-Bold.woff2
│   │   │   │   │   ├── KaTeX_Caligraphic-Regular.woff2
│   │   │   │   │   ├── KaTeX_Fraktur-Bold.woff2
│   │   │   │   │   ├── KaTeX_Fraktur-Regular.woff2
│   │   │   │   │   ├── KaTeX_Main-BoldItalic.woff2
│   │   │   │   │   ├── KaTeX_Main-Bold.woff2
│   │   │   │   │   ├── KaTeX_Main-Italic.woff2
│   │   │   │   │   ├── KaTeX_Main-Regular.woff2
│   │   │   │   │   ├── KaTeX_Math-BoldItalic.woff2
│   │   │   │   │   ├── KaTeX_Math-Italic.woff2
│   │   │   │   │   ├── KaTeX_SansSerif-Bold.woff2
│   │   │   │   │   ├── KaTeX_SansSerif-Italic.woff2
│   │   │   │   │   ├── KaTeX_SansSerif-Regular.woff2
│   │   │   │   │   ├── KaTeX_Script-Regular.woff2
│   │   │   │   │   ├── KaTeX_Size1-Regular.woff2
│   │   │   │   │   ├── KaTeX_Size2-Regular.woff2
│   │   │   │   │   ├── KaTeX_Size3-Regular.woff2
│   │   │   │   │   ├── KaTeX_Size4-Regular.woff2
│   │   │   │   │   ├── KaTeX_Typewriter-Regular.woff2
│   │   │   │   │   └── material-icons.woff2
│   │   │   │   ├── highlight.default.min.css
│   │   │   │   ├── katex.min.css
│   │   │   │   ├── markdown.css
│   │   │   │   └── omn-go-core.css
│   │   │   ├── js
│   │   │   │   ├── auto-render.min.js
│   │   │   │   ├── Bookmarker.js
│   │   │   │   ├── highlight.min.js
│   │   │   │   ├── katex.min.js
│   │   │   │   └── omn-go-core.js
│   │   │   └── json
│   │   │       └── test.json
│   │   ├── index.html
│   │   └── md
│   │       ├── Bookmarks.md
│   │       ├── QuickNotes.md
│   │       ├── ScriptRules.md
│   │       ├── Test
│   │       │   └── OMN-Go
│   │       │       ├── Console.md
│   │       │       ├── Fetch.md
│   │       │       └── OMN-Go.md
│   │       └── Welcome.md
│   ├── handlers.go
│   ├── markdown.go
│   ├── middleware.go
│   ├── server.go
│   ├── storage.go
│   └── version.go
├── doc
│   ├── github_workflow.md
│   ├── initial_prompt.md
│   ├── OMN-Go_1.3.25_Context.md
│   ├── README.md
│   └── URLs.md
├── Dockerfile
├── go.mod
├── local
│   ├── build.sh
│   ├── force_build.sh
│   ├── generate_context_script.bash
│   ├── icons
│   │   ├── android_adaptive_vector_studio_v9_7.html
│   │   ├── android_smart_grid.svg
│   │   ├── android_vector_xml_previewer.html
│   │   ├── clean_ic_background.svg
│   │   ├── clean_ic_foreground_fdroid.svg
│   │   ├── clean_ic_foreground.svg
│   │   ├── omn_ic_adaptive.svg
│   │   └── res-drawable.zip
│   ├── initial
│   │   ├── offline_asset_downloader.sh
│   │   └── project_setup_script.py
│   └── update_script.py
├── main_desktop.go
├── output-binaries
│   ├── omn-go-v1.3.25-arm64-v8a-release.apk
│   ├── omn-go-v1.3.25-armeabi-v7a-release.apk
│   ├── omn-go-v1.3.25-desktop-linux-amd64
│   ├── omn-go-v1.3.25-desktop-windows-amd64.exe
│   ├── omn-go-v1.3.25-universal-release.apk
│   ├── omn-go-v1.3.25-x86_64-release.apk
│   └── omn-go-v1.3.25-x86-release.apk
└── README.md

29 directories, 91 files

### FULL PR0JECT DIRECTORY TREE END

