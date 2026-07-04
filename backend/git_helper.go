package backend

import (
	"encoding/json"
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

func (a *App) ensureGitignore() {
	gitignorePath := filepath.Join(a.StorageDir, ".gitignore")
	//gitignoreBase := "# OMN-Go sync ignore\nconfig.json\n*.html\n/md/local/\n"
	gitignoreBase := `
# OMN-Go sync ignore
config.json
*.html
*.woff2
/html/css/omn-go-core.css
/html/js/omn-go-core.js
/html/js/omn-go-sse.js
/md/local/
`
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		os.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)
		log.Printf("[sync] Created .gitignore")
	}
}

func (a *App) getOrInitRepo() (*git.Repository, error) {
	log.Printf("[sync] Opening repo at %s", a.StorageDir)

	baseFS := osfs.New(a.StorageDir)
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
		if initErr := a.manualGitInit(a.StorageDir); initErr != nil {
			return nil, fmt.Errorf("manual init failed: %v", initErr)
		}
		repo, err = git.Open(storer, wtFS)
		if err != nil {
			return nil, fmt.Errorf("failed to open manually created repo: %v", err)
		}
		a.ensureGitignore()
		log.Printf("[sync] Repo initialized")
	} else {
		log.Printf("[sync] Repo opened successfully")
	}

	// Remote setup/selection happens separately, in
	// ensureRemotesAndGetActive — only sync operations need it (a plain
	// status/preview read doesn't touch any remote), so it isn't done
	// unconditionally here.
	return repo, nil
}

// ---------------------------------------------------------------
// Remote management — one git remote per configured server slot
// ---------------------------------------------------------------
//
// Earlier revisions of this file kept a single "origin" remote and rewrote
// its URL to match whichever server slot was active. That turned out to be
// unwanted: switching the active slot (or editing its URL) would silently
// repoint "origin" every time, with no separate history/identity per
// server. The model here instead is:
//
//   - "origin" is a one-time bootstrap remote. It's created only if it
//     doesn't already exist (seeded from whatever server happens to be
//     active at that moment) and is never modified again afterwards. It
//     exists purely as a fallback for the case where the active slot has
//     no URL configured — not as something that tracks config changes.
//   - Every server slot with a non-empty URL gets its own persistent
//     remote, named deterministically by slot index ("gitserver0" ..
//     "gitserver4") rather than by the user-editable "Name" field, so
//     renaming a server in Config doesn't orphan its remote. These ARE
//     kept in sync with config on every call: added when a slot gains a
//     URL, updated when a slot's URL changes, removed when a slot is
//     cleared.
//   - Sync operations use whichever remote corresponds to the currently
//     active slot, falling back to "origin" only if that slot has no URL.

// slotRemoteName returns the deterministic git remote name for a given
// GitServers slot index.
func slotRemoteName(index int) string {
	return fmt.Sprintf("gitserver%d", index)
}

// ensureOriginRemote creates "origin" the first time it's missing, seeded
// from fallbackURL, and otherwise leaves it untouched.
func (a *App) ensureOriginRemote(repo *git.Repository, fallbackURL string) error {
	if _, err := repo.Remote("origin"); err == nil {
		return nil // already exists — this remote is never modified again
	}
	if fallbackURL == "" {
		return nil // nothing to seed it with yet; try again on a later sync
	}
	log.Printf("[sync] Remote origin missing, seeding it once from %s", fallbackURL)
	_, err := repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{fallbackURL},
	})
	return err
}

