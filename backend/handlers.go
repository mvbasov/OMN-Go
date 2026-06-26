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
	"sort"
	"github.com/go-git/go-git/v5/storage"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

func getConfigPageBody() string {
	return fmt.Sprintf(`
<div class="config-panel">
    <h2 class="config-title">Configuration Dashboard</h2>
    <form id="configForm" onsubmit="saveConfig(event)" class="config-form">
        <div class="config-field">
            <label class="config-label">Server Port</label>
            <input type="number" id="cfgPort" value="%d" class="config-input" required />
        </div>
        <div class="config-field">
            <label class="config-label">Admin Password</label>
            <input type="password" id="cfgAdminPwd" value="%s" class="config-input" required />
        </div>
        <div class="config-field">
            <label class="config-label">Guest Password</label>
            <input type="password" id="cfgGuestPwd" value="%s" class="config-input" required />
        </div>
        <div class="config-field">
            <label class="config-label">Author Name</label>
            <input type="text" id="cfgAuthor" value="%s" class="config-input" />
        </div>
        <div class="config-field config-checkbox-row">
            <input type="checkbox" id="cfgUseInternal" %s class="config-checkbox" />
            <label for="cfgUseInternal" class="config-label config-checkbox-label">Use HTML Internal Editor</label>
        </div>
        <div class="config-field">
            <label class="config-label">Desktop External Editor Command</label>
            <input type="text" id="cfgExtCmd" value="%s" class="config-input" />
            <small class="config-hint">Example: <code>subl</code> or <code>code</code> or <code>nano</code></small>
        </div>
        <div class="config-field">
            <label class="config-label">Sync Remote (git URL)</label>
            <input type="text" id="cfgSyncRemote" value="%s" class="config-input" placeholder="git@host:repo.git" />
        </div>
        <div class="config-field">
            <label class="config-label">Sync SSH Key Path (relative to storage dir)</label>
            <input type="text" id="cfgSyncSSHKey" value="%s" class="config-input" placeholder="omngo_sync_key" />
        </div>
        <div class="config-field">
            <label class="config-label">Sync SSH Passphrase (optional)</label>
            <input type="password" id="cfgSyncPassphrase" value="%s" class="config-input" placeholder="leave empty if none" />
        </div>
        <button type="submit" class="config-save-btn">Save Configuration</button>
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
        params.append("sync_remote", document.getElementById("cfgSyncRemote").value);
        params.append("sync_ssh_key", document.getElementById("cfgSyncSSHKey").value);
        params.append("sync_ssh_passphrase", document.getElementById("cfgSyncPassphrase").value);

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
		appConfig.DesktopExtCmd,
		appConfig.SyncRemote, appConfig.SyncSSHKey, appConfig.SyncSSHPassphrase)
}

func getExternalEditPageBody(fileName string, viewURL string) string {
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
`, appConfig.DesktopExtCmd, fileName, viewURL)
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
		appConfig.SyncRemote = r.FormValue("sync_remote")
		appConfig.SyncSSHKey = r.FormValue("sync_ssh_key")
		appConfig.SyncSSHPassphrase = r.FormValue("sync_ssh_passphrase")

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

	var filePath string
	if strings.HasSuffix(name, ".md") {
		filePath = filepath.Join(storageDir, "md", filepath.Clean(name))
	} else {
		filePath = filepath.Join(storageDir, "html", filepath.Clean(name))
	}

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
	// Compute the correct view URL (.html for markdown, raw name otherwise)
	viewURL := name
	if strings.HasSuffix(name, ".md") {
		viewURL = strings.TrimSuffix(name, ".md") + ".html"
	}
	waitBody := getExternalEditPageBody(name, viewURL)
	compiledWait := compilePageWithBody(name, fmt.Appendf(nil, "Title: Refresh %s\nDate: %s\nCategory: Action\n\n", name, time.Now().Format("2006-01-02 15:04:05")), waitBody)
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

func manualGitInit(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/master\n"), 0644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0755); err != nil {
		return err
	}
	config := []byte("[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n")
	if err := os.WriteFile(filepath.Join(gitDir, "config"), config, 0644); err != nil {
		return err
	}
	return nil
}

