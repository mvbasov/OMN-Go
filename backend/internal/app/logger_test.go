package app

// ---------------------------------------------------------------------
// The log transport and the level rule
//
// Two things are pinned here. The shape of a line, because the browser
// parses it. And the ban on log.Printf. A line that skips the Debugf, Infof
// and Errf methods of a logger carries no level. It therefore escapes every
// filter a person sets on the Config page.
// ---------------------------------------------------------------------

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
)

// TestEmitLogLineShape pins the text the browser parses. omn-go-api.js reads
// the tag and the level out of each line, and applySyncLogLine skips the
// level word before it matches a sync stage. A change here breaks both.
func TestEmitLogLineShape(t *testing.T) {
	a := newTestApp(t)

	ch := a.logs.Subscribe()
	defer a.logs.Unsubscribe(ch)

	a.log(logx.Sync).Debugf("Staging file: %s", "Note.md")
	a.log(logx.Assets).Infof("%d asset(s) refreshed", 3)
	a.log(logx.Edit).Errf("cannot run %q", "subl")

	want := []string{
		"[sync] (debug) Staging file: Note.md\n",
		"[assets] (info) 3 asset(s) refreshed\n",
		"[edit] (error) cannot run \"subl\"\n",
	}
	for _, w := range want {
		got := <-ch
		if !strings.HasSuffix(got, w) {
			t.Errorf("log line %q does not end with %q", got, w)
		}
		// The stamp is the standard log package layout, so stdout and the
		// stream look the same whatever wrote the line.
		if len(got) != len(w)+len(logx.TimeLayout) {
			t.Errorf("log line %q does not carry a %d-character time stamp",
				got, len(logx.TimeLayout))
		}
	}
}

// TestLogLineEnabled pins the two axes. A fault always prints. A debug or an
// info line needs its level on and its tag ticked. A reader who asks for
// less noise never asks for fewer faults.
func TestLogLineEnabled(t *testing.T) {
	a := newTestApp(t)

	a.applyLogFilter(config.Config{LogDebug: false, LogInfo: false, LogTags: config.LogTagsDefault})
	if !a.logLineEnabled(logx.LevelError, logx.Sync) {
		t.Error("an error was filtered out with both levels off")
	}
	if a.logLineEnabled(logx.LevelDebug, logx.Sync) || a.logLineEnabled(logx.LevelInfo, logx.Sync) {
		t.Error("a quiet level printed with both levels off")
	}

	a.applyLogFilter(config.Config{LogDebug: true, LogInfo: true, LogTags: []string{"assets"}})
	if !a.logLineEnabled(logx.LevelDebug, logx.Assets) {
		t.Error("a ticked tag was filtered out with debug on")
	}
	if a.logLineEnabled(logx.LevelDebug, logx.Sync) {
		t.Error("an unticked tag printed with debug on")
	}
	if !a.logLineEnabled(logx.LevelError, logx.Sync) {
		t.Error("an error was filtered out by an unticked tag")
	}

	// Before loadConfig runs the cache is empty. Every line the application
	// writes that early is a fault, so faults only is the safe answer.
	fresh := &App{}
	if !fresh.logLineEnabled(logx.LevelError, logx.Server) {
		t.Error("an error was filtered out before the config loaded")
	}
	if fresh.logLineEnabled(logx.LevelInfo, logx.Server) {
		t.Error("an info line printed before the config loaded")
	}
}

// The ring holds a line that the stdout switches suppressed.
//
// A person who turned debug off and then met a fault needs the debug
// lines of that moment more than anybody. The switches say what a reader
// wants to SEE, and never what the application keeps.
func TestLogHistoryHoldsASuppressedLine(t *testing.T) {
	a := newTestApp(t)
	a.applyLogFilter(config.Config{LogDebug: false, LogInfo: false, LogTags: []string{}})

	if a.logLineEnabled(logx.LevelDebug, logx.Sync) {
		t.Fatal("the filter lets a debug line through, thus this test proves nothing")
	}
	a.log(logx.Sync).Debugf("a step that stdout never shows")

	for _, line := range a.logs.Snapshot() {
		if strings.Contains(line, "a step that stdout never shows") {
			return
		}
	}
	t.Error("the ring lost a line that the stdout filter suppressed")
}

// ----------------------------------------------------------------------
// GET /api/logs/history
// ----------------------------------------------------------------------