// ensureSlotRemotes adds/updates/removes one remote per GitServers slot to
// match cfg, and returns the remote name a sync should use: the active
// slot's own remote if it has a URL configured, or "origin" as a fallback.
func (a *App) ensureSlotRemotes(repo *git.Repository, cfg Config) (activeRemoteName string, err error) {
	for i, gs := range cfg.GitServers {
		name := slotRemoteName(i)
		url := strings.TrimSpace(gs.URL)

		remote, rErr := repo.Remote(name)
		if url == "" {
			if rErr == nil {
				log.Printf("[sync] Removing remote %s (slot %d cleared)", name, i)
				if dErr := repo.DeleteRemote(name); dErr != nil {
					log.Printf("[sync] Warning: failed to remove remote %s: %v", name, dErr)
				}
			}
			continue
		}

		if rErr != nil {
			log.Printf("[sync] Adding remote %s -> %s", name, url)
			if _, cErr := repo.CreateRemote(&gitconfig.RemoteConfig{Name: name, URLs: []string{url}}); cErr != nil {
				return "", fmt.Errorf("failed to add remote %s: %v", name, cErr)
			}
			continue
		}

		existing := remote.Config().URLs
		if len(existing) == 1 && existing[0] == url {
			continue // already up to date
		}
		log.Printf("[sync] Remote %s URL changed (%v -> %s), updating", name, existing, url)
		if dErr := repo.DeleteRemote(name); dErr != nil {
			return "", fmt.Errorf("failed to update remote %s: %v", name, dErr)
		}
		if _, cErr := repo.CreateRemote(&gitconfig.RemoteConfig{Name: name, URLs: []string{url}}); cErr != nil {
			return "", fmt.Errorf("failed to update remote %s: %v", name, cErr)
		}
	}

	if cfg.ActiveGitIndex >= 0 && cfg.ActiveGitIndex < len(cfg.GitServers) {
		if strings.TrimSpace(cfg.GitServers[cfg.ActiveGitIndex].URL) != "" {
			return slotRemoteName(cfg.ActiveGitIndex), nil
		}
	}
	log.Printf("[sync] Active server slot has no URL configured, falling back to origin")
	return "origin", nil
}

// ensureRemotesAndGetActive reconciles all git remotes against the current
// config (see the block comment above) and returns which remote name the
// caller should use for this sync.
func (a *App) ensureRemotesAndGetActive(repo *git.Repository) (string, error) {
	cfg := a.GetConfig()

	bootstrapURL := ""
	if cfg.ActiveGitIndex >= 0 && cfg.ActiveGitIndex < len(cfg.GitServers) {
		bootstrapURL = strings.TrimSpace(cfg.GitServers[cfg.ActiveGitIndex].URL)
	}
	if err := a.ensureOriginRemote(repo, bootstrapURL); err != nil {
		return "", err
	}

	return a.ensureSlotRemotes(repo, cfg)
}


func (a *App) manualGitInit(dir string) error {
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
	a.protectGitDirs()
	a.ensureGitignore()

	config := []byte("[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n")
	if err := os.WriteFile(filepath.Join(gitDir, "config"), config, 0644); err != nil {
		return err
	}
	return nil
}

// a.loadGitignoreMatcher returns a matcher for the worktree's .gitignore patterns.
func (a *App) loadGitignoreMatcher(wt *git.Worktree) (gitignore.Matcher, error) {
	patterns, err := gitignore.ReadPatterns(wt.Filesystem, []string{})
	if err != nil {
		return nil, err
	}
	return gitignore.NewMatcher(patterns), nil
}

// ---------------------------------------------------------------
// Manual staging (bypasses go‑git’s Add entirely)
// ---------------------------------------------------------------

