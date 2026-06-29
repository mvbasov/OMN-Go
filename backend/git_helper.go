package backend

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/filesystem"

	cryptossh "golang.org/x/crypto/ssh"
	gitconfig "github.com/go-git/go-git/v5/config"
)

// ----------------------------------------------------------------------
// Android filesystem workarounds
// ----------------------------------------------------------------------

type NoLockFS struct {
	billy.Filesystem
}

func (fs *NoLockFS) Create(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Create(filename)
	if err != nil {
		return nil, err
	}
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) Open(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Open(filename)
	if err != nil {
		return nil, err
	}
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	f, err := fs.Filesystem.OpenFile(filename, flag, perm)
	if err != nil {
		return nil, err
	}
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) TempFile(dir, prefix string) (billy.File, error) {
	f, err := fs.Filesystem.TempFile(dir, prefix)
	if err != nil {
		return nil, err
	}
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) Chroot(path string) (billy.Filesystem, error) {
	c, err := fs.Filesystem.Chroot(path)
	if err != nil {
		return nil, err
	}
	return &NoLockFS{c}, nil
}

type NoLockFile struct {
	billy.File
}

func (f *NoLockFile) Lock() error   { return nil }
func (f *NoLockFile) Unlock() error { return nil }

// ---------------------------------------------------------------
// Stable mtime wrapper (forces content‑hash based status check)
// ---------------------------------------------------------------

type stableMtimeFS struct {
	billy.Filesystem
}

func (fs *stableMtimeFS) Stat(path string) (os.FileInfo, error) {
	fi, err := fs.Filesystem.Stat(path)
	if err != nil {
		return nil, err
	}
	return &stableFileInfo{fi}, nil
}

func (fs *stableMtimeFS) Lstat(path string) (os.FileInfo, error) {
	fi, err := fs.Filesystem.Lstat(path)
	if err != nil {
		return nil, err
	}
	return &stableFileInfo{fi}, nil
}

type stableFileInfo struct {
	os.FileInfo
}

func (fi *stableFileInfo) ModTime() time.Time {
	return time.Unix(0, 0)
}

// ---------------------------------------------------------------
// Repository initialisation
// ---------------------------------------------------------------

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
	stableFS := &stableMtimeFS{baseFS}
	wtFS := &NoLockFS{stableFS}

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

// loadGitignoreMatcher returns a matcher for the worktree's .gitignore patterns.
func loadGitignoreMatcher(wt *git.Worktree) (gitignore.Matcher, error) {
	patterns, err := gitignore.ReadPatterns(wt.Filesystem, []string{})
	if err != nil {
		return nil, err
	}
	return gitignore.NewMatcher(patterns), nil
}

// ---------------------------------------------------------------
// Manual staging (bypasses go‑git’s Add entirely)
// ---------------------------------------------------------------

// manualStageFile streams the file content into a new blob and updates the index.
func manualStageFile(repo *git.Repository, wt *git.Worktree, name string) error {
	fullPath := filepath.Join(storageDir, name)
	stat, err := os.Lstat(fullPath)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return nil
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Stream file to object database (memory‑safe)
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, f); err != nil {
		w.Close()
		return err
	}
	w.Close()

	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return err
	}

	// Update or add index entry
	idx, err := repo.Storer.Index()
	if err != nil {
		return err
	}
	var entry *index.Entry
	for _, e := range idx.Entries {
		if e.Name == name {
			entry = e
			break
		}
	}
	if entry == nil {
		entry = &index.Entry{Name: name}
		idx.Entries = append(idx.Entries, entry)
	}
	entry.Hash = hash
	entry.Size = uint32(stat.Size())
	entry.ModifiedAt = stat.ModTime()
	entry.Mode = filemode.Regular

	return repo.Storer.SetIndex(idx)
}

// ---------------------------------------------------------------
// Staging & committing (manual staging with gitignore filter)
// ---------------------------------------------------------------

func commitLocalChanges(repo *git.Repository, wTree *git.Worktree) (bool, error) {
	// Load gitignore matcher
	matcher, err := loadGitignoreMatcher(wTree)
	if err != nil {
		log.Printf("[sync] Warning: could not load .gitignore: %v", err)
		matcher = gitignore.NewMatcher(nil) // no ignore
	}

	log.Printf("[sync] Checking worktree status")
	status, err := wTree.Status()
	if err != nil {
		return false, fmt.Errorf("status check error: %v", err)
	}
	if status.IsClean() {
		log.Printf("[sync] Nothing to commit")
		return false, nil
	}

	hasRealChanges := false
	for name, fileStat := range status {
		// Skip ignored files
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			log.Printf("[sync] Ignoring %s (matches .gitignore)", name)
			continue
		}

		if fileStat.Worktree == git.Deleted {
			log.Printf("[sync] Staging deletion: %s", name)
			_, err := wTree.Remove(name)
			if err != nil {
				log.Printf("[sync] Warning: failed to remove %s: %v", name, err)
			} else {
				hasRealChanges = true
			}
		} else if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			log.Printf("[sync] Staging file: %s", name)
			if err := manualStageFile(repo, wTree, name); err != nil {
				log.Printf("[sync] Warning: manual staging failed for %s: %v", name, err)
			} else {
				log.Printf("[sync] Staged %s successfully", name)
				hasRealChanges = true
			}
		}
	}

	if !hasRealChanges {
		log.Printf("[sync] No real changes could be staged (FUSE false-dirty or ignored)")
		return false, nil
	}

	// Ensure required directories exist
	repairAndroidGitDirs()

	log.Printf("[sync] Committing staged changes")
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
	if err == git.ErrEmptyCommit {
		log.Printf("[sync] Commit aborted: git.ErrEmptyCommit")
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("commit error: %v", err)
	}

	log.Printf("[sync] Committed with hash: %s", commitHash.String())
	return true, nil
}

// ---------------------------------------------------------------
// Sync operations (download / upload)
// ---------------------------------------------------------------

func executeSyncDownload(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, force bool) error {
	if force {
		log.Printf("[sync] Force Download: Fetching and Hard Resetting")

		if runtime.GOOS == "android" {
			tmpDir := filepath.Join(storageDir, ".git", "tmp")
			os.MkdirAll(tmpDir, 0755)
			os.Setenv("TMPDIR", tmpDir)
			ensureGitignore()
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
		if err != nil {
			return fmt.Errorf("hard reset failed: %v", err)
		}

		repo.Storer.SetReference(plumbing.NewHashReference(
			plumbing.ReferenceName("refs/heads/main"), ref.Hash()))
		repo.Storer.SetReference(plumbing.NewSymbolicReference(
			plumbing.HEAD, plumbing.ReferenceName("refs/heads/main")))
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

// ---------------------------------------------------------------
// HTTP handler
// ---------------------------------------------------------------

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

// ---------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------

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