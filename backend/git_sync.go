package backend

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// ----------------------------------------------------------------------
// The sync paths
// ----------------------------------------------------------------------
//
// This file was part of git_helper.go until 26.09.22. See the banner of
// git_repo.go for the split and for what each file holds.
//
// SyncRepo at the end of this file is the one entry point. It reads an
// action word and calls one of the paths above it. Each path leaves the
// repository in a state that the next action can start from. The two
// checkpoint files at the top of this file are what makes a failed merge
// reversible.
// ---------------------------------------------------------------
// Pre-merge checkpoint (backs "pull_abort")
// ---------------------------------------------------------------
//
// pull_mark moves the local branch ref forward to the remote tip. A later
// commit can then fast-forward-push cleanly, once the user resolves the
// injected conflict markers. pull_abort has to undo that. The local HEAD
// hash from *before* pull_mark ran is thus stashed in a small file under
// .git/. A file survives an app restart, and it needs no change to the App
// struct in server.go. An in-memory App field would do neither.

func (a *App) premergeHeadPath() string {
	return filepath.Join(a.StorageDir, ".git", "OMNGO_PREMERGE_HEAD")
}

func (a *App) savePremergeHead(h plumbing.Hash) {
	if err := os.WriteFile(a.premergeHeadPath(), []byte(h.String()), 0644); err != nil {
		a.logErrf(logSync, "failed to save pre-merge HEAD: %v", err)
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
// Pending merge parent (backs a real two-parent merge commit)
// ---------------------------------------------------------------
//
// pull_mark writes conflict markers into the working tree. It must not
// pretend that a merge happened, and a move of the branch ref onto the
// remote tip would do that. Such a move discards the local commit from the
// history of the branch, and only OMNGO_PREMERGE_HEAD can then recover it.
// It also makes a plain linear commit on top of remote, and not a real git
// merge.
//
// HEAD stays where it is, and the remote tip is recorded here. The next
// real commit, in commitLocalChanges, reads this and includes it as a
// second parent. That makes a real merge commit, once the user has
// resolved the injected conflict markers by hand.

func (a *App) mergeParentPath() string {
	return filepath.Join(a.StorageDir, ".git", "OMNGO_MERGE_PARENT")
}

func (a *App) saveMergeParent(h plumbing.Hash) {
	if err := os.WriteFile(a.mergeParentPath(), []byte(h.String()), 0644); err != nil {
		a.logErrf(logSync, "failed to save pending merge parent: %v", err)
	}
}

func (a *App) loadMergeParent() (plumbing.Hash, bool) {
	data, err := os.ReadFile(a.mergeParentPath())
	if err != nil {
		return plumbing.ZeroHash, false
	}
	h := plumbing.NewHash(strings.TrimSpace(string(data)))
	if h.IsZero() {
		return plumbing.ZeroHash, false
	}
	return h, true
}

func (a *App) clearMergeParent() {
	os.Remove(a.mergeParentPath())
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
		a.logErrf(logSync, "force pull: could not compute status for cleanup: %v", err)
		return
	}
	for name, fileStat := range status {
		if fileStat.Worktree != git.Untracked {
			continue
		}
		// Explicit safety net, the same as in commitLocalChanges,
		// syncPush and handleSyncPreview. config.json holds the local
		// admin and guest passwords of this device, and its server
		// list. A sync must never touch it, whatever .gitignore says at
		// that moment.
		//
		// The matcher alone is not safe here. The checkout of a force
		// pull can leave .gitignore in a state where the matcher no
		// longer protects the file. The file comes from the remote, it
		// can lack this line, and it can be stale for a moment. That is
		// exactly what deleted it.
		if name == "config.json" {
			a.logDebugf(logSync, "force pull: keeping root config.json (preserve locally)")
			continue
		}
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			a.logDebugf(logSync, "force pull: keeping ignored file %s", name)
			continue
		}
		full := filepath.Join(a.StorageDir, name)
		if err := os.Remove(full); err != nil {
			a.logErrf(logSync, "force pull: failed to delete %s: %v", name, err)
		} else {
			a.logDebugf(logSync, "force pull: deleted untracked file %s", name)
		}
	}
}

// ---------------------------------------------------------------
// pull
// ---------------------------------------------------------------

