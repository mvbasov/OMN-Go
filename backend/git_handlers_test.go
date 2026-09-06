package backend

// ----------------------------------------------------------------------
// The two sync endpoints
// ----------------------------------------------------------------------
//
// git_handlers.go held 4 of 73 statements under test before 26.09.36.
// That is 5.5 percent, and it was the lowest number in the package.
//
// B1 and B2 built the harness of git_sync_test.go and tested the sync
// PATHS. Each one calls SyncRepo or a syncXxx method directly. No test
// went through an http.Request, thus nothing held the layer between the
// browser and the sync code:
//
//   - the translation of the force checkbox into a *_force action.
//   - the default action when the request names none.
//   - the two form conventions that the frontend uses.
//   - the map from a sync error to a wire status.
//   - the file list of a conflict, which the modal lists.
//   - each guard and each answer of the upload preview.
//
// **This is the code that can destroy the notes of a person, and its
// outermost layer had no test.**
//
// THE HARNESS IS THE ONE OF git_sync_test.go. gsRemote makes a bare
// repository on disk, gsApp points a git-server slot at it, and
// gsSeedRemote writes a commit into it. Read the banner of that file for
// why a bare repository on disk is enough, and why the slot needs a key
// that parses.
//
// The helpers below carry the gh prefix. They build a request, call the
// handler and read the answer. Each one is about the HTTP layer, and
// nothing here repeats a test of the sync paths.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ghSync posts a form to handleSync and answers the recorder.
//
// The frontend posts action, force and message in the body. See the
// comment at the top of handleSync.
func ghSync(t *testing.T, a *App, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/sync", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.handleSync(w, r)
	return w
}

// ghSyncQuery calls handleSync with a query string and no body.
//
// The conflict buttons of the page use this form. Both must work, and
// that is why handleSync calls ParseForm and then FormValue.
func ghSyncQuery(t *testing.T, a *App, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/sync?"+query, nil)
	w := httptest.NewRecorder()
	a.handleSync(w, r)
	return w
}

// ghPreview calls handleSyncPreview with a query string.
func ghPreview(t *testing.T, a *App, method, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/api/sync/preview?"+query, nil)
	w := httptest.NewRecorder()
	a.handleSyncPreview(w, r)
	return w
}

// ghBody decodes a JSON answer into a map. It fails the test when the
// body is not JSON, which is the fault that writeSyncJSON exists to stop.
func ghBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, w.Body.String())
	}
	return out
}

// ghStatus reads the status word of a JSON answer.
func ghStatus(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	status, _ := ghBody(t, w)["status"].(string)
	return status
}

// ghPreviewAnswer decodes the answer of the upload preview.
func ghPreviewAnswer(t *testing.T, w *httptest.ResponseRecorder) syncPreviewResponse {
	t.Helper()
	var out syncPreviewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("the preview answer is not JSON: %v\n%s", err, w.Body.String())
	}
	return out
}

// ghCommitLocally makes one commit in the storage repository and pushes
// nothing. The upload preview then has a clean worktree and a commit that
// the remote has never seen.
func ghCommitLocally(t *testing.T, a *App, rel, content, message string) plumbing.Hash {
	t.Helper()
	gsWrite(t, a, rel, content)
	repo, err := git.PlainOpen(a.StorageDir)
	if err != nil {
		t.Fatalf("opening the storage repository: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree of the storage repository: %v", err)
	}
	if _, err := wt.Add(rel); err != nil {
		t.Fatalf("staging %s: %v", rel, err)
	}
	hash, err := wt.Commit(message, &git.CommitOptions{Author: gsSignature()})
	if err != nil {
		t.Fatalf("committing %s: %v", rel, err)
	}
	return hash
}

// ----------------------------------------------------------------------
// The two JSON writers
// ----------------------------------------------------------------------

