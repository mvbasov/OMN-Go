package gitsync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"net.basov.omngo/backend/internal/logx"
)

// syncPull fast-forwards the local branch to the remote tip. It returns a
// conflict for a diverged history or for a tracked change that is not
// committed. The page then offers pull_abort and pull_mark.
func (svc Service) syncPull(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	svc.Log(logx.Sync).Infof("Pull: fetching %s", remoteName)
	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth, Progress: &syncProgressWriter{log: svc.Log(logx.Sync)}})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %w", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("failed to find %s/master: %v", remoteName, err)
	}

	localHead, headErr := repo.Head()
	if headErr == nil && localHead.Hash() == remoteRef.Hash() {
		svc.Log(logx.Sync).Infof("Pull: already up to date")
		return nil
	}

	// Refuse when a tracked file has a change. See TrackedWorktreeIsDirty.
	dirty, dErr := TrackedWorktreeIsDirty(wTree)
	if dErr != nil {
		return fmt.Errorf("status check failed: %v", dErr)
	}
	if dirty {
		svc.Log(logx.Sync).Infof("Pull: local tracked changes present, cannot fast-forward")
		return svc.newSyncConflict(repo, wTree, remoteRef)
	}

	// Allow only a fast-forward. A HEAD that is not an ancestor of the remote
	// tip holds commits that the remote does not have. A jump to the remote
	// tree would lose them. An unborn branch has nothing to lose.
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
			svc.Log(logx.Sync).Infof("Pull: fast-forward not possible (local has unpushed commits)")
			return svc.newSyncConflict(repo, wTree, remoteRef)
		}
	}

	// Read the paths that HEAD tracks. A pull removes a path that the remote
	// deleted. It never removes a path that git never tracked, for example
	// config.json or a database file.
	oldPaths, err := OldTrackedPaths(repo)
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

	// WriteTreeToWorktree writes only the paths of the remote tree. See
	// doc/decisions/0010-write-a-pull-without-the-checkout-of-go-git.md.
	newPaths, err := svc.WriteTreeToWorktree(repo, wTree, remoteTree)
	if err != nil {
		return fmt.Errorf("failed to write remote tree: %v", err)
	}

	// Remove each path that HEAD tracked and that the remote tree does not
	// hold, for example a note that another device deleted.
	for p := range oldPaths {
		if newPaths[p] {
			continue
		}
		full := svc.Layout.File(p)
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			svc.Log(logx.Sync).Errf("pull: failed to remove file no longer tracked upstream (%s): %v", p, err)
		} else {
			svc.Log(logx.Sync).Debugf("pull: removed file no longer tracked upstream: %s", p)
		}
	}

	if err := repo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.ReferenceName("refs/heads/master"), remoteRef.Hash())); err != nil {
		return fmt.Errorf("failed to move local branch: %v", err)
	}

	svc.Log(logx.Sync).Infof("Pull: fast-forward complete")
	return nil
}

// syncPullMerge is the action pull_mark. It writes diff3 conflict markers
// into each file of ConflictingPaths, with a BASE part from the merge base.
// HEAD does not move, and the next commit becomes a merge commit. See
// OMNGO_MERGE_PARENT.
func (svc Service) syncPullMerge(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth, Progress: &syncProgressWriter{log: svc.Log(logx.Sync)}})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %w", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("remote master not found: %v", err)
	}

	localHead, err := repo.Head()
	if err != nil {
		return fmt.Errorf("local HEAD not found: %v", err)
	}
	svc.SavePremergeHead(localHead.Hash())

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

	// ConflictingPaths gives the same list that the conflict dialog showed.
	paths, err := ConflictingPaths(wTree, remoteTree)
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

	// Record the remote tip as the second parent of the next commit. Do not
	// move HEAD onto it. That makes no merge commit, and the local commit
	// becomes unreachable.
	svc.SaveMergeParent(remoteRef.Hash())
	svc.Log(logx.Sync).Infof("Pull: 3-way conflict markers written, awaiting manual resolution")
	return nil
}

// syncPullAbort resets the branch and the working tree to the HEAD from
// before pull_mark. With no saved HEAD, it does nothing.
func (svc Service) syncPullAbort(wTree *git.Worktree) error {
	hash, ok := svc.LoadPremergeHead()
	if !ok {
		svc.Log(logx.Sync).Infof("pull_abort: nothing to abort")
		return nil
	}
	if err := wTree.Reset(&git.ResetOptions{Commit: hash, Mode: git.HardReset}); err != nil {
		return fmt.Errorf("abort reset failed: %v", err)
	}
	svc.ClearPremergeHead()
	svc.ClearMergeParent()
	svc.Log(logx.Sync).Infof("pull_abort: restored local state to %s", hash.String())
	return nil
}