// syncPull fast-forwards local to origin/master when possible. When it is
// not possible it returns ErrSyncConflict. The caller can then offer the
// user a choice between "pull_abort" and "pull_mark", which is the 3-way
// merge. A diverged history makes it impossible, and so does an unstaged
// local change in the way.
//
// trackedWorktreeIsDirty reports whether any TRACKED file has a local
// modification that has not been committed. Untracked content never
// counts, for example a new note not yet committed or a db_json export.
// A tracked file is exactly what writeTreeToWorktree in syncPull below
// would silently overwrite with the copy of the remote.
//
// The native Worktree.Pull() of go-git, used here previously, refused on
// its own to fast-forward over a dirty tracked file, with
// ErrUnstagedChanges. Our own worktree-writing logic replaced it, thus
// that same check has to be explicit now. Without it a pull would quietly
// discard an uncommitted edit.
func trackedWorktreeIsDirty(wTree *git.Worktree) (bool, error) {
	status, err := wTree.Status()
	if err != nil {
		return false, err
	}
	for _, fileStat := range status {
		if fileStat.Worktree == git.Untracked {
			continue
		}
		if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			return true, nil
		}
	}
	return false, nil
}

// syncProgressWriter relays the sideband progress of git into the normal
// log. An example line is "Counting objects: 45% ...". The line thus
// reaches the browser over the /api/logs SSE stream, like every other log
// line, and it shows in the sync progress overlay.
//
// It is passed as FetchOptions.Progress and as PushOptions.Progress.
// Without it the network phase reports nothing at all between "fetching"
// and "complete", and that phase is reliably the longest part of a sync.
//
// Two details that the sideband format forces:
//   - A progress line ends with '\r', and not with '\n'. The remote
//     rewrites one line in place to animate a percentage. A split on '\n'
//     alone would buffer a whole transfer into one message.
//   - Those rewrites arrive fast. JSLogger drops a message when the
//     channel of a client is full, see logger.go. An unthrottled relay
//     would flood that buffer and push out the more useful stage lines.
//     One line for each interval is enough for a progress display.
//
// go-git writes to Progress from the one goroutine that drains the pack
// stream, thus the unsynchronised buffer below is safe. Each call site
// passes its own instance anyway.
type syncProgressWriter struct {
	// app is the only way this writer can reach a logger. go-git owns the
	// call, so the sideband text arrives with no receiver of its own. Every
	// construction site is a method on *App and passes itself.
	app  *App
	buf  []byte
	last time.Time
}

const syncProgressInterval = 300 * time.Millisecond

func (w *syncProgressWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	// Keep only what follows the last terminator. A newer line replaces
	// each earlier progress line.
	if i := bytes.LastIndexAny(w.buf, "\r\n"); i != -1 {
		line := string(w.buf[:i])
		w.buf = append(w.buf[:0], w.buf[i+1:]...)
		if j := strings.LastIndexAny(line, "\r\n"); j != -1 {
			line = line[j+1:]
		}
		line = strings.TrimSpace(line)
		// Servers normally send bare progress text and the "remote:" label is
		// the client's convention (mirrored here). Strip it if a server does
		// send one, so the line cannot come out as "remote: remote: ...".
		line = strings.TrimSpace(strings.TrimPrefix(line, "remote:"))
		if line != "" && time.Since(w.last) >= syncProgressInterval {
			w.last = time.Now()
			w.app.logDebugf(logSync, "remote: %s", line)
		}
	}
	// Never report an error: failing to log progress must not abort a fetch
	// or push that is otherwise working fine.
	return len(p), nil
}