// The endpoint answers the ring, oldest line first, as JSON.
func TestLogHistoryEndpointAnswersTheRing(t *testing.T) {
	a := newTestApp(t)
	for _, line := range []string{"first\n", "second\n", "third\n"} {
		a.logs.Broadcast(line, false)
	}

	rec := httptest.NewRecorder()
	a.handleLogHistory(rec, httptest.NewRequest(http.MethodGet, "/api/logs/history", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("the endpoint answered %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("the content type is %q, want application/json", got)
	}
	var body struct {
		Status string   `json:"status"`
		Cap    int      `json:"cap"`
		Lines  []string `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, rec.Body.String())
	}
	if body.Status != "success" {
		t.Errorf("the status is %q, want success. See section 1.4 of doc/API.md.", body.Status)
	}
	if body.Cap != logx.HistoryCap {
		t.Errorf("the answer names a cap of %d, want %d", body.Cap, logx.HistoryCap)
	}
	want := []string{"first\n", "second\n", "third\n"}
	if len(body.Lines) != len(want) {
		t.Fatalf("the answer holds %d lines, want %d: %q", len(body.Lines), len(want), body.Lines)
	}
	for i := range want {
		if body.Lines[i] != want[i] {
			t.Errorf("line %d is %q, want %q", i, body.Lines[i], want[i])
		}
	}
}

// An empty ring answers an array, and never null.
//
// A reader of the answer maps over lines without a guard, the same as
// the sync answers do. See newSyncConflict.
func TestLogHistoryEndpointAnswersAnArrayWhenEmpty(t *testing.T) {
	a := newTestApp(t)

	rec := httptest.NewRecorder()
	a.handleLogHistory(rec, httptest.NewRequest(http.MethodGet, "/api/logs/history", nil))
	if strings.Contains(rec.Body.String(), `"lines":null`) {
		t.Errorf("an empty ring answered %s, want an empty array", rec.Body.String())
	}
}

// The endpoint is ADMIN ONLY, and so is the stream beside it.
//
// A LAN share gives no log line, live or held. An open stream would make
// the guard on the ring useless. A remote caller who holds the stream open
// reads the same lines as the server writes them.
//
// This test drives the REAL registration through a real mux. A guard
// that registerRoutes forgets to wrap is then a failure here, and a test
// of the handler alone can never see that.
func TestLogHistoryEndpointIsAdminOnly(t *testing.T) {
	a := newTestApp(t)
	mux := http.NewServeMux()
	a.registerRoutes(mux)

	cases := []struct {
		remote, cookie string
		want           int
	}{
		{"127.0.0.1:1", "", http.StatusOK},        // the local bypass
		{"192.168.1.9:1", "admin", http.StatusOK}, // an admin of the LAN
		{"192.168.1.9:1", "guest", http.StatusUnauthorized},
		{"192.168.1.9:1", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/logs/history", nil)
		req.RemoteAddr = c.remote
		if c.cookie != "" {
			req.AddCookie(sessionCookie(t, a, c.cookie))
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s with the cookie %q answered %d, want %d",
				c.remote, c.cookie, rec.Code, c.want)
		}
	}
}

// The stream is ADMIN ONLY as well.
//
// A log line names a note, a remote, a path and a fault. A remote caller
// reads none of them.
//
// This test drives the REAL registration through a real mux, the same as
// the history test above. The request of each allowed case carries a
// context that is already canceled, because handleLogsSSE blocks until
// the context ends.
func TestLogStreamIsAdminOnly(t *testing.T) {
	a := newTestApp(t)
	mux := http.NewServeMux()
	a.registerRoutes(mux)

	cases := []struct {
		remote, cookie string
		want           int
	}{
		{"127.0.0.1:1", "", http.StatusOK},        // the local bypass
		{"192.168.1.9:1", "admin", http.StatusOK}, // an admin of the LAN
		{"192.168.1.9:1", "guest", http.StatusUnauthorized},
		{"192.168.1.9:1", "", http.StatusUnauthorized},
	}
	before := countLogClients(a)
	for _, c := range cases {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodGet, "/api/logs", nil).WithContext(ctx)
		req.RemoteAddr = c.remote
		if c.cookie != "" {
			req.AddCookie(sessionCookie(t, a, c.cookie))
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s with the cookie %q answered %d, want %d",
				c.remote, c.cookie, rec.Code, c.want)
		}
	}

	// A refused request must never reach the handler, thus it must
	// register no client of the stream.
	if got := countLogClients(a); got != before {
		t.Errorf("the stream holds %d clients and it held %d before", got, before)
	}
}

// The stream keeps its own address, and the history pattern shadows it
// not at all.
//
// Both are exact patterns of a ServeMux. A trailing slash on either one
// would make it a subtree and take the other address with it.
func TestLogStreamKeepsItsOwnAddress(t *testing.T) {
	a := newTestApp(t)
	rec := &routeRecorder{}
	a.registerRoutes(rec)

	var found int
	for _, p := range rec.patterns {
		switch p {
		case "/api/logs", "/api/logs/history":
			found++
		case "/api/logs/":
			t.Error("the stream is registered as a subtree, thus it takes " +
				"/api/logs/history with it")
		}
	}
	if found != 2 {
		t.Errorf("the two log patterns are not both registered: %d of 2", found)
	}
}

// ----------------------------------------------------------------------
// The Log page
// ----------------------------------------------------------------------
//
// /OMNGoLogs.html holds no log line of its own. It reads
// /api/logs/history one time, and then it adds each new line of
// /api/logs. omn-go-logs.js does that work.
//
// These two tests are the pair that the Status page carries, for the same
// two reasons. See internal/status/status_test.go.

// The page must reach both addresses and carry no line of its own.
func TestLogsPageIsAReaderOfTheTwoAddresses(t *testing.T) {
	a := newTestApp(t)

	// httptest.NewRequest gives each request the address 192.0.2.1, which
	// is another machine as far as hasRole is concerned. The owner of the
	// device connects from the loopback address, and that is the request
	// this test makes. See isLocalConnection in middleware.go.
	req := httptest.NewRequest(http.MethodGet, "/OMNGoLogs.html", nil)
	req.RemoteAddr = "127.0.0.1:41000"

	rec := httptest.NewRecorder()
	a.serveLogsPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"/api/logs/history", "omn-lg-reload", "omn-lg-copy", "omn-lg-filter-toggle",
		"omn-lg-levels", "omn-lg-tags", "omn-lg-body",
		"/js/OMN-Go/omn-go-logs.js", "/css/OMN-Go/omn-go-logs.css",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page misses %q", want)
		}
	}
	// The body of the page starts empty. Each line comes from an address,
	// thus the template can carry no log line of its own.
	if !strings.Contains(body, "Loading…") {
		t.Error("the page does not start empty. It must read the ring.")
	}
	// The two sync buttons of the shell are what lets a reader start a
	// sync here and watch it. On Android there is one screen, thus a
	// second page is not an answer. See the banner of omn-go-logs.js.
	if !strings.Contains(body, `data-action="sync" data-arg="upload"`) {
		t.Error("the page lost the Upload button of the shell")
	}
	if !strings.Contains(body, `data-action="sync" data-arg="download"`) {
		t.Error("the page lost the Download button of the shell")
	}
	// The script and the stylesheet are files. An inline script or an
	// inline style would cost its bytes in this template alone, and the
	// project keeps the two apart. See section 4 of CLAUDE.md.
	if strings.Contains(logsPageTmpl, "<script>") {
		t.Error("the template holds an inline script")
	}
	if strings.Contains(logsPageTmpl, "<style>") {
		t.Error("the template holds an inline style")
	}
}

// A remote caller gets a page and not a line of plain text.
//
// The route carries no authMiddleware for that reason. See page_access.go.
func TestLogsPageAnswersARemoteCallerWithAPage(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) { c.ShareLAN = true })

	req := httptest.NewRequest(http.MethodGet, "/OMNGoLogs.html", nil)
	req.RemoteAddr = "192.168.1.44:51000" // another machine on the network
	// A signed cookie, and not the bare word "guest". The server refuses
	// an unsigned value. See session.go.
	req.AddCookie(sessionCookie(t, a, "guest"))

	rec := routeServe(a, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want a page", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "for the admin of this device") {
		t.Error("a remote caller did not get the refusal page")
	}
	if strings.Contains(body, "omn-lg-reload") {
		t.Error("a remote caller got the reader script")
	}
}

// Each App writes into its own hub. A line of one App never reaches the
// ring or the stream of another App.
func TestEachAppHasItsOwnLog(t *testing.T) {
	a, b := newTestApp(t), newTestApp(t)
	ch := b.logs.Subscribe()
	defer b.logs.Unsubscribe(ch)

	a.log(logx.Sync).Errf("a line of the first App")
	for _, line := range b.logs.Snapshot() {
		if strings.Contains(line, "a line of the first App") {
			t.Fatal("the ring of the second App holds a line of the first App")
		}
	}
	select {
	case line := <-ch:
		t.Errorf("the stream of the second App got %q", line)
	default:
	}
}