// writeTreeFromDir recursively creates a sorted git tree object from the given directory.
// It skips .git and .gitignore, and ensures entries are sorted by name.
func writeTreeFromDir(dir string, storer storage.Storer) (plumbing.Hash, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return plumbing.Hash{}, err
	}
	// Sort directory entries for deterministic order
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	entries := []object.TreeEntry{}
	for _, f := range files {
		if f.Name() == ".git" || f.Name() == ".gitignore" {
			continue
		}
		fullPath := filepath.Join(dir, f.Name())
		if f.IsDir() {
			subTreeHash, err := writeTreeFromDir(fullPath, storer)
			if err != nil {
				return plumbing.Hash{}, err
			}
			entries = append(entries, object.TreeEntry{
				Name: f.Name(),
				Mode: 0040000,
				Hash: subTreeHash,
			})
		} else {
			data, err := os.ReadFile(fullPath)
			if err != nil {
				return plumbing.Hash{}, err
			}
			blobObj := storer.NewEncodedObject()
			blobObj.SetType(plumbing.BlobObject)
			blobObj.SetSize(int64(len(data)))
			w, err := blobObj.Writer()
			if err != nil {
				return plumbing.Hash{}, err
			}
			if _, err = w.Write(data); err != nil {
				return plumbing.Hash{}, err
			}
			w.Close()
			blobHash, err := storer.SetEncodedObject(blobObj)
			if err != nil {
				return plumbing.Hash{}, err
			}
			entries = append(entries, object.TreeEntry{
				Name: f.Name(),
				Mode: 0100644,
				Hash: blobHash,
			})
		}
	}
	// Build tree object
	treeObj := object.Tree{Entries: entries}
	encoded := storer.NewEncodedObject()
	if err := treeObj.Encode(encoded); err != nil {
		return plumbing.Hash{}, err
	}
	return storer.SetEncodedObject(encoded)
}

func ensureGitignore() {
	gitignorePath := filepath.Join(storageDir, ".gitignore")
	gitignoreBase := "# OMN-Go sync ignore\nconfig.json\n*.html\n"
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		os.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)
		log.Printf("[sync] Created .gitignore")
	}
	if appConfig.SyncSSHKey != "" {
		keyPath := appConfig.SyncSSHKey
		if !filepath.IsAbs(keyPath) {
			keyPath = filepath.Join(storageDir, keyPath)
		}
		relKey, err := filepath.Rel(storageDir, keyPath)
		if err == nil && !strings.HasPrefix(relKey, "..") {
			current, _ := os.ReadFile(gitignorePath)
			if !strings.Contains(string(current), relKey) {
				newContent := string(current) + "\n" + relKey + "\n"
				os.WriteFile(gitignorePath, []byte(newContent), 0644)
				log.Printf("[sync] Added %s to .gitignore", relKey)
			}
		}
	}
}

func getOrInitRepo() (*git.Repository, error) {
	log.Printf("[sync] Opening repo at %s", storageDir)
	repo, err := git.PlainOpen(storageDir)
	if err != nil {
		log.Printf("[sync] Repo not found, initializing...")
		repo, err = git.PlainInit(storageDir, false)
		if err != nil {
			log.Printf("[sync] git.PlainInit failed: %v; attempting manual init", err)
			if initErr := manualGitInit(storageDir); initErr != nil {
				return nil, fmt.Errorf("manual init failed: %v", initErr)
			}
			repo, err = git.PlainOpen(storageDir)
			if err != nil {
				return nil, fmt.Errorf("failed to open manually created repo: %v", err)
			}
		}
		log.Printf("[sync] Repo initialized")
	} else {
		log.Printf("[sync] Repo opened successfully")
	}

	_, err = repo.Remote("origin")
	if err != nil {
		log.Printf("[sync] Remote origin missing, adding")
		_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
			Name: "origin",
			URLs: []string{appConfig.SyncRemote},
		})
		if err != nil {
			return nil, fmt.Errorf("remote add failed: %v", err)
		}
	}
	return repo, nil
}