func (a *App) syncPull(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	a.logInfof(logSync, "Pull: fetching %s", remoteName)
	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth, Progress: &syncProgressWriter{app: a}})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %v", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("failed to find %s/master: %v", remoteName, err)
	}

	localHead, headErr := repo.Head()
	if headErr == nil && localHead.Hash() == remoteRef.Hash() {
		a.logInfof(logSync, "Pull: already up to date")
		return nil
	}

	// Refuse over dirty tracked files, exactly like the native Pull this
	// replaces used to (ErrUnstagedChanges) - see trackedWorktreeIsDirty.
	dirty, dErr := trackedWorktreeIsDirty(wTree)
	if dErr != nil {
		return fmt.Errorf("status check failed: %v", dErr)
	}
	if dirty {
		a.logInfof(logSync, "Pull: local tracked changes present, cannot fast-forward")
		return a.newSyncConflict(repo, wTree, remoteRef)
	}

	// Fast-forward only. Refuse when the local HEAD is not an ancestor of
	// the remote tip. The refusal uses the same sentinel that the caller
	// already handles, and it matches the ErrNonFastForwardUpdate of the
	// native Pull. Such a HEAD means that this device has unpushed commits
	// of its own. A blind jump to the tree of the remote would strand
	// them. An unborn local branch, with nothing committed here yet,
	// counts as "already an ancestor". There is nothing to strand.
	if headErr == nil {
		localCommit, cErr := repo.CommitObject(localHead.Hash())
		if cErr != nil {
			return fmt.Errorf("local HEAD commit lookup failed: %v", cErr)
		}
		remoteCommit, rcErr := repo.CommitObject(remoteRef.Hash())
		if rcErr != nil {
			return fmt.Errorf("remote commit lookup failed: %v", rcErr)
		}
		isAncestor, aErr := localCommit.IsAncestor(remoteCommit)
		if aErr != nil {
			return fmt.Errorf("ancestry check failed: %v", aErr)
		}
		if !isAncestor {
			a.logInfof(logSync, "Pull: fast-forward not possible (local has unpushed commits)")
			return a.newSyncConflict(repo, wTree, remoteRef)
		}
	}

	// What was tracked before this pull. A file that the remote deleted can
	// then be told apart from a file that was never tracked at all. The
	// second kind is config.json, the .sqlite file of a user database, or
	// anything else that this app manages outside git. None of those is
	// ever a candidate for removal here.
	oldPaths, err := oldTrackedPaths(repo)
	if err != nil {
		return fmt.Errorf("failed to read current tracked tree: %v", err)
	}

	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return fmt.Errorf("remote commit lookup failed: %v", err)
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return fmt.Errorf("remote tree lookup failed: %v", err)
	}

	// writeTreeToWorktree, which syncPullForce shares, only creates or
	// overwrites the paths that the tree of the remote holds. It touches
	// nothing else. That is the real fix.
	//
	// The native Worktree.Pull() of go-git, used here previously, applies
	// a fast-forward through the same worktree-reconciliation machinery as
	// Checkout and Reset. Force Pull already found that this machinery
	// does not limit itself to the files that git tracks. See the doc
	// comment of that function.
	//
	// A gitignored file that is never tracked can live in the same
	// directory tree, and the .sqlite file of a user database is one. That
	// machinery could delete and silently recreate it, even on a PLAIN
	// pull. The on-disk identity of the file then changes under an open
	// connection to it. That is exactly what "attempt to write a readonly
	// database (1032)", or SQLITE_READONLY_DBMOVED, means. On its next
	// write the connection sees that the file it opened is no longer the
	// file at that path.
	newPaths, err := a.writeTreeToWorktree(repo, wTree, remoteTree)
	if err != nil {
		return fmt.Errorf("failed to write remote tree: %v", err)
	}

	// Remove files that WERE tracked before this pull but are no longer
	// part of the remote's tree (e.g. a note deleted on another device).
	// Only ever considers paths that were genuinely tracked.
	for p := range oldPaths {
		if newPaths[p] {
			continue
		}
		full := filepath.Join(a.StorageDir, p)
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			a.logErrf(logSync, "pull: failed to remove file no longer tracked upstream (%s): %v", p, err)
		} else {
			a.logDebugf(logSync, "pull: removed file no longer tracked upstream: %s", p)
		}
	}

	if err := repo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.ReferenceName("refs/heads/master"), remoteRef.Hash())); err != nil {
		return fmt.Errorf("failed to move local branch: %v", err)
	}

	a.logInfof(logSync, "Pull: fast-forward complete")
	return nil
}

// syncPullMerge implements the "3 way diff merge" option that follows a
// pull conflict. It writes standard diff3-style conflict markers into
// every locally modified file that also differs from the remote copy.
// Those markers are <<<<<<< LOCAL, ||||||| BASE, ======= and >>>>>>>
// REMOTE. The BASE section comes from the git merge-base where one can be
// found.
//
// The local branch ref then moves to the remote tip, and the contents of
// the working tree stay as they are. The user resolves the markers by hand
// and commits, and the parent of that commit is then the remote tip. A
// normal push can fast-forward. The pre-merge HEAD is saved, thus
// "pull_abort" can undo this.
func (a *App) syncPullMerge(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth, Progress: &syncProgressWriter{app: a}})
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

	// Same set of files the conflict modal previewed (see conflictingPaths):
	// tracked, locally modified, and differing from the remote copy. Sharing
	// this helper is what guarantees the marked files match what the user was
	// shown.
	paths, err := conflictingPaths(wTree, remoteTree)
	if err != nil {
		return fmt.Errorf("status error: %v", err)
	}

	for _, path := range paths {
		file, err := wTree.Filesystem.Open(path)
		if err != nil {
			continue
		}
		localContent, _ := io.ReadAll(file)
		file.Close()

		// conflictingPaths already found that the remote has this file, and
		// that its content differs from the local one. The remote copy is
		// read again here only to build the marker body.
		remoteFile, err := remoteTree.File(path)
		if err != nil {
			continue
		}
		remoteContentStr, _ := remoteFile.Contents()

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

	// Record the remote tip as a pending second parent. Do not move the
	// local branch ref onto it. A reset of HEAD to remoteRef here was the
	// previous approach, and it is not a real git merge. No merge commit
	// is ever created. The real local commit becomes unreachable from any
	// branch, and only OMNGO_PREMERGE_HEAD can recover it. Nothing beyond
	// a log line records that a merge was needed. The next commit of the
	// user then reads as a plain commit on the tip of the remote.
	//
	// commitLocalChanges looks for this pending parent. The user resolves
	// the conflict markers by hand and commits. It then makes a real
	// two-parent merge commit from the local HEAD and this remote tip.
	// That is what a "3-way merge" has to produce.
	a.saveMergeParent(remoteRef.Hash())
	a.logInfof(logSync, "Pull: 3-way conflict markers written, awaiting manual resolution")
	return nil
}

