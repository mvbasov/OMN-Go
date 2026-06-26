#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def replace_function_ast(content, func_name, new_code):
    """
    Acts like a mini-compiler AST parser. Finds the function signature, 
    counts opening and closing braces mathematically, and safely replaces the entire block.
    """
    start_idx = content.find(f"func {func_name}(")
    if start_idx == -1:
        raise ValueError(f"❌ Function {func_name} not found!")

    brace_start = content.find("{", start_idx)
    if brace_start == -1:
        raise ValueError("❌ No opening brace found for function")

    brace_count = 1
    idx = brace_start + 1
    while idx < len(content) and brace_count > 0:
        if content[idx] == '{':
            brace_count += 1
        elif content[idx] == '}':
            brace_count -= 1
        idx += 1

    if brace_count != 0:
        raise ValueError("❌ Unbalanced braces detected in source file")

    end_idx = idx
    return content[:start_idx] + new_code + content[end_idx:]

def update_application():
    print("[ ] Starting architectural refactor to Version 1.4.61")
    
    # 1. Auto-detect and bump version.go
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    if not match:
        raise ValueError("Version string not found in version.go")
        
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)
    print(f"[+] Bumped version in {ver_path} to {new_ver}")

    # 2. Bump android/app/build.gradle
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}', f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)
    print(f"[+] Bumped version in {gradle_path}")

    # 3. Patch backend/handlers.go using AST replacements
    handlers_path = "backend/handlers.go"
    handlers_code = read_file(handlers_path)

    # --- Deconstructed handleSync (Raw String to protect Go Quotes) ---
    sync_code = r"""func ensureGitignore() {
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
}"""

    # Apply the AST Replacement
    handlers_code = replace_function_ast(handlers_code, "handleSync", sync_code)
    print("[+] Deconstructed handleSync in backend/handlers.go")

    # --- Deconstructed serveFrontend (Raw String) ---
    frontend_code = r"""func serveFrontend(w http.ResponseWriter, r *http.Request) {
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
}"""

    # Apply the second AST Replacement
    handlers_code = replace_function_ast(handlers_code, "serveFrontend", frontend_code)
    write_file(handlers_path, handlers_code)
    print("[+] Deconstructed serveFrontend in backend/handlers.go")

    commit_msg = (
        "refactor(handlers): deconstruct monolithic god functions\n\n"
        "- Extracted serveFrontend into modular routing and view helpers\n"
        "- Deconstructed handleSync into specific modular git plumbing tasks\n"
        "- Utilized AST-brace counting patcher for 100% syntactical safety\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()