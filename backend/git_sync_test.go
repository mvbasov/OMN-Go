package backend

// ----------------------------------------------------------------------
// The sync test harness
// ----------------------------------------------------------------------
//
// git_helper.go was the largest file of the project and the least
// tested. It became four files in 26.09.22, and git_sync.go holds the
// code below. Before this file, syncPull, syncPullMerge, syncPullForce, syncPullAbort,
// syncPush, SyncRepo, getOrInitRepo, manualStageFile and getSSHAuth each
// had NO test at all. That is the code that can destroy the notes of a
// user. The banners of syncPull and writeTreeToWorktree each name a
// data-loss fault that already happened.
//
// The reason for the gap was the belief that a sync needs a git server.
// It does not. go-git talks to a BARE REPOSITORY ON DISK with no git
// binary, no network and no SSH daemon. gsRemote makes one in t.TempDir()
// and the tests drive the real sync code against it.
//
// THE ONE THING THAT NEEDS CARE IS THE AUTHENTICATION. getSSHAuth reads
// the active slot and refuses an empty SSHKeyData, thus SyncRepo stops
// before it reaches a sync path. A test that wants SyncRepo must give the
// slot a key that parses.
//
// gsSSHKey makes such a key. The key is real, and the local transport of
// go-git ignores it. A fetch from a path on disk gives the same answer
// with the key and with no auth at all. The production code is therefore
// unchanged. Each test still goes through getSSHAuth the way the
// application does.
//
// WHAT THIS HARNESS DOES NOT COVER. It cannot test the SSH transport
// itself, which needs a server. It cannot test a network failure. Each
// test below is about what OMN-Go does with the objects that a fetch
// brings, which is where each known fault of this subsystem was.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	cryptossh "golang.org/x/crypto/ssh"
)

// gsSignature is the author of each commit that the harness makes. A
// commit with no author fails, and a fixed one keeps a test readable.
func gsSignature() *object.Signature {
	return &object.Signature{Name: "Harness", Email: "harness@example.invalid"}
}

// gsSSHKey returns a private key in the OpenSSH PEM form that
// getSSHAuth parses. See the banner for why a test needs one.
func gsSSHKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("making a test key: %v", err)
	}
	block, err := cryptossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("encoding the test key: %v", err)
	}
	return string(pem.EncodeToMemory(block))
}

// gsRemote makes an empty bare repository and returns its path. The path
// goes into a git-server slot as the URL of that slot.
func gsRemote(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "remote.git")
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatalf("making the bare remote: %v", err)
	}
	return dir
}

// gsApp builds an application whose active git-server slot points at
// remote. The storage directory is empty except for md/ and html/, the
// same as each other test of this package.
func gsApp(t *testing.T, remote string) *App {
	t.Helper()
	a := newTestApp(t)
	a.WithConfig(func(c *Config) {
		c.GitServers = make([]GitServerConfig, maxGitServers)
		c.GitServers[0].Name = "harness"
		c.GitServers[0].URL = remote
		c.GitServers[0].SSHKeyData = gsSSHKey(t)
		c.ActiveGitIndex = 0
	})
	return a
}

// gsSeedRemote writes one commit into the bare repository, from a work
// tree of its own. Call it more than one time to build a history.
//
// A value of "" deletes that path in the new commit. That is how a test
// makes the remote drop a file.
func gsSeedRemote(t *testing.T, remote, message string, files map[string]string) plumbing.Hash {
	t.Helper()
	work := filepath.Join(t.TempDir(), "seed")

	// PlainClone refuses an empty repository, thus the first commit needs
	// an init and a remote of its own.
	repo, err := git.PlainClone(work, false, &git.CloneOptions{URL: remote})
	if err != nil {
		if err := os.MkdirAll(work, 0o755); err != nil {
			t.Fatalf("making the seed work tree: %v", err)
		}
		repo, err = git.PlainInit(work, false)
		if err != nil {
			t.Fatalf("init of the seed work tree: %v", err)
		}
		if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{
			Name: "origin", URLs: []string{remote},
		}); err != nil {
			t.Fatalf("adding the seed remote: %v", err)
		}
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("seed worktree: %v", err)
	}
	for rel, content := range files {
		full := filepath.Join(work, filepath.FromSlash(rel))
		if content == "" {
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				t.Fatalf("removing %s in the seed: %v", rel, err)
			}
			if _, err := wt.Remove(rel); err != nil {
				t.Fatalf("staging the removal of %s: %v", rel, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("making the directory of %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("writing %s in the seed: %v", rel, err)
		}
		if _, err := wt.Add(rel); err != nil {
			t.Fatalf("staging %s: %v", rel, err)
		}
	}

	hash, err := wt.Commit(message, &git.CommitOptions{Author: gsSignature()})
	if err != nil {
		t.Fatalf("committing in the seed: %v", err)
	}
	if err := repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []gitconfig.RefSpec{"refs/heads/master:refs/heads/master"},
	}); err != nil {
		t.Fatalf("pushing the seed: %v", err)
	}
	return hash
}