// The answer of /api/sync must be JSON, and it must stay JSON when the
// message holds a quote or a backslash.
//
// The code before 26.08 built the body with fmt.Sprintf. A remote whose
// error text held a quote then produced a body that no parser reads, and
// the page showed nothing at all. json.Marshal escapes it.
func TestSyncJSONWriterEscapesTheMessage(t *testing.T) {
	w := httptest.NewRecorder()
	writeSyncJSON(w, "error", `a "quoted" name and a \ backslash`)

	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("the content type is %q, want application/json", got)
	}
	body := ghBody(t, w)
	if body["status"] != "error" {
		t.Errorf("the status is %v, want error", body["status"])
	}
	if body["message"] != `a "quoted" name and a \ backslash` {
		t.Errorf("the message came back as %v", body["message"])
	}
}

// The conflict answer must carry an ARRAY under files, and never null.
//
// The modal maps over that value without a guard. A null there is a
// TypeError in the page, and the person sees a conflict with no list and
// no way forward.
func TestSyncConflictWriterAlwaysSendsAnArray(t *testing.T) {
	w := httptest.NewRecorder()
	writeSyncConflictJSON(w, "diverged", nil)

	if strings.Contains(w.Body.String(), `"files":null`) {
		t.Errorf("the body carries a null file list: %s", w.Body.String())
	}
	body := ghBody(t, w)
	files, ok := body["files"].([]interface{})
	if !ok {
		t.Fatalf("files is %T, want an array", body["files"])
	}
	if len(files) != 0 {
		t.Errorf("files holds %v, want an empty array", files)
	}
	if body["status"] != "conflict" {
		t.Errorf("the status is %v, want conflict", body["status"])
	}
}

// ----------------------------------------------------------------------
// handleSync
// ----------------------------------------------------------------------

// A pull that works answers success, and it brings the file.
func TestHandleSyncPullsAndAnswersSuccess(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
	a := gsApp(t, remote)

	w := ghSync(t, a, url.Values{"action": {"pull"}})
	if got := ghStatus(t, w); got != "success" {
		t.Fatalf("the status is %q, want success. The body is %s", got, w.Body.String())
	}
	if got := gsRead(t, a, "md/One.md"); got != "the first text\n" {
		t.Errorf("the pull left %q on disk", got)
	}
}

// A request with no action pulls.
//
// The default sits in handleSync and not in SyncRepo, which answers an
// unknown action with a fault. A person who opens /api/sync with no
// parameter therefore reads the notes of the remote, and writes nothing.
func TestHandleSyncDefaultsToPull(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
	a := gsApp(t, remote)

	if got := ghStatus(t, ghSync(t, a, url.Values{})); got != "success" {
		t.Fatalf("the status is %q, want success", got)
	}
	if !gsExists(a, "md/One.md") {
		t.Error("the request with no action brought no file, thus it did not pull")
	}
}

// The handler reads a query string as well as a body.
//
// The conflict buttons of the page use a query string, and omn-go-sse.js
// posts a body. handleSync calls ParseForm first for that reason. A
// change to FormValue alone would break one of the two callers quietly.
func TestHandleSyncReadsTheQueryString(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
	a := gsApp(t, remote)

	if got := ghStatus(t, ghSyncQuery(t, a, "action=pull")); got != "success" {
		t.Fatalf("the status is %q, want success", got)
	}
	if !gsExists(a, "md/One.md") {
		t.Error("the query string did not reach the action")
	}
}

// A conflict answers the status word AND the list of files in contention.
//
// The modal lists that array. A conflict with no list leaves the person
// with a refusal and no information about what stands in the way.
func TestHandleSyncAnswersTheConflictFileList(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	gsWrite(t, a, "md/One.md", "the text of this device\n")
	gsSeedRemote(t, remote, "second", map[string]string{"md/One.md": "the text of the other device\n"})

	w := ghSync(t, a, url.Values{"action": {"pull"}})
	body := ghBody(t, w)
	if body["status"] != "conflict" {
		t.Fatalf("the status is %v, want conflict. The body is %s", body["status"], w.Body.String())
	}
	files, ok := body["files"].([]interface{})
	if !ok {
		t.Fatalf("files is %T, want an array", body["files"])
	}
	if len(files) != 1 || files[0] != "md/One.md" {
		t.Errorf("files holds %v, want md/One.md alone", files)
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, "abort") {
		t.Errorf("the message is %q, and it does not name the two choices", msg)
	}
}