// syncPullAbort discards an in-progress pull_mark, restoring local state
// (both the branch ref and the working tree) to what it was immediately
// before pull_mark ran. If there is nothing to abort, this is a no-op.
func (a *App) syncPullAbort(wTree *git.Worktree) error {
	hash, ok := a.loadPremergeHead()
	if !ok {
		a.logInfof(logSync, "pull_abort: nothing to abort")
		return nil
	}
	if err := wTree.Reset(&git.ResetOptions{Commit: hash, Mode: git.HardReset}); err != nil {
		return fmt.Errorf("abort reset failed: %v", err)
	}
	a.clearPremergeHead()
	a.clearMergeParent()
	a.logInfof(logSync, "pull_abort: restored local state to %s", hash.String())
	return nil
}

// writeTreeToWorktree writes every blob of tree into the filesystem of
// wTree. It answers the set of paths that it wrote. It deliberately does
// NOT ask go-git to reconcile the rest of the worktree against tree, the
// way Worktree.Checkout and Worktree.Reset do inside. It only creates or
// overwrites the exact paths that tree holds, and it touches nothing else.
//
// See the comment in syncPullForce for why that distinction is the whole
// point. Checkout(Force: true), even called correctly, does not limit its
// "make the worktree match" behavior to the files that git knows about.
//
// Two states make it fall back. The first is a repo with no commit yet to
// diff against, which is a fresh install before this device has ever
// completed a sync. The second is a partially-reconciled state. The
// fallback reconciles everything on disk against the target tree, and it
// deletes whatever is not part of that tree. config.json goes with it,
// although the file was never tracked and is always in .gitignore. The
// fallback reads neither the tracked status nor .gitignore.
func (a *App) writeTreeToWorktree(repo *git.Repository, wTree *git.Worktree, tree *object.Tree) (map[string]bool, error) {
	newIndex := &index.Index{Version: 2}
	written := map[string]bool{}

	fileIter := tree.Files()
	defer fileIter.Close()

	err := fileIter.ForEach(func(f *object.File) error {
		reader, err := f.Reader()
		if err != nil {
			return fmt.Errorf("open blob for %s: %v", f.Name, err)
		}
		defer reader.Close()

		if dir := filepath.Dir(f.Name); dir != "." {
			if err := wTree.Filesystem.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("mkdir for %s: %v", f.Name, err)
			}
		}
		out, err := wTree.Filesystem.Create(f.Name)
		if err != nil {
			return fmt.Errorf("create %s: %v", f.Name, err)
		}
		_, copyErr := io.Copy(out, reader)
		closeErr := out.Close()
		if copyErr != nil {
			return fmt.Errorf("write %s: %v", f.Name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %v", f.Name, closeErr)
		}

		size := uint32(0)
		var modTime time.Time
		if info, statErr := wTree.Filesystem.Stat(f.Name); statErr == nil {
			size = uint32(info.Size())
			modTime = info.ModTime()
		}

		newIndex.Entries = append(newIndex.Entries, &index.Entry{
			Name:       f.Name,
			Hash:       f.Hash,
			Mode:       f.Mode,
			Size:       size,
			ModifiedAt: modTime,
		})
		written[f.Name] = true
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := repo.Storer.SetIndex(newIndex); err != nil {
		return nil, fmt.Errorf("failed to update index: %v", err)
	}
	return written, nil
}

// oldTrackedPaths answers every path that the current HEAD commit of the
// repo tracks. It answers an empty set when there is no HEAD yet, which is
// an unborn branch. That is the fresh-install case, where nothing has ever
// been tracked.
func oldTrackedPaths(repo *git.Repository) (map[string]bool, error) {
	paths := map[string]bool{}
	head, err := repo.Head()
	if err != nil {
		return paths, nil
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("HEAD commit lookup failed: %v", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("HEAD tree lookup failed: %v", err)
	}
	fileIter := tree.Files()
	defer fileIter.Close()
	err = fileIter.ForEach(func(f *object.File) error {
		paths[f.Name] = true
		return nil
	})
	return paths, err
}

// syncPullForce resets local to match origin/master exactly. It then
// deletes each file that git does not track and that .gitignore does not
// cover. Only a *force* pull may delete such a file.
func (a *App) syncPullForce(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	a.logInfof(logSync, "Force pull: fetching %s", remoteName)

	if runtime.GOOS == "android" {
		tmpDir := filepath.Join(a.StorageDir, ".git", "tmp")
		os.MkdirAll(tmpDir, 0755)
		os.Setenv("TMPDIR", tmpDir)
		a.ensureGitignore()
	}

	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth, Progress: &syncProgressWriter{app: a}})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %v", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("failed to find %s/master: %v", remoteName, err)
	}

	// What was tracked before this pull. A file that the remote deleted is
	// safe to remove, and a file that was never tracked at all is not. The
	// second kind is config.json, or anything else that this app manages
	// outside git. Neither is ever a candidate for removal here, whatever
	// the new tree holds.
	oldPaths, err := oldTrackedPaths(repo)
	if err != nil {
		return fmt.Errorf("failed to read current tracked tree: %v", err)
	}

	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return fmt.Errorf("remote commit lookup failed: %v", err)
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return fmt.Errorf("remote tree lookup failed: %v", err)
	}

	newPaths, err := a.writeTreeToWorktree(repo, wTree, remoteTree)
	if err != nil {
		return fmt.Errorf("failed to write remote tree: %v", err)
	}

	// Remove files that WERE tracked before this pull but are no longer
	// part of the remote's tree (e.g. a note deleted on another device).
	// Only ever considers paths that were genuinely tracked, so nothing
	// this app manages outside git is a candidate for removal here.
	for p := range oldPaths {
		if newPaths[p] {
			continue
		}
		full := filepath.Join(a.StorageDir, p)
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			a.logErrf(logSync, "force pull: failed to remove file no longer tracked upstream (%s): %v", p, err)
		} else {
			a.logDebugf(logSync, "force pull: removed file no longer tracked upstream: %s", p)
		}
	}

	if err := repo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.ReferenceName("refs/heads/master"), remoteRef.Hash())); err != nil {
		return fmt.Errorf("failed to move local branch: %v", err)
	}

	matcher, mErr := a.loadGitignoreMatcher(wTree)
	if mErr != nil {
		a.logErrf(logSync, "force pull: could not load .gitignore, skipping untracked cleanup: %v", mErr)
	} else {
		a.cleanUntrackedFiles(wTree, matcher)
	}

	a.clearPremergeHead() // any pending 3-way merge is now moot
	a.clearMergeParent()
	a.logInfof(logSync, "Force pull complete")
	return nil
}

