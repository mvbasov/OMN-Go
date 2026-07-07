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
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (a *App) getConfigPageBody() string {
	cfg := a.GetConfig() // snapshot under RLock; render against the copy

	// Redundant safety net so rendering never indexes a short slice.
	for len(cfg.GitServers) < maxGitServers {
		cfg.GitServers = append(cfg.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(cfg.GitServers)+1)})
	}

	view := configPageView{
		ServerPort:    cfg.ServerPort,
		AdminPassword: cfg.AdminPassword,
		GuestPassword: cfg.GuestPassword,
		Author:        cfg.Author,
		UseInternalEd: cfg.UseInternalEd,
		DesktopExtCmd: cfg.DesktopExtCmd,
		Theme:         cfg.Theme,
		ShareLAN:      cfg.ShareLAN,
	}
	for i, gs := range cfg.GitServers {
		view.GitServers = append(view.GitServers, gitServerView{
			Index:      i,
			Slot:       i + 1,
			Active:     cfg.ActiveGitIndex == i,
			Name:       gs.Name,
			URL:        gs.URL,
			SSHKeyData: gs.SSHKeyData,
			Password:   gs.Password,
		})
	}

	return renderConfigPage(view)
}

func (a *App) getExternalEditPageBody(fileName string, viewURL string) string {
	view := externalEditView{
		Cmd:      a.GetConfig().DesktopExtCmd,
		FileName: fileName,
		ViewURL:  viewURL,
	}
	return renderExternalEditPage(view)
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		cfg := a.GetConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
		return
	}
	if r.Method == "POST" {
		// The listen socket is bound exactly once at startup, so flipping
		// ShareLAN can only take effect through a full process restart.
		// Capture the pre-save value to detect the flip and tell the
		// frontend, which then drives /api/restart.
		prevShareLAN := a.GetConfig().ShareLAN

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
			// Whitelisted via normalizeTheme: anything but light/dark
			// (including a missing field) becomes auto.
			c.Theme = normalizeTheme(r.FormValue("theme"))
			// Unchecked checkboxes are simply absent from the form, so
			// this correctly reads as false when the box is cleared.
			// Takes effect on next start (see server.go bind logic).
			c.ShareLAN = r.FormValue("share_lan") == "true"
			// Apply active git index from radio selection
			if idxStr := r.FormValue("active_git_index"); idxStr != "" {
				var idx int
				fmt.Sscanf(idxStr, "%d", &idx)
				if idx >= 0 && idx < len(c.GitServers) {
					c.ActiveGitIndex = idx
				}
			}
			// Update all git server slots
			for i := 0; i < maxGitServers; i++ {
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
		data, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			log.Printf("handleConfig: failed to marshal config: %v", err)
			http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
			return
		}
		configPath := filepath.Join(a.StorageDir, "config.json")
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			log.Printf("handleConfig: failed to write %s: %v", configPath, err)
			http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
			return
		}
		if snapshot.ShareLAN != prevShareLAN {
			// Saved fine, but the new bind address only exists after a
			// restart. The frontend reacts to this exact string (see
			// saveConfig in omn-go-sse.js) by calling /api/restart.
			w.Write([]byte("RestartRequired"))
			return
		}
		w.Write([]byte("Saved"))
		return
	}
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// handleRestart restarts the whole application process so startup-bound
// state - above all the listen address chosen from ShareLAN - is rebuilt
// from the just-saved config. Triggered by the frontend right after a
// config save that flipped ShareLAN.
//
// Per platform:
//   - Android: plain os.Exit(0). The process (Go server, WebView UI - the
//     whole app) terminates; ServerService is START_STICKY, so the system
//     recreates it shortly after, which restarts the Go server with the
//     new config and rebuilds the notification/foreground state from that
//     same config (see ServerService.onStartCommand). The UI closes; the
//     user reopens the app manually - at which point MainActivity also
//     re-evaluates which permissions LAN sharing now needs.
//   - Desktop: spawn a fresh copy of our own executable (marked with
//     OMN_GO_RESTARTED=1 so main_desktop.go doesn't open a second browser
//     tab), then exit. The bind-retry loop in server.go absorbs the brief
//     window where the child races the parent's socket teardown.
//
// The HTTP response is written before any of this happens so the browser
// actually receives it; the exit runs on a delayed goroutine.
func (a *App) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	log.Printf("[restart] restart requested via /api/restart")
	w.Write([]byte("Restarting"))

	go func() {
		time.Sleep(500 * time.Millisecond) // let the response flush

		if runtime.GOOS == "android" {
			os.Exit(0)
		}

		exe, err := os.Executable()
		if err != nil {
			log.Printf("[restart] cannot locate own executable, not restarting: %v", err)
			return
		}
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Env = append(os.Environ(), "OMN_GO_RESTARTED=1")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			// Failing to spawn must NOT kill the running server - a
			// working old instance beats no instance at all.
			log.Printf("[restart] failed to start replacement process, keeping current one: %v", err)
			return
		}
		log.Printf("[restart] replacement process started (pid %d), exiting", cmd.Process.Pid)
		os.Exit(0)
	}()
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

	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)
	filePath := htmlPath
	if isPage {
		filePath = mdPath
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
	// Compute the correct view URL (.html for a page, raw name for a plain
	// asset) using the same isPage/baseName decision made above.
	viewURL := name
	if isPage {
		viewURL = baseName + ".html"
	}
	waitBody := a.getExternalEditPageBody(name, viewURL)
	compiledWait := a.compilePageWithBody(name, fmt.Appendf(nil, "Title: Refresh %s\nDate: %s\nCategory: Action\n\n", name, time.Now().Format("2006-01-02 15:04:05")), waitBody, false)
	w.Write(a.injectRuntimeVars(compiledWait))
}