// A refused push answers push_conflict, and the page then offers the
// force control. A plain error answer hides that control.
func TestHandleSyncAnswersPushConflict(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	gsSeedRemote(t, remote, "from the other device", map[string]string{"md/Other.md": "other\n"})
	gsWrite(t, a, "md/Two.md", "a note of this device\n")

	w := ghSync(t, a, url.Values{"action": {"push"}, "message": {"add a note"}})
	if got := ghStatus(t, w); got != "push_conflict" {
		t.Fatalf("the status is %q, want push_conflict. The body is %s", got, w.Body.String())
	}
}

// A push with no message answers needs_commit_message, and the page then
// asks for one. The notes stay where they are.
func TestHandleSyncAsksForACommitMessage(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	before := gsRemoteHead(t, remote)
	gsWrite(t, a, "md/Two.md", "a new note\n")

	w := ghSync(t, a, url.Values{"action": {"push"}, "message": {"   "}})
	if got := ghStatus(t, w); got != "needs_commit_message" {
		t.Fatalf("the status is %q, want needs_commit_message. The body is %s",
			got, w.Body.String())
	}
	if got := gsRemoteHead(t, remote); got != before {
		t.Error("the refused push still moved the remote head")
	}
}

// An action that SyncRepo does not know answers a plain error, and the
// message names the action. A person who mistypes then reads what they
// typed.
func TestHandleSyncAnswersErrorForAnUnknownAction(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)

	body := ghBody(t, ghSync(t, a, url.Values{"action": {"sideways"}}))
	if body["status"] != "error" {
		t.Fatalf("the status is %v, want error", body["status"])
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, "sideways") {
		t.Errorf("the message is %q, and it does not name the action", msg)
	}
}

// The force checkbox is a separate field, and handleSync turns it into
// the canonical action.
//
// SyncRepo knows pull_force and push_force and knows no force flag. The
// translation is the whole reason this branch exists. A test of SyncRepo
// alone can never reach it.
//
// Each case below proves the translation by its EFFECT. A plain pull over
// a local change gives a conflict, and a forced one takes the remote copy.
// A plain push against a diverged remote gives push_conflict, and a forced
// one writes over the remote.
func TestHandleSyncTranslatesTheForceCheckbox(t *testing.T) {
	for _, c := range []struct {
		name   string
		action string
	}{
		{"pull", "pull"},
		{"the pull_ff alias", "pull_ff"},
		{"the download alias", "download"},
	} {
		remote := gsRemote(t)
		gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
		a := gsApp(t, remote)
		if err := a.SyncRepo("pull", ""); err != nil {
			t.Fatalf("%s: the first pull: %v", c.name, err)
		}
		gsWrite(t, a, "md/One.md", "the text of this device\n")
		gsSeedRemote(t, remote, "second",
			map[string]string{"md/One.md": "the text of the other device\n"})

		w := ghSync(t, a, url.Values{"action": {c.action}, "force": {"true"}})
		if got := ghStatus(t, w); got != "success" {
			t.Errorf("%s with force answered %q, want success. The body is %s",
				c.name, got, w.Body.String())
			continue
		}
		if got := gsRead(t, a, "md/One.md"); got != "the text of the other device\n" {
			t.Errorf("%s with force left %q, thus it did not become pull_force", c.name, got)
		}
	}

	for _, c := range []struct {
		name   string
		action string
	}{
		{"push", "push"},
		{"the upload alias", "upload"},
	} {
		remote := gsRemote(t)
		gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
		a := gsApp(t, remote)
		if err := a.SyncRepo("pull", ""); err != nil {
			t.Fatalf("%s: the first pull: %v", c.name, err)
		}
		gsSeedRemote(t, remote, "from the other device",
			map[string]string{"md/Other.md": "other\n"})
		gsWrite(t, a, "md/Two.md", "a note of this device\n")

		w := ghSync(t, a, url.Values{
			"action": {c.action}, "force": {"true"}, "message": {"take my copy"},
		})
		if got := ghStatus(t, w); got != "success" {
			t.Errorf("%s with force answered %q, want success. The body is %s",
				c.name, got, w.Body.String())
			continue
		}
		if gsRemoteHead(t, remote) != gsLocalHead(t, a) {
			t.Errorf("%s with force left the remote behind, thus it did not "+
				"become push_force", c.name)
		}
	}
}