// ---------------------------------------------------------------
// push
// ---------------------------------------------------------------

// NOTE: this file also held a hasUnpushedCommits() helper here. syncPush()
// below called it to skip the repo.Push call entirely. It did that
// whenever a fresh repo.Fetch against the ACTIVE remote showed the local
// HEAD already matching the master of that remote.
//
// It was removed, and the comment on syncPush says why. It made "push"
// silently do nothing against whichever remote it happened to check. That
// is exactly wrong with several remotes in play, which are the git server
// profiles or slots. It is wrong when the remote that was switched to has
// not seen the local HEAD yet.

// syncPush implements both "push" and "force push":
//
//   - With nothing uncommitted, this still attempts the real push.
//     repo.Push itself reports git.NoErrAlreadyUpToDate safely when the
//     active remote truly has nothing new. That report is now the ONLY
//     thing this function trusts for that decision.
//   - An earlier version tried to predict the answer ahead of time. It
//     used a separate hasUnpushedCommits fetch-and-compare against the
//     active remote, only to skip the push call when nothing seemed to
//     have changed. That produced a real bug with several configured git
//     remotes, the profiles. A person committed and pushed while profile
//     1 was active, switched to profile 2, and pressed upload again. The
//     log then said "nothing to commit, nothing to push", and the remote
//     of profile 2 was never contacted, although profile 2 had never seen
//     that commit. The working tree was clean, and correctly so, because
//     there was nothing NEW to commit. The own fetch-and-compare of the
//     pre-check against profile 2 could be wrong in ways that a real
//     repo.Push attempt against profile 2 is not. It was removed, and the
//     real remote is now always asked through the push itself.
//   - With uncommitted local changes, a commit message is required.
//     Without one it returns ErrCommitMessageRequired.
//   - A force push always requires a commit message up front, also with
//     no pending change to commit. It is a destructive operation on the
//     remote.
//   - A non-force push that is rejected as non-fast-forward returns
//     ErrPushConflict, and it does nothing further. Per the spec, no
//     auto-pull and no auto-merge is attempted.
//   - A force push always pushes with Force:true. It overwrites the
//     remote to match local, whatever the divergence is.
func (a *App) syncPush(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName, message string, force bool) error {
	// NOTE: a database is deliberately NOT exported here anymore. A backup
	// is a manual whole-database snapshot file, made from the /db_backups
	// page. See db_backup.go. Each backup file under html/db_backup/ is
	// committed and pushed like any other tracked file. The push never
	// invents new database state on its own.

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

	// A pending pull_mark merge must always end in a real commit. The
	// hand-resolution of the user can match HEAD exactly, and the status
	// then looks clean. Without this the merge parent record stays
	// forever, and no merge commit is ever produced.
	if _, mergePending := a.loadMergeParent(); mergePending {
		hasRelevantChanges = true
	}

	needsMessage := hasRelevantChanges || force
	if needsMessage && strings.TrimSpace(message) == "" {
		return ErrCommitMessageRequired
	}

	if hasRelevantChanges {
		if _, cErr := a.commitLocalChanges(repo, wTree, message); cErr != nil {
			return fmt.Errorf("commit failed: %v", cErr)
		}
	}

	a.logInfof(logSync, "Pushing to %s master (force=%v)", remoteName, force)
	err = repo.Push(&git.PushOptions{
		RemoteName: remoteName,
		Auth:       auth,
		RefSpecs:   []gitconfig.RefSpec{"refs/heads/master:refs/heads/master"},
		Force:      force,
		Progress:   &syncProgressWriter{app: a},
	})
	if err == git.NoErrAlreadyUpToDate {
		a.logInfof(logSync, "push: remote %s already up to date", remoteName)
		return nil
	}
	if err != nil {
		if !force && isNonFastForward(err) {
			a.logErrf(logSync, "push: rejected as non-fast-forward, leaving local state untouched")
			return ErrPushConflict
		}
		return fmt.Errorf("push failed: %v", err)
	}
	return nil
}

