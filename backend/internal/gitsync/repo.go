package gitsync

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/filesystem"
	cryptossh "golang.org/x/crypto/ssh"
	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/storage"
)

// ----------------------------------------------------------------------
// The repository: the ignore rules, the remotes, and the commit
// ----------------------------------------------------------------------
//
// Seven files hold the git code:
//
//	fs.go        The go-billy wrappers for Android.
//	repo.go      The ignore rules, the remotes and the SSH key.
//	commit.go    The commit, and the paths that it must not track.
//	sync.go      SyncRepo, the sync errors and the shared helpers.
//	pull.go      The pull paths.
//	push.go      The push, and the test for commits that wait.
//	handlers.go  The HTTP handlers of /api/sync.

// GitignorePatterns is the one list for the sync .gitignore. EnsureGitignore
// writes it for a new install, and it adds each missing line to an old one.
//
// The order is part of the rule. A "!" line must follow the pattern that it
// includes again. A negation of a directory alone, for example
// "!/html/images/", is NOT here. With the go-git matcher, it includes each
// file below the directory, and "/html/images/*" then has no effect.
//
// Keep GitignoreLocalOnlyPattern last. The go-git matcher reads the patterns
// from the end, thus the last match wins. A local-only name thus stays out of
// git below a "!" line, for example html/images/local-map.svg.
var GitignorePatterns = []string{
	"config.json",
	"assets_version",
	// session_secret is the HMAC key of the session cookie. A device with the
	// key of another device can make a valid cookie for it.
	"session_secret",
	// known_hosts holds the git server keys that this device trusts. A pull
	// must not change them. See host_keys.go.
	"known_hosts",
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
	"/html/css/OMN-Go/omn-go-logs.css",
	"/html/css/OMN-Go/omn-go-status.css",
	"/html/css/OMN-Go/omn-go-config.css",
	"/html/css/OMN-Go/omn-go-files.css",
	"/html/css/OMN-Go/omn-go-search.css",
	"/html/css/OMN-Go/omn-go-notfound.css",
	"/html/css/OMN-Go/omn-go-external.css",
	"/html/css/OMN-Go/omn-go-editor.css",
	"/html/css/OMN-Go/omn-go-db.css",
	"/html/css/OMN-Go/highlight.default.min.css",
	"/html/css/OMN-Go/katex.min.css",
	"/html/js/OMN-Go/omn-go-compat.js",
	"/html/js/OMN-Go/omn-go-console.js",
	"/html/js/OMN-Go/omn-go-core.js",
	"/html/js/OMN-Go/omn-go-highlight.js",
	"/html/js/OMN-Go/omn-go-nav.js",
	"/html/js/OMN-Go/omn-go-share.js",
	"/html/js/OMN-Go/omn-go-api.js",
	"/html/js/OMN-Go/omn-go-config.js",
	"/html/js/OMN-Go/omn-go-sync.js",
	"/html/js/OMN-Go/omn-go-bookmark.js",
	"/html/js/OMN-Go/omn-go-search.js",
	"/html/js/OMN-Go/omn-go-logs.js",
	"/html/js/OMN-Go/omn-go-status.js",
	"/html/js/OMN-Go/omn-go-editor.js",
	"/html/js/OMN-Go/auto-render.min.js",
	"/html/js/OMN-Go/katex.min.js",
	"/html/js/OMN-Go/highlight.min.js",
	"/html/js/OMN-Go/Bookmarker.js",
	// A .txt beside a note lives in md/, and the server copies it into html/.
	// Only the md/ file goes to git. See IsDerivedTextPath.
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
	GitignoreLocalOnlyPattern,
}

// GitignoreLocalOnlyPattern is the .gitignore form of the local-only rule of
// internal/storage/paths.go. The pattern has no "/", thus go-git compares it
// with each segment of a path, at each depth.
const GitignoreLocalOnlyPattern = storage.LocalOnlyPrefix + "*"

// obsoleteGitignoreLines are the lines that EnsureGitignore deletes from an
// existing file.
var obsoleteGitignoreLines = map[string]bool{
	// These lines negate a directory alone. See GitignorePatterns.
	"!/html/images/":       true,
	"!/html/images/icons/": true,
	// The general local-* rule covers the same files.
	"/html/db_backup/local-*/": true,
	// These lines name the old places of the app files. removeRetiredAssets
	// in internal/storage/assets.go deletes the files. See storage.RetiredAssets.
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
	// NOT HERE: /html/js/OMN-Go/omn-go-sse.js, the old name of omn-go-api.js.
	// A device with an older version adds that line again when it is absent,
	// thus a delete here would change .gitignore back and forth.
}