// resolveNewPageTarget resolves a newly-requested page name the same way a
// bare relative link on the source page would resolve (see
// rewriteInternalLink in markdown.go):
//   - a bare name with no "/" is created as a sibling of source, in the
//     same directory - not at the storage root
//   - a leading "/" is treated as absolute, anchored at the storage root
//   - a target that already specifies its own directory is used as-is
//
// source is defensively trimmed of any stray leading/trailing slashes
// before computing its directory: a trailing slash (e.g. "path/file/"
// instead of "path/file") would otherwise make the directory computation
// treat the whole string as its own directory, producing "path/file/new"
// instead of the intended sibling "path/new".
func (a *App) resolveNewPageTarget(source, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return target
	}

	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}

	if strings.Contains(target, "/") {
		return path.Clean(target)
	}

	source = strings.Trim(strings.TrimSpace(source), "/")
	if source == "" {
		return target
	}

	dir := path.Dir(source)
	if dir == "." {
		return target
	}
	return dir + "/" + target
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

// saveUploadedFile does the shared work behind handleUpload and
// handleUploadJSON: parse the multipart form, pull out the named file
// field, and copy it into destDir/<original filename>. Every step that can
// fail now actually reports failure to the caller instead of the previous
// os.Create(...)/io.Copy(...) with errors discarded via "_" - a full disk
// or a permissions problem used to look identical to a successful upload
// from the browser's point of view.
func (a *App) saveUploadedFile(r *http.Request, formField, destDir string) (filename string, err error) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB
		return "", fmt.Errorf("parse form: %w", err)
	}
	file, header, err := r.FormFile(formField)
	if err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}
	defer file.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}

	destPath := filepath.Join(destDir, header.Filename)
	dest, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create destination file: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		return "", fmt.Errorf("write destination file: %w", err)
	}
	return header.Filename, nil
}

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	imgDir := filepath.Join(a.StorageDir, "html", "images")
	filename, err := a.saveUploadedFile(r, "image", imgDir)
	if err != nil {
		log.Printf("handleUpload: %v", err)
		http.Error(w, "Upload failed", http.StatusInternalServerError)
		return
	}
	w.Write(fmt.Appendf(nil, "![%s](/images/%s)", filename, filename))
}

func (a *App) handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	jsonDir := filepath.Join(a.StorageDir, "html", "user_json")
	filename, err := a.saveUploadedFile(r, "file", jsonDir)
	if err != nil {
		log.Printf("handleUploadJSON: %v", err)
		http.Error(w, "Upload failed", http.StatusInternalServerError)
		return
	}
	w.Write(fmt.Appendf(nil, "[%s](/user_json/%s)", filename, filename))
}