// gsRemoteHead returns the commit that refs/heads/master of the bare
// repository names.
func gsRemoteHead(t *testing.T, remote string) plumbing.Hash {
	t.Helper()
	repo, err := git.PlainOpen(remote)
	if err != nil {
		t.Fatalf("opening the remote: %v", err)
	}
	ref, err := repo.Reference(plumbing.ReferenceName("refs/heads/master"), true)
	if err != nil {
		t.Fatalf("reading refs/heads/master of the remote: %v", err)
	}
	return ref.Hash()
}

// gsLocalHead returns the commit that HEAD of the storage repository
// names. It reports the zero hash when the branch has no commit.
func gsLocalHead(t *testing.T, a *App) plumbing.Hash {
	t.Helper()
	repo, err := git.PlainOpen(a.StorageDir)
	if err != nil {
		t.Fatalf("opening the storage repository: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		return plumbing.ZeroHash
	}
	return head.Hash()
}

// gsWrite writes one file into the storage directory and makes each
// directory above it.
func gsWrite(t *testing.T, a *App, rel, content string) {
	t.Helper()
	full := filepath.Join(a.StorageDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("making the directory of %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", rel, err)
	}
}

// gsRead returns the content of one file of the storage directory. It
// fails the test when the file is absent, thus a caller that expects an
// absent file must use gsExists.
func gsRead(t *testing.T, a *App, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(a.StorageDir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(data)
}

// gsExists tells whether one path of the storage directory is there.
func gsExists(a *App, rel string) bool {
	_, err := os.Stat(filepath.Join(a.StorageDir, filepath.FromSlash(rel)))
	return err == nil
}

// ----------------------------------------------------------------------
// The harness itself
// ----------------------------------------------------------------------

// The harness must build a remote that go-git can read. A fault of the
// harness would otherwise read as a fault of the application.
func TestSyncHarnessBuildsAReadableRemote(t *testing.T) {
	remote := gsRemote(t)
	first := gsSeedRemote(t, remote, "first", map[string]string{
		"md/One.md": "one\n",
	})
	if got := gsRemoteHead(t, remote); got != first {
		t.Fatalf("the remote head is %s, want %s", got, first)
	}

	second := gsSeedRemote(t, remote, "second", map[string]string{
		"md/Two.md": "two\n",
	})
	if second == first {
		t.Fatal("the second commit has the hash of the first one")
	}
	if got := gsRemoteHead(t, remote); got != second {
		t.Fatalf("the remote head is %s, want the second commit %s", got, second)
	}
}

// getSSHAuth refuses an empty key and stops SyncRepo before any sync
// path. The harness therefore plants a key that parses. This test holds
// that rule: a change of the key format in getSSHAuth breaks here, and
// not in six tests at once.
func TestSyncHarnessGivesAnAuthThatParses(t *testing.T) {
	a := gsApp(t, gsRemote(t))

	auth, err := a.getSSHAuth()
	if err != nil {
		t.Fatalf("getSSHAuth with the harness key: %v", err)
	}
	if auth == nil {
		t.Fatal("getSSHAuth gave no auth method and no error")
	}

	// The same call with no key must still fail. A test that forgets the
	// key would otherwise pass for the wrong reason.
	b := newTestApp(t)
	b.WithConfig(func(c *Config) {
		c.GitServers = make([]GitServerConfig, maxGitServers)
		c.GitServers[0].URL = "git@example.invalid:notes.git"
		c.ActiveGitIndex = 0
	})
	if _, err := b.getSSHAuth(); err == nil {
		t.Error("getSSHAuth accepted an empty SSH key")
	}
}

// getOrInitRepo makes the repository on the first call and opens it on
// each call after that. It also writes .gitignore. This is the entry
// point of each sync, thus a fault here stops everything below it.
func TestSyncHarnessInitializesTheStorageRepo(t *testing.T) {
	a := gsApp(t, gsRemote(t))

	repo, err := a.getOrInitRepo()
	if err != nil {
		t.Fatalf("getOrInitRepo: %v", err)
	}
	if repo == nil {
		t.Fatal("getOrInitRepo gave no repository and no error")
	}
	if !gsExists(a, ".git") {
		t.Error("getOrInitRepo wrote no .git directory")
	}
	if !gsExists(a, ".gitignore") {
		t.Error("getOrInitRepo wrote no .gitignore")
	}

	// The second call opens the same repository and makes no second one.
	if _, err := a.getOrInitRepo(); err != nil {
		t.Fatalf("the second getOrInitRepo: %v", err)
	}
}

// ensureRemotesAndGetActive names the remote of the active slot. Each
// sync path takes that name, thus a wrong name reaches every path.
func TestSyncHarnessResolvesTheActiveRemote(t *testing.T) {
	remote := gsRemote(t)
	a := gsApp(t, remote)

	repo, err := a.getOrInitRepo()
	if err != nil {
		t.Fatalf("getOrInitRepo: %v", err)
	}
	name, err := a.ensureRemotesAndGetActive(repo)
	if err != nil {
		t.Fatalf("ensureRemotesAndGetActive: %v", err)
	}
	if name != slotRemoteName(0) {
		t.Fatalf("the active remote is %q, want %q", name, slotRemoteName(0))
	}

	r, err := repo.Remote(name)
	if err != nil {
		t.Fatalf("the remote %q does not exist: %v", name, err)
	}
	if urls := r.Config().URLs; len(urls) != 1 || urls[0] != remote {
		t.Errorf("the remote points at %v, want %s", urls, remote)
	}
}

// ----------------------------------------------------------------------
// The first sync path
// ----------------------------------------------------------------------

// A pull into a storage directory with no commit of its own brings each
// file of the remote. This is the path that a new device takes. It is
// also the proof that the harness drives the real code and not a copy.
//
// The call goes through SyncRepo, thus it covers getOrInitRepo,
// ensureRemotesAndGetActive, getSSHAuth, the action switch and syncPull
// together. B2 adds one test for each of the other five paths.
func TestSyncPullFastForward(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{
		"md/One.md":        "Title: One\n\nthe first note\n",
		"md/sub/Deep.md":   "a note in a subdirectory\n",
		"html/user_json/x": "not a note\n",
	})
	a := gsApp(t, remote)

	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("SyncRepo(pull): %v", err)
	}

	if got := gsRead(t, a, "md/One.md"); !strings.Contains(got, "the first note") {
		t.Errorf("md/One.md holds %q", got)
	}
	if got := gsRead(t, a, "md/sub/Deep.md"); !strings.Contains(got, "subdirectory") {
		t.Errorf("md/sub/Deep.md holds %q", got)
	}
	if local, want := gsLocalHead(t, a), gsRemoteHead(t, remote); local != want {
		t.Errorf("the local head is %s, want the remote head %s", local, want)
	}

	// A second pull with nothing new must answer without a fault. The
	// application calls this path at each start of a sync.
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the second SyncRepo(pull): %v", err)
	}
}