// nonFastForwardText is the message that go-git writes when a remote
// refuses a push. See isNonFastForward.
const nonFastForwardText = "non-fast-forward update"

// isNonFastForward tells whether a push failed because the remote holds a
// commit that this device does not have.
//
// THE TEST OF THE MESSAGE IS NOT A CHOICE. go-git builds this error with
// fmt.Errorf at three places in remote.go, and it wraps no sentinel. See
// go-git v5.13.1, remote.go lines 828, 1106 and 1115.
//
// The value git.ErrNonFastForwardUpdate lives in worktree.go and belongs
// to Pull alone. A test of that value against the answer of Push is
// therefore never true.
//
// Until 26.09.11 syncPush made exactly that test. Each rejected push
// therefore left this function as a general fault. syncErrorStatus then
// gave the status "error" in place of "push_conflict". The frontend shows
// a plain alert for the first word and the push-conflict modal for the
// second. The reader thus lost the one control that offers a force push.
// See showPushConflictModal in omn-go-sse.js.
//
// The value test stays first. A later go-git that wraps the sentinel then
// matches without a change here.
func isNonFastForward(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, git.ErrNonFastForwardUpdate) {
		return true
	}
	return strings.HasPrefix(err.Error(), nonFastForwardText)
}

// ---------------------------------------------------------------
// "Is there anything to push?" - for the upload preview
// ---------------------------------------------------------------

// unpushedState answers, for the ACTIVE remote, the question the upload
// preview needs: is there a local commit this remote has not got?
//
// A worktree with nothing pending is NOT the same thing as nothing to push,
// and conflating the two is how commits get stranded. Two ways it happens,
// both reported from real use:
//
//   - a commit succeeds, the push that follows fails (the network dropped).
//     The worktree is now clean and the commit exists only locally; pressing
//     Upload again must retry the push, not report "nothing to commit".
//   - the git profile is switched. The commit was pushed to slot 0 and the
//     worktree is clean, but slot 1's remote has never seen it.
//
// This is deliberately the second time that this file has had to learn
// that lesson. syncPush once carried a hasUnpushedCommits() pre-check that
// SKIPPED the push, and it was removed for causing exactly the
// profile-switch failure above. See the note above syncPush.
//
// The difference in direction matters, and it is the whole design here.
// That check could suppress a push that was needed. This one can only
// offer a push that turns out to be unnecessary, and repo.Push reports
// NoErrAlreadyUpToDate for that harmlessly.
type unpushedState struct {
	// Unpushed is "there is, or may be, something to push". It errs towards
	// true on purpose: a needless push attempt costs a round trip, a
	// suppressed one costs the user their commits.
	Unpushed bool
	Remote   string
	// Verified records that the remote itself was asked, and not the local
	// remote-tracking ref alone. Only the "nothing to push" answer is worth
	// a round trip. This is thus false whenever the local refs alone
	// settled it, and that covers every case where they made Unpushed true.
	Verified bool
	Error    string
}

// syncPreviewResponse is the body of GET /api/sync/preview?action=upload.
//
// Files alone used to be the whole answer, and the frontend read an empty
// list as "nothing to do". That is why a commit whose push failed could not
// be retried. The three fields after it are the missing half of that
// answer.
type syncPreviewResponse struct {
	Files       []string `json:"files"`
	Unpushed    bool     `json:"unpushed"`
	Remote      string   `json:"remote,omitempty"`
	Verified    bool     `json:"verified,omitempty"`
	RemoteError string   `json:"remote_error,omitempty"`
}