// A force value that is not the word true is not a force.
//
// The comparison is against the exact string. A checkbox that sends "on",
// which is what a bare HTML checkbox sends, must not force a pull over
// the changes of the person.
func TestHandleSyncForcesOnlyOnTheWordTrue(t *testing.T) {
	for _, value := range []string{"on", "1", "yes", "TRUE", ""} {
		remote := gsRemote(t)
		gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "the first text\n"})
		a := gsApp(t, remote)
		if err := a.SyncRepo("pull", ""); err != nil {
			t.Fatalf("the first pull: %v", err)
		}
		gsWrite(t, a, "md/One.md", "the text of this device\n")
		gsSeedRemote(t, remote, "second",
			map[string]string{"md/One.md": "the text of the other device\n"})

		w := ghSync(t, a, url.Values{"action": {"pull"}, "force": {value}})
		if got := ghStatus(t, w); got != "conflict" {
			t.Errorf("force=%q answered %q, want conflict. A value that is not "+
				"the word true must never write over a local change.", value, got)
		}
		if got := gsRead(t, a, "md/One.md"); got != "the text of this device\n" {
			t.Errorf("force=%q lost the local text, which is now %q", value, got)
		}
	}
}

// A body that ParseForm cannot read answers a plain error and not a
// panic. The message says which part failed.
func TestHandleSyncAnswersABadForm(t *testing.T) {
	a := gsApp(t, gsRemote(t))

	r := httptest.NewRequest("POST", "/api/sync", strings.NewReader("action=%zz"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.handleSync(w, r)

	body := ghBody(t, w)
	if body["status"] != "error" {
		t.Fatalf("the status is %v, want error", body["status"])
	}
	if msg, _ := body["message"].(string); !strings.HasPrefix(msg, "bad request") {
		t.Errorf("the message is %q, and it does not name the fault", msg)
	}
}

// ----------------------------------------------------------------------
// handleSyncPreview
// ----------------------------------------------------------------------

// The preview reads and changes nothing, thus it takes GET alone.
func TestSyncPreviewRefusesAnotherMethod(t *testing.T) {
	a := gsApp(t, gsRemote(t))
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		w := ghPreview(t, a, method, "action=upload")
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s answered %d, want 405", method, w.Code)
		}
	}
}

// The preview answers for the upload action alone. A download preview
// would need a fetch, and no page asks for one.
func TestSyncPreviewRefusesAnotherAction(t *testing.T) {
	a := gsApp(t, gsRemote(t))
	for _, action := range []string{"", "download", "pull", "push"} {
		w := ghPreview(t, a, "GET", "action="+action)
		if w.Code != http.StatusBadRequest {
			t.Errorf("action=%q answered %d, want 400", action, w.Code)
		}
	}
}