// trackedWorktreeIsDirty is the guard that stops a pull over a local
// change. Each pull path asks it, thus a wrong answer either loses the
// work of the reader or refuses a pull that is safe.
//
// The test also proves that gsWrite reaches the same work tree that the
// sync code reads. B2 uses that helper to make each conflict.
func TestSyncHarnessSeesALocalChange(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("SyncRepo(pull): %v", err)
	}

	repo, err := a.getOrInitRepo()
	if err != nil {
		t.Fatalf("getOrInitRepo: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	dirty, err := trackedWorktreeIsDirty(wt)
	if err != nil {
		t.Fatalf("trackedWorktreeIsDirty after a clean pull: %v", err)
	}
	if dirty {
		t.Error("the work tree reads as dirty after a pull that changed nothing")
	}

	gsWrite(t, a, "md/One.md", "one, and a local change\n")
	dirty, err = trackedWorktreeIsDirty(wt)
	if err != nil {
		t.Fatalf("trackedWorktreeIsDirty after a local change: %v", err)
	}
	if !dirty {
		t.Error("a changed tracked file does not read as dirty, thus a pull would write over it")
	}

	// An untracked file is not a local change. .gitignore covers most of
	// them, and a pull must not refuse over a file that git never held.
	gsWrite(t, a, "html/Scratch.html", "not tracked\n")
	dirty, err = trackedWorktreeIsDirty(wt)
	if err != nil {
		t.Fatalf("trackedWorktreeIsDirty with an untracked file: %v", err)
	}
	if !dirty {
		t.Error("the tracked change of the step above was lost")
	}
}

// ----------------------------------------------------------------------
// The six sync paths
// ----------------------------------------------------------------------
//
// One test for each action of the SyncRepo switch. Two more tests hold a
// rule that a banner of git_sync.go names and that no test held. A pull
// must not touch a database file. A pull must also remake the html/ copy
// of a text file that lives beside a note.

// A note that another device deleted must go away here as well. The rule
// is narrow on purpose. syncPull removes a path that WAS tracked and is
// no longer in the remote tree, and it removes nothing else. A wider rule
// would delete config.json and each database file.
func TestSyncPullRemovesAFileTheRemoteDropped(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "two notes", map[string]string{
		"md/Keep.md": "keep me\n",
		"md/Drop.md": "drop me\n",
	})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	if !gsExists(a, "md/Drop.md") {
		t.Fatal("the first pull did not bring md/Drop.md")
	}

	// A file that git never tracked must survive the pull below.
	gsWrite(t, a, "html/user_json/local-notes.json", "{}\n")

	gsSeedRemote(t, remote, "drop one note", map[string]string{"md/Drop.md": ""})
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the second pull: %v", err)
	}

	if gsExists(a, "md/Drop.md") {
		t.Error("md/Drop.md is still here after the remote dropped it")
	}
	if !gsExists(a, "md/Keep.md") {
		t.Error("md/Keep.md went away with the dropped note")
	}
	if !gsExists(a, "html/user_json/local-notes.json") {
		t.Error("the pull deleted a file that git never tracked")
	}
}

