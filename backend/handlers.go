package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (a *App) getConfigPageBody() string {
	cfg := a.GetConfig() // snapshot under RLock; render against the copy

	// Redundant safety net so rendering never indexes a short slice.
	for len(cfg.GitServers) < 5 {
		cfg.GitServers = append(cfg.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(cfg.GitServers)+1)})
	}

	checkedStr := ""
	if cfg.UseInternalEd {
		checkedStr = "checked"
	}

	gitHTML := "<h3>Git Servers</h3>"
	for i, gs := range cfg.GitServers {
		checked := ""
		if cfg.ActiveGitIndex == i {
			checked = "checked"
	}
		gitHTML += fmt.Sprintf(`
			<div style="border: 1px solid #ccc; padding: 15px; margin-bottom: 15px; border-radius: 6px; background: #ffffff; color: black; box-shadow: 0 2px 4px rgba(0,0,0,0.05);">
				<label style="font-weight: bold; display: flex; align-items: center; gap: 8px; margin-bottom: 10px; font-size: 16px; color: #2c3e50;">
					<input type="radio" name="active_git_index" value="%d" %s style="transform: scale(1.2);"> Use as Active Server (Slot %d)
				</label>
				<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">
					<input type="text" id="git_name_%d" name="git_name_%d" value="%s" placeholder="Server Name" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">
					<input type="text" id="git_url_%d" name="git_url_%d" value="%s" placeholder="Git URL (git@...)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">
					<textarea id="git_key_%d" name="git_key_%d" placeholder="SSH Private Key" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px; font-family: monospace; min-height: 60px;">%s</textarea>
					<input type="password" id="git_pass_%d" name="git_pass_%d" value="%s" placeholder="Key Password (Optional)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">
				</div>
			</div>`, i, checked, i+1, i, i, gs.Name, i, i, gs.URL, i, i, gs.SSHKeyData, i, i, gs.Password)
	}

	return fmt.Sprintf(`
<div class="config-panel">
    <h2 class="config-title">Configuration Dashboard</h2>
    <form id="configForm" class="config-form">
        <div class="config-field">
            <label class="config-label">Server Port</label>
            <input type="number" id="cfgPort" name="server_port" value="%d" class="config-input" required />
        </div>
        <div class="config-field">
            <label class="config-label">Admin Password</label>
            <input type="password" id="cfgAdminPwd" name="admin_password" value="%s" class="config-input" required />
        </div>
        <div class="config-field">
            <label class="config-label">Guest Password</label>
            <input type="password" id="cfgGuestPwd" name="guest_password" value="%s" class="config-input" required />
        </div>
        <div class="config-field">
            <label class="config-label">Author Name</label>
            <input type="text" id="cfgAuthor" name="author" value="%s" class="config-input" />
        </div>
        <div class="config-field config-checkbox-row">
            <input type="checkbox" id="cfgInternalEd" name="use_internal_editor" value="true" %s />
            <label class="config-label">Use Internal Editor</label>
        </div>
        <div class="config-field">
            <label class="config-label">Desktop External Cmd</label>
            <input type="text" id="cfgDesktopExtCmd" name="desktop_ext_cmd" value="%s" class="config-input" />
        </div>

        %s

        <div class="config-field" style="margin-top: 20px;">
            <button type="button" class="btn-primary" onclick="saveConfig()">Save Configuration</button>
        </div>
    </form>
</div>
`, cfg.ServerPort, cfg.AdminPassword, cfg.GuestPassword, cfg.Author, checkedStr, cfg.DesktopExtCmd, gitHTML)
}