// aheadOfRemote compares local HEAD with the active remote.
//
// It looks locally FIRST. It goes to the network only to check the one
// answer that would stop the user pushing. Each slot owns its own named
// remote, see ensureSlotRemotes. refs/remotes/<slot>/master is thus the
// view of that profile, and a slot that was never contacted has no such
// ref. That reads as "might be ahead", and that is the safe reading.
//
// The remote round trip is a refs listing, which is git ls-remote, and not
// a fetch. It downloads no object, and it is reached only when the local
// view already says there is nothing to do.
func (a *App) aheadOfRemote(repo *git.Repository, remoteName string, auth transport.AuthMethod) unpushedState {
	out := unpushedState{Remote: remoteName}

	head, err := repo.Head()
	if err != nil {
		// No commits yet: nothing to push, and nothing to check.
		a.logDebugf(logSync, "preview: no local HEAD (%v)", err)
		return out
	}

	trackRef, tErr := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if tErr != nil {
		a.logDebugf(logSync, "preview: %s has no known master yet - treating local HEAD as unpushed", remoteName)
		out.Unpushed = true
		return out
	}
	if trackRef.Hash() != head.Hash() {
		a.logDebugf(logSync, "preview: local HEAD %s differs from %s/master %s",
			head.Hash().String()[:7], remoteName, trackRef.Hash().String()[:7])
		out.Unpushed = true
		return out
	}

	// The local view says level. That is the claim that would stop the user
	// pushing, and it is the claim that was wrong before. It is thus worth
	// a round trip.
	remote, rErr := repo.Remote(remoteName)
	if rErr != nil {
		out.Error = rErr.Error()
		return out
	}
	refs, lErr := remote.List(&git.ListOptions{Auth: auth})
	if lErr != nil {
		// Unreachable. Report that, and do not claim to have checked. A
		// push would fail with the same error, thus an offer of one gains
		// nothing. To imply silently that the remote agreed would be a lie.
		a.logErrf(logSync, "preview: could not list %s: %v", remoteName, lErr)
		out.Error = lErr.Error()
		return out
	}

	out.Verified = true
	for _, ref := range refs {
		if ref.Name() == plumbing.Master {
			out.Unpushed = ref.Hash() != head.Hash()
			if out.Unpushed {
				a.logDebugf(logSync, "preview: %s/master is at %s, local HEAD is %s",
					remoteName, ref.Hash().String()[:7], head.Hash().String()[:7])
			}
			return out
		}
	}
	// The remote has no master branch at all - an empty repository, or one
	// this profile has never been pushed to. The first push creates it.
	a.logDebugf(logSync, "preview: %s has no master branch yet", remoteName)
	out.Unpushed = true
	return out
}

// ---------------------------------------------------------------
// Dispatcher
// ---------------------------------------------------------------

// Sentinel errors for the sync state machine. These are the signals that a
// sync operation raises for a condition that the USER must resolve, and not
// for a real failure. handleSync translates each one into its wire status
// with errors.Is, through syncErrorStatus. A producer and a consumer can
// thus never drift apart over a bare string, the way the previous
// fmt.Errorf("CONFLICT_DETECTED") sentinels could.
var (
	// ErrSyncConflict: a plain pull cannot fast-forward - diverged history,
	// or uncommitted local changes in the way. The user chooses abort or a
	// 3-way merge.
	ErrSyncConflict = errors.New("sync: fast-forward not possible")
	// ErrPushConflict: a non-force push was rejected because the remote has
	// commits the local branch does not have. The user must pull first.
	ErrPushConflict = errors.New("sync: push rejected, remote has new commits")
	// ErrCommitMessageRequired: there are changes to commit (or a force push
	// was requested) but no commit message was supplied.
	ErrCommitMessageRequired = errors.New("sync: commit message required")
)

// syncConflictError wraps ErrSyncConflict with the list of files in
// contention, thus handleSync can show them in the conflict modal. It
// unwraps to ErrSyncConflict. errors.Is(err, ErrSyncConflict), and thus
// syncErrorStatus, keeps matching it exactly as before. A caller that reads
// the status alone is unaffected, and handleSync can take the file list out
// with errors.As.
type syncConflictError struct {
	Files []string
}

func (e *syncConflictError) Error() string { return ErrSyncConflict.Error() }
func (e *syncConflictError) Unwrap() error { return ErrSyncConflict }