func (a *App) handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Welcome"
	}

	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)

	if !isPage {
		data, err := os.ReadFile(htmlPath)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		w.Write(data)
		return
	}

	data, err := os.ReadFile(mdPath)
	if err == nil {
		w.Write(data)
		return
	}

	// Not on disk yet - fall back to the embedded default, or synthesize a
	// fresh empty page. Either way, persist it so this fallback only ever
	// runs once per page. Failures here are logged (not fatal to the
	// request) since the in-memory `data` we're about to serve is still
	// correct even if we can't cache it to disk.
	embedPath := "frontend/md/" + baseName + ".md"
	if embedData, embedErr := staticFS.ReadFile(embedPath); embedErr == nil {
		data = embedData
	} else {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		authorLine := ""
		if a.GetConfig().Author != "" {
			authorLine = fmt.Sprintf("\nAuthor: %s", a.GetConfig().Author)
		}
		data = []byte(fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n", baseName, timestamp, authorLine))
	}

	if mkErr := os.MkdirAll(filepath.Dir(mdPath), 0755); mkErr != nil {
		log.Printf("handleGetNote: failed to create directory for %q: %v", baseName, mkErr)
	} else if writeErr := os.WriteFile(mdPath, data, 0644); writeErr != nil {
		log.Printf("handleGetNote: failed to persist new page %q: %v", baseName, writeErr)
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

	// A bare target name (no "/") used to always land at the storage root
	// regardless of which page it was created from, so creating "test"
	// while viewing "local/local" produced "test" instead of "local/test" -
	// inconsistent with how a bare relative link on that same page resolves
	// (see rewriteInternalLink). Resolve it the same way here: relative to
	// source's directory unless target is itself absolute or already
	// specifies a directory.
	rawTarget := target
	target = a.resolveNewPageTarget(source, target)

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

			// The href embedded in source must match how source itself
			// will resolve it when clicked, not the fully-resolved
			// storage path. A bare rawTarget (no "/") was resolved above
			// relative to source's own directory (see
			// resolveNewPageTarget) - inserting that same bare name here
			// lets the browser's ordinary relative-link resolution land
			// on exactly that file, since it resolves relative to
			// source's directory too. Inserting the already-resolved
			// "target" instead (e.g. "path/new") used to have the browser
			// resolve it AGAIN relative to source's directory when
			// clicked, doubling the nesting into "path/path/new". A
			// rawTarget that already specified its own directory (or was
			// absolute) is anchored at the storage root, so it needs an
			// explicit leading "/" here to avoid being misresolved the
			// same relative way.
			linkHref := strings.TrimSpace(rawTarget)
			if strings.Contains(linkHref, "/") {
				linkHref = "/" + target
			}
			linkStr := fmt.Sprintf("* [%s](%s)", title, linkHref)
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

	w.Write([]byte(target))
}

func (a *App) handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		http.Error(w, "Missing name", http.StatusBadRequest)
		return
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")

	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)

	if !isPage {
		if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
			log.Printf("handleSaveNote: mkdir failed for %q: %v", name, err)
			http.Error(w, "Failed to save", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(htmlPath, []byte(content), 0644); err != nil {
			log.Printf("handleSaveNote: write failed for %q: %v", name, err)
			http.Error(w, "Failed to save", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("Saved"))
		return
	}

	content = a.ensureHeaderModified(content, baseName)

	// Write the markdown source first. This is the source of truth - if it
	// fails, bail out and tell the caller "Saved" is a lie, rather than
	// going on to compile/write the HTML from content that never actually
	// made it to disk.
	if err := os.MkdirAll(filepath.Dir(mdPath), 0755); err != nil {
		log.Printf("handleSaveNote: mkdir failed for %q: %v", baseName, err)
		http.Error(w, "Failed to save", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(mdPath, []byte(content), 0644); err != nil {
		log.Printf("handleSaveNote: write failed for %q: %v", baseName, err)
		http.Error(w, "Failed to save", http.StatusInternalServerError)
		return
	}

	// The compiled HTML is a derived cache of the markdown we just saved
	// successfully - if this part fails, the note itself is still safe on
	// disk, so log it and let the next page load recompile it (serveHTMLPage
	// already recompiles whenever the .md is newer than the .html) rather
	// than reporting the save itself as failed.
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
		log.Printf("handleSaveNote: mkdir failed for compiled html of %q: %v", baseName, err)
	} else {
		compiled := a.compilePage(baseName, []byte(content))
		if err := os.WriteFile(htmlPath, compiled, 0644); err != nil {
			log.Printf("handleSaveNote: failed to write compiled html for %q: %v", baseName, err)
		}
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

	mdPath, htmlPath, name, _ := a.resolvePageName(name)

	htmlStat, errHtml := os.Stat(htmlPath)
	mdStat, errMd := os.Stat(mdPath)

	forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
	if forceRefresh || os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
		a.recompileMarkdownPage(name, mdPath, htmlPath, errMd)
	}

	w.Header().Set("Content-Type", "text/html")
	data, err := os.ReadFile(htmlPath)
	if err == nil {
		w.Write(a.injectRuntimeVars(data))
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
	compiled := a.compilePageWithBody("Config", []byte("Title: Config\nCategory: Settings\n\n"), body, false)
	w.Write(a.injectRuntimeVars(compiled))
}

func (a *App) serveEditor(w http.ResponseWriter, r *http.Request, path string) {
	relPath := strings.TrimPrefix(path, "/")

	if !a.GetConfig().UseInternalEd {
		http.Redirect(w, r, "/api/edit-external?name="+url.QueryEscape(relPath), http.StatusSeeOther)
		return
	}

	mdPath, htmlPath, _, isPage := a.resolvePageName(relPath)
	filePath := htmlPath
	if isPage {
		filePath = mdPath
	}

	var rawContent []byte
	if data, err := os.ReadFile(filePath); err == nil {
		rawContent = data
	} else {
		if mkErr := os.MkdirAll(filepath.Dir(filePath), 0755); mkErr != nil {
			log.Printf("serveEditor: mkdir failed for %q: %v", filePath, mkErr)
		} else if writeErr := os.WriteFile(filePath, []byte{}, 0644); writeErr != nil {
			log.Printf("serveEditor: failed to create empty file %q: %v", filePath, writeErr)
		}
		rawContent = []byte{}
	}
	
	escapedContent := a.htmlEscape(string(rawContent))
	customBody := "<pre style=\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\">" + escapedContent + "</pre>"
	compiled := a.compilePageWithBody(relPath, rawContent, customBody, true)

	w.Header().Set("Content-Type", "text/html")
	w.Write(a.injectRuntimeVars(compiled))
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

