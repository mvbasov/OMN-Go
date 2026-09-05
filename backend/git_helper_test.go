package backend

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// newTestRepo initializes a real on-disk git repo inside a fresh temp dir
// and returns the App rooted there plus the repo and worktree handles.
func newTestRepo(t *testing.T) (*App, *git.Repository, *git.Worktree) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	return &App{StorageDir: dir}, repo, wt
}

func testCommit(t *testing.T, wt *git.Worktree, msg string) plumbing.Hash {
	t.Helper()
	h, err := wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return h
}

func writeAndAdd(t *testing.T, a *App, wt *git.Worktree, rel, content string) {
	t.Helper()
	full := filepath.Join(a.StorageDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(rel); err != nil {
		t.Fatalf("Add(%s): %v", rel, err)
	}
}

func headTree(t *testing.T, repo *git.Repository) *object.Tree {
	t.Helper()
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	return tree
}

// gitignoreLines returns the set of trimmed, non-empty lines of the sync
// .gitignore currently on disk.
func gitignoreLines(t *testing.T, a *App) map[string]int {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(a.StorageDir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	counts := map[string]int{}
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		counts[trimmed]++
	}
	return counts
}

// A fresh install writes the .gitignore straight from gitignorePatterns. This
// pins the exact bytes so an accidental edit to gitignorePatterns (reordering,
// a dropped/added entry) is caught, and proves the single-source join produces
// the same file the old hand-written literal did.
func TestEnsureGitignoreFreshInstall(t *testing.T) {
	a := &App{StorageDir: t.TempDir()}
	a.ensureGitignore()

	want := "# OMN-Go sync ignore\n" +
		"config.json\n" +
		"assets_version\n" +
		"session_secret\n" +
		"/asset_backups/\n" +
		"*.html\n" +
		"*.woff2\n" +
		"*.woff\n" +
		"/html/images/*\n" +
		"!/html/images/*.svg\n" +
		"/html/images/icons/*\n" +
		"!/html/images/icons/*.svg\n" +
		"/html/css/OMN-Go/omn-go-core.css\n" +
		"/html/css/OMN-Go/Bookmarker.css\n" +
		"/html/css/OMN-Go/highlight.default.min.css\n" +
		"/html/css/OMN-Go/katex.min.css\n" +
		"/html/js/OMN-Go/omn-go-compat.js\n" +
		"/html/js/OMN-Go/omn-go-core.js\n" +
		"/html/js/OMN-Go/omn-go-sse.js\n" +
		"/html/js/OMN-Go/omn-go-editor.js\n" +
		"/html/js/OMN-Go/auto-render.min.js\n" +
		"/html/js/OMN-Go/katex.min.js\n" +
		"/html/js/OMN-Go/highlight.min.js\n" +
		"/html/js/OMN-Go/Bookmarker.js\n" +
		"/html/**/*.txt\n" +
		"/md/AndroidIntents.md\n" +
		"/md/BookmarksHowTo.md\n" +
		"/md/Database.md\n" +
		"/md/Editor.md\n" +
		"/md/OMNGoTags.md\n" +
		"/md/ScriptRules.md\n" +
		"/md/SQLImport.md\n" +
		"/md/UserManual.md\n" +
		"/md/local/\n" +
		"/db/\n" +
		"local-*\n"

	got, err := os.ReadFile(filepath.Join(a.StorageDir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if string(got) != want {
		t.Errorf("fresh .gitignore mismatch:\n got %q\nwant %q", got, want)
	}
}

// The backfill must add every gitignorePatterns entry missing from an existing
// install, matching whole lines - not substrings. The regression it guards:
// "*.woff" is a substring of "*.woff2", so a strings.Contains check would see
// an install that already has "*.woff2" and wrongly conclude "*.woff" is
// present, leaving raw .woff fonts committable. It must also NOT duplicate a
// pattern that is already there.
func TestEnsureGitignoreBackfillLineExact(t *testing.T) {
	a := &App{StorageDir: t.TempDir()}
	// An old install that predates most of the current list: it has *.woff2
	// (the substring trap) but not *.woff, and is missing /db/ etc.
	existing := "# OMN-Go sync ignore\nconfig.json\n*.html\n*.woff2\n/md/local/\n"
	if err := os.WriteFile(filepath.Join(a.StorageDir, ".gitignore"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	a.ensureGitignore()

	counts := gitignoreLines(t, a)
	// The substring case: *.woff must be added even though *.woff2 is present.
	if counts["*.woff"] != 1 {
		t.Errorf("*.woff appears %d time(s), want exactly 1 (substring-of-*.woff2 backfill)", counts["*.woff"])
	}
	// Already-present patterns are not duplicated.
	if counts["*.woff2"] != 1 {
		t.Errorf("*.woff2 appears %d time(s), want exactly 1 (no duplicate)", counts["*.woff2"])
	}
	if counts["config.json"] != 1 {
		t.Errorf("config.json appears %d time(s), want exactly 1", counts["config.json"])
	}
	// Every pattern from the single source ends up present exactly once.
	for _, patt := range gitignorePatterns {
		if counts[patt] != 1 {
			t.Errorf("pattern %q appears %d time(s), want exactly 1", patt, counts[patt])
		}
	}
}

// An already-complete .gitignore must be left byte-for-byte untouched: the
// backfill finds nothing missing, so it must not rewrite (and in particular
// must not append a duplicate trailing block).
func TestEnsureGitignoreNoRewriteWhenComplete(t *testing.T) {
	a := &App{StorageDir: t.TempDir()}
	a.ensureGitignore() // write the canonical file
	before, err := os.ReadFile(filepath.Join(a.StorageDir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	a.ensureGitignore() // second pass must be a no-op
	after, err := os.ReadFile(filepath.Join(a.StorageDir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("second ensureGitignore rewrote a complete file:\nbefore %q\nafter  %q", before, after)
	}
}

// The general "local-*" rule replaced "/html/db_backup/local-*/". An
// install that still has the old line must lose it, and it must get the
// new one. The old line is not harmful, but two rules for one job drift
// apart, and the file must stay equal to gitignorePatterns.
func TestEnsureGitignoreDropsTheObsoleteLocalDatabaseRule(t *testing.T) {
	a := &App{StorageDir: t.TempDir()}
	existing := "# OMN-Go sync ignore\nconfig.json\n*.html\n/db/\n/html/db_backup/local-*/\n"
	if err := os.WriteFile(filepath.Join(a.StorageDir, ".gitignore"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	a.ensureGitignore()

	counts := gitignoreLines(t, a)
	if counts["/html/db_backup/local-*/"] != 0 {
		t.Errorf("the obsolete database rule is still in the file %d time(s)", counts["/html/db_backup/local-*/"])
	}
	if counts[gitignoreLocalOnlyPattern] != 1 {
		t.Errorf("%q appears %d time(s), want exactly 1", gitignoreLocalOnlyPattern, counts[gitignoreLocalOnlyPattern])
	}
	// The two buggy negations must still go away.
	if counts["!/html/images/"] != 0 || counts["!/html/images/icons/"] != 0 {
		t.Error("a directory-only negation came back")
	}
}

// The .gitignore pattern and isLocalOnlyPath are two forms of one rule:
// the pattern decides for a new file, and the function decides for the
// index. They must agree, and the pattern must match a name at each
// depth. This test reads the canonical list, thus it also pins the
// position of "local-*" at the end: go-git reads the patterns from the
// end and stops at the first match.
func TestGitignoreMatchesEachLocalOnlyPath(t *testing.T) {
	patterns := make([]gitignore.Pattern, 0, len(gitignorePatterns))
	for _, p := range gitignorePatterns {
		patterns = append(patterns, gitignore.ParsePattern(p, nil))
	}
	matcher := gitignore.NewMatcher(patterns)

	cases := []struct {
		path []string
		want bool
		why  string
	}{
		{[]string{"html", "user_json", "local-data.json"}, true, "the name starts with local-"},
		{[]string{"user_json", "local-data.json"}, true, "the same name at another depth"},
		{[]string{"local-scratch.md"}, true, "the name at the root"},
		{[]string{"md", "local-drafts", "Monday.md"}, true, "a directory that starts with local-"},
		{[]string{"html", "db_backup", "local-counters", "2026_pixel.jsonl"}, true, "the database rule, as before"},
		{[]string{"html", "images", "local-map.svg"}, true, "local- wins over the svg re-inclusion"},
		{[]string{"md", "Welcome.md"}, false, "a normal note"},
		{[]string{"html", "user_json", "data.json"}, false, "a normal data file"},
		{[]string{"md", "mylocal-data.md"}, false, "local- is not at the start of the name"},
		{[]string{"html", "images", "map.svg"}, false, "the svg re-inclusion still operates"},
	}

	for _, c := range cases {
		full := strings.Join(c.path, "/")
		if got := matcher.Match(c.path, false); got != c.want {
			t.Errorf("matcher.Match(%q) = %v, want %v (%s)", full, got, c.want, c.why)
		}
		if got := isLocalOnlyPath(full); got != c.want {
			t.Errorf("isLocalOnlyPath(%q) = %v, want %v (%s)", full, got, c.want, c.why)
		}
	}
}

// A local-only file can be in the index from a time before the rule. It
// must leave the index at the next commit, and it must stay on the disk.
// The preview must name it before that commit.
func TestCommitLocalChangesUntracksALocalOnlyFile(t *testing.T) {
	a, repo, wt := newTestRepo(t)
	a.ensureGitignore()

	const localRel = "html/user_json/local-data.json"
	const keepRel = "md/Notes.md"
	writeAndAdd(t, a, wt, localRel, `{"device":"pixel7"}`)
	writeAndAdd(t, a, wt, keepRel, "# Notes\n")
	writeAndAdd(t, a, wt, ".gitignore", string(mustRead(t, filepath.Join(a.StorageDir, ".gitignore"))))
	testCommit(t, wt, "before the rule")

	// The preview must announce the removal, although no content changed.
	pending := a.untrackTrackedPaths(repo)
	if len(pending) != 1 || pending[0] != localRel+localOnlyPreviewNote {
		t.Errorf("untrackTrackedPaths = %v, want [%s]", pending, localRel+localOnlyPreviewNote)
	}

	committed, err := a.commitLocalChanges(repo, wt, "stop to track the local-only files")
	if err != nil {
		t.Fatalf("commitLocalChanges: %v", err)
	}
	if !committed {
		t.Fatal("no commit was made, want the removal of the local-only file")
	}

	tree := headTree(t, repo)
	if _, err := tree.File(localRel); err == nil {
		t.Errorf("%s is still in the commit", localRel)
	}
	if _, err := tree.File(keepRel); err != nil {
		t.Errorf("%s left the commit, but it is a normal note: %v", keepRel, err)
	}

	// The file itself must stay. "git rm --cached", not "git rm".
	if _, err := os.Stat(filepath.Join(a.StorageDir, localRel)); err != nil {
		t.Errorf("the local-only file left the disk: %v", err)
	}

	// A second run finds nothing more to do.
	if left := a.untrackTrackedPaths(repo); len(left) != 0 {
		t.Errorf("the index still holds %v", left)
	}
}

// A local-only file that git never tracked must not make a commit, and a
// force pull must keep it. cleanUntrackedFiles deletes an untracked file
// only when no .gitignore pattern matches it.
func TestForcePullKeepsALocalOnlyFile(t *testing.T) {
	a, repo, wt := newTestRepo(t)
	a.ensureGitignore()
	writeAndAdd(t, a, wt, ".gitignore", string(mustRead(t, filepath.Join(a.StorageDir, ".gitignore"))))
	writeAndAdd(t, a, wt, "md/Notes.md", "# Notes\n")
	testCommit(t, wt, "first")

	const localRel = "html/user_json/local-data.json"
	full := filepath.Join(a.StorageDir, filepath.FromSlash(localRel))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(`{"device":"pixel7"}`), 0644); err != nil {
		t.Fatal(err)
	}

	matcher, err := a.loadGitignoreMatcher(wt)
	if err != nil {
		t.Fatalf("loadGitignoreMatcher: %v", err)
	}
	a.cleanUntrackedFiles(wt, matcher)

	if _, err := os.Stat(full); err != nil {
		t.Errorf("force pull deleted the local-only file: %v", err)
	}

	committed, err := a.commitLocalChanges(repo, wt, "nothing to say")
	if err != nil {
		t.Fatalf("commitLocalChanges: %v", err)
	}
	if committed {
		t.Error("a commit was made, but the only new file is local-only")
	}
}

// mustRead reads a file or fails the test.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

// buildFlatTree stores each name->content as a blob and assembles a single
// root-level tree object, returning it loaded (storer-backed) so *object.Tree
// .File() works. Used to stand in for a fetched remote tree without a network
// remote. Keeps files at the repo root so no nested subtrees are needed.
func buildFlatTree(t *testing.T, repo *git.Repository, files map[string]string) *object.Tree {
	t.Helper()
	var entries []object.TreeEntry
	for name, content := range files {
		obj := repo.Storer.NewEncodedObject()
		obj.SetType(plumbing.BlobObject)
		w, err := obj.Writer()
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
		w.Close()
		h, err := repo.Storer.SetEncodedObject(obj)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Regular, Hash: h})
	}
	// go-git requires a tree's entries to be sorted by name; map iteration
	// above is unordered, so sort before encoding or decoding rejects it.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	tree := &object.Tree{Entries: entries}
	enc := repo.Storer.NewEncodedObject()
	if err := tree.Encode(enc); err != nil {
		t.Fatal(err)
	}
	th, err := repo.Storer.SetEncodedObject(enc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := object.GetTree(repo.Storer, th)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// conflictingPaths is the single source of truth for both the conflict-modal
// file list and syncPullMerge's marker-writing loop, so its selection must be
// exactly: a tracked file with an uncommitted local modification whose content
// also differs from the remote copy. Everything else - identical-to-remote
// edits, files the remote lacks, and unmodified files - must be excluded.
func TestConflictingPaths(t *testing.T) {
	a, repo, wt := newTestRepo(t)
	writeAndAdd(t, a, wt, "A.md", "base A")
	writeAndAdd(t, a, wt, "B.md", "base B")
	writeAndAdd(t, a, wt, "C.md", "base C")
	writeAndAdd(t, a, wt, "D.md", "base D")
	writeAndAdd(t, a, wt, "E.md", "base E")
	testCommit(t, wt, "initial")

	// Remote (upstream) tree: A and B changed, C unchanged, D and E absent.
	remoteTree := buildFlatTree(t, repo, map[string]string{
		"A.md": "remote A",
		"B.md": "remote B",
		"C.md": "base C",
	})

	// Local uncommitted edits:
	//   A.md: local != remote                 -> CONFLICT
	//   B.md: edited to EXACTLY remote content -> not a conflict
	//   C.md: edited locally, remote==base,
	//         local != remote                  -> CONFLICT
	//   D.md: edited locally, remote lacks it   -> not a conflict
	//   E.md: NOT edited (clean)                -> never considered
	overwrite(t, a, "A.md", "local A")
	overwrite(t, a, "B.md", "remote B")
	overwrite(t, a, "C.md", "local C")
	overwrite(t, a, "D.md", "local D")

	got, err := conflictingPaths(wt, remoteTree)
	if err != nil {
		t.Fatalf("conflictingPaths: %v", err)
	}
	want := []string{"A.md", "C.md"} // sorted
	if len(got) != len(want) {
		t.Fatalf("conflictingPaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("conflictingPaths = %v, want %v", got, want)
		}
	}
}

// overwrite writes a file WITHOUT staging it, so it shows up as a worktree
// modification (git.Modified) - the state conflictingPaths keys on.
func overwrite(t *testing.T, a *App, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(a.StorageDir, rel), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// Fresh install: no commit exists yet. oldTrackedPaths must report an
// empty set (not an error) - this is the exact state in which the old
// Checkout(Force:true) path deleted config.json.
func TestOldTrackedPathsUnbornRepo(t *testing.T) {
	_, repo, _ := newTestRepo(t)
	paths, err := oldTrackedPaths(repo)
	if err != nil {
		t.Fatalf("unexpected error on unborn repo: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected empty set on unborn repo, got %v", paths)
	}
}

func TestOldTrackedPathsAfterCommit(t *testing.T) {
	a, repo, wt := newTestRepo(t)
	writeAndAdd(t, a, wt, "md/Note.md", "Title: N\n\nbody")
	writeAndAdd(t, a, wt, "html/Note.html", "<html></html>")
	testCommit(t, wt, "initial")

	paths, err := oldTrackedPaths(repo)
	if err != nil {
		t.Fatalf("oldTrackedPaths: %v", err)
	}
	for _, want := range []string{"md/Note.md", "html/Note.html"} {
		if !paths[want] {
			t.Errorf("expected %q tracked, got %v", want, paths)
		}
	}
	if paths["config.json"] {
		t.Error("config.json wrongly reported as tracked")
	}
}

// The core force-pull regression test: writeTreeToWorktree must restore or
// overwrite exactly the files in the given tree - and must never touch a
// file outside it, no matter what state the worktree is in.
func TestWriteTreeToWorktreeRestoresTrackedOnly(t *testing.T) {
	a, repo, wt := newTestRepo(t)

	// Tracked content, committed.
	writeAndAdd(t, a, wt, "md/Keep.md", "original keep")
	writeAndAdd(t, a, wt, "md/Restore.md", "original restore")
	testCommit(t, wt, "initial")

	// This device's local, never-tracked config - present in the same dir,
	// exactly like a real install.
	configPath := filepath.Join(a.StorageDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"server_port":8080}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Sabotage the worktree the way a broken/partial state would:
	// one tracked file deleted, one modified.
	if err := os.Remove(filepath.Join(a.StorageDir, "md", "Restore.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.StorageDir, "md", "Keep.md"), []byte("local edit"), 0644); err != nil {
		t.Fatal(err)
	}

	written, err := a.writeTreeToWorktree(repo, wt, headTree(t, repo))
	if err != nil {
		t.Fatalf("writeTreeToWorktree: %v", err)
	}

	// Both tracked files back to tree content.
	for rel, want := range map[string]string{
		"md/Keep.md":    "original keep",
		"md/Restore.md": "original restore",
	} {
		got, err := os.ReadFile(filepath.Join(a.StorageDir, rel))
		if err != nil {
			t.Fatalf("%s missing after write: %v", rel, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
		if !written[rel] {
			t.Errorf("%s not reported in written set", rel)
		}
	}

	// THE invariant: config.json untouched, byte for byte.
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config.json was deleted: %v", err)
	}
	if string(got) != `{"server_port":8080}` {
		t.Errorf("config.json modified: %q", got)
	}
	if written["config.json"] {
		t.Error("config.json wrongly reported as written")
	}

	// Index must reflect the tree. Two checks, both unambiguous:
	//
	// 1) Authoritative: read the index back and verify each file has an
	//    entry whose blob hash matches the tree's.
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatalf("reading back index: %v", err)
	}
	tree := headTree(t, repo)
	for _, rel := range []string{"md/Keep.md", "md/Restore.md"} {
		entry, err := idx.Entry(rel)
		if err != nil {
			t.Errorf("%s missing from rebuilt index: %v", rel, err)
			continue
		}
		treeFile, err := tree.File(rel)
		if err != nil {
			t.Fatalf("tree.File(%s): %v", rel, err)
		}
		if entry.Hash != treeFile.Hash {
			t.Errorf("%s index hash %s != tree hash %s", rel, entry.Hash, treeFile.Hash)
		}
	}

	// 2) Status: check MAP MEMBERSHIP directly. go-git's Status map only
	//    contains changed/untracked files - clean files are absent - and
	//    status.File() fabricates a default Untracked entry for absent
	//    paths, which is what made the previous version of this assertion
	//    misreport clean files as dirty. Absent from the map = clean =
	//    pass; present is a failure unless explicitly Unmodified.
	status, err := wt.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	for _, rel := range []string{"md/Keep.md", "md/Restore.md"} {
		if fs, inMap := status[rel]; inMap &&
			(fs.Worktree != git.Unmodified || fs.Staging != git.Unmodified) {
			t.Errorf("%s not clean after index rebuild: staging=%q worktree=%q",
				rel, string(fs.Staging), string(fs.Worktree))
		}
	}
}

// A tree that gained a new nested file must have its directories created.
func TestWriteTreeToWorktreeCreatesNestedDirs(t *testing.T) {
	a, repo, wt := newTestRepo(t)
	writeAndAdd(t, a, wt, "md/deep/nested/New.md", "nested content")
	testCommit(t, wt, "initial")

	// Wipe the whole md tree, as if this device never had it.
	if err := os.RemoveAll(filepath.Join(a.StorageDir, "md")); err != nil {
		t.Fatal(err)
	}

	if _, err := a.writeTreeToWorktree(repo, wt, headTree(t, repo)); err != nil {
		t.Fatalf("writeTreeToWorktree: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "deep", "nested", "New.md"))
	if err != nil {
		t.Fatalf("nested file not recreated: %v", err)
	}
	if string(got) != "nested content" {
		t.Errorf("nested file content = %q", got)
	}
}

func TestPremergeHeadRoundTrip(t *testing.T) {
	a := &App{StorageDir: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(a.StorageDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Nothing saved yet.
	if _, ok := a.loadPremergeHead(); ok {
		t.Error("loadPremergeHead reported a hash before any save")
	}

	h := plumbing.NewHash("0123456789abcdef0123456789abcdef01234567")
	a.savePremergeHead(h)
	got, ok := a.loadPremergeHead()
	if !ok {
		t.Fatal("saved pre-merge head not loadable")
	}
	if got != h {
		t.Errorf("loaded %s, want %s", got, h)
	}

	a.clearPremergeHead()
	if _, ok := a.loadPremergeHead(); ok {
		t.Error("pre-merge head still loadable after clear")
	}
}

func TestMergeParentRoundTrip(t *testing.T) {
	a := &App{StorageDir: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(a.StorageDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	h := plumbing.NewHash("89abcdef0123456789abcdef0123456789abcdef")
	a.saveMergeParent(h)
	got, ok := a.loadMergeParent()
	if !ok || got != h {
		t.Fatalf("merge parent round trip failed: got %s ok=%v", got, ok)
	}
	a.clearMergeParent()
	if _, ok := a.loadMergeParent(); ok {
		t.Error("merge parent still loadable after clear")
	}
}

// ---------------------------------------------------------------
// aheadOfRemote: "is there anything to push?"
// ---------------------------------------------------------------
//
// A clean worktree is not the same thing as nothing to upload, and conflating
// them stranded commits in two ways users actually hit: a commit whose push
// failed, and a git profile switched after a successful push. Both leave the
// worktree clean and the active remote behind.
//
// These cover the LOCAL half of the comparison, which is the half that catches
// both. The network half - the ls-remote that confirms "nothing to push"
// before saying so - needs a real remote and is not exercised here.

// setTrackingRef points refs/remotes/<remote>/master at a commit, the way a
// successful fetch or push does.
func setTrackingRef(t *testing.T, repo *git.Repository, remote string, h plumbing.Hash) {
	t.Helper()
	ref := plumbing.NewHashReference(plumbing.NewRemoteReferenceName(remote, "master"), h)
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatal(err)
	}
}

// The reported case: the commit succeeded, the push did not. The worktree is
// clean and the commit exists only locally.
func TestAheadOfRemoteAfterAFailedPush(t *testing.T) {
	a, repo, wt := newTestRepo(t)

	writeAndAdd(t, a, wt, "md/First.md", "Title: First\n\nbody\n")
	first := testCommit(t, wt, "pushed")
	setTrackingRef(t, repo, "slot0", first)

	writeAndAdd(t, a, wt, "md/Note.md", "Title: Note\n\nbody\n")
	testCommit(t, wt, "committed but not pushed")

	got := a.aheadOfRemote(repo, "slot0", nil)
	if !got.Unpushed {
		t.Error("a commit the remote has not seen was reported as nothing to push")
	}
	if got.Verified {
		t.Error("no remote was contacted; Verified must not claim otherwise")
	}
}

// The other reported case: the commit was pushed to one profile, then the user
// switched profiles. Each slot owns its own named remote, so the new one
// simply has no master yet - which must read as "might be ahead".
func TestAheadOfRemoteAfterAProfileSwitch(t *testing.T) {
	a, repo, wt := newTestRepo(t)

	writeAndAdd(t, a, wt, "md/First.md", "Title: First\n\nbody\n")
	h := testCommit(t, wt, "pushed to the old profile")
	setTrackingRef(t, repo, "slot0", h)

	// slot1 has never been contacted.
	got := a.aheadOfRemote(repo, "slot1", nil)
	if !got.Unpushed {
		t.Error("a remote that has never seen this branch was reported as up to date")
	}
}

// With the local view saying level, the answer is only trustworthy if the
// remote itself agrees - so it must NOT report "nothing to push" as verified
// when it could not ask.
func TestAheadOfRemoteReportsWhenItCouldNotAsk(t *testing.T) {
	a, repo, wt := newTestRepo(t)

	writeAndAdd(t, a, wt, "md/First.md", "Title: First\n\nbody\n")
	h := testCommit(t, wt, "level with the remote")
	setTrackingRef(t, repo, "slot0", h)

	got := a.aheadOfRemote(repo, "slot0", nil) // no such remote configured
	if got.Verified {
		t.Error("Verified set although no remote could be listed")
	}
	if got.Error == "" {
		t.Error("no reason given for the unverified answer")
	}
}

// An empty repository has nothing to push and nothing to check.
func TestAheadOfRemoteWithNoCommits(t *testing.T) {
	a, repo, _ := newTestRepo(t)

	if got := a.aheadOfRemote(repo, "slot0", nil); got.Unpushed {
		t.Error("an unborn branch was reported as having something to push")
	}
}

// A .txt beside a note lives in md/ and is COPIED into html/, which is where
// its URL resolves (note_files.go). Only the md/ original belongs in git.
func TestGitignoreExcludesDerivedTextCopies(t *testing.T) {
	patterns := make([]gitignore.Pattern, 0, len(gitignorePatterns))
	for _, p := range gitignorePatterns {
		patterns = append(patterns, gitignore.ParsePattern(p, nil))
	}
	matcher := gitignore.NewMatcher(patterns)

	cases := []struct {
		path []string
		want bool
		why  string
	}{
		{[]string{"html", "log.txt"}, true, "the copy of a root note's text file"},
		{[]string{"html", "project", "log.txt"}, true, "one directory down"},
		{[]string{"html", "a", "b", "c", "log.txt"}, true, "deeper still"},
		{[]string{"md", "log.txt"}, false, "THE ORIGINAL - git must carry this one"},
		{[]string{"md", "project", "log.txt"}, false, "the original, one directory down"},
		{[]string{"html", "notes.txt.md"}, false, "a note whose name ends in .txt.md"},
		{[]string{"md", "Welcome.md"}, false, "a normal note"},
	}

	for _, c := range cases {
		full := strings.Join(c.path, "/")
		if got := matcher.Match(c.path, false); got != c.want {
			t.Errorf("matcher.Match(%q) = %v, want %v (%s)", full, got, c.want, c.why)
		}
		// The matcher and the untrack rule have to agree, or a file is
		// ignored for new commits and kept in the index for ever.
		if got := isDerivedTextPath(full); got != c.want {
			t.Errorf("isDerivedTextPath(%q) = %v, want %v (%s)", full, got, c.want, c.why)
		}
	}
}

// The html/ copy can be in the index from a time before the rule. It must
// leave the index at the next commit, and it must stay on the disk - the
// other device rebuilds its own copy from the md/ original it already has.
func TestCommitLocalChangesUntracksADerivedTextCopy(t *testing.T) {
	a, repo, wt := newTestRepo(t)
	a.ensureGitignore()

	const copyRel = "html/project/log.txt"
	const srcRel = "md/project/log.txt"
	writeAndAdd(t, a, wt, srcRel, "one\ntwo\n")
	writeAndAdd(t, a, wt, copyRel, "one\ntwo\n")
	writeAndAdd(t, a, wt, ".gitignore", string(mustRead(t, filepath.Join(a.StorageDir, ".gitignore"))))
	testCommit(t, wt, "before the rule")

	pending := a.untrackTrackedPaths(repo)
	if len(pending) != 1 || pending[0] != copyRel+derivedTextPreviewNote {
		t.Errorf("untrackTrackedPaths = %v, want [%s]", pending, copyRel+derivedTextPreviewNote)
	}

	committed, err := a.commitLocalChanges(repo, wt, "stop to track the copies under html/")
	if err != nil {
		t.Fatalf("commitLocalChanges: %v", err)
	}
	if !committed {
		t.Fatal("no commit was made, want the removal of the copy")
	}

	tree := headTree(t, repo)
	if _, err := tree.File(copyRel); err == nil {
		t.Errorf("%s is still in the commit", copyRel)
	}
	if _, err := tree.File(srcRel); err != nil {
		t.Errorf("%s left the commit, but it is the original: %v", srcRel, err)
	}

	// "git rm --cached", not "git rm": the served copy stays on this device.
	if _, err := os.Stat(filepath.Join(a.StorageDir, copyRel)); err != nil {
		t.Errorf("the html/ copy left the disk: %v", err)
	}

	if left := a.untrackTrackedPaths(repo); len(left) != 0 {
		t.Errorf("the index still holds %v", left)
	}
}

// A pull brings the md/ original and no html/ copy. Rebuilding the copy is
// what keeps the file's URL working before the next start.
func TestSyncNoteFilesToHTMLRebuildsAfterAPull(t *testing.T) {
	a := &App{StorageDir: t.TempDir()}
	src := filepath.Join(a.StorageDir, "md", "project", "log.txt")
	if err := os.MkdirAll(filepath.Dir(src), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("one\ntwo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	a.syncNoteFilesToHTML()

	copyPath := filepath.Join(a.StorageDir, "html", "project", "log.txt")
	got, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatalf("the html/ copy was not made: %v", err)
	}
	if string(got) != "one\ntwo\n" {
		t.Errorf("the copy holds %q", got)
	}
}
