package backend

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
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
)

// ----------------------------------------------------------------------
// The repository: the ignore rules, the remotes, and the commit
// ----------------------------------------------------------------------
//
// THE SPLIT OF git_helper.go. That file held 2195 lines until 26.09.22.
// It held the Android filesystem wrappers, the repository setup, each
// sync path, and the two HTTP handlers. A reader who looked for one of
// them read past the other three. Four files hold that code now, and no
// line of it changed in the move:
//
//	git_fs.go        The go-billy wrappers for Android.
//	git_repo.go      This file. The ignore rules, the remotes, the
//	                 SSH key, the staging, and commitLocalChanges.
//	git_sync.go      The six sync paths, the merge checkpoints, and
//	                 SyncRepo, which chooses between them.
//	git_handlers.go  handleSync, handleSyncPreview, and the two JSON
//	                 writers that they answer with.
//
// This file answers the question "what does the repository look like
// before a sync starts". It makes the repository and it writes
// .gitignore. It holds one remote for each configured server slot, and
// it reads the SSH key of the active slot. Last, it stages and commits
// what the device changed.
// ---------------------------------------------------------------
// Repository initialisation
// ---------------------------------------------------------------

// gitignorePatterns is the single source of truth for the sync .gitignore.
// Both the initial file (gitignoreBase in ensureGitignore) and the backfill
// that updates existing installs are built from this one list, so adding a
// pattern here is all it takes - the two can no longer drift.
//
// Order matters: a "!negation" must follow the pattern it re-includes
// (e.g. "/html/images/*" before "!/html/images/*.svg"). The bare
// "!/html/images/" / "!/html/images/icons/" negations are deliberately NOT
// here: with go-git's matcher a directory-only negation re-includes every
// file under the directory, silently undoing "/html/images/*". ensureGitignore
// actively strips those buggy lines from any file that still has them.
//
// gitignoreLocalOnlyPattern is the last entry, and the position is part
// of the rule. go-git's matcher reads the patterns from the end and
// stops at the first match, thus the last pattern wins. A file with a
// local-only name thus stays out of git below a "!" negation, for
// example html/images/local-map.svg.
var gitignorePatterns = []string{
	"config.json",
	"assets_version",
	// The HMAC key of the session cookie. It belongs to one device, and a
	// device that holds the key of another device can make a valid cookie
	// for it. See session.go.
	"session_secret",
	"/asset_backups/",
	"*.html",
	"*.woff2",
	"*.woff",
	"/html/images/*",
	"!/html/images/*.svg",
	"/html/images/icons/*",
	"!/html/images/icons/*.svg",
	"/html/css/OMN-Go/omn-go-core.css",
	"/html/css/OMN-Go/Bookmarker.css",
	"/html/css/OMN-Go/highlight.default.min.css",
	"/html/css/OMN-Go/katex.min.css",
	"/html/js/OMN-Go/omn-go-compat.js",
	"/html/js/OMN-Go/omn-go-core.js",
	"/html/js/OMN-Go/omn-go-sse.js",
	"/html/js/OMN-Go/omn-go-editor.js",
	"/html/js/OMN-Go/auto-render.min.js",
	"/html/js/OMN-Go/katex.min.js",
	"/html/js/OMN-Go/highlight.min.js",
	"/html/js/OMN-Go/Bookmarker.js",
	// A .txt beside a note is written in md/ and COPIED into html/, which is
	// where its URL resolves (note_files.go, 26.08.31). Only the md/ copy is
	// the file; the html/ one is made from it at every start. Two copies of
	// one text in git is one too many, and the pair of them is what a merge
	// conflict looks for.
	"/html/**/*.txt",
	"/md/AndroidIntents.md",
	"/md/BookmarksHowTo.md",
	"/md/Database.md",
	"/md/Editor.md",
	"/md/OMNGoTags.md",
	"/md/ScriptRules.md",
	"/md/SQLImport.md",
	"/md/UserManual.md",
	"/md/local/",
	"/db/",
	gitignoreLocalOnlyPattern,
}

// ---------------------------------------------------------------
// The local-only name rule
// ---------------------------------------------------------------
//
// A name that starts with "local-" makes a file, or a whole directory,
// stay on this device. The database backups had this rule first: a
// database with a name such as local-page_counters kept its backups out
// of git through the pattern "/html/db_backup/local-*/". The rule is now
// general and applies to each path.
//
// Examples of a path that git ignores:
//
//	html/user_json/local-data.json     a name that starts with local-
//	md/local-drafts/Monday.md          a directory that starts with local-
//	html/db_backup/local-counters/...  the database rule, as before
//
// The match is on a complete path segment and it is case-sensitive, the
// same as each other pattern in a .gitignore file. "mylocal-data.json"
// is thus a normal file that git tracks.
//
// The rule gives two results:
//
//   - The file does not go to the other devices. A commit does not
//     contain it, and a push does not send it.
//   - A force pull keeps the file. cleanUntrackedFiles deletes an
//     untracked file, but not one that .gitignore matches.
const localOnlyPrefix = "local-"