func (svc Service) EnsureGitignore() {
	gitignorePath := svc.Layout.File(storage.GitignoreFilename)
	gitignoreBase := "# OMN-Go sync ignore\n" + strings.Join(GitignorePatterns, "\n") + "\n"
	content, err := os.ReadFile(gitignorePath)
	if os.IsNotExist(err) {
		os.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)
		svc.Log(logx.Sync).Infof("Created .gitignore")
		return
	}
	if err != nil {
		return
	}

	// Delete each obsolete line. The loop below cannot do it, because the
	// lines are present. See obsoleteGitignoreLines.
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

	// Add each missing pattern to an existing file. Without this, an old
	// install never ignores a new entry, for example the SQLite files in
	// /db/. The test is a whole-line match, because "*.woff" is a substring
	// of "*.woff2".
	present := map[string]bool{}
	for _, line := range strings.Split(string(content), "\n") {
		present[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, patt := range GitignorePatterns {
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
			svc.Log(logx.Sync).Errf("cannot update .gitignore: %v", err)
			return
		}
		svc.Log(logx.Sync).Infof("Updated .gitignore (rewritten=%v, appended=%v)", rewritten, appended)
	}
}

func (svc Service) GetOrInitRepo() (*git.Repository, error) {
	svc.Log(logx.Sync).Debugf("Opening repo at %s", string(svc.Layout))

	baseFS := osfs.New(string(svc.Layout))
	wtFS := WorktreeFS(baseFS)

	dotFS, err := wtFS.Chroot(".git")
	if err != nil {
		return nil, fmt.Errorf("chroot .git failed: %v", err)
	}

	storer := filesystem.NewStorage(dotFS, cache.NewObjectLRUDefault())
	repo, err := git.Open(storer, wtFS)

	if err != nil {
		svc.Log(logx.Sync).Infof("Repo not found, initializing...")
		if initErr := svc.manualGitInit(string(svc.Layout)); initErr != nil {
			return nil, fmt.Errorf("manual init failed: %v", initErr)
		}
		repo, err = git.Open(storer, wtFS)
		if err != nil {
			return nil, fmt.Errorf("failed to open manually created repo: %v", err)
		}
		svc.EnsureGitignore()
		svc.Log(logx.Sync).Infof("Repo initialized")
	} else {
		svc.Log(logx.Sync).Debugf("Repo opened successfully")
		// Add each new pattern at each open. Without this, a commit can take
		// a file that .gitignore must cover, for example a test image.
		svc.EnsureGitignore()
	}

	// Only a sync needs a remote, thus EnsureRemotesAndGetActive sets the
	// remotes. A status read touches no remote.
	return repo, nil
}

// The app keeps one git remote for each server slot. See
// doc/decisions/0012-keep-one-remote-for-each-git-server-slot.md.
//
// "origin" gets the URL of the active slot one time, and it never changes. A
// sync uses it only when the active slot has no URL. Each slot with a URL has
// the remote gitserver<index>. The name does not come from the Name field,
// because a person can change that field. EnsureSlotRemotes adds, changes and
// removes these remotes at each sync.

// SlotRemoteName answers the remote name of a slot index.
func SlotRemoteName(index int) string {
	return fmt.Sprintf("gitserver%d", index)
}

// EnsureOriginRemote makes "origin" from fallbackURL when it is missing. It
// never changes an existing origin.
func (svc Service) EnsureOriginRemote(repo *git.Repository, fallbackURL string) error {
	if _, err := repo.Remote("origin"); err == nil {
		return nil // already exists — this remote is never modified again
	}
	if fallbackURL == "" {
		return nil // nothing to seed it with yet; try again on a later sync
	}
	svc.Log(logx.Sync).Infof("Remote origin missing, seeding it once from %s", RedactGitURL(fallbackURL))
	_, err := repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{fallbackURL},
	})
	return err
}

// EnsureSlotRemotes makes the remotes match cfg. It answers the remote of the
// active slot, or "origin" when that slot has no URL.
func (svc Service) EnsureSlotRemotes(repo *git.Repository, cfg config.Config) (activeRemoteName string, err error) {
	for i, gs := range cfg.GitServers {
		name := SlotRemoteName(i)
		url := strings.TrimSpace(gs.URL)

		remote, rErr := repo.Remote(name)
		if url == "" {
			if rErr == nil {
				svc.Log(logx.Sync).Infof("Removing remote %s (slot %d cleared)", name, i)
				if dErr := repo.DeleteRemote(name); dErr != nil {
					svc.Log(logx.Sync).Errf("failed to remove remote %s: %v", name, dErr)
				}
			}
			continue
		}

		if rErr != nil {
			svc.Log(logx.Sync).Infof("Adding remote %s -> %s", name, RedactGitURL(url))
			if _, cErr := repo.CreateRemote(&gitconfig.RemoteConfig{Name: name, URLs: []string{url}}); cErr != nil {
				return "", fmt.Errorf("failed to add remote %s: %v", name, cErr)
			}
			continue
		}

		existing := remote.Config().URLs
		if len(existing) == 1 && existing[0] == url {
			continue // already up to date
		}
		old := make([]string, len(existing))
		for j, u := range existing {
			old[j] = RedactGitURL(u)
		}
		svc.Log(logx.Sync).Infof("Remote %s URL changed (%v -> %s), updating", name, old, RedactGitURL(url))
		if dErr := repo.DeleteRemote(name); dErr != nil {
			return "", fmt.Errorf("failed to update remote %s: %v", name, dErr)
		}
		if _, cErr := repo.CreateRemote(&gitconfig.RemoteConfig{Name: name, URLs: []string{url}}); cErr != nil {
			return "", fmt.Errorf("failed to update remote %s: %v", name, cErr)
		}
	}

	if cfg.ActiveGitIndex >= 0 && cfg.ActiveGitIndex < len(cfg.GitServers) {
		if strings.TrimSpace(cfg.GitServers[cfg.ActiveGitIndex].URL) != "" {
			return SlotRemoteName(cfg.ActiveGitIndex), nil
		}
	}
	svc.Log(logx.Sync).Infof("Active server slot has no URL configured, falling back to origin")
	return "origin", nil
}

