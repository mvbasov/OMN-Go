import os
import re

def create_file(filepath, content):
    print(f"\n[CREATE] {filepath}")
    os.makedirs(os.path.dirname(filepath), exist_ok=True)
    with open(filepath, "w", encoding="utf-8") as f:
        f.write(content)
    print("  [+] SUCCESS: File created/overwritten.")

def bump_versions():
    print("\n[VERSION BUMP] Upgrading to 1.4.0")
    
    html_path = "backend/frontend/index.html"
    if os.path.exists(html_path):
        with open(html_path, "r", encoding="utf-8") as f:
            content = f.read()
        content = re.sub(r'const APP_VERSION = "1\.3\.\d+";', 'const APP_VERSION = "1.4.0";', content)
        with open(html_path, "w", encoding="utf-8") as f:
            f.write(content)
        print(f"  [+] Bumped version in {html_path}")

    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r", encoding="utf-8") as f:
            content = f.read()
        content = re.sub(r'versionCode\s+\d+', 'versionCode 10400', content)
        content = re.sub(r'versionName\s+"1\.3\.\d+"', 'versionName "1.4.0"', content)
        with open(gradle_path, "w", encoding="utf-8") as f:
            f.write(content)
        print(f"  [+] Bumped version in {gradle_path}")

def modularize_backend():
    print("==================================================")
    print(" OMN-Go Architecture Overhaul (Target: V1.4.0)")
    print("==================================================")
    
    bump_versions()

    # --- 1. config.go ---
    config_go = r"""package backend

const APP_VERSION = "1.4.0"

type Config struct {
	ServerPort    int               `json:"server_port"`
	AdminPassword string            `json:"admin_password"`
	GuestPassword string            `json:"guest_password"`
	Author        string            `json:"author"`
	UseInternalEd bool              `json:"use_internal_editor"`
	DesktopExtCmd string            `json:"desktop_ext_cmd"`
	MimeTypes     map[string]string `json:"mime_types"`
}

var (
	storageDir  string
	appConfig   Config
	activeConns int
)
"""

    # --- 2. os_android.go ---
    os_android_go = r"""//go:build android

package backend

// GetStorageDir returns the isolated media storage directory for Android.
func GetStorageDir() string {
	return "/storage/emulated/0/Android/media/net.basov.omngo"
}

// OpenExternalEditor triggers an Android intent to open the markdown file.
func OpenExternalEditor(path string) error {
	// Android WebView wrapper intercepts omngo:// URLs natively,
	// so the Go backend doesn't need to execute anything here.
	return nil
}
"""

    # --- 3. os_desktop.go ---
    os_desktop_go = r"""//go:build !android

package backend

import (
	"log"
	"os/exec"
	"runtime"
)

// GetStorageDir returns the local data directory for Desktop environments.
func GetStorageDir() string {
	return "./data"
}

// OpenExternalEditor opens the markdown file using the OS's default editor or a custom command.
func OpenExternalEditor(path string) error {
	if appConfig.DesktopExtCmd != "" {
		log.Printf("Opening %s with custom command: %s", path, appConfig.DesktopExtCmd)
		return exec.Command(appConfig.DesktopExtCmd, path).Start()
	}

	log.Println("Opening with default system editor:", path)
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", path).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	case "darwin":
		err = exec.Command("open", path).Start()
	default:
		log.Println("Unsupported desktop OS for external editor.")
	}
	return err
}
"""

    # --- 4. embed.go ---
    embed_go = r"""package backend

import (
	"embed"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/html frontend/md
var staticFS embed.FS

// ExtractDefaultMarkdown ensures the welcome and initial markdown files exist.
func ExtractDefaultMarkdown() {
	entries, _ := staticFS.ReadDir("frontend/md")
	for _, e := range entries {
		target := filepath.Join(storageDir, "md", e.Name())
		if _, err := os.Stat(target); os.IsNotExist(err) {
			data, _ := staticFS.ReadFile("frontend/md/" + e.Name())
			os.MkdirAll(filepath.Dir(target), 0755)
			os.WriteFile(target, data, 0644)
		}
	}
}

// serveLazyEmbed extracts and serves static HTML/JS/CSS assets lazily.
func serveLazyEmbed(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	path = strings.TrimPrefix(path, "/")
	
	targetPath := filepath.Join(storageDir, "html", path)
	
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		data, err := staticFS.ReadFile("frontend/html/" + path)
		if err == nil {
			os.MkdirAll(filepath.Dir(targetPath), 0755)
			os.WriteFile(targetPath, data, 0644)
		}
	}

	ext := filepath.Ext(targetPath)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		switch ext {
		case ".css": mimeType = "text/css"
		case ".js": mimeType = "application/javascript"
		case ".html": mimeType = "text/html"
		default: mimeType = "text/plain"
		}
	}
	w.Header().Set("Content-Type", mimeType)

	if file, err := os.Open(targetPath); err == nil {
		defer file.Close()
		io.Copy(w, file)
	} else {
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
"""

    # --- 5. markdown.go ---
    markdown_go = r"""package backend

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

func compilePage(title string, mdContent []byte) []byte {
	content := string(mdContent)
	parts := strings.SplitN(content, "\n\n", 2)
	body := content
	if len(parts) > 1 && strings.Contains(parts[0], ":") {
		firstLine := strings.Split(parts[0], "\n")[0]
		if strings.Contains(firstLine, ":") && !strings.HasPrefix(firstLine, " ") && !strings.HasPrefix(firstLine, "#") && !strings.HasPrefix(firstLine, "<") {
			body = parts[1]
		}
	}

	re := regexp.MustCompile(`(?s)\$\$.*?\$\$`)
	body = re.ReplaceAllStringFunc(body, func(m string) string {
		return strings.ReplaceAll(m, "_", "\\_")
	})

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Typographer),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithHardWraps(), html.WithUnsafe()),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(body), &buf); err != nil {
		return []byte(err.Error())
	}

	htmlStr := string(frontendHTML)
	htmlStr = strings.Replace(htmlStr, "{{TITLE}}", title, 1)
	htmlStr = strings.Replace(htmlStr, "{{CONTENT}}", buf.String(), 1)
	return []byte(htmlStr)
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
"""

    # --- 6. handlers_api.go ---
    handlers_api_go = r"""package backend

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func handleQuickNote(w http.ResponseWriter, r *http.Request) {
	entry := r.FormValue("entry")
	if entry == "" {
		return
	}
	path := filepath.Join(storageDir, "md", "QuickNotes.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(content), "\n")
	var newContent []string
	now := time.Now().Format("2006-01-02 15:04:05")
	inserted := false
	for _, l := range lines {
		if strings.HasPrefix(l, "## ") && !inserted {
			newContent = append(newContent, fmt.Sprintf("## %s", now))
			newContent = append(newContent, entry)
			newContent = append(newContent, "")
			inserted = true
		}
		newContent = append(newContent, l)
	}
	fullMarkdown := strings.Join(newContent, "\n")
	fullMarkdown = ensureHeaderModified(fullMarkdown, "Quick Notes")
	os.WriteFile(path, []byte(fullMarkdown), 0644)
	http.Redirect(w, r, "/QuickNotes.html", http.StatusSeeOther)
}

func handleBookmark(w http.ResponseWriter, r *http.Request) {
	entry := r.FormValue("entry")
	if entry == "" {
		return
	}
	path := filepath.Join(storageDir, "md", "Bookmarks.md")
	data, err := os.ReadFile(path)
	if err == nil {
		content := string(data)
		marker := "<!-- BOOKMARKS_START -->"
		if strings.Contains(content, marker) {
			newContent := strings.Replace(content, marker, marker+"\n"+entry, 1)
			newContent = ensureHeaderModified(newContent, "Incoming bookmarks")
			os.WriteFile(path, []byte(newContent), 0644)
		}
	}
	http.Redirect(w, r, "/Bookmarks.html", http.StatusSeeOther)
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()
	os.MkdirAll(filepath.Join(storageDir, "images"), 0755)
	dest, _ := os.Create(filepath.Join(storageDir, "images", header.Filename))
	defer dest.Close()
	io.Copy(dest, file)
	w.Write([]byte("/images/" + header.Filename))
}

func handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()
	os.MkdirAll(filepath.Join(storageDir, "user_json"), 0755)
	dest, _ := os.Create(filepath.Join(storageDir, "user_json", header.Filename))
	defer dest.Close()
	io.Copy(dest, file)
	w.Write([]byte("/user_json/" + header.Filename))
}

func handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		return
	}
	if name == "Config" {
		w.Write([]byte("Configuration is handled in the UI."))
		return
	}
	path := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))
	content, err := os.ReadFile(path)
	if err != nil {
		humanTitle := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		authorLine := ""
		if appConfig.Author != "" {
			authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
		}
		defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n", humanTitle, timestamp, authorLine)
		w.Write([]byte(defaultContent))
		return
	}
	w.Write(content)
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

		humanTitle := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSuffix(cleanName, ".md"), "-", " "), "_", " ")
		content = ensureHeaderModified(content, humanTitle)

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
			
			// Recompile Source HTML immediately
			htmlPath := filepath.Join(storageDir, "html", source+".html")
			compiled := compilePage(source, []byte(content))
			os.MkdirAll(filepath.Dir(htmlPath), 0755)
			os.WriteFile(htmlPath, compiled, 0644)
		}
	}

	w.Write([]byte("Created"))
}

func handleEditExternal(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		return
	}
	path := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))

	err := OpenExternalEditor(path)
	if err != nil {
		// Log silently or handle if necessary
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`
		<html>
		<head>
			<meta name="viewport" content="width=device-width, initial-scale=1.0">
			<style>
				body { font-family: sans-serif; display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; background: #f4f4f4; margin: 0; }
				button { padding: 15px 30px; font-size: 18px; background: #007bff; color: white; border: none; border-radius: 8px; cursor: pointer; }
			</style>
		</head>
		<body>
			<h2>Waiting for External Editor...</h2>
			<p>When you are finished saving, click below to return.</p>
			<button onclick="window.location.replace('/%s.html')">Return to OMN-Go</button>
		</body>
		</html>
	`, name)))
}
"""

    # --- 7. handlers_web.go ---
    handlers_web_go = r"""package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		pwd := r.FormValue("password")
		if pwd == appConfig.AdminPassword {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "admin", Path: "/", MaxAge: 86400 * 30})
		} else if pwd == appConfig.GuestPassword {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "guest", Path: "/", MaxAge: 86400 * 30})
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Write([]byte(`
		<html>
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<body style="font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; background: #f4f4f4; margin: 0;">
			<form method="POST" style="background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1);">
				<h2 style="margin-top: 0;">OMN-Go Login</h2>
				<input type="password" name="password" placeholder="Password" style="width: 100%; padding: 10px; margin-bottom: 15px; border: 1px solid #ccc; border-radius: 4px;" autofocus>
				<button type="submit" style="width: 100%; padding: 10px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer;">Login</button>
			</form>
		</body>
		</html>
	`))
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		appConfig.ServerPort = 8080 // Hardcode or parse if editable
		appConfig.AdminPassword = r.FormValue("admin_password")
		appConfig.GuestPassword = r.FormValue("guest_password")
		appConfig.Author = r.FormValue("author")
		appConfig.UseInternalEd = r.FormValue("use_internal_editor") == "true"
		
		cfgPath := filepath.Join(storageDir, "config.json")
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(cfgPath, data, 0644)
		w.Write([]byte("Config saved!"))
		return
	}
}

func getConfigPageBody() []byte {
	htmlStr := string(frontendHTML)
	htmlStr = strings.Replace(htmlStr, "{{TITLE}}", "Config", 1)

	cfgUI := fmt.Sprintf(`
		<div style="max-width: 600px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1);">
			<h2>System Configuration</h2>
			
			<div style="margin-bottom: 20px;">
				<label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Server Port</label>
				<input type="number" id="cfgPort" value="%d" disabled style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box; background: #eee;" />
			</div>
			
			<div style="margin-bottom: 20px;">
				<label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Admin Password</label>
				<input type="text" id="cfgAdminPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
			</div>
			
			<div style="margin-bottom: 20px;">
				<label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Guest Password</label>
				<input type="text" id="cfgGuestPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
			</div>

			<div style="margin-bottom: 20px;">
				<label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Author Name</label>
				<input type="text" id="cfgAuthor" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
			</div>
			
			<div style="margin-bottom: 20px; display: flex; align-items: center; gap: 10px;">
				<input type="checkbox" id="cfgUseInternal" %s style="width: 20px; height: 20px; cursor: pointer;" />
				<label style="font-weight: 600; color: #444; cursor: pointer;" for="cfgUseInternal">Use Internal Web Editor</label>
			</div>

			<button onclick="saveConfig()" style="width: 100%%; padding: 12px; background: #28a745; color: white; border: none; border-radius: 4px; font-size: 16px; cursor: pointer; font-weight: 600;">Save Configuration</button>
		</div>

		<script>
		async function saveConfig() {
			const params = new URLSearchParams();
			params.append("admin_password", document.getElementById("cfgAdminPwd").value);
			params.append("guest_password", document.getElementById("cfgGuestPwd").value);
			params.append("author", document.getElementById("cfgAuthor").value);
			params.append("use_internal_editor", document.getElementById("cfgUseInternal").checked ? "true" : "false");
			
			const res = await fetch('/api/config', { method: 'POST', body: params });
			if (res.ok) {
				alert('Configuration Saved!');
				window.location.reload();
			} else {
				alert('Failed to save configuration.');
			}
		}
		</script>
	`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author,
		func() string {
			if appConfig.UseInternalEd {
				return "checked"
			}
			return ""
		}())

	htmlStr = strings.Replace(htmlStr, "{{CONTENT}}", cfgUI, 1)
	return []byte(htmlStr)
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/Welcome.html"
	}

	if path == "/Config.html" {
		w.Write(getConfigPageBody())
		return
	}

	if strings.HasSuffix(path, ".html") && !strings.HasPrefix(path, "/html/") {
		name := strings.TrimPrefix(strings.TrimSuffix(path, ".html"), "/")
		htmlPath := filepath.Join(storageDir, "html", filepath.Clean(name+".html"))
		mdPath := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))

		htmlStat, errHtml := os.Stat(htmlPath)
		mdStat, errMd := os.Stat(mdPath)

		forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
		if forceRefresh || os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
			if os.IsNotExist(errMd) {
				timestamp := time.Now().Format("2006-01-02 15:04:05")
				authorLine := ""
				if appConfig.Author != "" {
					authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
				}
				humanName := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
				defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n", humanName, timestamp, authorLine)
				os.MkdirAll(filepath.Dir(mdPath), 0755)
				os.WriteFile(mdPath, []byte(defaultContent), 0644)
			}

			mdContent, err := os.ReadFile(mdPath)
			if err == nil {
				if errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime()) {
					humanNameExt := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
					updatedContent := ensureHeaderModified(string(mdContent), humanNameExt)
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

		content, err := os.ReadFile(htmlPath)
		if err == nil {
			if !appConfig.UseInternalEd {
				content = bytes.Replace(content, []byte(`id="toggleBtn"`), []byte(`id="toggleBtn" style="display:none;"`), 1)
			}
			w.Write(content)
			return
		}
	}

	serveLazyEmbed(w, r)
}
"""

    # --- 8. server.go (New minimal entry point) ---
    server_go = r"""package backend

import (
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if requireAdmin && cookie.Value != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func serveStorageDir(subDir string, cType string) http.Handler {
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

func GetServerPort() int {
	return appConfig.ServerPort
}

func StartServer() {
	storageDir = GetStorageDir()
	log.Printf("Using storage directory: %s", storageDir)

	os.MkdirAll(filepath.Join(storageDir, "md"), 0755)
	os.MkdirAll(filepath.Join(storageDir, "html"), 0755)

	cfgPath := filepath.Join(storageDir, "config.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		json.Unmarshal(data, &appConfig)
	} else {
		appConfig = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
			Author:        "Anonymous",
			UseInternalEd: true,
		}
		data, _ = json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(cfgPath, data, 0644)
	}

	if appConfig.MimeTypes != nil {
		for ext, typ := range appConfig.MimeTypes {
			mime.AddExtensionType(ext, typ)
		}
	}

	ExtractDefaultMarkdown()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveFrontend)
	
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
	err := http.ListenAndServe(bindAddr, mux)
	if err != nil {
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "listen" {
			log.Println("Port binding to 0.0.0.0 failed, falling back to 127.0.0.1 (IPv6 Loopback workaround)")
			bindAddr = fmt.Sprintf("127.0.0.1:%d", appConfig.ServerPort)
			log.Printf("OMN-Go Backend running on %s", bindAddr)
			if errFallback := http.ListenAndServe(bindAddr, mux); errFallback != nil {
				log.Fatalf("Server fallback failed: %v", errFallback)
			}
		} else {
			log.Fatalf("Server failed: %v", err)
		}
	}
}
"""

    create_file("backend/config.go", config_go)
    create_file("backend/os_android.go", os_android_go)
    create_file("backend/os_desktop.go", os_desktop_go)
    create_file("backend/embed.go", embed_go)
    create_file("backend/markdown.go", markdown_go)
    create_file("backend/handlers_api.go", handlers_api_go)
    create_file("backend/handlers_web.go", handlers_web_go)
    create_file("backend/server.go", server_go) # Overwrites the old monolith!

    print("\n==================================================")
    print(" Architecture Update Complete! Ready for compilation.")
    print("==================================================")
    
    commit_msg = "refactor(core): decouple monolithic server.go into scalable domain logic via build constraints\n\nVersion bumped to 1.4.0"
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    modularize_backend()