package gitsync

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/storage"
)

// ----------------------------------------------------------------------
// The sync paths
// ----------------------------------------------------------------------
//
// SyncRepo at the end of this file is the one entry point. Two files under
// .git/ make a merge reversible, also over a restart. OMNGO_PREMERGE_HEAD
// holds the HEAD from before pull_mark, and pull_abort resets to it.

func (svc Service) premergeHeadPath() string {
	return svc.Layout.Git("OMNGO_PREMERGE_HEAD")
}

func (svc Service) SavePremergeHead(h plumbing.Hash) {
	if err := os.WriteFile(svc.premergeHeadPath(), []byte(h.String()), 0644); err != nil {
		svc.Log(logx.Sync).Errf("failed to save pre-merge HEAD: %v", err)
	}
}

func (svc Service) LoadPremergeHead() (plumbing.Hash, bool) {
	data, err := os.ReadFile(svc.premergeHeadPath())
	if err != nil {
		return plumbing.ZeroHash, false
	}
	h := plumbing.NewHash(strings.TrimSpace(string(data)))
	if h.IsZero() {
		return plumbing.ZeroHash, false
	}
	return h, true
}

func (svc Service) ClearPremergeHead() {
	os.Remove(svc.premergeHeadPath())
}

// OMNGO_MERGE_PARENT holds the remote tip that pull_mark merges. HEAD does
// not move. At the next commit, CommitLocalChanges reads the file and makes a
// real merge commit with two parents.

func (svc Service) mergeParentPath() string {
	return svc.Layout.Git("OMNGO_MERGE_PARENT")
}

func (svc Service) SaveMergeParent(h plumbing.Hash) {
	if err := os.WriteFile(svc.mergeParentPath(), []byte(h.String()), 0644); err != nil {
		svc.Log(logx.Sync).Errf("failed to save pending merge parent: %v", err)
	}
}

func (svc Service) LoadMergeParent() (plumbing.Hash, bool) {
	data, err := os.ReadFile(svc.mergeParentPath())
	if err != nil {
		return plumbing.ZeroHash, false
	}
	h := plumbing.NewHash(strings.TrimSpace(string(data)))
	if h.IsZero() {
		return plumbing.ZeroHash, false
	}
	return h, true
}

func (svc Service) ClearMergeParent() {
	os.Remove(svc.mergeParentPath())
}

// CleanUntrackedFiles deletes each untracked file that .gitignore does not
// match. Only a force pull calls it. A plain pull or push never touches a
// file that git does not track.
func (svc Service) CleanUntrackedFiles(wTree *git.Worktree, matcher gitignore.Matcher) {
	status, err := wTree.Status()
	if err != nil {
		svc.Log(logx.Sync).Errf("force pull: could not compute status for cleanup: %v", err)
		return
	}
	for name, fileStat := range status {
		if fileStat.Worktree != git.Untracked {
			continue
		}
		// Test the name as a safety net, the same as the other sync paths.
		// config.json holds the passwords of this device, whatever the
		// .gitignore of the remote says.
		if name == "config.json" {
			svc.Log(logx.Sync).Debugf("force pull: keeping root config.json (preserve locally)")
			continue
		}
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			svc.Log(logx.Sync).Debugf("force pull: keeping ignored file %s", name)
			continue
		}
		full := svc.Layout.File(name)
		if err := os.Remove(full); err != nil {
			svc.Log(logx.Sync).Errf("force pull: failed to delete %s: %v", name, err)
		} else {
			svc.Log(logx.Sync).Debugf("force pull: deleted untracked file %s", name)
		}
	}
}