// A pull must refuse while a tracked file holds a change that no commit
// carries. The alternative is a silent loss of the work of the reader.
// The answer is ErrSyncConflict, which handleSync turns into the modal
// that offers an abort and a 3-way merge.
func TestSyncPullRefusesOverALocalChange(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}

	gsWrite(t, a, "md/One.md", "the text of this device\n")
	gsSeedRemote(t, remote, "second", map[string]string{"md/One.md": "the text of the other device\n"})

	err := a.SyncRepo("pull", "")
	if !errors.Is(err, ErrSyncConflict) {
		t.Fatalf("the pull answered %v, want ErrSyncConflict", err)
	}
	if got := gsRead(t, a, "md/One.md"); got != "the text of this device\n" {
		t.Errorf("the pull changed the local file to %q", got)
	}

	// The conflict carries the files in contention, and the modal lists
	// them. A conflict with no list leaves the reader with no information.
	var conflict *syncConflictError
	if errors.As(err, &conflict) {
		if len(conflict.Files) != 1 || conflict.Files[0] != "md/One.md" {
			t.Errorf("the conflict names %v, want md/One.md alone", conflict.Files)
		}
	} else {
		t.Error("the conflict carries no file list")
	}
}

// pull_mark writes diff3 markers into each file in contention. The reader
// then resolves them by hand and pushes. The local branch stays where it
// is. The remote tip is saved as a pending second parent, thus the next
// commit is a real merge commit. See the banner of syncPullMerge.
func TestSyncPullMergeWritesMarkers(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	base := gsLocalHead(t, a)

	gsWrite(t, a, "md/One.md", "the text of this device\n")
	gsSeedRemote(t, remote, "second", map[string]string{"md/One.md": "the text of the other device\n"})

	if err := a.SyncRepo("pull_mark", ""); err != nil {
		t.Fatalf("SyncRepo(pull_mark): %v", err)
	}

	got := gsRead(t, a, "md/One.md")
	for _, want := range []string{
		"<<<<<<< LOCAL",
		"the text of this device",
		"||||||| BASE",
		"the first text",
		"=======",
		"the text of the other device",
		">>>>>>> REMOTE",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the marked file has no %q. It holds:\n%s", want, got)
		}
	}

	if head := gsLocalHead(t, a); head != base {
		t.Errorf("pull_mark moved the local branch to %s, want %s", head, base)
	}
	if _, ok := a.loadMergeParent(); !ok {
		t.Error("pull_mark saved no merge parent, thus the next commit is not a merge")
	}
	if _, ok := a.loadPremergeHead(); !ok {
		t.Error("pull_mark saved no pre-merge head, thus pull_abort has nothing to restore")
	}
}