// a.manualStageFile streams the file content into a new blob and updates the index.
func (a *App) manualStageFile(repo *git.Repository, wt *git.Worktree, name string) error {
	fullPath := filepath.Join(a.StorageDir, name)
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
// SSH authentication
// ---------------------------------------------------------------

func (a *App) getSSHAuth() (transport.AuthMethod, error) {
	// Take one consistent snapshot instead of four separate reads of
	// a.Config — otherwise a concurrent /api/config POST could change
	// ActiveGitIndex or GitServers between reads and mix fields from two
	// different server entries.
	cfg := a.GetConfig()
	gs := cfg.GitServers[cfg.ActiveGitIndex]

	sshUser := "git"
	if idx := strings.Index(gs.URL, "@"); idx != -1 {
		sshUser = gs.URL[:idx]
	}
	log.Printf("[sync] SSH user: %s", sshUser)

	keyData := gs.SSHKeyData
	if keyData == "" {
		log.Printf("[sync] Error: No SSH key configured")
		return nil, fmt.Errorf("no SSH key configured")
	}

	var signer cryptossh.Signer
	var err error
	passphrase := gs.Password
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

// ---------------------------------------------------------------
// Staging & committing (manual staging with gitignore filter)
// ---------------------------------------------------------------

func (a *App) commitLocalChanges(repo *git.Repository, wTree *git.Worktree, message string) (bool, error) {
	// Load gitignore matcher
	matcher, err := a.loadGitignoreMatcher(wTree)
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

		// Exclude root config.json explicitly
		if name == "config.json" {
			log.Printf("[sync] Ignoring root config.json (preserve locally)")
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
			if err := a.manualStageFile(repo, wTree, name); err != nil {
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

	log.Printf("[sync] Committing staged changes")
	authorName := a.GetConfigAuthor()
	authorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"
	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}

	commitHash, err := wTree.Commit(message, &git.CommitOptions{
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
// Pre-merge checkpoint (backs "pull_abort")
// ---------------------------------------------------------------
//
// pull_mark moves the local branch ref forward to the remote tip so a
// subsequent commit can fast-forward-push cleanly once the user resolves
// the injected conflict markers. pull_abort needs to be able to undo that,
// so we stash the local HEAD hash from *before* pull_mark ran in a small
// file under .git/. Using a file (rather than an in-memory App field) means
// this survives an app restart and needs no changes to the App struct in
// server.go.

func (a *App) premergeHeadPath() string {
	return filepath.Join(a.StorageDir, ".git", "OMNGO_PREMERGE_HEAD")
}

func (a *App) savePremergeHead(h plumbing.Hash) {
	if err := os.WriteFile(a.premergeHeadPath(), []byte(h.String()), 0644); err != nil {
		log.Printf("[sync] failed to save pre-merge HEAD: %v", err)
	}
}

func (a *App) loadPremergeHead() (plumbing.Hash, bool) {
	data, err := os.ReadFile(a.premergeHeadPath())
	if err != nil {
		return plumbing.ZeroHash, false
	}
	h := plumbing.NewHash(strings.TrimSpace(string(data)))
	if h.IsZero() {
		return plumbing.ZeroHash, false
	}
	return h, true
}

func (a *App) clearPremergeHead() {
	os.Remove(a.premergeHeadPath())
}

// ---------------------------------------------------------------
// Force-pull cleanup
// ---------------------------------------------------------------

// cleanUntrackedFiles deletes any file the worktree reports as Untracked,
// *unless* it matches a .gitignore pattern. Only "force pull" calls this —
// a plain pull/push must never touch files outside git's own tracked set.
func (a *App) cleanUntrackedFiles(wTree *git.Worktree, matcher gitignore.Matcher) {
	status, err := wTree.Status()
	if err != nil {
		log.Printf("[sync] force pull: could not compute status for cleanup: %v", err)
		return
	}
	for name, fileStat := range status {
		if fileStat.Worktree != git.Untracked {
			continue
		}
		// Explicit safety net, same as commitLocalChanges/syncPush/
		// handleSyncPreview: config.json holds this device's local
		// admin/guest passwords and server list and must never be
		// touched by sync, regardless of what .gitignore currently
		// says. Relying on the matcher alone isn't safe here — a force
		// pull's checkout can leave .gitignore in a state (fetched from
		// remote, possibly without this line, or momentarily stale)
		// where the matcher no longer protects it, which is exactly
		// what deleted it.
		if name == "config.json" {
			log.Printf("[sync] force pull: keeping root config.json (preserve locally)")
			continue
		}
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			log.Printf("[sync] force pull: keeping ignored file %s", name)
			continue
		}
		full := filepath.Join(a.StorageDir, name)
		if err := os.Remove(full); err != nil {
			log.Printf("[sync] force pull: failed to delete %s: %v", name, err)
		} else {
			log.Printf("[sync] force pull: deleted untracked file %s", name)
		}
	}
}

// ---------------------------------------------------------------
// pull
// ---------------------------------------------------------------

// syncPull fast-forwards local to origin/master when possible. When it is
// not possible (diverged history, or unstaged local changes in the way) it
// returns the CONFLICT_DETECTED sentinel so the caller can offer the user a
// choice between "pull_abort" and "pull_mark" (3-way merge).
func (a *App) syncPull(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	log.Printf("[sync] Pull: fetching %s", remoteName)
	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %v", err)
	}

	err = wTree.Pull(&git.PullOptions{
		RemoteName: remoteName,
		Auth:       auth,
		Force:      false,
	})
	if err == git.ErrNonFastForwardUpdate || err == git.ErrUnstagedChanges {
		log.Printf("[sync] Pull: fast-forward not possible (%v)", err)
		return fmt.Errorf("CONFLICT_DETECTED")
	}
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("pull failed: %v", err)
	}
	log.Printf("[sync] Pull: fast-forward complete")
	return nil
}

// syncPullMerge implements the "3 way diff merge" option offered after a
// pull conflict. For every locally modified file that also differs from
// the remote copy, it writes standard diff3-style conflict markers
// (<<<<<<< LOCAL / ||||||| BASE / ======= / >>>>>>> REMOTE) using the
// git merge-base as the BASE section where one can be found. The local
// branch ref is then moved to the remote tip (working tree contents are
// preserved) so that once the user hand-resolves the markers and commits,
// that commit's parent is the remote tip and a normal push can
// fast-forward. The pre-merge HEAD is saved so "pull_abort" can undo this.
func (a *App) syncPullMerge(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %v", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("remote master not found: %v", err)
	}

	localHead, err := repo.Head()
	if err != nil {
		return fmt.Errorf("local HEAD not found: %v", err)
	}
	a.savePremergeHead(localHead.Hash())

	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return fmt.Errorf("remote commit lookup failed: %v", err)
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return fmt.Errorf("remote tree lookup failed: %v", err)
	}

	var baseTree *object.Tree
	if localCommit, cErr := repo.CommitObject(localHead.Hash()); cErr == nil {
		if bases, mErr := localCommit.MergeBase(remoteCommit); mErr == nil && len(bases) > 0 {
			baseTree, _ = bases[0].Tree()
		}
	}

	status, err := wTree.Status()
	if err != nil {
		return fmt.Errorf("status error: %v", err)
	}

	for path, fileStatus := range status {
		if fileStatus.Worktree != git.Modified && fileStatus.Staging != git.Modified {
			continue
		}

		file, err := wTree.Filesystem.Open(path)
		if err != nil {
			continue
		}
		localContent, _ := io.ReadAll(file)
		file.Close()

		remoteFile, err := remoteTree.File(path)
		if err != nil {
			continue // remote doesn't have this file — nothing to reconcile
		}
		remoteContentStr, _ := remoteFile.Contents()
		if string(localContent) == remoteContentStr {
			continue
		}

		baseSection := ""
		if baseTree != nil {
			if baseFile, bErr := baseTree.File(path); bErr == nil {
				if baseContent, cErr := baseFile.Contents(); cErr == nil && baseContent != string(localContent) {
					baseSection = fmt.Sprintf("||||||| BASE\n%s", baseContent)
				}
			}
		}

		conflictText := fmt.Sprintf("<<<<<<< LOCAL (Your changes)\n%s%s=======\n%s>>>>>>> REMOTE (Incoming from origin)\n",
			string(localContent), baseSection, remoteContentStr)

		if outFile, oErr := wTree.Filesystem.OpenFile(path, os.O_RDWR|os.O_TRUNC, 0644); oErr == nil {
			outFile.Write([]byte(conflictText))
			outFile.Close()
		}
	}

	if err := wTree.Reset(&git.ResetOptions{Commit: remoteRef.Hash(), Mode: git.MixedReset}); err != nil {
		return fmt.Errorf("reset to remote failed: %v", err)
	}
	log.Printf("[sync] Pull: 3-way conflict markers written, awaiting manual resolution")
	return nil
}

// syncPullAbort discards an in-progress pull_mark, restoring local state
// (both the branch ref and the working tree) to what it was immediately
// before pull_mark ran. If there is nothing to abort, this is a no-op.
func (a *App) syncPullAbort(wTree *git.Worktree) error {
	hash, ok := a.loadPremergeHead()
	if !ok {
		log.Printf("[sync] pull_abort: nothing to abort")
		return nil
	}
	if err := wTree.Reset(&git.ResetOptions{Commit: hash, Mode: git.HardReset}); err != nil {
		return fmt.Errorf("abort reset failed: %v", err)
	}
	a.clearPremergeHead()
	log.Printf("[sync] pull_abort: restored local state to %s", hash.String())
	return nil
}

// syncPullForce resets local to exactly match origin/master, then deletes
// any file that is neither tracked by git nor covered by .gitignore, per
// the requirement that only a *force* pull is allowed to delete such files.
func (a *App) syncPullForce(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	log.Printf("[sync] Force pull: fetching %s", remoteName)

	if runtime.GOOS == "android" {
		tmpDir := filepath.Join(a.StorageDir, ".git", "tmp")
		os.MkdirAll(tmpDir, 0755)
		os.Setenv("TMPDIR", tmpDir)
		a.ensureGitignore()
	}

	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %v", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("failed to find %s/master: %v", remoteName, err)
	}

	if err := repo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.ReferenceName("refs/heads/master"), remoteRef.Hash())); err != nil {
		return fmt.Errorf("failed to move local branch: %v", err)
	}

	if err := wTree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName("refs/heads/master"),
		Force:  true,
	}); err != nil {
		return fmt.Errorf("checkout failed: %v", err)
	}

	matcher, mErr := a.loadGitignoreMatcher(wTree)
	if mErr != nil {
		log.Printf("[sync] force pull: could not load .gitignore, skipping untracked cleanup: %v", mErr)
	} else {
		a.cleanUntrackedFiles(wTree, matcher)
	}

	a.clearPremergeHead() // any pending 3-way merge is now moot
	log.Printf("[sync] Force pull complete")
	return nil
}

