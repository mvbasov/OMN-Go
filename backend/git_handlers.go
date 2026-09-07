package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// ----------------------------------------------------------------------
// The sync HTTP handlers
// ----------------------------------------------------------------------
//
// This file was part of git_helper.go until 26.09.22. See the banner of
// git_repo.go for the split and for what each file holds.
//
// Two endpoints reach the sync code. /api/sync runs one action and
// answers with a status word. /api/sync/preview answers with what a sync
// would do, and it changes nothing. doc/API.md holds both shapes.
// ---------------------------------------------------------------
// HTTP handler
// ---------------------------------------------------------------

// writeSyncJSON writes a small {"status":..., "message":...} JSON body.
// It uses json.Marshal, and not a JSON literal from fmt.Sprintf as the
// original code did. An error message that holds a quote or a backslash
// thus cannot make the body invalid.
func writeSyncJSON(w http.ResponseWriter, status, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status, "message": message})
}

// writeSyncConflictJSON is writeSyncJSON's conflict variant: it adds the list
// of files in contention under "files" so the conflict modal can list them.
// files is always an array (never null) in the JSON, so the frontend can map
// over it without a guard.
func writeSyncConflictJSON(w http.ResponseWriter, message string, files []string) {
	if files == nil {
		files = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "conflict",
		"message": message,
		"files":   files,
	})
}

func (a *App) handleSync(w http.ResponseWriter, r *http.Request) {
	// r.FormValue reads from both the URL query string and a POST body,
	// which is application/x-www-form-urlencoded or multipart. The
	// frontend uses both conventions in different places. omn-go-sse.js
	// posts action, force and message in the body, and the
	// conflict-resolution buttons in index.html reach this endpoint with a
	// query string. This handler thus accepts either one.
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

	// The "Force" checkbox of the UI is a separate field, and not a
	// distinct action name. It is translated into the canonical *_force
	// action here, thus SyncRepo reads one vocabulary only.
	if force {
		switch action {
		case "pull", "pull_ff", "download":
			action = "pull_force"
		case "push", "upload":
			action = "push_force"
		}
	}

	if err := a.SyncRepo(action, message); err != nil {
		if status, msg, ok := syncErrorStatus(err); ok {
			// A conflict carries the list of files in contention (see
			// syncConflictError); surface it so the modal can show which
			// files "Mark Conflicts" would touch. Any other status - or a
			// bare ErrSyncConflict with no file list attached - falls back
			// to the plain {status, message} body.
			var ce *syncConflictError
			if status == "conflict" && errors.As(err, &ce) {
				writeSyncConflictJSON(w, msg, ce.Files)
			} else {
				writeSyncJSON(w, status, msg)
			}
		} else {
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

	// Same reasoning as handleSync: do not let a status/preview read run
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

	// A path that git still tracks but must not leaves the repository at
	// the next commit. See untrackLocalOnlyPaths. Such a path is a
	// local-only name, or a .txt under html/ that is a copy of the file in
	// md/. The status finds no such file if the content did not change,
	// thus the index gets its own read here. The preview must show each
	// change that the commit makes, and this one deletes a file on the
	// other devices.
	files = append(files, a.untrackTrackedPaths(repo)...)

	// No database dry-run here anymore. A backup is an ordinary file under
	// html/db_backup/, written when the user presses "Backup now". See
	// db_backup.go. A pending backup thus already shows up in the status
	// scan above, like every other changed file. The preview needs no
	// special database handling to stay accurate.

	// A clean worktree does not mean there is nothing to upload. A commit
	// whose push failed leaves commits that this remote has never seen,
	// and so does a profile switched after a successful push. An answer
	// about "what is pending" alone is what let the frontend say "Nothing
	// to commit" and stop.
	//
	// Only asked when there is nothing pending anyway: with files to commit
	// the upload proceeds regardless, and the answer would not change what
	// happens next.
	ahead := unpushedState{}
	if len(files) == 0 {
		if remoteName, rErr := a.ensureRemotesAndGetActive(repo); rErr != nil {
			ahead.Error = rErr.Error()
		} else if auth, aErr := a.getSSHAuth(); aErr != nil {
			// No usable key. The local half of the comparison still stands,
			// and it is the half that catches a failed push.
			ahead = a.aheadOfRemote(repo, remoteName, nil)
			if !ahead.Verified && ahead.Error == "" {
				ahead.Error = aErr.Error()
			}
		} else {
			ahead = a.aheadOfRemote(repo, remoteName, auth)
		}
	}

	// files is always an array, never null, so the frontend can read .length
	// without a guard.
	if files == nil {
		files = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(syncPreviewResponse{
		Files:       files,
		Unpushed:    ahead.Unpushed,
		Remote:      ahead.Remote,
		Verified:    ahead.Verified,
		RemoteError: ahead.Error,
	})
}