// pull_abort undoes a pull_mark. The file goes back to the content of the
// last commit, and the two saved hashes go away. A reader who opens the
// markers and changes their mind needs this to work.
func TestSyncPullAbortRestoresTheHead(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	base := gsLocalHead(t, a)

	gsWrite(t, a, "md/One.md", "the text of this device\n")
	gsSeedRemote(t, remote, "second", map[string]string{"md/One.md": "the text of the other device\n"})
	if err := a.SyncRepo("pull_mark", ""); err != nil {
		t.Fatalf("SyncRepo(pull_mark): %v", err)
	}

	if err := a.SyncRepo("pull_abort", ""); err != nil {
		t.Fatalf("SyncRepo(pull_abort): %v", err)
	}

	if got := gsRead(t, a, "md/One.md"); got != "the first text\n" {
		t.Errorf("the file holds %q after the abort, want the committed text", got)
	}
	if head := gsLocalHead(t, a); head != base {
		t.Errorf("the head is %s after the abort, want %s", head, base)
	}
	if _, ok := a.loadPremergeHead(); ok {
		t.Error("the abort left the pre-merge head on disk")
	}
	if _, ok := a.loadMergeParent(); ok {
		t.Error("the abort left the merge parent on disk")
	}

	// A second abort has nothing to undo and must answer without a fault.
	if err := a.SyncRepo("pull_abort", ""); err != nil {
		t.Errorf("the second abort: %v", err)
	}
}

// A push commits each local change and sends the branch. This is the one
// path that writes to the remote, thus it is the one path that another
// device sees.
func TestSyncPush(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	before := gsRemoteHead(t, remote)

	gsWrite(t, a, "md/Two.md", "a note of this device\n")
	if err := a.SyncRepo("push", "add a note"); err != nil {
		t.Fatalf("SyncRepo(push): %v", err)
	}

	after := gsRemoteHead(t, remote)
	if after == before {
		t.Fatal("the push moved the remote head nowhere")
	}
	if local := gsLocalHead(t, a); local != after {
		t.Errorf("the local head is %s and the remote head is %s", local, after)
	}

	// A third device must see the note. A clone of the bare repository
	// stands for that device.
	clone := filepath.Join(t.TempDir(), "third")
	if _, err := git.PlainClone(clone, false, &git.CloneOptions{URL: remote}); err != nil {
		t.Fatalf("cloning the remote: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(clone, "md", "Two.md"))
	if err != nil {
		t.Fatalf("md/Two.md did not reach the remote: %v", err)
	}
	if string(data) != "a note of this device\n" {
		t.Errorf("the remote holds %q", string(data))
	}
}

// A push with changes and no message must refuse. The frontend then asks
// the reader for one. A commit with an empty message is a commit that
// nobody can read later.
func TestSyncPushNeedsAMessage(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	before := gsRemoteHead(t, remote)

	gsWrite(t, a, "md/Two.md", "a new note\n")
	if err := a.SyncRepo("push", "   "); !errors.Is(err, ErrCommitMessageRequired) {
		t.Fatalf("the push answered %v, want ErrCommitMessageRequired", err)
	}
	if got := gsRemoteHead(t, remote); got != before {
		t.Error("the refused push still moved the remote head")
	}
}

// A push that the remote refuses must leave the local state as it is. The
// reader pulls first, and no automatic merge happens. See the banner of
// syncPush.
func TestSyncPushRefusesANonFastForward(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}

	// Another device pushes while this one holds an older commit.
	gsSeedRemote(t, remote, "from the other device", map[string]string{"md/Other.md": "other\n"})
	remoteHead := gsRemoteHead(t, remote)

	gsWrite(t, a, "md/Two.md", "a note of this device\n")
	err := a.SyncRepo("push", "add a note")
	if !errors.Is(err, ErrPushConflict) {
		t.Fatalf("the push answered %v, want ErrPushConflict", err)
	}
	if got := gsRemoteHead(t, remote); got != remoteHead {
		t.Error("the refused push changed the remote")
	}
	if !gsExists(a, "md/Two.md") {
		t.Error("the refused push removed the local note")
	}
}

// A force push writes over the remote. The reader asks for this after a
// conflict that a merge cannot answer.
func TestSyncPushForceOverwritesTheRemote(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	gsSeedRemote(t, remote, "from the other device", map[string]string{"md/Other.md": "other\n"})

	gsWrite(t, a, "md/Two.md", "a note of this device\n")
	if err := a.SyncRepo("push_force", "take my copy"); err != nil {
		t.Fatalf("SyncRepo(push_force): %v", err)
	}

	if local, want := gsLocalHead(t, a), gsRemoteHead(t, remote); local != want {
		t.Errorf("the local head is %s and the remote head is %s", local, want)
	}
	clone := filepath.Join(t.TempDir(), "third")
	if _, err := git.PlainClone(clone, false, &git.CloneOptions{URL: remote}); err != nil {
		t.Fatalf("cloning the remote: %v", err)
	}
	if _, err := os.Stat(filepath.Join(clone, "md", "Other.md")); err == nil {
		t.Error("the force push kept the commit of the other device")
	}
}

// THIS IS THE RULE THAT THE BANNER OF syncPull DESCRIBES AND THAT NO TEST
// HELD. A database file is untracked and .gitignore covers it. A pull
// must not remove it and must not write it again.
//
// The identity of the file on disk is what matters, and not the content
// alone. SQLite holds an open handle. A file that goes away and comes
// back with the same bytes is a different file to that handle. The next
// write then answers "attempt to write a readonly database (1032)".
// os.SameFile compares the identity.
func TestSyncPullKeepsTheDatabaseFile(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}

	gsWrite(t, a, "db/notes.sqlite", "not a real database, and that is enough\n")
	before, err := os.Stat(filepath.Join(a.StorageDir, "db", "notes.sqlite"))
	if err != nil {
		t.Fatalf("the database file is absent: %v", err)
	}

	gsSeedRemote(t, remote, "second", map[string]string{"md/Two.md": "two\n"})
	for _, action := range []string{"pull", "pull_force"} {
		if err := a.SyncRepo(action, ""); err != nil {
			t.Fatalf("SyncRepo(%s): %v", action, err)
		}
		after, err := os.Stat(filepath.Join(a.StorageDir, "db", "notes.sqlite"))
		if err != nil {
			t.Fatalf("%s removed the database file: %v", action, err)
		}
		if !os.SameFile(before, after) {
			t.Errorf("%s replaced the database file with another file of the same name", action)
		}
	}
}