// gitignoreLocalOnlyPattern is the .gitignore form of the same rule. The
// pattern has no "/", thus go-git compares it with each segment of a
// path, at each depth.
const gitignoreLocalOnlyPattern = localOnlyPrefix + "*"

// isLocalOnlyPath tells if a StorageDir-relative path is local-only: the
// name of the file, or the name of a directory above it, starts with
// "local-".
//
// The function is the rule for the index (see untrackLocalOnlyPaths).
// The .gitignore pattern is the rule for a new file. The two must agree,
// and a test compares them.
func isLocalOnlyPath(name string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(name), "/") {
		if strings.HasPrefix(segment, localOnlyPrefix) {
			return true
		}
	}
	return false
}

// obsoleteGitignoreLines are the lines that ensureGitignore deletes from
// an existing file. The "append what is missing" loop cannot do this,
// because the lines are present.
var obsoleteGitignoreLines = map[string]bool{
	// A directory-only negation re-includes each file below the
	// directory with the go-git matcher. It thus undoes
	// "/html/images/*". The long note in ensureGitignore gives the
	// details.
	"!/html/images/":       true,
	"!/html/images/icons/": true,
	// The general "local-*" rule replaced this line. The result for a
	// database backup is the same, and each other file now gets it too.
	"/html/db_backup/local-*/": true,
	// 26.09.12 moved each app asset below html/js/OMN-Go/ and
	// html/css/OMN-Go/, and it dropped html/css/markdown.css. The old
	// lines must go, or .gitignore grows a line for a file that no
	// longer exists at each version. removeRetiredAssets in assets.go
	// deletes the files themselves. See retiredAssets.
	"/html/css/omn-go-core.css":           true,
	"/html/css/Bookmarker.css":            true,
	"/html/css/highlight.default.min.css": true,
	"/html/css/katex.min.css":             true,
	"/html/css/markdown.css":              true,
	"/html/js/omn-go-compat.js":           true,
	"/html/js/omn-go-core.js":             true,
	"/html/js/omn-go-sse.js":              true,
	"/html/js/omn-go-editor.js":           true,
	"/html/js/auto-render.min.js":         true,
	"/html/js/katex.min.js":               true,
	"/html/js/highlight.min.js":           true,
	"/html/js/Bookmarker.js":              true,
}