// ---------------------------------------------------------------
// push
// ---------------------------------------------------------------

// hasUnpushedCommits reports whether local HEAD differs from the given
// remote's master after a fresh fetch — used by a plain "push" to decide
// whether there is really nothing to do when the working tree is clean.
func (a *App) hasUnpushedCommits(repo *git.Repository, auth transport.AuthMethod, remoteName string) (bool, error) {
	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return false, err
	}
	localHead, err := repo.Head()
	if err != nil {
		return false, err
	}
	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		// No remote ref yet (e.g. first-ever push) — treat as ahead.
		return true, nil
	}
	return localHead.Hash() != remoteRef.Hash(), nil
}

// syncPush implements both "push" and "force push":
//
//   - If there is nothing uncommitted and nothing already ahead of the
//     remote, it stops (returns nil) without contacting the remote.
//   - If there ARE uncommitted local changes, a commit message is
//     required; without one it returns COMMIT_MESSAGE_REQUIRED.
//   - A force push always requires a commit message up front (even if
//     there happen to be no pending changes to commit), since it is a
//     destructive operation on the remote.
//   - A non-force push that is rejected as non-fast-forward returns
//     PUSH_CONFLICT_DETECTED and otherwise does nothing further — per
//     spec, no auto-pull/auto-merge is attempted.
//   - A force push always pushes with Force:true, overwriting the remote
//     to match local regardless of divergence.
func (a *App) syncPush(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName, message string, force bool) error {
	matcher, mErr := a.loadGitignoreMatcher(wTree)
	if mErr != nil {
		matcher = gitignore.NewMatcher(nil)
	}
	status, err := wTree.Status()
	if err != nil {
		return fmt.Errorf("status error: %v", err)
	}

	hasRelevantChanges := false
	for name, fileStat := range status {
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			continue
		}
		if name == "config.json" {
			continue
		}
		if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			hasRelevantChanges = true
			break
		}
	}

	needsMessage := hasRelevantChanges || force
	if needsMessage && strings.TrimSpace(message) == "" {
		return fmt.Errorf("COMMIT_MESSAGE_REQUIRED")
	}

	if hasRelevantChanges {
		if _, cErr := a.commitLocalChanges(repo, wTree, message); cErr != nil {
			return fmt.Errorf("commit failed: %v", cErr)
		}
	}

	if !force && !hasRelevantChanges {
		ahead, aErr := a.hasUnpushedCommits(repo, auth, remoteName)
		if aErr == nil && !ahead {
			log.Printf("[sync] push: nothing to commit, nothing to push")
			return nil
		}
	}

	log.Printf("[sync] Pushing to %s master (force=%v)", remoteName, force)
	err = repo.Push(&git.PushOptions{
		RemoteName: remoteName,
		Auth:       auth,
		RefSpecs:   []gitconfig.RefSpec{"refs/heads/master:refs/heads/master"},
		Force:      force,
	})
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}
	if err != nil {
		if !force && err == git.ErrNonFastForwardUpdate {
			log.Printf("[sync] push: rejected as non-fast-forward, leaving local state untouched")
			return fmt.Errorf("PUSH_CONFLICT_DETECTED")
		}
		return fmt.Errorf("push failed: %v", err)
	}
	return nil
}