// A text file beside a note lives in md/ and the URL of that file reads
// from html/. Git carries the md/ copy alone, thus a pull that brings a
// new md/log.txt must remake the html/ copy. pullDone in SyncRepo does
// that, and nothing tested it.
func TestSyncPullBringsTheHTMLCopyOfANoteFile(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "a note and its text file", map[string]string{
		"md/One.md":  "one\n",
		"md/log.txt": "the first line\n",
	})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("SyncRepo(pull): %v", err)
	}

	if !gsExists(a, "html/log.txt") {
		t.Fatal("the pull left html/log.txt absent, thus the URL of that file answers 404")
	}
	if got := gsRead(t, a, "html/log.txt"); got != "the first line\n" {
		t.Errorf("html/log.txt holds %q", got)
	}
}

// A force pull writes the remote copy over a local change. That is what
// the reader asks for, and it is the difference against a plain pull,
// which refuses. See TestSyncPullRefusesOverALocalChange.
func TestSyncPullForceDiscardsALocalChange(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}

	gsWrite(t, a, "md/One.md", "the text of this device\n")
	gsSeedRemote(t, remote, "second", map[string]string{"md/One.md": "the text of the other device\n"})

	if err := a.SyncRepo("pull_force", ""); err != nil {
		t.Fatalf("SyncRepo(pull_force): %v", err)
	}
	if got := gsRead(t, a, "md/One.md"); got != "the text of the other device\n" {
		t.Errorf("the force pull left %q", got)
	}
	if local, want := gsLocalHead(t, a), gsRemoteHead(t, remote); local != want {
		t.Errorf("the local head is %s, want the remote head %s", local, want)
	}
}

// SyncRepo answers a name it does not know with a fault, and it does no
// work. A typing fault in the frontend must never read as a success.
func TestSyncRepoRefusesAnUnknownAction(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)

	if err := a.SyncRepo("pulll", ""); err == nil {
		t.Error("SyncRepo accepted an action that does not exist")
	}
}

