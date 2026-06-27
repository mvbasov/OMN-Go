package backend

import (
	"fmt"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	cryptossh "golang.org/x/crypto/ssh"
	gitconfig "github.com/go-git/go-git/v5/config"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
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
	
	baseFS := osfs.New(storageDir)
	wtFS := &NoLockFS{baseFS}
	dotFS, err := wtFS.Chroot(".git")
	if err != nil {
		return nil, fmt.Errorf("chroot .git failed: %v", err)
	}
	
	storer := filesystem.NewStorage(dotFS, cache.NewObjectLRUDefault())
	repo, err := git.Open(storer, wtFS)
	
	if err != nil {
		log.Printf("[sync] Repo not found, initializing...")
		if initErr := manualGitInit(storageDir); initErr != nil {
			return nil, fmt.Errorf("manual init failed: %v", initErr)
		}
		repo, err = git.Open(storer, wtFS)
		if err != nil {
			return nil, fmt.Errorf("failed to open manually created repo: %v", err)
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
func GetConfigAuthor() string {
	if appConfig.Author != "" {
		return appConfig.Author
	}
	return "OMN-Go User"
}

func GetInsecureSSHAuth(user, keyPath, passphrase string) (transport.AuthMethod, error) {
	publicKeys, err := gitssh.NewPublicKeysFromFile(user, keyPath, passphrase)
	if err != nil {
		return nil, err
	}
	publicKeys.HostKeyCallbackHelper = gitssh.HostKeyCallbackHelper{
		HostKeyCallback: cryptossh.InsecureIgnoreHostKey(),
	}
	return publicKeys, nil
}
// --- Android Flock() Bypass ---
// Android's sdcardfs does not implement file locking (ENOSYS / function not implemented).
// This wrapper neutralizes Lock() calls gracefully across go-git operations.

type NoLockFS struct {
	billy.Filesystem
}

func (fs *NoLockFS) Create(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Create(filename)
	if err != nil { return nil, err }
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) Open(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Open(filename)
	if err != nil { return nil, err }
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	f, err := fs.Filesystem.OpenFile(filename, flag, perm)
	if err != nil { return nil, err }
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) TempFile(dir, prefix string) (billy.File, error) {
	f, err := fs.Filesystem.TempFile(dir, prefix)
	if err != nil { return nil, err }
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) Chroot(path string) (billy.Filesystem, error) {
	c, err := fs.Filesystem.Chroot(path)
	if err != nil { return nil, err }
	return &NoLockFS{c}, nil
}

type NoLockFile struct {
	billy.File
}

func (f *NoLockFile) Lock() error {
	return nil // Safely bypass Android flock ENOSYS
}

func (f *NoLockFile) Unlock() error {
	return nil // Safely bypass Android flock ENOSYS
}