// The preview lists each changed file, and it never lists config.json.
//
// config.json holds each password and each SSH key. It is out of the
// commit by rule, thus a preview that names it would promise an upload
// that never happens.
func TestSyncPreviewListsTheChangedFiles(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	gsWrite(t, a, "md/Two.md", "a new note\n")
	gsWrite(t, a, "config.json", `{"share_lan":false}`)

	got := ghPreviewAnswer(t, ghPreview(t, a, "GET", "action=upload"))
	if !ghHas(got.Files, "md/Two.md") {
		t.Errorf("the preview lists %v, and md/Two.md is not in it", got.Files)
	}
	for _, f := range got.Files {
		if f == "config.json" {
			t.Error("the preview names config.json, which no commit carries")
		}
	}
}

// A file that .gitignore covers is not in the preview.
//
// A local- name stays on the device by rule 9 of CLAUDE.md section 1. A
// preview that lists it says that the other device will receive it.
func TestSyncPreviewSkipsAnIgnoredFile(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	gsWrite(t, a, "md/local-Scratch.md", "for this device alone\n")

	got := ghPreviewAnswer(t, ghPreview(t, a, "GET", "action=upload"))
	for _, f := range got.Files {
		if strings.Contains(f, "local-Scratch") {
			t.Errorf("the preview lists %q, and the local- rule keeps it here", f)
		}
	}
}

// The first preview after a pull lists .gitignore.
//
// getOrInitRepo writes .gitignore, and no commit carries it until the
// first push of this device. The file is therefore a real pending
// change, and the preview is right to name it. This test records that,
// so that a reader of a bug report knows it is the design.
func TestSyncPreviewListsTheGitignoreOfAFreshRepository(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}

	got := ghPreviewAnswer(t, ghPreview(t, a, "GET", "action=upload"))
	if !ghHas(got.Files, ".gitignore") {
		t.Errorf("the preview lists %v, and .gitignore is not in it. The app "+
			"writes that file, and the first commit of a device carries it.",
			got.Files)
	}
}

// files is an array in the JSON, and never null.
//
// The page reads .length on it without a guard. This is the same rule as
// the conflict answer above, and the two writers are separate code.
func TestSyncPreviewAnswersAnArrayAndNeverNull(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	// The push carries .gitignore away. See the test above.
	if err := a.SyncRepo("push", "the first commit of this device"); err != nil {
		t.Fatalf("the push: %v", err)
	}

	w := ghPreview(t, a, "GET", "action=upload")
	if !strings.Contains(w.Body.String(), `"files":[]`) {
		t.Errorf("a clean tree answered %s, want an empty array under files",
			w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("the content type is %q, want application/json", got)
	}
}

// A commit that the remote has never seen reads as something to upload,
// although the worktree is clean.
//
// A clean worktree does not mean that there is nothing to send. A push
// that failed, or a profile that changed, leaves a commit behind. The
// page said "Nothing to commit" and stopped before this answer existed.
func TestSyncPreviewReportsAnUnpushedCommit(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	// The push makes the tree level, thus the commit below is the only
	// thing that the remote has not seen.
	if err := a.SyncRepo("push", "the first commit of this device"); err != nil {
		t.Fatalf("the push: %v", err)
	}
	ghCommitLocally(t, a, "md/Two.md", "a note of this device\n", "committed and not pushed")

	got := ghPreviewAnswer(t, ghPreview(t, a, "GET", "action=upload"))
	if len(got.Files) != 0 {
		t.Fatalf("the worktree is not clean, thus this test proves nothing: %v", got.Files)
	}
	if !got.Unpushed {
		t.Error("a commit that the remote never saw reads as nothing to upload")
	}
}

// A tree that agrees with the remote reports nothing to upload.
func TestSyncPreviewReportsALevelTree(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	gsWrite(t, a, "md/Two.md", "a new note\n")
	if err := a.SyncRepo("push", "add a note"); err != nil {
		t.Fatalf("the push: %v", err)
	}

	got := ghPreviewAnswer(t, ghPreview(t, a, "GET", "action=upload"))
	if len(got.Files) != 0 {
		t.Errorf("the preview lists %v after a push that worked", got.Files)
	}
	if got.Unpushed {
		t.Error("a tree that is level with the remote reads as ahead of it")
	}
}

// A path that git still tracks and must not appears in the preview with
// the reason beside it.
//
// The status scan cannot find such a file, because its content did not
// change. The commit deletes it on each other device, and the preview
// must say so before the person presses the button.
func TestSyncPreviewNamesAPathThatLeavesTheRepository(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{
		"md/One.md":          "one\n",
		"md/local-Device.md": "an old commit tracked this\n",
		"html/Note.txt":      "a copy of the file in md/\n",
	})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}

	got := ghPreviewAnswer(t, ghPreview(t, a, "GET", "action=upload"))
	if !ghHasPrefix(got.Files, "md/local-Device.md") {
		t.Errorf("the preview lists %v, and the tracked local- path is not in it",
			got.Files)
	}
	if !ghHasPrefix(got.Files, "html/Note.txt") {
		t.Errorf("the preview lists %v, and the tracked html/ copy is not in it",
			got.Files)
	}
	for _, f := range got.Files {
		if strings.HasPrefix(f, "md/local-Device.md") && !strings.Contains(f, "local-only") {
			t.Errorf("the row %q gives no reason", f)
		}
	}
}

