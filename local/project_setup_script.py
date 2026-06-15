import os

def create_file(path, content):
    """Safely creates directories and writes content to the target file."""
    os.makedirs(os.path.dirname(path) if os.path.dirname(path) else '.', exist_ok=True)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content.strip() + "\n")
    print(f"Created: {path}")

def generate_project():
    print("Initializing GoOMN Baseline...")

    # ==========================================
    # 1. GO MODULE
    # ==========================================
    go_mod = """
module net.basov.goomn

go 1.22

require golang.org/x/mobile v0.0.0-20231127183840-76ac68780225
    """
    create_file("go.mod", go_mod)

    # ==========================================
    # 2. SERVER LOGIC (Cross-Platform)
    # ==========================================
    server_go = r"""
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const APP_VERSION = "1.0.0"

type Config struct {
	ServerPort    int    `json:"server_port"`
	AdminPassword string `json:"admin_password"`
	GuestPassword string `json:"guest_password"`
}

var (
	storageDir  string
	appConfig   Config
	activeConns int
)

func initStorage() {
	if runtime.GOOS == "android" {
		storageDir = "/storage/emulated/0/Media/net.basov.goomn"
	} else {
		storageDir = "./data"
	}

	// 1. Create Isolated Storage
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage: %v", err)
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
	welcomePath := filepath.Join(storageDir, "Welcome.md")
	if _, err := os.Stat(welcomePath); os.IsNotExist(err) {
		welcomeContent := "Title: Welcome\nDate: 2026-06-14 12:00:00\nCategory: System\n\nWelcome to GoOMN. Start editing!"
		os.WriteFile(welcomePath, []byte(welcomeContent), 0644)
	}

	quickPath := filepath.Join(storageDir, "QuickNotes.md")
	if _, err := os.Stat(quickPath); os.IsNotExist(err) {
		quickContent := "Title: Quick Notes\nDate: 2026-06-14 12:00:00\nCategory: Log\n\n"
		os.WriteFile(quickPath, []byte(quickContent), 0644)
	}

	bmPath := filepath.Join(storageDir, "Bookmarks.html")
	if _, err := os.Stat(bmPath); os.IsNotExist(err) {
		bmContent := `<script>bookmarks = [
<!-- Don't edit body below this line -->
];</script>`
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
	path := filepath.Join(storageDir, "QuickNotes.md")
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
	entry := fmt.Sprintf("\n---\n#### %s\n%s\n", timestamp, note)
	
	newContent := append(lines[:insertIdx], append([]string{entry}, lines[insertIdx:]...)...)
	os.WriteFile(path, []byte(strings.Join(newContent, "\n")), 0644)
	w.Write([]byte("Saved"))
}

func handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")
	
	path := filepath.Join(storageDir, "Bookmarks.html")
	data, _ := os.ReadFile(path)
	
	marker := "<script>bookmarks = [\n<!-- Don't edit body below this line -->"
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	tagsArr := strings.Split(tags, ",")
	tagsJSON, _ := json.Marshal(tagsArr)
	
	newEntry := fmt.Sprintf(`    {
      "date": "%s",
      "url": "%s",
      "title": "%s",
      "tags": %s,
      "notes": ["%s"]
    },`, timestamp, url, title, string(tagsJSON), notes)
	
	replaced := strings.Replace(string(data), marker, marker+"\n"+newEntry, 1)
	os.WriteFile(path, []byte(replaced), 0644)
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

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "frontend/index.html")
}

func runServer() {
	initStorage()
	
	mux := http.NewServeMux()
	mux.HandleFunc("/", serveFrontend)
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/api/quick", authMiddleware(handleQuickNote, true))
	mux.HandleFunc("/api/bookmark", authMiddleware(handleBookmark, true))
	mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
	
	port := fmt.Sprintf(":%d", appConfig.ServerPort)
	log.Printf("GoOMN Backend running on %s", port)
	http.ListenAndServe(port, connectionMiddleware(mux))
}
    """
    create_file("server.go", server_go)

    # ==========================================
    # 3. DESKTOP ENTRY (!android)
    # ==========================================
    main_desktop_go = r"""
//go:build !android

package main

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"
)

func main() {
	go runServer()
	
	// Wait for server to bind
	time.Sleep(500 * time.Millisecond)
	url := fmt.Sprintf("http://localhost:%d", appConfig.ServerPort)
	
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
    """
    create_file("main_desktop.go", main_desktop_go)

    # ==========================================
    # 4. ANDROID ENTRY (android)
    # ==========================================
    main_android_go = r"""
//go:build android

package main

import (
	"fmt"
	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/gl"
)

func main() {
	go runServer()

	// High-performance canvas avoiding AppCompat.
	// Render a color block to represent Server Status due to 5MB size constraints
	// and to avoid heavyweight FreeType/font libraries inside the GL loop.
	app.Main(func(a app.App) {
		var glctx gl.Context
		for e := range a.Events() {
			switch e := a.Filter(e).(type) {
			case lifecycle.Event:
				if e.Crosses(lifecycle.StageVisible) == lifecycle.CrossOn {
					glctx, _ = e.DrawContext.(gl.Context)
					a.Send(paint.Event{})
				} else if e.Crosses(lifecycle.StageVisible) == lifecycle.CrossOff {
					glctx = nil
				}
			case paint.Event:
				if glctx == nil || e.External {
					continue
				}
				
				// Print to logcat so server IP is visible to debugging tools
				fmt.Printf("GoOMN Active: Port %d, Connections: %d\n", appConfig.ServerPort, activeConns)
				
				// Clear to a Green/Dark theme to signify active state
				glctx.ClearColor(0.0, 0.2, 0.1, 1.0)
				glctx.Clear(gl.COLOR_BUFFER_BIT)
				a.Publish()
			}
		}
	})
}
    """
    create_file("main_android.go", main_android_go)

    # ==========================================
    # 5. FRONTEND (HTML/JS/Tailwind)
    # ==========================================
    frontend_html = r"""
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GoOMN Editor</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
    <script>const APP_VERSION = "1.0.0";</script>
</head>
<body class="bg-gray-50 text-gray-900 h-screen flex flex-col font-sans">
    
    <!-- Login Overlay -->
    <div id="loginOverlay" class="fixed inset-0 bg-gray-900 bg-opacity-75 flex items-center justify-center z-50">
        <div class="bg-white p-8 rounded shadow-lg w-96">
            <h2 class="text-2xl font-bold mb-4">GoOMN Login</h2>
            <input type="password" id="pwdInput" class="w-full border p-2 mb-4 rounded" placeholder="Admin or Guest Password">
            <button onclick="login()" class="w-full bg-blue-600 text-white p-2 rounded">Enter</button>
        </div>
    </div>

    <!-- Main UI -->
    <div class="flex flex-1 overflow-hidden" id="mainUI" style="display:none;">
        <div class="w-64 bg-gray-800 text-white p-4 flex flex-col">
            <h1 class="text-xl font-bold mb-6">GoOMN <span class="text-xs text-gray-400">v1.0.0</span></h1>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="bg-indigo-600 p-2 rounded mb-2 w-full admin-only">Quick Note</button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="bg-teal-600 p-2 rounded mb-4 w-full admin-only">Add Bookmark</button>
            <div class="flex-1 overflow-y-auto">
                <p class="text-gray-400 text-sm">Notes list placeholder...</p>
            </div>
        </div>

        <div class="flex-1 p-6 relative">
            <div class="flex h-full gap-4">
                <textarea id="editor" class="w-1/2 h-full border rounded p-4 font-mono admin-only" placeholder="Markdown content... Drag images here to upload."></textarea>
                <div id="preview" class="w-1/2 h-full border rounded p-4 bg-white overflow-y-auto prose max-w-none"></div>
            </div>
        </div>
    </div>

    <!-- Quick Note Modal -->
    <div id="quickPanel" class="hidden absolute top-20 left-1/3 w-1/3 bg-white border shadow-xl p-4 rounded z-40">
        <h3 class="font-bold mb-2">Quick Note</h3>
        <textarea id="quickText" class="w-full h-32 border p-2 mb-2"></textarea>
        <button onclick="submitQuickNote()" class="bg-blue-600 text-white px-4 py-2 rounded">Append to QuickNotes.md</button>
    </div>

    <!-- Bookmark Modal -->
    <div id="bmPanel" class="hidden absolute top-20 right-10 w-1/3 bg-white border shadow-xl p-4 rounded z-40">
        <h3 class="font-bold mb-2">Ingest Bookmark</h3>
        <input id="bmUrl" class="w-full border p-2 mb-2" placeholder="URL">
        <input id="bmTitle" class="w-full border p-2 mb-2" placeholder="Title">
        <input id="bmTags" class="w-full border p-2 mb-2" placeholder="Tags (comma separated)">
        <textarea id="bmNotes" class="w-full border p-2 mb-2 h-16" placeholder="Notes"></textarea>
        <button onclick="submitBookmark()" class="bg-blue-600 text-white px-4 py-2 rounded">Inject to Bookmarks.html</button>
    </div>

    <script>
        // Init preview updates
        document.getElementById('editor').addEventListener('input', (e) => {
            document.getElementById('preview').innerHTML = marked.parse(e.target.value);
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
                document.querySelectorAll('.admin-only').forEach(el => el.disabled = true);
                editor.readOnly = true;
                editor.placeholder = "Read-only mode (Guest)";
            }
        }

        async function submitQuickNote() {
            const fd = new URLSearchParams();
            fd.append('note', document.getElementById('quickText').value);
            const res = await fetch('/api/quick', { method: 'POST', body: fd });
            if(res.ok) {
                document.getElementById('quickText').value = '';
                document.getElementById('quickPanel').classList.add('hidden');
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
            }
        }
    </script>
</body>
</html>
    """
    create_file("frontend/index.html", frontend_html)

    # ==========================================
    # 6. DOCKERFILE (Multi-Stage Linux/Android)
    # ==========================================
    dockerfile = r"""
# STAGE 1: Toolchains & Cache
FROM golang:1.22-bookworm AS builder
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
    sdkmanager "platforms;android-33" "build-tools;33.0.2" "ndk;25.2.9519653"

# Install GoMobile
RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init

# STAGE 2: Dependency Lock
WORKDIR /app
COPY go.mod ./
RUN go mod tidy && go mod download

# STAGE 3: Build & Pack
COPY . .

# Desktop Binary (Linux example)
RUN GOOS=linux GOARCH=amd64 go build -o bin/goomn-desktop server.go main_desktop.go

# Android APK (Under 5MB, No AppCompat)
RUN gomobile build -target=android -androidapi 21 -javapkg net.basov.goomn.fdroid -o bin/goomn.apk server.go main_android.go
    """
    create_file("Dockerfile", dockerfile)

    print("\n--- Project generation complete! ---")
    print("Refer to README.md for build instructions.")

if __name__ == "__main__":
    generate_project()