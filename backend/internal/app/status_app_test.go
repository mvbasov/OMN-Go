package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
)

// The Android layer hands its addresses to Go, because Go cannot read
// them on a phone. They must reach lan_urls, without a duplicate of the
// address that the probe already found.
func TestStatusLANAddressesFromAndroid(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) { c.ShareLAN = true })

	stRunning(t, a)
	SetLANAddresses(" 192.168.5.5 , 10.0.0.7 ,, 192.168.5.5 ")

	res, _ := getStatus(t, a, "sections=server")
	got := strings.Join(res.Server.LANURLs, " ")
	for _, want := range []string{"http://192.168.5.5:", "http://10.0.0.7:"} {
		if !strings.Contains(got, want) {
			t.Errorf("lan_urls %v misses %q", res.Server.LANURLs, want)
		}
	}
	count := 0
	for _, u := range res.Server.LANURLs {
		if strings.HasPrefix(u, "http://192.168.5.5:") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("192.168.5.5 appears %d times in %v, want 1", count, res.Server.LANURLs)
	}

	// Sharing off answers with no address, whatever Android sent.
	a.config.Update(func(c *config.Config) { c.ShareLAN = false })
	res, _ = getStatus(t, a, "sections=server")
	if len(res.Server.LANURLs) != 0 {
		t.Errorf("lan_urls = %v with sharing off, want none", res.Server.LANURLs)
	}
}

// SetAndroidPackage wins over the derivation, and the derivation answers
// when the setter never ran.
func TestStatusAndroidPackage(t *testing.T) {
	a := &App{StorageDir: "/storage/emulated/0/Android/media/net.basov.omngo.fdroid"}
	stRunning(t, a)
	if got := a.statusService().AndroidPackage(); got != "net.basov.omngo.fdroid" {
		t.Errorf("derived package = %q, want net.basov.omngo.fdroid", got)
	}

	SetAndroidPackage("net.basov.omngo")
	if got := a.statusService().AndroidPackage(); got != "net.basov.omngo" {
		t.Errorf("set package = %q, want net.basov.omngo", got)
	}
}

// A remote caller gets a page, not the line of plain text that authMiddleware
// writes. This is the rule the file index follows.
func TestStatusPageAnswersARemoteCallerWithAPage(t *testing.T) {
	a := newTestApp(t)
	a.config.Update(func(c *config.Config) { c.ShareLAN = true })

	req := httptest.NewRequest(http.MethodGet, "/OMNGoStatus.html", nil)
	req.RemoteAddr = "192.168.1.44:51000" // another machine on the network
	// A signed cookie, and not the bare word "guest": the server refuses
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
	if strings.Contains(body, "omn-st-storage") {
		t.Error("a remote caller got the reader script")
	}
}

// stRunning makes a the runningApp of one test. The cleanup clears
// runningApp and earlyEnv, thus the next test starts with neither.
func stRunning(t *testing.T, a *App) {
	t.Helper()
	stClearRunning()
	setRunningApp(a)
	t.Cleanup(stClearRunning)
}

func stClearRunning() {
	runningMu.Lock()
	defer runningMu.Unlock()
	runningApp = nil
	earlyEnv.SetPackage("")
	earlyEnv.SetAddresses(nil)
}

// ServerService.java calls SetAndroidPackage before StartServer. The App of
// StartServer must still get the value.
func TestAnEarlyAndroidFactReachesTheApp(t *testing.T) {
	stClearRunning()
	t.Cleanup(stClearRunning)
	SetAndroidPackage("net.basov.omngo.fdroid")
	SetLANAddresses("10.0.0.9")

	a := &App{}
	setRunningApp(a)
	if got := a.statusService().AndroidPackage(); got != "net.basov.omngo.fdroid" {
		t.Errorf("the package is %q, want net.basov.omngo.fdroid", got)
	}
	if got := a.android.LANAddresses(); !slices.Equal(got, []string{"10.0.0.9"}) {
		t.Errorf("the addresses are %v, want [10.0.0.9]", got)
	}

	SetLANAddresses("10.0.0.8")
	if got := a.android.LANAddresses(); !slices.Equal(got, []string{"10.0.0.8"}) {
		t.Errorf("a setter after the start gave %v, want [10.0.0.8]", got)
	}
}

// getStatus sends GET /api/status to the App and decodes the server section.
// The tests of package status have their own copy for the whole answer.
func getStatus(t *testing.T, a *App, query string) (*statusAnswer, *httptest.ResponseRecorder) {
	t.Helper()
	target := "/api/status"
	if query != "" {
		target += "?" + query
	}
	rec := httptest.NewRecorder()
	a.handleStatus(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var res statusAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	return &res, rec
}

// statusAnswer holds the fields of the /api/status answer that the tests of
// this file read.
type statusAnswer struct {
	Server *struct {
		LANURLs []string `json:"lan_urls"`
	} `json:"server"`
}
