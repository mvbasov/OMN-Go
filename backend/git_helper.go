package backend

import (
	"runtime"
	"fmt"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	cryptossh "golang.org/x/crypto/ssh"
	gitconfig "github.com/go-git/go-git/v5/config"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

func ensureGitignore() {
	gitignorePath := filepath.Join(storageDir, ".gitignore")
	gitignoreBase := "# OMN-Go sync ignore\nconfig.json\n*.html\n/md/local/\n"
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		os.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)
		log.Printf("[sync] Created .gitignore")
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
			URLs: []string{appConfig.GitServers[appConfig.ActiveGitIndex].URL},
		})
		if err != nil {
			return nil, fmt.Errorf("remote add failed: %v", err)
		}
	}
	return repo, nil
}



func getSSHAuth() (transport.AuthMethod, error) {
	sshUser := "git"
	if idx := strings.Index(appConfig.GitServers[appConfig.ActiveGitIndex].URL, "@"); idx != -1 {
		sshUser = appConfig.GitServers[appConfig.ActiveGitIndex].URL[:idx]
	}
	log.Printf("[sync] SSH user: %s", sshUser)

	keyData := appConfig.GitServers[appConfig.ActiveGitIndex].SSHKeyData
	if keyData == "" {
		log.Printf("[sync] Error: No SSH key configured")
		return nil, fmt.Errorf("no SSH key configured")
	}

	var signer cryptossh.Signer
	var err error
	passphrase := appConfig.GitServers[appConfig.ActiveGitIndex].Password
	if passphrase == "" {
		signer, err = cryptossh.ParsePrivateKey([]byte(keyData))
	} else {
		signer, err = cryptossh.ParsePrivateKeyWithPassphrase([]byte(keyData), []byte(passphrase))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key: %v", err)
	}

	publicKeys := &gitssh.PublicKeys{User: sshUser, Signer: signer}
	publicKeys.HostKeyCallbackHelper = gitssh.HostKeyCallbackHelper{
		HostKeyCallback: cryptossh.InsecureIgnoreHostKey(),
	}
	log.Printf("[sync] SSH auth method created using inline key data")
	return publicKeys, nil
}

func commitLocalChanges(repo *git.Repository, wTree *git.Worktree) (bool, error) {
	log.Printf("[sync] Staging all changes")
	// FIX: Android FUSE filesystem bug causes AddWithOptions to miss deleted files.
	wkStatus, _ := wTree.Status()
	for path, fileStatus := range wkStatus {
		if fileStatus.Worktree == git.Deleted {
			wTree.Remove(path)
		} else if fileStatus.Worktree != git.Unmodified && fileStatus.Worktree != git.Untracked {
			wTree.Add(path)
		}
	}
	// BULLETPROOF STAGING: Manually iterate to avoid FUSE entry bugs
	{
		wkStatus, _ := wTree.Status()
		for path, fileStatus := range wkStatus {
			if fileStatus.Worktree == git.Deleted {
				wTree.Remove(path)
			} else if fileStatus.Worktree != git.Unmodified {
				wTree.Add(path)
			}
		}
	}
	var err error // Clear legacy AddWithOptions error state
	if err != nil {
		log.Printf("[LOG] [GO] [sync] Staging warning (Ignored, proceeding): %v", err)
	}
	status, _ := wTree.Status()
	if status.IsClean() {
		log.Printf("[sync] Nothing to commit")
		return false, nil
	}
	
	log.Printf("[sync] Uncommitted changes detected, committing via go-git")
	authorName := GetConfigAuthor()
	authorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"
	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}

	commitHash, err := wTree.Commit("Local changes before sync", &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err != nil {
		return false, fmt.Errorf("commit error: %v", err)
	}
	log.Printf("[sync] Committed with hash: %s", commitHash.String())

	return true, nil
}

func executeSyncDownload(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, force bool) error {
	if force {
		log.Printf("[sync] Force Download: Fetching and Hard Resetting")
		// Fix Android TMPDIR constraint for go-git packfiles
		if runtime.GOOS == "android" {
			tmpDir := "/storage/emulated/0/Android/media/net.basov.omngo/.git/tmp"
			os.MkdirAll(tmpDir, 0755)
			os.Setenv("TMPDIR", tmpDir)
		autoGitIgnore(".git/tmp") // Lock out temporary git files
		}
		err := repo.Fetch(&git.FetchOptions{RemoteName: "origin", Auth: auth})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return fmt.Errorf("fetch failed: %v", err)
		}
		ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", "master"), true)
		if err != nil {
			return fmt.Errorf("failed to find origin/master: %v", err)
		}
		err = wTree.Checkout(&git.CheckoutOptions{
			Hash:  ref.Hash(),
			Force: true,
		})
		// Fix detached HEAD and update branch ref safely
		repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/main"), ref.Hash()))
		repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main")))
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

	if appConfig.GitServers[appConfig.ActiveGitIndex].URL == "" {
		log.Printf("[sync] Error: sync_remote not configured")
		http.Error(w, "Sync not configured: missing sync_remote in config.json", 500)
		return
	}

	log.Printf("[sync] Remote: %s", appConfig.GitServers[appConfig.ActiveGitIndex].URL)

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

	committed, err := commitLocalChanges(repo, wTree)
	if err != nil {
		http.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)
		return
	}
	if !committed && action == "upload" {
		w.Write([]byte("Nothing to push"))
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


// [OMN-Go 1.5.8] Strong Empty Commit Check & Gitignore Enforcer
func safeCommit(w *git.Worktree, msg string, opts *git.CommitOptions) (plumbing.Hash, error) {
	status, err := w.Status()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if status.IsClean() {
		// Strong check: explicitly bypass commit if tree is perfectly clean
		return plumbing.ZeroHash, nil
	}
	// Fix Android FUSE missing directory bug causing writeTreeFromDir crashes
	os.MkdirAll(filepath.Join(storageDir, ".git", "objects", "pack"), 0755)
	os.MkdirAll(filepath.Join(storageDir, ".git", "objects", "info"), 0755)
	return w.Commit(msg, opts)
}


// repairAndroidGitDirs fixes the Android FUSE Media Scanner bug by forcing 
// the recreation of empty git directories immediately before any commit.
func repairAndroidGitDirs() {
	if runtime.GOOS == "android" {
		gitRoot := filepath.Join(storageDir, ".git")
		os.MkdirAll(filepath.Join(gitRoot, "objects", "pack"), 0755)
		os.MkdirAll(filepath.Join(gitRoot, "objects", "info"), 0755)
		os.MkdirAll(filepath.Join(gitRoot, "refs", "heads"), 0755)
		os.MkdirAll(filepath.Join(gitRoot, "refs", "tags"), 0755)
		os.MkdirAll(filepath.Join(gitRoot, "refs", "remotes", "origin"), 0755)
	}
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