func (a *App) getExternalEditPageBody(fileName string, viewURL string) string {
	return fmt.Sprintf(`
<div class="ext-edit-panel">
    <div class="ext-edit-icon">📝</div>
    <h2 class="ext-edit-title">Editing Externally</h2>
    <p class="ext-edit-msg">
        We have launched <strong>%s</strong> to edit <code>%s</code>. Please complete your changes in your editor, save the file, and click the button below to view the updated file.
    </p>
    <button onclick="window.location.replace('/%s')" class="ext-edit-btn">
        Press after edit to refresh view
    </button>
</div>
`, a.GetConfig().DesktopExtCmd, fileName, viewURL)
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		cfg := a.GetConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
		return
	}
	if r.Method == "POST" {
		var snapshot Config
		a.WithConfig(func(c *Config) {
			portStr := r.FormValue("server_port")
			var port int
			fmt.Sscanf(portStr, "%d", &port)
			if port > 0 {
				c.ServerPort = port
			}
			c.AdminPassword = r.FormValue("admin_password")
			c.GuestPassword = r.FormValue("guest_password")
			c.Author = r.FormValue("author")
			c.UseInternalEd = r.FormValue("use_internal_editor") == "true"
			c.DesktopExtCmd = r.FormValue("desktop_ext_cmd")
			// Apply active git index from radio selection
			if idxStr := r.FormValue("active_git_index"); idxStr != "" {
				var idx int
				fmt.Sscanf(idxStr, "%d", &idx)
				if idx >= 0 && idx < len(c.GitServers) {
					c.ActiveGitIndex = idx
				}
			}
			// Update all 5 git server slots
			for i := 0; i < 5; i++ {
				name := r.FormValue(fmt.Sprintf("git_name_%d", i))
				url := r.FormValue(fmt.Sprintf("git_url_%d", i))
				keyData := r.FormValue(fmt.Sprintf("git_key_%d", i))
				pass := r.FormValue(fmt.Sprintf("git_pass_%d", i))
				// update fields if any non‑empty value is supplied (allows clearing)
				if name != "" || url != "" || keyData != "" || pass != "" {
					c.GitServers[i].Name = name
					c.GitServers[i].URL = url
					c.GitServers[i].SSHKeyData = keyData
					c.GitServers[i].Password = pass
				}
			}
			snapshot = *c
		})

		// Persist outside the lock — file I/O shouldn't block other
		// goroutines that only need a config read.
		data, _ := json.MarshalIndent(snapshot, "", "  ")
		configPath := filepath.Join(a.StorageDir, "config.json")
		os.WriteFile(configPath, data, 0644)
		w.Write([]byte("Saved"))
		return
	}
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

func (a *App) handleEditExternal(w http.ResponseWriter, r *http.Request) {
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

	var filePath string
	if strings.HasSuffix(name, ".md") {
		filePath = filepath.Join(a.StorageDir, "md", filepath.Clean(name))
	} else {
		filePath = filepath.Join(a.StorageDir, "html", filepath.Clean(name))
	}

	var cmd *exec.Cmd
	cmdStr := strings.TrimSpace(a.GetConfig().DesktopExtCmd)

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
	// Compute the correct view URL (.html for markdown, raw name otherwise)
	viewURL := name
	if strings.HasSuffix(name, ".md") {
		viewURL = strings.TrimSuffix(name, ".md") + ".html"
	}
	waitBody := a.getExternalEditPageBody(name, viewURL)
	compiledWait := a.compilePageWithBody(name, fmt.Appendf(nil, "Title: Refresh %s\nDate: %s\nCategory: Action\n\n", name, time.Now().Format("2006-01-02 15:04:05")), waitBody)
	w.Write(compiledWait)
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	cfg := a.GetConfig()
	pwd := r.FormValue("password")
	role := ""
	if pwd == cfg.AdminPassword {
		role = "admin"
	} else if pwd == cfg.GuestPassword {
		role = "guest"
	}

	if role != "" {
		http.SetCookie(w, &http.Cookie{Name: "session_role", Value: role, Path: "/"})
		w.Write([]byte("OK"))
	} else {
		http.Error(w, "Invalid", http.StatusUnauthorized)
	}
}

func (a *App) handleQuickNote(w http.ResponseWriter, r *http.Request) {
	note := r.FormValue("note")
	if note == "" {
		return
	}
	path := filepath.Join(a.StorageDir, "md", "QuickNotes.md")
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
	fullMarkdown = a.ensureHeaderModified(fullMarkdown, "Quick Notes")
	os.WriteFile(path, []byte(fullMarkdown), 0644)

	// Update Dynamic Precompile instantly
	compiled := a.compilePage("QuickNotes", []byte(fullMarkdown))
	os.WriteFile(filepath.Join(a.StorageDir, "html", "QuickNotes.html"), compiled, 0644)

	w.Write([]byte("Saved"))
}

func (a *App) handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")

	path := filepath.Join(a.StorageDir, "md", "Bookmarks.md")
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
			newContent = a.ensureHeaderModified(newContent, "Incoming bookmarks")
			os.WriteFile(path, []byte(newContent), 0644)
			// Update Dynamic Precompile instantly
			compiled := a.compilePage("Bookmarks", []byte(newContent))
			os.WriteFile(filepath.Join(a.StorageDir, "html", "Bookmarks.html"), compiled, 0644)
		}
	}
	w.Write([]byte("Saved"))
}

// writeTreeFromDir recursively creates a sorted git tree object from the given directory.
// It skips .git and .gitignore, and ensures entries are sorted by name.

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	imgDir := filepath.Join(a.StorageDir, "html", "images")
	os.MkdirAll(imgDir, 0755)

	destPath := filepath.Join(imgDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)

	w.Write(fmt.Appendf(nil, "![%s]({filename}/images/%s)", header.Filename, header.Filename))
}