// TrackedWorktreeIsDirty reports whether a TRACKED file has a change that is
// not committed. A pull overwrites each tracked file, thus it must refuse
// first. The native Pull of go-git made this check. See
// doc/decisions/0010-write-a-pull-without-the-checkout-of-go-git.md.
func TrackedWorktreeIsDirty(wTree *git.Worktree) (bool, error) {
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

// syncProgressWriter sends the sideband progress of git, for example
// "Counting objects: 45%", to the log and thus to the sync overlay. A
// progress line ends with '\r', because the remote writes it again in place.
// The writer sends one line for each interval, because the log hub drops a
// line when a client channel is full. go-git calls Write from one goroutine.
type syncProgressWriter struct {
	// go-git makes the call, thus the writer carries the logger.
	log  logx.Logger
	buf  []byte
	last time.Time
}

const syncProgressInterval = 300 * time.Millisecond

func (w *syncProgressWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	// Keep the text after the last terminator. A newer progress line replaces
	// an older one.
	if i := bytes.LastIndexAny(w.buf, "\r\n"); i != -1 {
		line := string(w.buf[:i])
		w.buf = append(w.buf[:0], w.buf[i+1:]...)
		if j := strings.LastIndexAny(line, "\r\n"); j != -1 {
			line = line[j+1:]
		}
		line = strings.TrimSpace(line)
		// The client adds the label "remote:". Remove a label that the server
		// sent, or the line reads "remote: remote:".
		line = strings.TrimSpace(strings.TrimPrefix(line, "remote:"))
		if line != "" && time.Since(w.last) >= syncProgressInterval {
			w.last = time.Now()
			w.log.Debugf("remote: %s", line)
		}
	}
	// Never return an error. A fault of the progress log must not stop a
	// fetch or a push.
	return len(p), nil
}

// These are the sentinel errors of the sync. Each one is a state that the
// user must resolve, and not a failure. SyncErrorStatus maps each one to its
// wire status with errors.Is.
var (
	// ErrSyncConflict: a plain pull cannot fast-forward. The user chooses
	// abort or a 3-way merge.
	ErrSyncConflict = errors.New("sync: fast-forward not possible")
	// ErrPushConflict: the remote refused a push without force. The user must
	// pull first.
	ErrPushConflict = errors.New("sync: push rejected, remote has new commits")
	// ErrCommitMessageRequired: a commit or a force push needs a message.
	ErrCommitMessageRequired = errors.New("sync: commit message required")
)

// SyncConflictError carries the paths in conflict to the dialog. It unwraps
// to ErrSyncConflict, thus errors.Is still matches it.
type SyncConflictError struct {
	Files []string
}

func (e *SyncConflictError) Error() string { return ErrSyncConflict.Error() }
func (e *SyncConflictError) Unwrap() error { return ErrSyncConflict }

// ConflictingPaths answers, sorted, each tracked path with an uncommitted
// change that differs from the remote copy. The conflict dialog and the
// marker loop of syncPullMerge both read it, thus the two always agree.
func ConflictingPaths(wTree *git.Worktree, remoteTree *object.Tree) ([]string, error) {
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

// newSyncConflict answers a SyncConflictError with the paths in conflict.
// When the list fails, it answers the bare ErrSyncConflict. A conflict thus
// never becomes a hard error.
func (svc Service) newSyncConflict(repo *git.Repository, wTree *git.Worktree, remoteRef *plumbing.Reference) error {
	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return ErrSyncConflict
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return ErrSyncConflict
	}
	files, err := ConflictingPaths(wTree, remoteTree)
	if err != nil {
		return ErrSyncConflict
	}
	return &SyncConflictError{Files: files}
}

// SyncErrorStatus maps a sync error to the {status, message} that runSync in
// omn-go-sync.js reads. ok is false for each other error, and the caller then
// answers "error".
func SyncErrorStatus(err error) (status, message string, ok bool) {
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

// SyncRepo runs one sync action under State.mu. The switch at the end lists
// each action and its aliases. Only push and push_force read message.
func (svc Service) SyncRepo(action string, message string) error {
	svc.State.mu.Lock()
	defer svc.State.mu.Unlock()

	repo, err := svc.GetOrInitRepo()
	if err != nil {
		return err
	}
	wTree, err := Worktree(repo)
	if err != nil {
		return err
	}
	remoteName, err := svc.EnsureRemotesAndGetActive(repo)
	if err != nil {
		return err
	}
	auth, err := svc.GetSSHAuth()
	if err != nil {
		return err
	}

	// After a pull, make the html/ copy of each text file beside a note
	// again. Git carries only the md/ file. See IsDerivedTextPath. The walk
	// is cheap when nothing changed.
	pullDone := func(err error) error {
		if err == nil {
			storage.SyncNoteFilesToHTML(svc.Layout, svc.Log(logx.NoteFiles))
		}
		return err
	}

	switch action {
	case "push", "upload":
		return svc.syncPush(repo, wTree, auth, remoteName, message, false)
	case "push_force", "upload_force":
		return svc.syncPush(repo, wTree, auth, remoteName, message, true)
	case "pull", "pull_ff", "download":
		return pullDone(svc.syncPull(repo, wTree, auth, remoteName))
	case "pull_mark":
		return pullDone(svc.syncPullMerge(repo, wTree, auth, remoteName))
	case "pull_abort", "abort":
		return pullDone(svc.syncPullAbort(wTree))
	case "pull_force", "download_force":
		return pullDone(svc.syncPullForce(repo, wTree, auth, remoteName))
	}
	return fmt.Errorf("unknown sync action: %s", action)
}