// A path that git tracks AND .gitignore covers appears one time.
//
// wTree.Status() applies .gitignore to an UNTRACKED file by itself, thus
// the matcher of handleSyncPreview changes no answer for such a file. It
// is not dead code. Git ignores nothing that it already tracks. A
// tracked local- name that the person edits therefore DOES reach the
// status, and the matcher keeps it out of the pending list.
//
// untrackTrackedPaths then adds the same path back with its reason. The
// row that the person reads says "git stops to track it", which is what
// the commit does. Without the matcher the list holds the path twice,
// and the two rows say different things.
func TestSyncPreviewListsATrackedIgnoredPathOneTime(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{
		"md/One.md":          "one\n",
		"md/local-Device.md": "an old commit tracked this\n",
	})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	// The person edits the tracked local- file. Git reports it, because
	// git ignores nothing that it already tracks.
	gsWrite(t, a, "md/local-Device.md", "and now this device changed it\n")

	got := ghPreviewAnswer(t, ghPreview(t, a, "GET", "action=upload"))
	rows := 0
	for _, f := range got.Files {
		if strings.HasPrefix(f, "md/local-Device.md") {
			rows++
			if !strings.Contains(f, "local-only") {
				t.Errorf("the row %q gives no reason", f)
			}
		}
	}
	if rows != 1 {
		t.Errorf("the preview holds %d rows for md/local-Device.md, want 1. "+
			"The list is %v", rows, got.Files)
	}
}

// config.json never reaches the preview, and not even when git tracks it.
//
// The file holds each password and each SSH key. A preview that names it
// says that the upload sends them.
//
// WHICH GUARD DOES THE WORK. Two stand in handleSyncPreview: the
// gitignore matcher, and a check of the name. The matcher is the one
// that acts. getOrInitRepo backfills each line of gitignorePatterns into
// .gitignore on EVERY open, thus config.json is always covered by the
// time loadGitignoreMatcher reads the file. The check by name is
// unreachable behind it. See the report of 26.09.36.
//
// This test holds the OUTCOME and not one guard. It also holds the fact
// that the chain rests on: the backfill puts the line back.
func TestSyncPreviewNeverListsConfigJSON(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{
		"md/One.md":   "one\n",
		"config.json": "{\"share_lan\":false}\n",
	})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	gsWrite(t, a, "config.json", "{\"share_lan\":true}\n")

	// A person edits .gitignore and drops the line. The next open of the
	// repository must put it back.
	gsWrite(t, a, ".gitignore", "assets_version\n")

	got := ghPreviewAnswer(t, ghPreview(t, a, "GET", "action=upload"))
	for _, f := range got.Files {
		if f == "config.json" {
			t.Fatalf("the preview names config.json. The upload would then "+
				"promise to send each password. The list is %v", got.Files)
		}
	}
	if !strings.Contains(gsRead(t, a, ".gitignore"), "config.json") {
		t.Error("the open of the repository did not put the config.json line back. " +
			"The matcher is then blind to it, and only the check by name stands.")
	}
}