func (a *App) handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	jsonDir := filepath.Join(a.StorageDir, "html", "user_json")
	os.MkdirAll(jsonDir, 0755)

	destPath := filepath.Join(jsonDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)

	w.Write(fmt.Appendf(nil, "[%s]({filename}/user_json/%s)", header.Filename, header.Filename))
}

func (a *App) handleGetNote(w http.ResponseWriter, r *http.Request) {
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
		path = filepath.Join(a.StorageDir, "md", filepath.Clean(cleanName))
		data, err = os.ReadFile(path)
		if err != nil {
			embedPath := "frontend/md/" + cleanName
			data, err = staticFS.ReadFile(embedPath)
			if err != nil {
				title := strings.TrimSuffix(cleanName, ".md")
				timestamp := time.Now().Format("2006-01-02 15:04:05")
				authorLine := ""
				if a.GetConfig().Author != "" {
					authorLine = fmt.Sprintf("\nAuthor: %s", a.GetConfig().Author)
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
		path = filepath.Join(a.StorageDir, "html", filepath.Clean(name))
		data, err = os.ReadFile(path)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
	}
	w.Write(data)
}

func (a *App) handleNewPage(w http.ResponseWriter, r *http.Request) {
	source := r.FormValue("source")
	target := r.FormValue("target")
	title := r.FormValue("title")

	if target == "" || title == "" {
		http.Error(w, "Missing fields", http.StatusBadRequest)
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	targetMdPath := filepath.Join(a.StorageDir, "md", target+".md")
	if _, err := os.Stat(targetMdPath); os.IsNotExist(err) {
		authorLine := ""
		if a.GetConfig().Author != "" {
			authorLine = fmt.Sprintf("\nAuthor: %s", a.GetConfig().Author)
		}
		defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nModified: %s\nCategory: Notes%s\n\n", title, now, now, authorLine)
		os.MkdirAll(filepath.Dir(targetMdPath), 0755)
		os.WriteFile(targetMdPath, []byte(defaultContent), 0644)
	}

	if source != "" {
		sourceMdPath := filepath.Join(a.StorageDir, "md", source+".md")
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

			content = a.ensureHeaderModified(content, source)
			os.WriteFile(sourceMdPath, []byte(content), 0644)

			// Recompile Source HTML immediately to prevent caching delays
			htmlPath := filepath.Join(a.StorageDir, "html", source+".html")
			compiled := a.compilePage(source, []byte(content))
			os.MkdirAll(filepath.Dir(htmlPath), 0755)
			os.WriteFile(htmlPath, compiled, 0644)
		}
	}

	w.Write([]byte("Created"))
}

func (a *App) handleSaveNote(w http.ResponseWriter, r *http.Request) {
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
		path = filepath.Join(a.StorageDir, "md", filepath.Clean(cleanName))

		content = a.ensureHeaderModified(content, strings.TrimSuffix(cleanName, ".md"))

		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)

		htmlPath := filepath.Join(a.StorageDir, "html", strings.TrimSuffix(cleanName, ".md")+".html")
		os.MkdirAll(filepath.Dir(htmlPath), 0755)
		compiled := a.compilePage(strings.TrimSuffix(cleanName, ".md"), []byte(content))
		os.WriteFile(htmlPath, compiled, 0644)

	} else {
		path = filepath.Join(a.StorageDir, "html", filepath.Clean(name))
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)
	}

	w.Write([]byte("Saved"))
}

func (a *App) serveFrontend(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" || path == "/index.html" {
		http.Redirect(w, r, "/Welcome.html", http.StatusSeeOther)
		return
	}

	if strings.HasSuffix(path, ".html") {
		a.serveHTMLPage(w, r, path)
		return
	}

	if r.URL.Query().Get("edit") == "true" {
		a.serveEditor(w, r, path)
		return
	}

	a.serveStaticAsset(w, r, path)
}

func (a *App) serveHTMLPage(w http.ResponseWriter, r *http.Request, path string) {
	name := strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".html")
	// Defensive: page names should never carry a .md suffix of their own.
	// A caller that mistakenly requests "Welcome.md.html" (extension baked
	// into the name before ".html" was appended) would otherwise produce
	// mdPath "md/Welcome.md.md" below. Stripping it here makes that whole
	// class of double-extension bug impossible regardless of the caller.
	name = strings.TrimSuffix(name, ".md")

	if name == "Config" {
		a.serveConfigPage(w)
		return
	}

	htmlPath := filepath.Join(a.StorageDir, "html", filepath.Clean(name+".html"))
	mdPath := filepath.Join(a.StorageDir, "md", filepath.Clean(name+".md"))

	htmlStat, errHtml := os.Stat(htmlPath)
	mdStat, errMd := os.Stat(mdPath)

	forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
	if forceRefresh || os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
		a.recompileMarkdownPage(name, mdPath, htmlPath, errMd)
	}

	w.Header().Set("Content-Type", "text/html")
	data, err := os.ReadFile(htmlPath)
	if err == nil {
		injected := strings.Replace(string(data), "</head>", fmt.Sprintf("<script>var APP_VERSION = \"%s\"; var USE_INTERNAL_ED = %t;</script></head>", APP_VERSION, a.GetConfig().UseInternalEd), 1)
		w.Write([]byte(injected))
	} else {
		http.ServeFile(w, r, htmlPath)
	}
}

