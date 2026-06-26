package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

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
