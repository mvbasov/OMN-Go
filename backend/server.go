package backend

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

const APP_VERSION = "1.2.32"

type Config struct {
	ServerPort    int               `json:"server_port"`
	AdminPassword string            `json:"admin_password"`
	GuestPassword string            `json:"guest_password"`
	UseInternalEd bool              `json:"use_internal_editor"`
	DesktopExtCmd string            `json:"desktop_ext_cmd"`
	MimeTypes     map[string]string `json:"mime_types"`
}

//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/html frontend/md
var staticFS embed.FS

var (
	storageDir  string
	appConfig   Config
	activeConns int
)

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
	configPath := filepath.Join(storageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		appConfig = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
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

	initDefaultPage("Welcome.md", "Title: Welcome
Date: 2026-06-14 12:00:00
Category: System

Welcome to OMN-Go! Start editing.

- [Help](Welcome)
- [Scripting Rules](ScriptRules.md)
- [Bookmarks](Bookmarks)
- [Quick Notes](QuickNotes)")
	initDefaultPage("ScriptRules.md", "Title: JS Scripting Rules
Date: 2026-06-15
Category: System

# JavaScript Guidelines for OMN-Go

Because OMN-Go is rendered server-side, keep scripts wrapped in block scopes.")
	initDefaultPage("QuickNotes.md", "Title: Quick Notes
Date: 2026-06-14 12:00:00
Category: Log

")
	initDefaultPage("Bookmarks.md", "Title: Incoming bookmarks
Date: 2026-06-15 20:00:00
Author: 
Tags: Bookmarks

<script>bookmarks = [
<!-- Don't edit body below this line -->
];
</script>")

	// Precompile all notes to data/html/ at startup
	precompileAllPages()
}

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

	lines := strings.Split(string(mdContent), "\n")
	for _, line := range lines {
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
	metadataStr := fmt.Sprintf("File: %s.md\n%s", name, strings.Join(headers, "\n"))

	layout := string(frontendHTML)

	title := "OMN-Go - " + name
	for _, h := range headers {
		if strings.HasPrefix(h, "Title:") {
			title = strings.TrimSpace(strings.TrimPrefix(h, "Title:"))
			break
		}
	}

	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PAGE_TITLE -->", title)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PREVIEW_BODY -->", renderedBody)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", htmlEscape(string(mdContent)))
	layout = strings.ReplaceAll(layout, "/* OMN_GO_PAGE_NAME_JS */", fmt.Sprintf(`let currentNote = "%s";`, name))
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", metadataStr)

	return []byte(layout)
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
`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword,
		func() string { if appConfig.UseInternalEd { return "checked" }; return "" }(),
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
    <button onclick="window.location.href='/%s.html'" style="background: #0056b3; color: white; border: none; padding: 15px 30px; border-radius: 6px; font-weight: bold; cursor: pointer; font-size: 18px; transition: background 0.2s; box-shadow: 0 2px 5px rgba(0,0,0,0.2);">
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
	compiledWait := compilePageWithBody(pageName, []byte(fmt.Sprintf("Title: Refresh %s\nDate: %s\nCategory: Action\n\n", pageName, time.Now().Format("2006-01-02 15:04:05"))), waitBody)
	w.Write(compiledWait)
}

// Simple connection tracker for Android WebView synchronization
func connectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeConns++
		next.ServeHTTP(w, r)
		activeConns--
	})
}

func isLocalConnection(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
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

	jsonDir := filepath.Join(storageDir, "html", "user_json")
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
				newContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes\n\n# %s\n\nStart editing this page!", title, timestamp, title)
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
		
		parts := strings.Split(content, "\n\n")
		if len(parts) > 0 && strings.Contains(parts[0], ":") {
			headerLines := strings.Split(parts[0], "\n")
			modIdx := -1
			for i, l := range headerLines {
				if strings.HasPrefix(l, "Modified:") {
					modIdx = i
					break
				}
			}
			now := time.Now().Format("2006-01-02 15:04:05")
			if modIdx != -1 {
				headerLines[modIdx] = fmt.Sprintf("Modified: %s", now)
			} else {
				headerLines = append(headerLines, fmt.Sprintf("Modified: %s", now))
			}
			parts[0] = strings.Join(headerLines, "\n")
			content = strings.Join(parts, "\n\n")
		}

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
			w.Write(compiled)
			return
		}

		htmlPath := filepath.Join(storageDir, "html", filepath.Clean(name+".html"))
		mdPath := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))

		htmlStat, errHtml := os.Stat(htmlPath)
		mdStat, errMd := os.Stat(mdPath)

		// Recompile if HTML is missing, OR if Markdown was modified more recently than HTML
		if os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
			if os.IsNotExist(errMd) {
				embedData, err := staticFS.ReadFile("frontend/md/" + name + ".md")
				if err == nil {
					os.MkdirAll(filepath.Dir(mdPath), 0755)
					os.WriteFile(mdPath, embedData, 0644)
				} else {
					timestamp := time.Now().Format("2006-01-02 15:04:05")
					defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes\n\n# %s\n\nStart editing this page!", name, timestamp, name)
					os.MkdirAll(filepath.Dir(mdPath), 0755)
					os.WriteFile(mdPath, []byte(defaultContent), 0644)
				}
			}

			mdContent, err := os.ReadFile(mdPath)
			if err == nil {
				compiled := compilePage(name, mdContent)
				os.MkdirAll(filepath.Dir(htmlPath), 0755)
				os.WriteFile(htmlPath, compiled, 0644)
			}
		}

		w.Header().Set("Content-Type", "text/html")
		http.ServeFile(w, r, htmlPath)
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
		if path == "/js/Bookmarker.js" {
			js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
			js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
			data = []byte(js)
		}
		
		// Copy extracted file directly to user data directory
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, data, 0644)
		
		w.Write(data)
		return
	}

	http.NotFound(w, r)
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
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in server: %v", r)
			}
		}()
		
		mux := http.NewServeMux()
		mux.HandleFunc("/", serveFrontend)
		serveStrict := func(ext, cType string) http.Handler {
			physicalDir := filepath.Join(storageDir, "html")
			fsHandler := http.FileServer(http.Dir(physicalDir))
			
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, ext) {
					http.Error(w, "Forbidden: Invalid file extension", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", cType)
				
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

		mux.Handle("/js/", serveStrict(".js", "application/javascript"))
		mux.Handle("/css/fonts/", serveStrict(".woff2", "font/woff2"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))
		mux.Handle("/json/", serveStrict(".json", "application/json"))
		
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