// ---------------------------------------------------------------
// Dispatcher
// ---------------------------------------------------------------

// SyncRepo runs a git sync action with no commit message. Kept exported and
// with its original single-argument signature for backward compatibility
// with any existing caller. New code — including handleSync below — should
// call SyncRepoWithMessage, which additionally accepts a commit message for
// the "push"/"push_force" actions.
func (a *App) SyncRepo(action string) error {
	return a.SyncRepoWithMessage(action, "")
}

// SyncRepoWithMessage implements the four required git operations:
//
//	pull        fast-forward if possible; returns the CONFLICT_DETECTED
//	            sentinel error if it is not (diverged history, or local
//	            changes in the way). Aliases: "pull_ff", "download".
//	pull_mark   after a "pull" conflict: writes 3-way conflict markers so
//	            the user can resolve them by hand ("make 3 way diff merge")
//	pull_abort  after a "pull" conflict: discards it, restoring local state.
//	            Alias: "abort".
//	pull_force  resets local to exactly match remote; also deletes any file
//	            that is neither tracked nor covered by .gitignore.
//	            Alias: "download_force".
//	push        commits (with the given message) if there are local
//	            changes, then pushes; does nothing further on conflict.
//	            Alias: "upload".
//	push_force  commits (with the given message) if needed, then
//	            force-pushes, resetting the remote to local state.
//	            Alias: "upload_force".
//
// All repo mutation is serialized via a.GitMutex, so this is safe to call
// from multiple goroutines at once (e.g. concurrent HTTP requests).
func (a *App) SyncRepoWithMessage(action string, message string) error {
	a.GitMutex.Lock()
	defer a.GitMutex.Unlock()

	repo, err := a.getOrInitRepo()
	if err != nil {
		return err
	}
	wTree, err := repo.Worktree()
	if err != nil {
		return err
	}
	remoteName, err := a.ensureRemotesAndGetActive(repo)
	if err != nil {
		return err
	}
	auth, err := a.getSSHAuth()
	if err != nil {
		return err
	}

	switch action {
	case "push", "upload":
		return a.syncPush(repo, wTree, auth, remoteName, message, false)
	case "push_force", "upload_force":
		return a.syncPush(repo, wTree, auth, remoteName, message, true)
	case "pull", "pull_ff", "download":
		return a.syncPull(repo, wTree, auth, remoteName)
	case "pull_mark":
		return a.syncPullMerge(repo, wTree, auth, remoteName)
	case "pull_abort", "abort":
		return a.syncPullAbort(wTree)
	case "pull_force", "download_force":
		return a.syncPullForce(repo, wTree, auth, remoteName)
	}
	return fmt.Errorf("unknown sync action: %s", action)
}