func (a *App) ensureGitignore() {
	gitignorePath := filepath.Join(a.StorageDir, ".gitignore")
	//gitignoreBase := "# OMN-Go sync ignore\nconfig.json\n*.html\n/md/local/\n"
	//
	// NOTE: no "!/html/images/" or "!/html/images/icons/" re-inclusion
	// line here (an earlier revision had them). Those would be needed
	// with the real `git` CLI, whose directory-ignore pruning stops it
	// from ever looking inside an ignored directory - the negation
	// re-opens the door so the *.svg exception below still gets seen.
	// This app never uses the `git` binary though: commitLocalChanges /
	// syncPush check every individual file path from wTree.Status()
	// directly against the gitignore matcher, so that traversal problem
	// does not apply here. Verified against go-git is
	// plumbing/format/gitignore matcher: a bare "!/html/images/"
	// pattern's globMatch does not require the whole path to be consumed
	// for a directory-only pattern unless the path stops exactly there,
	// so it ends up matching (and re-including) every file anywhere
	// under html/images/, not just the directory entry itself - silently
	// undoing "/html/images/*" for every file in it, including a plain
	// test PNG dropped there. Same issue applies to the icons/ subtree.
	gitignoreBase := "# OMN-Go sync ignore\n" + strings.Join(gitignorePatterns, "\n") + "\n"
	content, err := os.ReadFile(gitignorePath)
	if os.IsNotExist(err) {
		os.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)
		a.logInfof(logSync, "Created .gitignore")
		return
	}
	if err != nil {
		return
	}

	// Delete each obsolete line from a .gitignore that still has it.
	// Three kinds of install have such a line. One got the buggy
	// gitignoreBase. One got a file that a previous version of the
	// backfill loop wrote. One carries the old database-only rule for
	// the local-* names. The "append what is missing" loop below cannot do
	// this, because the lines are present. They are filtered out and the
	// file is written again. See obsoleteGitignoreLines for the reason of
	// each line.
	rewritten := false
	{
		var kept []string
		for _, line := range strings.Split(string(content), "\n") {
			if obsoleteGitignoreLines[strings.TrimSpace(line)] {
				rewritten = true
				continue
			}
			kept = append(kept, line)
		}
		if rewritten {
			content = []byte(strings.Join(kept, "\n"))
		}
	}

	// The file already exists (every install predating a given entry):
	// append entries that are missing rather than only handling the
	// file-absent case. Without this, /db/ - binary SQLite files that
	// must never be committed - would silently stay unignored on every
	// existing installation.
	// Append any pattern from gitignorePatterns not already present as an
	// EXACT line - a whole-line match, not a substring: "*.woff" is a
	// substring of "*.woff2", which strings.Contains would wrongly treat as
	// already present. Existing installs thus pick up patterns added to the
	// list after their .gitignore was first written.
	present := map[string]bool{}
	for _, line := range strings.Split(string(content), "\n") {
		present[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, patt := range gitignorePatterns {
		if !present[patt] {
			missing = append(missing, patt)
		}
	}
	appended := len(missing) > 0
	if appended {
		text := string(content)
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		content = []byte(text + strings.Join(missing, "\n") + "\n")
	}

	if rewritten || appended {
		if err := os.WriteFile(gitignorePath, content, 0644); err != nil {
			a.logErrf(logSync, "cannot update .gitignore: %v", err)
			return
		}
		a.logInfof(logSync, "Updated .gitignore (rewritten=%v, appended=%v)", rewritten, appended)
	}
}

func (a *App) getOrInitRepo() (*git.Repository, error) {
	a.logDebugf(logSync, "Opening repo at %s", a.StorageDir)

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
		a.logInfof(logSync, "Repo not found, initializing...")
		if initErr := a.manualGitInit(a.StorageDir); initErr != nil {
			return nil, fmt.Errorf("manual init failed: %v", initErr)
		}
		repo, err = git.Open(storer, wtFS)
		if err != nil {
			return nil, fmt.Errorf("failed to open manually created repo: %v", err)
		}
		a.ensureGitignore()
		a.logInfof(logSync, "Repo initialized")
	} else {
		a.logDebugf(logSync, "Repo opened successfully")
		// Backfill any .gitignore entries added to gitignoreBase after this
		// repo was first created (see the appended-entries loop in
		// ensureGitignore). Previously this only ran again on Android, via
		// syncPullForce - every other platform's already-existing repos
		// never got new patterns like /html/images/* applied, so files
		// meant to be ignored (e.g. a test image dropped into the app)
		// kept getting swept into commits on desktop installs.
		a.ensureGitignore()
	}

	// Remote setup/selection happens separately, in
	// ensureRemotesAndGetActive — only sync operations need it (a plain
	// status/preview read does not touch any remote), so it is not done
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
//   - "origin" is a one-time bootstrap remote. It is created only if it
//     does not already exist (seeded from whatever server happens to be
//     active at that moment) and is never modified again afterwards. It
//     exists purely as a fallback for the case where the active slot has
//     no URL configured — not as something that tracks config changes.
//   - Every server slot with a non-empty URL gets its own persistent
//     remote, named deterministically by slot index ("gitserver0" ..
//     "gitserver4") rather than by the user-editable "Name" field, so
//     renaming a server in Config does not orphan its remote. These ARE
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

// ensureOriginRemote creates "origin" the first time it is missing, seeded
// from fallbackURL, and otherwise leaves it untouched.
func (a *App) ensureOriginRemote(repo *git.Repository, fallbackURL string) error {
	if _, err := repo.Remote("origin"); err == nil {
		return nil // already exists — this remote is never modified again
	}
	if fallbackURL == "" {
		return nil // nothing to seed it with yet; try again on a later sync
	}
	a.logInfof(logSync, "Remote origin missing, seeding it once from %s", fallbackURL)
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
				a.logInfof(logSync, "Removing remote %s (slot %d cleared)", name, i)
				if dErr := repo.DeleteRemote(name); dErr != nil {
					a.logErrf(logSync, "failed to remove remote %s: %v", name, dErr)
				}
			}
			continue
		}

		if rErr != nil {
			a.logInfof(logSync, "Adding remote %s -> %s", name, url)
			if _, cErr := repo.CreateRemote(&gitconfig.RemoteConfig{Name: name, URLs: []string{url}}); cErr != nil {
				return "", fmt.Errorf("failed to add remote %s: %v", name, cErr)
			}
			continue
		}

		existing := remote.Config().URLs
		if len(existing) == 1 && existing[0] == url {
			continue // already up to date
		}
		a.logInfof(logSync, "Remote %s URL changed (%v -> %s), updating", name, existing, url)
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
	a.logInfof(logSync, "Active server slot has no URL configured, falling back to origin")
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
	a.logDebugf(logSync, "SSH user: %s", sshUser)

	keyData := gs.SSHKeyData
	if keyData == "" {
		a.logErrf(logSync, "No SSH key configured")
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
	a.logDebugf(logSync, "SSH auth method created using inline key data")
	return publicKeys, nil
}