func (a *App) recompileMarkdownPage(name, mdPath, htmlPath string, errMd error) {
	if os.IsNotExist(errMd) {
		embedData, err := staticFS.ReadFile("frontend/md/" + name + ".md")
		if err == nil {
			os.MkdirAll(filepath.Dir(mdPath), 0755)
			os.WriteFile(mdPath, embedData, 0644)
		} else {
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			authorLine := ""
			if a.GetConfig().Author != "" {
				authorLine = fmt.Sprintf("\nAuthor: %s", a.GetConfig().Author)
			}
			defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n", name, timestamp, authorLine)
			os.MkdirAll(filepath.Dir(mdPath), 0755)
			os.WriteFile(mdPath, []byte(defaultContent), 0644)
		}
	}

	mdContent, err := os.ReadFile(mdPath)
	if err == nil {
		htmlStat, errHtml := os.Stat(htmlPath)
		mdStat, errMd := os.Stat(mdPath)
		if errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime()) {
			updatedContent := a.ensureHeaderModified(string(mdContent), name)
			if updatedContent != string(mdContent) {
				os.WriteFile(mdPath, []byte(updatedContent), 0644)
				mdContent = []byte(updatedContent)
			}
		}
		compiled := a.compilePage(name, mdContent)
		os.MkdirAll(filepath.Dir(htmlPath), 0755)
		os.WriteFile(htmlPath, compiled, 0644)
	}
}

func (a *App) serveConfigPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	body := a.getConfigPageBody()
	compiled := a.compilePageWithBody("Config", []byte("Title: Config\nCategory: Settings\n\n"), body)
	injected := strings.Replace(string(compiled), "</head>", fmt.Sprintf("<script>var APP_VERSION = \"%s\"; var USE_INTERNAL_ED = %t;</script></head>", APP_VERSION, a.GetConfig().UseInternalEd), 1)
	w.Write([]byte(injected))
}

func (a *App) serveEditor(w http.ResponseWriter, r *http.Request, path string) {
	relPath := strings.TrimPrefix(path, "/")

	if !a.GetConfig().UseInternalEd {
		http.Redirect(w, r, "/api/edit-external?name="+url.QueryEscape(relPath), http.StatusSeeOther)
		return
	}

	var filePath string
	var rawContent []byte
	if strings.HasSuffix(relPath, ".md") {
		filePath = filepath.Join(a.StorageDir, "md", filepath.Clean(relPath))
	} else {
		filePath = filepath.Join(a.StorageDir, "html", filepath.Clean(relPath))
	}
	
	if data, err := os.ReadFile(filePath); err == nil {
		rawContent = data
	} else {
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, []byte{}, 0644)
		rawContent = []byte{}
	}
	
	escapedContent := a.htmlEscape(string(rawContent))
	customBody := "<pre style=\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\">" + escapedContent + "</pre>"
	compiled := a.compilePageWithBody(relPath, rawContent, customBody)
	
	scriptInjection := "<script>var IS_MARKDOWN = false; setTimeout(function(){ if(typeof toggleMode==='function') toggleMode(); }, 120);</script>"
	compiled = []byte(strings.Replace(string(compiled), "</head>", scriptInjection+"\n</head>", 1))
	w.Header().Set("Content-Type", "text/html")
	w.Write(compiled)
}

func (a *App) serveStaticAsset(w http.ResponseWriter, r *http.Request, path string) {
	ext := strings.ToLower(filepath.Ext(path))
	mimeType, exists := a.GetConfig().MimeTypes[ext]
	if !exists {
		mimeType = mime.TypeByExtension(ext)
	}
	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}

	filePath := filepath.Join(a.StorageDir, "html", filepath.Clean(path))
	if stat, err := os.Stat(filePath); err == nil && !stat.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}

	embedPath := "frontend" + filepath.Clean(path)
	if data, err := staticFS.ReadFile(embedPath); err == nil {
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, data, 0644)
		w.Write(data)
		return
	}

	http.NotFound(w, r)
}