// ---------------------------------------------------------------
// HTTP handler
// ---------------------------------------------------------------

// writeSyncJSON writes a small {"status":..., "message":...} JSON body.
// Using json.Marshal (rather than fmt.Sprintf-ing a JSON literal, as the
// original code did) avoids producing invalid JSON when an error message
// happens to contain a quote or backslash.
func writeSyncJSON(w http.ResponseWriter, status, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status, "message": message})
}

func (a *App) handleSync(w http.ResponseWriter, r *http.Request) {
	// r.FormValue reads from both the URL query string and a POST body
	// (application/x-www-form-urlencoded or multipart). The frontend uses
	// both conventions in different places — omn-go-sse.js posts
	// action/force/message in the body, while the conflict-resolution
	// buttons in index.html hit this endpoint with a query string — so
	// this handler needs to accept either.
	if err := r.ParseForm(); err != nil {
		writeSyncJSON(w, "error", fmt.Sprintf("bad request: %v", err))
		return
	}

	action := r.FormValue("action")
	if action == "" {
		action = "pull"
	}
	message := r.FormValue("message")
	force := r.FormValue("force") == "true"

	// The UI's "Force" checkbox is a separate field, not a distinct action
	// name — translate it into the canonical *_force action here so
	// SyncRepoWithMessage only has to deal with one vocabulary.
	if force {
		switch action {
		case "pull", "pull_ff", "download":
			action = "pull_force"
		case "push", "upload":
			action = "push_force"
		}
	}

	err := a.SyncRepoWithMessage(action, message)
	if err != nil {
		switch err.Error() {
		case "CONFLICT_DETECTED":
			writeSyncJSON(w, "conflict", "Fast-forward not possible. Choose abort or 3-way merge.")
		case "PUSH_CONFLICT_DETECTED":
			writeSyncJSON(w, "push_conflict", "Remote has new commits. Pull before pushing.")
		case "COMMIT_MESSAGE_REQUIRED":
			writeSyncJSON(w, "needs_commit_message", "Please provide a commit message.")
		default:
			writeSyncJSON(w, "error", err.Error())
		}
		return
	}

	writeSyncJSON(w, "success", "")
}