func getSSHAuth() (transport.AuthMethod, error) {
	sshUser := "git"
	if idx := strings.Index(appConfig.SyncRemote, "@"); idx != -1 {
		sshUser = appConfig.SyncRemote[:idx]
	}
	log.Printf("[sync] SSH user: %s", sshUser)

	if appConfig.SyncSSHKey == "" {
		log.Printf("[sync] Error: No SSH key configured")
		return nil, fmt.Errorf("no SSH key configured")
	}

	keyPath := appConfig.SyncSSHKey
	if !filepath.IsAbs(keyPath) {
		keyPath = filepath.Join(storageDir, keyPath)
	}
	log.Printf("[sync] Using SSH key: %s", keyPath)

	info, err := os.Stat(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key: %v", err)
	}
	log.Printf("[sync] Key file size: %d, mode: %s", info.Size(), info.Mode())

	auth, err := GetInsecureSSHAuth(sshUser, keyPath, appConfig.SyncSSHPassphrase)
	if err != nil {
		return nil, fmt.Errorf("GetInsecureSSHAuth error: %v", err)
	}
	log.Printf("[sync] SSH auth method created using crypto/ssh signer")
	return auth, nil
}

func commitLocalChanges(repo *git.Repository, wTree *git.Worktree) error {
	log.Printf("[sync] Staging all changes")
	_, err := wTree.Add(".")
	if err != nil {
		return err
	}
	status, _ := wTree.Status()
	if status.IsClean() {
		log.Printf("[sync] Nothing to commit")
		return nil
	}
	
	log.Printf("[sync] Uncommitted changes detected, building commit manually")
	authorName := GetConfigAuthor()
	authorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"
	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}

	treeHash, err := writeTreeFromDir(storageDir, repo.Storer)
	if err != nil {
		return fmt.Errorf("writeTreeFromDir error: %v", err)
	}
	
	headRef, errHead := repo.Head()
	var parents []plumbing.Hash
	if errHead == nil {
		parents = []plumbing.Hash{headRef.Hash()}
	}
	commit := &object.Commit{
		Author:       *sig,
		Committer:    *sig,
		Message:      "Local changes before sync",
		TreeHash:     treeHash,
		ParentHashes: parents,
	}
	obj := repo.Storer.NewEncodedObject()
	if err = commit.Encode(obj); err != nil {
		return fmt.Errorf("commit encode error: %v", err)
	}
	commitHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return fmt.Errorf("store commit error: %v", err)
	}
	refPath := filepath.Join(storageDir, ".git", "refs", "heads", "master")
	if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
		return fmt.Errorf("mkdirAll ref error: %v", err)
	}
	if err := os.WriteFile(refPath, []byte(commitHash.String()+"\n"), 0644); err != nil {
		return fmt.Errorf("write ref error: %v", err)
	}
	return nil
}

func executeSyncDownload(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, force bool) error {
	if force {
		log.Printf("[sync] Force Download: Fetching and Hard Resetting")
		err := repo.Fetch(&git.FetchOptions{RemoteName: "origin", Auth: auth})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return fmt.Errorf("fetch failed: %v", err)
		}
		ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", "master"), true)
		if err != nil {
			return fmt.Errorf("failed to find origin/master: %v", err)
		}
		err = wTree.Reset(&git.ResetOptions{Commit: ref.Hash(), Mode: git.HardReset})
		if err != nil {
			return fmt.Errorf("hard reset failed: %v", err)
		}
	} else {
		log.Printf("[sync] Pulling from origin master")
		err := wTree.Pull(&git.PullOptions{
			RemoteName:    "origin",
			Auth:          auth,
			ReferenceName: plumbing.NewBranchReferenceName("master"),
			SingleBranch:  true,
		})
		if err != nil && err != git.NoErrAlreadyUpToDate && !strings.Contains(err.Error(), "couldn't find remote ref") {
			return fmt.Errorf("pull failed: %v", err)
		}
	}
	return nil
}