// EnsureRemotesAndGetActive makes each remote match the config. It answers
// the remote for this sync.
func (svc Service) EnsureRemotesAndGetActive(repo *git.Repository) (string, error) {
	cfg := svc.Config

	bootstrapURL := ""
	if cfg.ActiveGitIndex >= 0 && cfg.ActiveGitIndex < len(cfg.GitServers) {
		bootstrapURL = strings.TrimSpace(cfg.GitServers[cfg.ActiveGitIndex].URL)
	}
	if err := svc.EnsureOriginRemote(repo, bootstrapURL); err != nil {
		return "", err
	}

	return svc.EnsureSlotRemotes(repo, cfg)
}

func (svc Service) manualGitInit(dir string) error {
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
	svc.protectGitDirs()
	svc.EnsureGitignore()

	config := []byte("[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n")
	if err := os.WriteFile(filepath.Join(gitDir, "config"), config, 0644); err != nil {
		return err
	}
	return nil
}

// LoadGitignoreMatcher answers a matcher for the .gitignore of the worktree
// and GitignorePatterns. The built-in list comes last, thus it wins. A force
// pull can write an old or a changed .gitignore from the remote. The list
// then still protects /db/, each local- file and session_secret.
func (svc Service) LoadGitignoreMatcher(wt *git.Worktree) (gitignore.Matcher, error) {
	patterns, err := gitignore.ReadPatterns(wt.Filesystem, []string{})
	if err != nil {
		return nil, err
	}
	for _, p := range GitignorePatterns {
		patterns = append(patterns, gitignore.ParsePattern(p, nil))
	}
	return gitignore.NewMatcher(patterns), nil
}

// manualStageFile writes the file into a new blob and sets its index entry.
// It does not use Add of go-git.
func (svc Service) manualStageFile(repo *git.Repository, wt *git.Worktree, name string) error {
	fullPath := svc.Layout.File(name)
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

	// Stream the file, thus a large file needs little memory.
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

func (svc Service) GetSSHAuth() (transport.AuthMethod, error) {
	// Read one copy of the config. Two separate reads could mix the fields of
	// two servers.
	cfg := svc.Config
	gs := cfg.GitServers[cfg.ActiveGitIndex]

	sshUser := "git"
	if idx := strings.Index(gs.URL, "@"); idx != -1 {
		sshUser = gs.URL[:idx]
	}
	svc.Log(logx.Sync).Debugf("SSH user: %s", sshUser)

	keyData := gs.SSHKeyData
	if keyData == "" {
		svc.Log(logx.Sync).Errf("No SSH key configured")
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
		HostKeyCallback:   svc.HostKeyCallback(),
		HostKeyAlgorithms: svc.HostKeyAlgorithms(SSHHostOf(gs.URL)),
	}
	svc.Log(logx.Sync).Debugf("SSH auth method created using inline key data")
	return publicKeys, nil
}

func (svc Service) configAuthor() string {
	if author := svc.Config.Author; author != "" {
		return author
	}
	return "OMN-Go User"
}

// protectGitDirs keeps the empty .git/objects directory on Android. The media
// scanner can delete an empty directory.
func (svc Service) protectGitDirs() {
	if runtime.GOOS != "android" {
		return
	}
	for _, dir := range []string{"objects"} {
		p := svc.Layout.Git(dir)
		if err := os.MkdirAll(p, 0755); err != nil {
			svc.Log(logx.Sync).Errf("MkdirAll %s failed: %v", p, err)
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

// RedactGitURL removes the password from a remote URL. The user name stays,
// because it is part of the address and not a secret. An address that the
// function cannot parse shows as "(hidden)". Each text that shows a remote
// URL, except the Config page of the admin, calls it.
func RedactGitURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		return raw // scp form, "git@host:path" - it carries no password
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "(hidden)"
	}
	if u.User != nil {
		if name := u.User.Username(); name != "" {
			u.User = url.User(name)
		} else {
			u.User = nil
		}
	}
	return u.String()
}
