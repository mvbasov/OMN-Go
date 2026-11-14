package gitsync

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"net.basov.omngo/backend/internal/render"
)

// ----------------------------------------------------------------------
// The sync HTTP handlers
// ----------------------------------------------------------------------
//
// /api/sync runs one action and answers with a status word. /api/sync/preview
// tells what an upload would send, and it changes nothing. doc/API.md holds
// both shapes. The banner of repo.go says what each git file holds.

// syncConflict is the conflict answer of /api/sync. It also sends "files",
// the paths in conflict, for the conflict dialog.
type syncConflict struct {
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Files   []string `json:"files"`
}

// NewSyncConflict makes a conflict answer. The list is never null, thus the
// dialog needs no guard.
func NewSyncConflict(message string, files []string) syncConflict {
	if files == nil {
		files = []string{}
	}
	return syncConflict{Status: "conflict", Message: message, Files: files}
}

func (svc Service) HandleSync(w http.ResponseWriter, r *http.Request) {
	// r.FormValue reads the form body first, and then the query string. The
	// page posts a form. TestHandleSyncReadsTheQueryString tests the query
	// string.
	if err := r.ParseForm(); err != nil {
		svc.writeJSON(w, http.StatusOK, render.JSONStatus{Status: "error", Message: "bad request: " + err.Error()})
		return
	}

	action := r.FormValue("action")
	if action == "" {
		action = "pull"
	}
	message := r.FormValue("message")
	force := r.FormValue("force") == "true"

	// The Force checkbox is a separate field. The handler changes the action
	// to its *_force name, thus SyncRepo reads one set of action names.
	if force {
		switch action {
		case "pull", "pull_ff", "download":
			action = "pull_force"
		case "push", "upload":
			action = "push_force"
		}
	}

	if err := svc.SyncRepo(action, message); err != nil {
		// A changed host key carries the two fingerprints, and the page
		// asks the person to trust the new key.
		if change, ok := HostKeyChangeOf(err); ok {
			svc.writeJSON(w, http.StatusOK, hostKeyAnswer{Status: "host_key_changed",
				Message: err.Error(), hostKeyChange: change})
			return
		}
		if status, msg, ok := SyncErrorStatus(err); ok {
			// A conflict carries the paths in conflict, and the dialog lists
			// them. Each other status gets the plain body.
			var ce *SyncConflictError
			if status == "conflict" && errors.As(err, &ce) {
				svc.writeJSON(w, http.StatusOK, NewSyncConflict(msg, ce.Files))
			} else {
				svc.writeJSON(w, http.StatusOK, render.JSONStatus{Status: status, Message: msg})
			}
		} else {
			svc.writeJSON(w, http.StatusOK, render.JSONStatus{Status: "error", Message: err.Error()})
		}
		return
	}

	svc.writeJSON(w, http.StatusOK, render.JSONStatus{Status: "success"})
}

func (svc Service) HandleSyncPreview(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	if action != "upload" {
		http.Error(w, "Only upload preview supported", 400)
		return
	}

	// State.mu stops this read while a sync changes the worktree.
	svc.State.mu.Lock()
	defer svc.State.mu.Unlock()

	repo, err := svc.GetOrInitRepo()
	if err != nil {
		http.Error(w, fmt.Sprintf("Repo init failed: %v", err), 500)
		return
	}
	wTree, err := Worktree(repo)
	if err != nil {
		http.Error(w, fmt.Sprintf("Worktree error: %v", err), 500)
		return
	}

	matcher, err := svc.LoadGitignoreMatcher(wTree)
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
		// Skip an ignored path and the config.json at the root.
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

	// A tracked path that git must not track leaves the repository at the
	// next commit, and the other devices delete it. See
	// untrackLocalOnlyPaths. The status does not show such a path when its
	// content did not change, thus the preview reads the index too.
	files = append(files, svc.UntrackTrackedPaths(repo)...)

	// A clean worktree can still hold a commit that the remote does not have:
	// a push failed, or the active profile changed. The page must then offer
	// the push. The check runs only when no file waits. With a file to
	// commit, the upload pushes in all cases. See
	// doc/decisions/0011-push-each-time-and-let-the-remote-answer.md.
	ahead := unpushedState{}
	if len(files) == 0 {
		if remoteName, rErr := svc.EnsureRemotesAndGetActive(repo); rErr != nil {
			ahead.Error = rErr.Error()
		} else if auth, aErr := svc.GetSSHAuth(); aErr != nil {
			// With no usable key, the local half of the check still finds a
			// failed push.
			ahead = svc.AheadOfRemote(repo, remoteName, nil)
			if !ahead.Verified && ahead.Error == "" {
				ahead.Error = aErr.Error()
			}
		} else {
			ahead = svc.AheadOfRemote(repo, remoteName, auth)
		}
	}

	// The list is never null, thus the page reads .length with no guard.
	if files == nil {
		files = []string{}
	}
	svc.writeJSON(w, http.StatusOK, SyncPreviewResponse{
		Files:       files,
		Unpushed:    ahead.Unpushed,
		Remote:      ahead.Remote,
		Verified:    ahead.Verified,
		RemoteError: ahead.Error,
	})
}
