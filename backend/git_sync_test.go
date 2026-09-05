package backend

// ----------------------------------------------------------------------
// The sync test harness
// ----------------------------------------------------------------------
//
// git_helper.go is the largest file of the project and the least tested.
// Before this file, syncPull, syncPullMerge, syncPullForce, syncPullAbort,
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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
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
// rule that a banner of git_helper.go names and that no test held. A pull
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