// ---------------------------------------------------------------
// Staging & committing (manual staging with gitignore filter)
// ---------------------------------------------------------------

// isDerivedTextPath reports whether a path is a copy that html/ holds of a
// text file whose original is in md/ (note_files.go).
//
// The md/ copy is the file. The html/ copy is made from it at every start,
// because that is where its URL resolves. Only one of the two belongs in
// git, and a device that pulls the md/ one rebuilds its own html/ one.
func isDerivedTextPath(name string) bool {
	name = filepath.ToSlash(name)
	return strings.HasPrefix(name, "html/") && isSyncedNoteFile(name)
}

// untrackReason says why a tracked path must leave the index, or "" when it
// must stay. It is the one rule, read by the removal below and by the upload
// preview, so the preview cannot promise something the commit does not do.
func untrackReason(name string) string {
	switch {
	case isLocalOnlyPath(name):
		return localOnlyPreviewNote
	case isDerivedTextPath(name):
		return derivedTextPreviewNote
	}
	return ""
}

// untrackLocalOnlyPaths removes each path that untrackReason names from the
// index and returns the number of the paths that it removed. The files stay
// on the disk: only the index entry goes away, the same as "git rm --cached".
// The next commit thus records a deletion, and a pull deletes the copy on
// the other device.
//
// That is the correct result for both rules. A local-only name says "this
// device only". A .txt under html/ is a copy of the file in md/, and the
// other device makes its own copy from the md/ file it already has (see
// syncNoteFilesToHTML, which SyncRepo also runs after a pull).
//
// The function reads the index directly, as manualStageFile does.
// go-git's Worktree.Remove deletes the file from the disk too, thus it is
// not usable here.
//
// ONLY these two rules cause a removal. The other patterns in the
// .gitignore list must not. A file such as md/UserManual.md, or a
// compiled *.html page, can be in the index of an old repository. A
// removal of each of those at one time would delete many files on the
// other devices.
func (a *App) untrackLocalOnlyPaths(repo *git.Repository) int {
	idx, err := repo.Storer.Index()
	if err != nil {
		a.logErrf(logSync, "cannot read the index to find the files to untrack: %v", err)
		return 0
	}

	kept := make([]*index.Entry, 0, len(idx.Entries))
	removed := 0
	for _, entry := range idx.Entries {
		if why := untrackReason(entry.Name); why != "" {
			a.logDebugf(logSync, "%s%s", entry.Name, why)
			removed++
			continue
		}
		kept = append(kept, entry)
	}
	if removed == 0 {
		return 0
	}

	idx.Entries = kept
	if err := repo.Storer.SetIndex(idx); err != nil {
		a.logErrf(logSync, "cannot write the index after the removal of %d file(s): %v", removed, err)
		return 0
	}
	return removed
}

// localOnlyPreviewNote goes after a path in the upload preview. The file
// is in the list, but the commit deletes it from the repository. It does
// not delete it from this device.
const localOnlyPreviewNote = " (local-only: git stops to track it)"

// derivedTextPreviewNote is the same for a .txt under html/. The file stays
// on this device and is made again at every start from the copy in md/.
const derivedTextPreviewNote = " (a copy of the file in md/: git stops to track it)"

// untrackTrackedPaths reads the index and returns each path that git still
// tracks and untrackLocalOnlyPaths is about to remove, with the note that
// says why, in sorted order. It changes nothing. The upload preview uses it
// to show what the commit does.
func (a *App) untrackTrackedPaths(repo *git.Repository) []string {
	idx, err := repo.Storer.Index()
	if err != nil {
		a.logErrf(logSync, "cannot read the index to find the files to untrack: %v", err)
		return nil
	}
	var out []string
	for _, entry := range idx.Entries {
		if why := untrackReason(entry.Name); why != "" {
			out = append(out, entry.Name+why)
		}
	}
	sort.Strings(out)
	return out
}