func executeSyncUpload(repo *git.Repository, auth transport.AuthMethod, force bool) error {
	log.Printf("[sync] Pushing to origin master (Force: %v)", force)
	err := repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs:   []gitconfig.RefSpec{"refs/heads/master:refs/heads/master"},
		Force:      force,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("push failed: %v", err)
	}
	return nil
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	log.Printf("[sync] Request received")
	if r.Method != "POST" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}

	if appConfig.SyncRemote == "" {
		log.Printf("[sync] Error: sync_remote not configured")
		http.Error(w, "Sync not configured: missing sync_remote in config.json", 500)
		return
	}

	log.Printf("[sync] Remote: %s", appConfig.SyncRemote)

	ensureGitignore()

	repo, err := getOrInitRepo()
	if err != nil {
		http.Error(w, fmt.Sprintf("Repo init failed: %v", err), 500)
		return
	}

	auth, err := getSSHAuth()
	if err != nil {
		http.Error(w, fmt.Sprintf("SSH auth failed: %v", err), 500)
		return
	}

	wTree, _ := repo.Worktree()
	action := r.FormValue("action")
	force := r.FormValue("force") == "true"

	if err := commitLocalChanges(repo, wTree); err != nil {
		http.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)
		return
	}

	if action == "download" {
		if err := executeSyncDownload(repo, wTree, auth, force); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	} else if action == "upload" {
		if err := executeSyncUpload(repo, auth, force); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	} else {
		http.Error(w, "Invalid action. Use 'upload' or 'download'.", 400)
		return
	}

	w.Write([]byte("Sync action completed successfully."))
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
		serveHTMLPage(w, r, path)
		return
	}

	if r.URL.Query().Get("edit") == "true" {
		serveEditor(w, r, path)
		return
	}

	serveStaticAsset(w, r, path)
}

func serveHTMLPage(w http.ResponseWriter, r *http.Request, path string) {
	name := strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".html")

	if name == "Config" {
		serveConfigPage(w)
		return
	}

	htmlPath := filepath.Join(storageDir, "html", filepath.Clean(name+".html"))
	mdPath := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))

	htmlStat, errHtml := os.Stat(htmlPath)
	mdStat, errMd := os.Stat(mdPath)

	forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
	if forceRefresh || os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
		recompileMarkdownPage(name, mdPath, htmlPath, errMd)
	}

	w.Header().Set("Content-Type", "text/html")
	data, err := os.ReadFile(htmlPath)
	if err == nil {
		injected := strings.Replace(string(data), "</head>", fmt.Sprintf("<script>var APP_VERSION = \"%s\"; var USE_INTERNAL_ED = %t;</script></head>", APP_VERSION, appConfig.UseInternalEd), 1)
		w.Write([]byte(injected))
	} else {
		http.ServeFile(w, r, htmlPath)
	}
}

func recompileMarkdownPage(name, mdPath, htmlPath string, errMd error) {
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
		htmlStat, errHtml := os.Stat(htmlPath)
		mdStat, errMd := os.Stat(mdPath)
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

func serveConfigPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	body := getConfigPageBody()
	compiled := compilePageWithBody("Config", []byte("Title: Config\nCategory: Settings\n\n"), body)
	injected := strings.Replace(string(compiled), "</head>", fmt.Sprintf("<script>var APP_VERSION = \"%s\"; var USE_INTERNAL_ED = %t;</script></head>", APP_VERSION, appConfig.UseInternalEd), 1)
	w.Write([]byte(injected))
}

func serveEditor(w http.ResponseWriter, r *http.Request, path string) {
	relPath := strings.TrimPrefix(path, "/")

	if !appConfig.UseInternalEd {
		http.Redirect(w, r, "/api/edit-external?name="+url.QueryEscape(relPath), http.StatusSeeOther)
		return
	}

	var filePath string
	var rawContent []byte
	if strings.HasSuffix(relPath, ".md") {
		filePath = filepath.Join(storageDir, "md", filepath.Clean(relPath))
	} else {
		filePath = filepath.Join(storageDir, "html", filepath.Clean(relPath))
	}
	
	if data, err := os.ReadFile(filePath); err == nil {
		rawContent = data
	} else {
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, []byte{}, 0644)
		rawContent = []byte{}
	}
	
	escapedContent := htmlEscape(string(rawContent))
	customBody := "<pre style=\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\">" + escapedContent + "</pre>"
	compiled := compilePageWithBody(relPath, rawContent, customBody)
	
	scriptInjection := "<script>var IS_MARKDOWN = false; setTimeout(function(){ if(typeof toggleMode==='function') toggleMode(); }, 120);</script>"
	compiled = []byte(strings.Replace(string(compiled), "</head>", scriptInjection+"\n</head>", 1))
	w.Header().Set("Content-Type", "text/html")
	w.Write(compiled)
}

func serveStaticAsset(w http.ResponseWriter, r *http.Request, path string) {
	ext := strings.ToLower(filepath.Ext(path))
	mimeType, exists := appConfig.MimeTypes[ext]
	if !exists {
		mimeType = mime.TypeByExtension(ext)
	}
	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}

	filePath := filepath.Join(storageDir, "html", filepath.Clean(path))
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
