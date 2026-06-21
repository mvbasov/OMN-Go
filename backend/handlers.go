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
			injected := strings.Replace(string(compiled), "</head>", "<script>var APP_VERSION = \""+APP_VERSION+"\";</script></head>", 1)
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
			injected := strings.Replace(string(data), "</head>", "<script>var APP_VERSION = \""+APP_VERSION+"\";</script></head>", 1)
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