func (a *App) handleSyncPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	action := r.URL.Query().Get("action")
	if action != "upload" {
		http.Error(w, "Only upload preview supported", 400)
		return
	}

	// Same reasoning as handleSync: don't let a status/preview read run
	// concurrently with an in-progress checkout/reset from another sync.
	a.GitMutex.Lock()
	defer a.GitMutex.Unlock()

	repo, err := a.getOrInitRepo()
	if err != nil {
		http.Error(w, fmt.Sprintf("Repo init failed: %v", err), 500)
		return
	}
	wTree, err := repo.Worktree()
	if err != nil {
		http.Error(w, fmt.Sprintf("Worktree error: %v", err), 500)
		return
	}

	matcher, err := a.loadGitignoreMatcher(wTree)
	if err != nil {
		matcher = gitignore.NewMatcher(nil)
	}

	status, err := wTree.Status()
	if err != nil {
		http.Error(w, fmt.Sprintf("Status error: %v", err), 500)
		return
	}

	var files []string
	for name, fileStat := range status {
		// Skip ignored and root config.json
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			continue
		}
		if name == "config.json" {
			continue
		}
		if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			files = append(files, name)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// ---------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------

func (a *App) GetConfigAuthor() string {
	if author := a.GetConfig().Author; author != "" {
		return author
	}
	return "OMN-Go User"
}

// NOTE: GetInsecureSSHAuth has no callers anywhere in this Go codebase, and
// with MainActivity.java confirmed to only call Backend.startServer() (all
// sync traffic goes through the HTTP handlers, same as desktop), there is
// no evidence any caller needs it. Still left in place rather than
// commented out, since it's exported and this file can't rule out every
// possible native caller. Safe to delete once you've confirmed that.
func (a *App) GetInsecureSSHAuth(user, keyPath, passphrase string) (transport.AuthMethod, error) {
	publicKeys, err := gitssh.NewPublicKeysFromFile(user, keyPath, passphrase)
	if err != nil {
		return nil, err
	}
	publicKeys.HostKeyCallbackHelper = gitssh.HostKeyCallbackHelper{
		HostKeyCallback: cryptossh.InsecureIgnoreHostKey(),
	}
	return publicKeys, nil
}

// Prevent Android media scanner delete critical empty directoryes
func (a *App) protectGitDirs() {
    if runtime.GOOS != "android" {
        return
    }
    //for _, dir := range []string{"objects", "refs"} {
    for _, dir := range []string{"objects"} {
        p := filepath.Join(a.StorageDir, ".git", dir)
        if err := os.MkdirAll(p, 0755); err != nil {
            log.Printf("[a.protectGitDirs] MkdirAll %s failed: %v", p, err)
            continue
        }
        keepFile := filepath.Join(p, ".gitkeep")
        if _, err := os.Stat(keepFile); os.IsNotExist(err) {
            if f, err := os.Create(keepFile); err == nil {
                f.Close()
            }
        }
    }
}