// conflictingPaths answers the sorted list of worktree paths that a 3-way
// "Mark Conflicts" merge would fill with conflict markers. syncPullMerge is
// that merge. A path qualifies when it is a tracked file with an
// uncommitted local modification whose content also differs from the
// incoming remote copy.
//
// This is the one authority, and two readers share it. The first is the
// conflict PREVIEW, which is the file list shown in the conflict modal and
// threaded out of syncPull through syncConflictError. The second is the
// marker-writing loop of syncPullMerge. The list that the user sees can
// thus never disagree with the files that later get markers.
//
// A file that the remote does not have is not a conflict, and neither is
// one whose local content already equals the remote copy. Both are left
// out, and that matches the own skips of syncPullMerge exactly.
func conflictingPaths(wTree *git.Worktree, remoteTree *object.Tree) ([]string, error) {
	status, err := wTree.Status()
	if err != nil {
		return nil, err
	}
	var paths []string
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
			continue // remote does not have this file - nothing to reconcile
		}
		remoteContentStr, _ := remoteFile.Contents()
		if string(localContent) == remoteContentStr {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

// newSyncConflict builds the error that wraps ErrSyncConflict and carries
// the list of files in contention with the fetched remote tip. A failure
// during the computation of that list falls back to the bare
// ErrSyncConflict sentinel. The conflict is still reported, without the
// file breakdown. This can thus never turn a conflict into a hard error.
func (a *App) newSyncConflict(repo *git.Repository, wTree *git.Worktree, remoteRef *plumbing.Reference) error {
	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return ErrSyncConflict
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return ErrSyncConflict
	}
	files, err := conflictingPaths(wTree, remoteTree)
	if err != nil {
		return ErrSyncConflict
	}
	return &syncConflictError{Files: files}
}

// syncErrorStatus maps a sync error to the wire {status, message} the
// frontend understands (see runSync in omn-go-sse.js). ok is false for a nil
// error or a generic failure, in which case the caller reports a plain
// "error" with the raw message. Keyed on errors.Is, so it keeps matching
// even if a sentinel is later wrapped with %w for extra context.
func syncErrorStatus(err error) (status, message string, ok bool) {
	switch {
	case errors.Is(err, ErrSyncConflict):
		return "conflict", "Fast-forward not possible. Choose abort or 3-way merge.", true
	case errors.Is(err, ErrPushConflict):
		return "push_conflict", "Remote has new commits. Pull before pushing.", true
	case errors.Is(err, ErrCommitMessageRequired):
		return "needs_commit_message", "Please provide a commit message.", true
	default:
		return "", "", false
	}
}

// SyncRepo implements the git sync operations. message is used only by the
// "push"/"push_force" actions (empty for the rest):
//
//	pull        fast-forward if possible. It returns ErrSyncConflict if it
//	            is not possible, from a diverged history or from local
//	            changes in the way. Aliases: "pull_ff", "download".
//	pull_mark   after a "pull" conflict. It writes 3-way conflict markers,
//	            thus the user can resolve them by hand. The UI calls this
//	            "make 3 way diff merge".
//	pull_abort  after a "pull" conflict. It discards the conflict and
//	            restores the local state. Alias: "abort".
//	pull_force  resets local to match the remote exactly. It also deletes
//	            each file that is neither tracked nor covered by
//	            .gitignore. Alias: "download_force".
//	push        commits with the given message when there are local
//	            changes, and then pushes. It does nothing further on a
//	            conflict. Alias: "upload".
//	push_force  commits with the given message when needed, and then
//	            force-pushes. That resets the remote to the local state.
//	            Alias: "upload_force".
//
// Previously this was two methods, SyncRepo(action) and
// SyncRepoWithMessage(action, message), kept as a backward-compat pair.
// The message-taking form is now the only one, and nothing calls the old
// signature.
//
// All repo mutation is serialized via a.GitMutex, so this is safe to call
// from multiple goroutines at once (e.g. concurrent HTTP requests).
func (a *App) SyncRepo(action string, message string) error {
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

	// After a pull, remake the html/ copy of every text file beside a note.
	// Git carries the md/ original and not the copy. See isDerivedTextPath.
	// A pull can bring a new md/log.txt, or delete the html/ copy that an
	// older commit still tracked. The URL of that file then has nothing
	// behind it until the next start. The walk reads nothing until it finds
	// a file to copy, thus this is cheap enough to run every time.
	pullDone := func(err error) error {
		if err == nil {
			a.syncNoteFilesToHTML()
		}
		return err
	}

	switch action {
	case "push", "upload":
		return a.syncPush(repo, wTree, auth, remoteName, message, false)
	case "push_force", "upload_force":
		return a.syncPush(repo, wTree, auth, remoteName, message, true)
	case "pull", "pull_ff", "download":
		return pullDone(a.syncPull(repo, wTree, auth, remoteName))
	case "pull_mark":
		return pullDone(a.syncPullMerge(repo, wTree, auth, remoteName))
	case "pull_abort", "abort":
		return pullDone(a.syncPullAbort(wTree))
	case "pull_force", "download_force":
		return pullDone(a.syncPullForce(repo, wTree, auth, remoteName))
	}
	return fmt.Errorf("unknown sync action: %s", action)
}