// With no usable key the preview still answers, and it says why it could
// not ask the remote.
//
// The local half of the comparison stands without a key, and that half
// is the one that catches a push that failed. An answer of 500 here
// would leave the person with no preview at all.
func TestSyncPreviewAnswersWithNoUsableKey(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	if err := a.SyncRepo("push", "the first commit of this device"); err != nil {
		t.Fatalf("the push: %v", err)
	}
	ghCommitLocally(t, a, "md/Two.md", "a note\n", "committed and not pushed")

	// The key goes away after the repository exists, thus the remote is
	// configured and the authentication is not.
	a.WithConfig(func(c *Config) { c.GitServers[0].SSHKeyData = "" })

	w := ghPreview(t, a, "GET", "action=upload")
	if w.Code != http.StatusOK {
		t.Fatalf("the preview answered %d, want 200. The body is %s", w.Code, w.Body.String())
	}
	got := ghPreviewAnswer(t, w)
	if !got.Unpushed {
		t.Error("the local half of the comparison found nothing to push")
	}
	if got.Verified {
		t.Error("no remote was asked, and the answer claims that one was")
	}
	if got.RemoteError == "" {
		t.Error("the answer gives no reason for the unverified state")
	}
}

// A remote that nothing can reach still gives an answer, with a reason.
//
// The person opens Upload on a train. The preview must say what it knows
// and what it could not ask, and it must not answer a bare 500.
func TestSyncPreviewAnswersWhenTheRemoteIsUnreachable(t *testing.T) {
	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)
	if err := a.SyncRepo("pull", ""); err != nil {
		t.Fatalf("the first pull: %v", err)
	}
	if err := a.SyncRepo("push", "the first commit of this device"); err != nil {
		t.Fatalf("the push: %v", err)
	}
	a.WithConfig(func(c *Config) { c.GitServers[0].URL = "/no/such/path.git" })

	w := ghPreview(t, a, "GET", "action=upload")
	if w.Code != http.StatusOK {
		t.Fatalf("the preview answered %d, want 200. The body is %s", w.Code, w.Body.String())
	}
	got := ghPreviewAnswer(t, w)
	if got.Verified {
		t.Error("no remote answered, and the preview claims that one did")
	}
	if got.RemoteError == "" {
		t.Error("the answer gives no reason, thus the page says nothing to the person")
	}
}

// The preview answers a fault of the repository as a 500 and not as a
// JSON body. The page then shows the text.
//
// The storage directory holds a file named .git here, thus PlainInit
// cannot make a repository in it.
func TestSyncPreviewAnswersARepositoryFault(t *testing.T) {
	a := gsApp(t, gsRemote(t))
	gitPath := filepath.Join(a.StorageDir, ".git")
	if err := os.RemoveAll(gitPath); err != nil {
		t.Fatalf("removing the repository: %v", err)
	}
	if err := os.WriteFile(gitPath, []byte("not a repository"), 0o644); err != nil {
		t.Fatalf("writing the blocking file: %v", err)
	}

	w := ghPreview(t, a, "GET", "action=upload")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("the preview answered %d, want 500. The body is %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Repo init failed") {
		t.Errorf("the body is %q, and it does not name the step that failed",
			w.Body.String())
	}
}

// ghHas tells whether a list holds one exact value.
func ghHas(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// ghHasPrefix tells whether a list holds a value that starts with want.
// The untrack rows carry a reason after the path.
func ghHasPrefix(list []string, want string) bool {
	for _, v := range list {
		if strings.HasPrefix(v, want) {
			return true
		}
	}
	return false
}
