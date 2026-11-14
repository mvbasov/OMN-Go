package gitsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/storage"
	"net.basov.omngo/backend/internal/testkit"
)

// Tests for internal/gitsync/repo.go and internal/gitsync/commit.go: the ignore
// rules, the staging, and the commit.
//
// sync_test.go holds the tests of the sync code. See the banner of
// internal/gitsync/repo.go for what each git file holds.
//
// THE SHARED HELPERS LIVE HERE. newTestRepo, testCommit, writeAndAdd,
// headTree, mustRead and overwrite build a real repository on disk, and
// sync_test.go calls each of them. This file is the base, because a
// sync test needs a repository and a repository test needs no sync.
// newTestRepo initializes a real on-disk git repo inside a fresh temp dir
// and returns the App rooted there plus the repo and worktree handles.
func newTestRepo(t *testing.T) (*testApp, *git.Repository, *git.Worktree) {
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
	return &testApp{App: &testkit.App{StorageDir: dir}}, repo, wt
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

func writeAndAdd(t *testing.T, a *testApp, wt *git.Worktree, rel, content string) {
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
func gitignoreLines(t *testing.T, a *testApp) map[string]int {
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

// A fresh install writes the .gitignore straight from
// GitignorePatterns. This pins the exact bytes, thus an accidental edit
// to GitignorePatterns is caught. A reorder counts, and so does a
// dropped or an added entry.
func TestEnsureGitignoreFreshInstall(t *testing.T) {
	a := &testApp{App: &testkit.App{StorageDir: t.TempDir()}}
	a.gitSync().EnsureGitignore()

	want := "# OMN-Go sync ignore\n" +
		"config.json\n" +
		"assets_version\n" +
		"session_secret\n" +
		"known_hosts\n" +
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
		"/html/css/OMN-Go/omn-go-logs.css\n" +
		"/html/css/OMN-Go/omn-go-status.css\n" +
		"/html/css/OMN-Go/omn-go-config.css\n" +
		"/html/css/OMN-Go/omn-go-files.css\n" +
		"/html/css/OMN-Go/omn-go-search.css\n" +
		"/html/css/OMN-Go/omn-go-notfound.css\n" +
		"/html/css/OMN-Go/omn-go-external.css\n" +
		"/html/css/OMN-Go/omn-go-editor.css\n" +
		"/html/css/OMN-Go/omn-go-db.css\n" +
		"/html/css/OMN-Go/highlight.default.min.css\n" +
		"/html/css/OMN-Go/katex.min.css\n" +
		"/html/js/OMN-Go/omn-go-compat.js\n" +
		"/html/js/OMN-Go/omn-go-console.js\n" +
		"/html/js/OMN-Go/omn-go-core.js\n" +
		"/html/js/OMN-Go/omn-go-highlight.js\n" +
		"/html/js/OMN-Go/omn-go-nav.js\n" +
		"/html/js/OMN-Go/omn-go-share.js\n" +
		"/html/js/OMN-Go/omn-go-api.js\n" +
		"/html/js/OMN-Go/omn-go-config.js\n" +
		"/html/js/OMN-Go/omn-go-sync.js\n" +
		"/html/js/OMN-Go/omn-go-bookmark.js\n" +
		"/html/js/OMN-Go/omn-go-search.js\n" +
		"/html/js/OMN-Go/omn-go-db.js\n" +
		"/html/js/OMN-Go/omn-go-dialog.js\n" +
		"/html/js/OMN-Go/omn-go-logs.js\n" +
		"/html/js/OMN-Go/omn-go-status.js\n" +
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

// The backfill must add every GitignorePatterns entry that an existing
// install misses. It matches a whole line, and not a substring.
//
// The regression that it guards. "*.woff" is a substring of "*.woff2",
// thus a strings.Contains check would see an install that already has
// "*.woff2" and answer that "*.woff" is present. A raw .woff font would
// then stay committable.
//
// It must also NOT duplicate a pattern that is already there.
func TestEnsureGitignoreBackfillLineExact(t *testing.T) {
	a := &testApp{App: &testkit.App{StorageDir: t.TempDir()}}
	// An old install that predates most of the current list: it has *.woff2
	// (the substring trap) but not *.woff, and is missing /db/ etc.
	existing := "# OMN-Go sync ignore\nconfig.json\n*.html\n*.woff2\n/md/local/\n"
	if err := os.WriteFile(filepath.Join(a.StorageDir, ".gitignore"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	a.gitSync().EnsureGitignore()

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
	for _, patt := range GitignorePatterns {
		if counts[patt] != 1 {
			t.Errorf("pattern %q appears %d time(s), want exactly 1", patt, counts[patt])
		}
	}
}

// An already-complete .gitignore must be left byte-for-byte untouched. The
// backfill finds nothing missing, thus it must not rewrite the file. Above
// all it must not append a duplicate trailing block.
func TestEnsureGitignoreNoRewriteWhenComplete(t *testing.T) {
	a := &testApp{App: &testkit.App{StorageDir: t.TempDir()}}
	a.gitSync().EnsureGitignore() // write the canonical file
	before, err := os.ReadFile(filepath.Join(a.StorageDir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	a.gitSync().EnsureGitignore() // second pass must be a no-op
	after, err := os.ReadFile(filepath.Join(a.StorageDir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("second Service.EnsureGitignore rewrote a complete file:\nbefore %q\nafter  %q", before, after)
	}
}

// The general "local-*" rule replaced "/html/db_backup/local-*/". An
// install that still has the old line must lose it, and it must get the
// new one. The old line is not harmful, but two rules for one job drift
// apart, and the file must stay equal to GitignorePatterns.
func TestEnsureGitignoreDropsTheObsoleteLocalDatabaseRule(t *testing.T) {
	a := &testApp{App: &testkit.App{StorageDir: t.TempDir()}}
	existing := "# OMN-Go sync ignore\nconfig.json\n*.html\n/db/\n/html/db_backup/local-*/\n"
	if err := os.WriteFile(filepath.Join(a.StorageDir, ".gitignore"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	a.gitSync().EnsureGitignore()

	counts := gitignoreLines(t, a)
	if counts["/html/db_backup/local-*/"] != 0 {
		t.Errorf("the obsolete database rule is still in the file %d time(s)", counts["/html/db_backup/local-*/"])
	}
	if counts[GitignoreLocalOnlyPattern] != 1 {
		t.Errorf("%q appears %d time(s), want exactly 1", GitignoreLocalOnlyPattern, counts[GitignoreLocalOnlyPattern])
	}
	// The two buggy negations must still go away.
	if counts["!/html/images/"] != 0 || counts["!/html/images/icons/"] != 0 {
		t.Error("a directory-only negation came back")
	}
}

// The .gitignore pattern and storage.IsLocalOnlyPath are two forms of one rule.
// The pattern decides for a new file, and the function decides for the
// index. They must agree, and the pattern must match a name at each
// depth. This test reads the canonical list, thus it also pins the
// position of "local-*" at the end. go-git reads the patterns from the
// end and stops at the first match.
func TestGitignoreMatchesEachLocalOnlyPath(t *testing.T) {
	patterns := make([]gitignore.Pattern, 0, len(GitignorePatterns))
	for _, p := range GitignorePatterns {
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
		if got := storage.IsLocalOnlyPath(full); got != c.want {
			t.Errorf("storage.IsLocalOnlyPath(%q) = %v, want %v (%s)", full, got, c.want, c.why)
		}
	}
}

// A local-only file can be in the index from a time before the rule. It
// must leave the index at the next commit, and it must stay on the disk.
// The preview must name it before that commit.
func TestCommitLocalChangesUntracksALocalOnlyFile(t *testing.T) {
	a, repo, wt := newTestRepo(t)
	a.gitSync().EnsureGitignore()

	const localRel = "html/user_json/local-data.json"
	const keepRel = "md/Notes.md"
	writeAndAdd(t, a, wt, localRel, `{"device":"pixel7"}`)
	writeAndAdd(t, a, wt, keepRel, "# Notes\n")
	writeAndAdd(t, a, wt, ".gitignore", string(mustRead(t, filepath.Join(a.StorageDir, ".gitignore"))))
	testCommit(t, wt, "before the rule")

	// The preview must announce the removal, although no content changed.
	pending := a.gitSync().UntrackTrackedPaths(repo)
	if len(pending) != 1 || pending[0] != localRel+LocalOnlyPreviewNote {
		t.Errorf("Service.UntrackTrackedPaths = %v, want [%s]", pending, localRel+LocalOnlyPreviewNote)
	}

	committed, err := a.gitSync().CommitLocalChanges(repo, wt, "stop to track the local-only files")
	if err != nil {
		t.Fatalf("Service.CommitLocalChanges: %v", err)
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
	if left := a.gitSync().UntrackTrackedPaths(repo); len(left) != 0 {
		t.Errorf("the index still holds %v", left)
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

// overwrite writes a file WITHOUT staging it, so it shows up as a worktree
// modification (git.Modified) - the state ConflictingPaths keys on.
func overwrite(t *testing.T, a *testApp, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(a.StorageDir, rel), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// A .txt beside a note lives in md/ and is COPIED into html/, which is where
// its URL resolves (internal/storage/note_files.go). Only the md/ original
// belongs in git.
func TestGitignoreExcludesDerivedTextCopies(t *testing.T) {
	patterns := make([]gitignore.Pattern, 0, len(GitignorePatterns))
	for _, p := range GitignorePatterns {
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
		if got := IsDerivedTextPath(full); got != c.want {
			t.Errorf("IsDerivedTextPath(%q) = %v, want %v (%s)", full, got, c.want, c.why)
		}
	}
}

// The html/ copy can be in the index from a time before the rule. It must
// leave the index at the next commit, and it must stay on the disk. The
// other device rebuilds its own copy from the md/ original it already has.
func TestCommitLocalChangesUntracksADerivedTextCopy(t *testing.T) {
	a, repo, wt := newTestRepo(t)
	a.gitSync().EnsureGitignore()

	const copyRel = "html/project/log.txt"
	const srcRel = "md/project/log.txt"
	writeAndAdd(t, a, wt, srcRel, "one\ntwo\n")
	writeAndAdd(t, a, wt, copyRel, "one\ntwo\n")
	writeAndAdd(t, a, wt, ".gitignore", string(mustRead(t, filepath.Join(a.StorageDir, ".gitignore"))))
	testCommit(t, wt, "before the rule")

	pending := a.gitSync().UntrackTrackedPaths(repo)
	if len(pending) != 1 || pending[0] != copyRel+DerivedTextPreviewNote {
		t.Errorf("Service.UntrackTrackedPaths = %v, want [%s]", pending, copyRel+DerivedTextPreviewNote)
	}

	committed, err := a.gitSync().CommitLocalChanges(repo, wt, "stop to track the copies under html/")
	if err != nil {
		t.Fatalf("Service.CommitLocalChanges: %v", err)
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

	if left := a.gitSync().UntrackTrackedPaths(repo); len(left) != 0 {
		t.Errorf("the index still holds %v", left)
	}
}

// A remote URL can hold a password. Each log line that names a remote shows
// the URL through RedactGitURL, thus the log holds no password.
func TestRemoteLogLinesHideThePassword(t *testing.T) {
	a := newTestApp(t)
	repo, err := a.gitSync().GetOrInitRepo()
	if err != nil {
		t.Fatal(err)
	}
	cfg := a.Config.Get()
	cfg.GitServers = []config.GitServer{{Name: "home", URL: "https://ann:FIRST-SECRET@example.com/n.git"}}
	if _, err := a.gitSync().EnsureSlotRemotes(repo, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.GitServers[0].URL = "https://ann:SECOND-SECRET@example.com/n.git"
	if _, err := a.gitSync().EnsureSlotRemotes(repo, cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.gitSync().EnsureOriginRemote(repo, "https://ann:THIRD-SECRET@example.com/n.git"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Join(a.Logs.Snapshot(), "\n")
	if !strings.Contains(lines, "ann@example.com") {
		t.Fatalf("the log names no remote:\n%s", lines)
	}
	for _, secret := range []string{"FIRST-SECRET", "SECOND-SECRET", "THIRD-SECRET"} {
		if strings.Contains(lines, secret) {
			t.Errorf("the log holds %s:\n%s", secret, lines)
		}
	}
}

// ---------------------------------------------------------------------
// storage.VersionDependentAssets and GitignorePatterns agree
//
// storage.VersionDependentAssets (internal/storage/assets.go) is the list of
// files that ship with the build and are refreshed on upgrade.
// GitignorePatterns (internal/gitsync/repo.go) keeps those same files
// out of the sync repo of the user. They are two hand-kept lists that must not
// drift. The search feature also makes the first list the single source of
// truth for the own code of OMN-Go. Its integrity thus matters more than it
// did. TestVersionDependentAssetsAllEmbedded covers the embed side. This test
// is the other half.
// ---------------------------------------------------------------------

func TestVersionDependentAssetsAreGitignored(t *testing.T) {
	ignored := map[string]bool{}
	for _, p := range GitignorePatterns {
		ignored[p] = true
	}
	for _, rel := range storage.VersionDependentAssets {
		if !ignored["/"+rel] {
			t.Errorf("storage.VersionDependentAssets has %q but GitignorePatterns has no %q - "+
				"a shipped file that gets committed to the user's repo will "+
				"conflict on every upgrade", rel, "/"+rel)
		}
	}
}

// No path may be in both lists. An entry in both would make the refresh
// install a file and the migration delete it, at each version, forever.
func TestRetiredAssetIsNotShipped(t *testing.T) {
	shipped := map[string]bool{}
	for _, rel := range storage.VersionDependentAssets {
		shipped[rel] = true
	}
	for _, rel := range append(append([]string(nil), storage.RetiredAssets...), storage.RetiredFonts...) {
		if shipped[rel] {
			t.Errorf("%s is retired and version-dependent at the same time", rel)
		}
		for _, pattern := range GitignorePatterns {
			if pattern == "/"+rel {
				t.Errorf("%s is retired and still in GitignorePatterns", rel)
			}
		}
	}
	if len(storage.RetiredFonts) == 0 {
		t.Error("storage.RetiredFonts is empty, thus the old font files stay on each device")
	}
}