// WriteTreeToWorktree makes each path of tree hold its blob, and it answers
// those paths. It touches nothing else. Checkout and Reset of go-git can
// delete each file outside the tree, also config.json.
// See doc/decisions/0010-write-a-pull-without-the-checkout-of-go-git.md.
//
// A FILE THAT ALREADY HOLDS ITS BLOB STAYS AS IT IS. A write gives a file
// a new modification time. The DB backup page and the page cache read
// that time, thus a write of each file made each backup "newer".
func (svc Service) WriteTreeToWorktree(repo *git.Repository, wTree *git.Worktree, tree *object.Tree) (map[string]bool, error) {
	newIndex := &index.Index{Version: 2}
	written := map[string]bool{}

	fileIter := tree.Files()
	defer fileIter.Close()

	err := fileIter.ForEach(func(f *object.File) error {
		if !worktreeHoldsBlob(wTree.Filesystem, f) {
			if err := writeBlob(wTree.Filesystem, f); err != nil {
				return err
			}
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

// worktreeHoldsBlob tells whether the file at the path of f already holds
// the bytes of f. Each fault answers false, and the caller writes the file.
func worktreeHoldsBlob(fs billy.Filesystem, f *object.File) bool {
	info, err := fs.Lstat(f.Name)
	if err != nil || !info.Mode().IsRegular() || info.Size() != f.Size {
		return false
	}
	in, err := fs.Open(f.Name)
	if err != nil {
		return false
	}
	defer in.Close()
	h := plumbing.NewHasher(plumbing.BlobObject, info.Size())
	if _, err := io.Copy(h, in); err != nil {
		return false
	}
	return h.Sum() == f.Hash
}

// writeBlob writes the bytes of f to its path, and makes the directory.
func writeBlob(fs billy.Filesystem, f *object.File) error {
	reader, err := f.Reader()
	if err != nil {
		return fmt.Errorf("open blob for %s: %v", f.Name, err)
	}
	defer reader.Close()

	if dir := filepath.Dir(f.Name); dir != "." {
		if err := fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir for %s: %v", f.Name, err)
		}
	}
	out, err := fs.Create(f.Name)
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
	return nil
}

// OldTrackedPaths answers each path that HEAD tracks. An unborn branch gives
// an empty set.
func OldTrackedPaths(repo *git.Repository) (map[string]bool, error) {
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

// syncPullForce makes the local branch match the remote tip. It then deletes
// each file that git does not track and that .gitignore does not cover. Only
// a force pull may delete such a file.
func (svc Service) syncPullForce(repo *git.Repository, wTree *git.Worktree, auth transport.AuthMethod, remoteName string) error {
	svc.Log(logx.Sync).Infof("Force pull: fetching %s", remoteName)

	if runtime.GOOS == "android" {
		tmpDir := svc.Layout.Git("tmp")
		os.MkdirAll(tmpDir, 0755)
		os.Setenv("TMPDIR", tmpDir)
		svc.WriteExcludeFile()
	}

	err := repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Auth: auth, Progress: &syncProgressWriter{log: svc.Log(logx.Sync)}})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %w", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, "master"), true)
	if err != nil {
		return fmt.Errorf("failed to find %s/master: %v", remoteName, err)
	}

	// Read the paths that HEAD tracks. See syncPull.
	oldPaths, err := OldTrackedPaths(repo)
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

	newPaths, err := svc.WriteTreeToWorktree(repo, wTree, remoteTree)
	if err != nil {
		return fmt.Errorf("failed to write remote tree: %v", err)
	}

	// Remove each path that the remote dropped. See syncPull.
	for p := range oldPaths {
		if newPaths[p] {
			continue
		}
		full := svc.Layout.File(p)
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			svc.Log(logx.Sync).Errf("force pull: failed to remove file no longer tracked upstream (%s): %v", p, err)
		} else {
			svc.Log(logx.Sync).Debugf("force pull: removed file no longer tracked upstream: %s", p)
		}
	}

	if err := repo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.ReferenceName("refs/heads/master"), remoteRef.Hash())); err != nil {
		return fmt.Errorf("failed to move local branch: %v", err)
	}

	matcher, mErr := svc.LoadGitignoreMatcher(wTree)
	if mErr != nil {
		svc.Log(logx.Sync).Errf("force pull: could not load .gitignore, skipping untracked cleanup: %v", mErr)
	} else {
		svc.CleanUntrackedFiles(wTree, matcher)
	}

	svc.ClearPremergeHead() // any pending 3-way merge is now moot
	svc.ClearMergeParent()
	svc.Log(logx.Sync).Infof("Force pull complete")
	return nil
}