func (a *App) commitLocalChanges(repo *git.Repository, wTree *git.Worktree, message string) (bool, error) {
	// Load gitignore matcher
	matcher, err := a.loadGitignoreMatcher(wTree)
	if err != nil {
		a.logErrf(logSync, "could not load .gitignore: %v", err)
		matcher = gitignore.NewMatcher(nil) // no ignore
	}

	// A local-only file can be in the index from a time before the rule,
	// or from before the user gave the file that name. A .gitignore
	// pattern does not remove a file from the index, thus the file keeps
	// its old behavior: a commit does not take the new content, and a
	// force pull writes the old content of the repository over the local
	// file. The removal below is the one-time answer, and it must run
	// before the status test: an unchanged tracked file makes no entry in
	// the status.
	unstaged := a.untrackLocalOnlyPaths(repo)

	a.logDebugf(logSync, "Checking worktree status")
	status, err := wTree.Status()
	if err != nil {
		return false, fmt.Errorf("status check error: %v", err)
	}
	_, mergePending := a.loadMergeParent()
	if status.IsClean() && !mergePending && unstaged == 0 {
		a.logInfof(logSync, "Nothing to commit")
		return false, nil
	}

	hasRealChanges := unstaged > 0
	for name, fileStat := range status {

		// Skip ignored files
		if matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
			a.logDebugf(logSync, "Ignoring %s (matches .gitignore)", name)
			continue
		}

		// Exclude root config.json explicitly
		if name == "config.json" {
			a.logDebugf(logSync, "Ignoring root config.json (preserve locally)")
			continue
		}

		if fileStat.Worktree == git.Deleted {
			a.logDebugf(logSync, "Staging deletion: %s", name)
			_, err := wTree.Remove(name)
			if err != nil {
				a.logErrf(logSync, "failed to remove %s: %v", name, err)
			} else {
				hasRealChanges = true
			}
		} else if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			a.logDebugf(logSync, "Staging file: %s", name)
			if err := a.manualStageFile(repo, wTree, name); err != nil {
				a.logErrf(logSync, "manual staging failed for %s: %v", name, err)
			} else {
				a.logDebugf(logSync, "Staged %s successfully", name)
				hasRealChanges = true
			}
		}
	}

	if !hasRealChanges && !mergePending {
		a.logInfof(logSync, "No real changes could be staged (FUSE false-dirty or ignored)")
		return false, nil
	}

	a.logDebugf(logSync, "Committing staged changes")
	authorName := a.GetConfigAuthor()
	authorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"
	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}

	commitOpts := &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	}

	// If a pull_mark 3-way merge is pending, this commit needs to
	// actually be a merge commit (parents: local HEAD and the remote tip
	// that was merged in), not a plain linear commit - otherwise no real
	// git merge ever took place, and the divergent remote history the
	// user just resolved conflict markers against would simply vanish
	// from the graph. go-git only auto-fills Parents with HEAD when the
	// caller leaves it empty, so HEAD has to be included explicitly here
	// alongside the pending remote parent.
	var pendingMergeParent plumbing.Hash
	hasPendingMerge := false
	if h, ok := a.loadMergeParent(); ok {
		headRef, hErr := repo.Head()
		if hErr != nil {
			return false, fmt.Errorf("could not resolve HEAD for pending merge commit: %v", hErr)
		}
		pendingMergeParent = h
		hasPendingMerge = true
		commitOpts.Parents = []plumbing.Hash{headRef.Hash(), h}
		// The merge resolution may leave the tree identical to one parent
		// (e.g. purely a "take remote's version" resolution) - that is
		// still a legitimate merge commit, not an empty no-op commit.
		commitOpts.AllowEmptyCommits = true
	}

	commitHash, err := wTree.Commit(message, commitOpts)
	if err == git.ErrEmptyCommit {
		a.logInfof(logSync, "Commit aborted: git.ErrEmptyCommit")
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("commit error: %v", err)
	}

	if hasPendingMerge {
		a.clearMergeParent()
		a.logInfof(logSync, "Committed merge with hash: %s (parents: HEAD, %s)", commitHash.String(), pendingMergeParent.String())
	} else {
		a.logInfof(logSync, "Committed with hash: %s", commitHash.String())
	}
	return true, nil
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

// Prevent Android media scanner delete critical empty directoryes
func (a *App) protectGitDirs() {
	if runtime.GOOS != "android" {
		return
	}
	//for _, dir := range []string{"objects", "refs"} {
	for _, dir := range []string{"objects"} {
		p := filepath.Join(a.StorageDir, ".git", dir)
		if err := os.MkdirAll(p, 0755); err != nil {
			a.logErrf(logSync, "MkdirAll %s failed: %v", p, err)
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