// isNonFastForward reads the MESSAGE of the answer of go-git, because
// go-git wraps no sentinel there. A string test is easy to break, thus it
// has a test of its own.
//
// The first row is the exact text of go-git v5.13.1. An upgrade of go-git
// that changes that text breaks this test, which is the point: the
// push-conflict modal would otherwise go away without a sound.
func TestIsNonFastForward(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"the message of go-git for a push", errors.New("non-fast-forward update: refs/heads/master"), true},
		{"the sentinel that Pull uses", git.ErrNonFastForwardUpdate, true},
		{"a wrapped sentinel", fmt.Errorf("push: %w", git.ErrNonFastForwardUpdate), true},
		{"another fault of the transport", errors.New("dial tcp: connection refused"), false},
		{"a message that names the words later", errors.New("the remote refused a non-fast-forward update"), false},
		{"no fault", nil, false},
	}
	for _, c := range cases {
		if got := isNonFastForward(c.err); got != c.want {
			t.Errorf("%s: isNonFastForward(%v) = %v, want %v", c.name, c.err, got, c.want)
		}
	}
}

// The whole chain must end at the word that the frontend reads. syncPush
// answers ErrPushConflict, syncErrorStatus turns that into
// "push_conflict", and omn-go-sse.js opens the modal on that word.
func TestNonFastForwardReachesThePushConflictStatus(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	gsSeedRemote(t, remote, "from the other device", map[string]string{"md/Other.md": "other\n"})
	gsWrite(t, a, "md/Two.md", "a note of this device\n")

	err := a.SyncRepo("push", "add a note")
	status, message, ok := syncErrorStatus(err)
	if !ok || status != "push_conflict" {
		t.Errorf("the status is %q (known=%v), want push_conflict. The reader then "+
			"gets a plain alert and never sees the force-push control.", status, ok)
	}
	if !strings.Contains(message, "Pull before pushing") {
		t.Errorf("the message is %q, and it does not say what to do", message)
	}
}

// ----------------------------------------------------------------------
// The sync paths against a repository on disk
// ----------------------------------------------------------------------
//
// Each test below moved here from git_helper_test.go in 26.09.22, when
// git_helper.go became four files. They read git_sync.go, and the tests
// above them drive a whole sync against a real remote.
//
// These call the shared helpers of git_repo_test.go, which build a
// repository on disk. The helpers of this file carry the gs prefix, thus
// the two sets never collide.

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
	// go-git requires the entries of a tree to be sorted by name. The map
	// iteration above is unordered. Sort before the encode, or the decode
	// rejects it.
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

// conflictingPaths is the one authority for the file list of the conflict
// modal, and for the marker-writing loop of syncPullMerge. Its selection
// must be exactly one thing. That thing is a tracked file with an
// uncommitted local modification whose content also differs from the remote
// copy. Everything else must be excluded. That covers an edit identical to
// the remote, a file that the remote lacks, and an unmodified file.
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

	// Local uncommitted edits.
	//   - A.md: local != remote                 -> CONFLICT
	//   - B.md: edited to EXACTLY remote content -> not a conflict
	//   - C.md: edited locally, remote==base,
	//     local != remote                       -> CONFLICT
	//   - D.md: edited locally, remote lacks it  -> not a conflict
	//   - E.md: NOT edited (clean)               -> never considered
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

// The core force-pull regression test. writeTreeToWorktree must restore or
// overwrite exactly the files of the given tree. It must never touch a file
// outside that tree, whatever state the worktree is in.
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

	// 2) Status: check MAP MEMBERSHIP directly. The Status map of go-git
	//    holds a changed or untracked file only, and a clean file is
	//    absent from it. status.File() makes a default Untracked entry for
	//    an absent path. That is what made the previous version of this
	//    assertion report a clean file as dirty. Absent from the map means
	//    clean, and it passes. Present is a failure, unless the entry is
	//    explicitly Unmodified.
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
// A clean worktree is not the same thing as nothing to upload. To read them
// as one stranded commits in two ways that users hit. The first is a commit
// whose push failed. The second is a git profile switched after a
// successful push. Both leave the worktree clean and the active remote
// behind.
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

// The other reported case. The commit was pushed to one profile, and then
// the user switched profiles. Each slot owns its own named remote, thus the
// new one has no master yet. That must read as "might be ahead".
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

// With the local view saying level, the answer holds only when the remote
// itself agrees. It must NOT report "nothing to push" as verified when it
// could not ask.
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
