package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// sessionReq builds a request that arrives from the network and not from
// the device. hasRole answers true for each local connection, thus a test
// of the cookie must not use one.
func sessionReq(t *testing.T, cookies ...*http.Cookie) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	r.RemoteAddr = "192.168.1.50:5555"
	for _, c := range cookies {
		r.AddCookie(c)
	}
	return r
}

// sessionCookie mints a valid signed cookie for one role. Other test
// files use it in place of the hand-written cookie they carried until
// 26.09.6.
func sessionCookie(t *testing.T, a *App, role string) *http.Cookie {
	t.Helper()
	signed, _ := a.newSessionCookies(role)
	if signed == nil {
		t.Fatalf("newSessionCookies(%q) made no cookie", role)
	}
	return signed
}

// A cookie that this install signed is accepted, and it carries the role
// that the login gave it.
func TestSignedCookieIsAccepted(t *testing.T) {
	a := newTestApp(t)
	for _, role := range []string{roleAdmin, roleGuest} {
		got := a.readSessionRole(sessionReq(t, sessionCookie(t, a, role)))
		if got != role {
			t.Errorf("readSessionRole for %q gave %q", role, got)
		}
	}
}

// THIS IS THE TEST FOR THE FAULT THAT 26.09.6 REPAIRS. Until that
// version the server read the cookie value and trusted it. A client on
// the network could therefore name its own role with no password. The
// bare word must never be accepted again.
func TestUnsignedCookieIsRefused(t *testing.T) {
	a := newTestApp(t)
	for _, value := range []string{"admin", "guest", "admin.", "admin..", ".."} {
		r := sessionReq(t, &http.Cookie{Name: sessionCookieName, Value: value})
		if got := a.readSessionRole(r); got != "" {
			t.Errorf("the value %q gave the role %q, want none", value, got)
		}
		if a.hasRole(r, true) {
			t.Errorf("the value %q passed hasRole as an admin", value)
		}
	}
}

// A changed role, a changed expiry or a changed signature each break the
// HMAC, thus each one is refused.
func TestChangedCookieIsRefused(t *testing.T) {
	a := newTestApp(t)
	good := sessionCookie(t, a, roleGuest).Value
	parts := strings.Split(good, ".")
	if len(parts) != 3 {
		t.Fatalf("the cookie value %q is not three parts", good)
	}

	cases := []struct {
		name  string
		value string
	}{
		{"the role becomes admin", "admin." + parts[1] + "." + parts[2]},
		{"the expiry moves", parts[0] + ".9999999999." + parts[2]},
		{"the signature changes", parts[0] + "." + parts[1] + ".AAAAAAAA"},
		{"the signature is empty", parts[0] + "." + parts[1] + "."},
	}
	for _, c := range cases {
		r := sessionReq(t, &http.Cookie{Name: sessionCookieName, Value: c.value})
		if got := a.readSessionRole(r); got != "" {
			t.Errorf("%s: gave the role %q, want none", c.name, got)
		}
	}
}

// A cookie that another install signed is refused, because the key of
// this install does not make that HMAC. This is what keeps a synced
// note tree from carrying a usable session between two devices.
func TestCookieOfAnotherInstallIsRefused(t *testing.T) {
	a := newTestApp(t)
	b := newTestApp(t)
	r := sessionReq(t, sessionCookie(t, b, roleAdmin))
	if got := a.readSessionRole(r); got != "" {
		t.Errorf("the cookie of another install gave the role %q, want none", got)
	}
}

// An expiry that passed is refused. The test signs the value by hand,
// because newSessionCookies always looks forward.
func TestExpiredCookieIsRefused(t *testing.T) {
	a := newTestApp(t)
	past := time.Now().Add(-time.Minute).Unix()
	value := roleAdmin + "." + strconv.FormatInt(past, 10) + "." + a.sessionMAC(roleAdmin, past)
	r := sessionReq(t, &http.Cookie{Name: sessionCookieName, Value: value})
	if got := a.readSessionRole(r); got != "" {
		t.Errorf("an expired cookie gave the role %q, want none", got)
	}
}

// The cookie lasts 30 days, which is the answer of the maintainer. A
// change of that number is a decision and not an accident, thus the test
// names it.
func TestSessionLastsThirtyDays(t *testing.T) {
	a := newTestApp(t)
	signed, hint := a.newSessionCookies(roleAdmin)
	want := time.Now().Add(30 * 24 * time.Hour)
	for _, c := range []*http.Cookie{signed, hint} {
		diff := c.Expires.Sub(want)
		if diff > time.Minute || diff < -time.Minute {
			t.Errorf("cookie %q expires at %v, want about %v", c.Name, c.Expires, want)
		}
	}
}

// The signed cookie is HttpOnly and the hint cookie is not. See the
// banner of session.go for why there are two.
func TestHintCookieIsReadableAndSignedCookieIsNot(t *testing.T) {
	a := newTestApp(t)
	signed, hint := a.newSessionCookies(roleGuest)
	if !signed.HttpOnly {
		t.Error("the signed cookie is not HttpOnly, thus a note script can read it and send it away")
	}
	if hint.HttpOnly {
		t.Error("the hint cookie is HttpOnly, thus checkRole in omn-go-sse.js cannot see a guest")
	}
	if hint.Value != roleGuest {
		t.Errorf("the hint cookie holds %q, want %q", hint.Value, roleGuest)
	}
	if signed.SameSite != http.SameSiteLaxMode || hint.SameSite != http.SameSiteLaxMode {
		t.Error("a session cookie lost its SameSite attribute")
	}
}

// The hint cookie is display only. A client that writes it alone gets
// nothing, because the server never reads it.
func TestHintCookieAloneGrantsNothing(t *testing.T) {
	a := newTestApp(t)
	r := sessionReq(t, &http.Cookie{Name: sessionHintCookieName, Value: roleAdmin})
	if got := a.readSessionRole(r); got != "" {
		t.Errorf("the hint cookie alone gave the role %q, want none", got)
	}
	if a.hasRole(r, true) {
		t.Error("the hint cookie alone passed hasRole as an admin")
	}
}

// A connection from the device is the owner, with no cookie and with a
// broken one. The Android WebView and the desktop browser both arrive
// this way, thus this rule is what keeps the application usable.
func TestLocalConnectionNeedsNoCookie(t *testing.T) {
	a := newTestApp(t)
	for _, addr := range []string{"127.0.0.1:5555", "[::1]:5555"} {
		r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		r.RemoteAddr = addr
		if !a.hasRole(r, true) {
			t.Errorf("a connection from %s is not an admin", addr)
		}
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "nonsense"})
		if !a.hasRole(r, true) {
			t.Errorf("a connection from %s lost the admin role over a bad cookie", addr)
		}
	}
}

// The key file is made one time, it holds 64 hexadecimal characters, and
// the mode keeps it away from another user of the same machine.
func TestSessionSecretFile(t *testing.T) {
	a := newTestApp(t)
	first := a.sessionSecret()
	if len(first) != sessionKeyBytes {
		t.Fatalf("the key is %d bytes, want %d", len(first), sessionKeyBytes)
	}

	path := filepath.Join(a.StorageDir, sessionSecretFilename)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the key file is absent: %v", err)
	}
	if runtimeSupportsFileMode() && info.Mode().Perm() != 0600 {
		t.Errorf("the key file has mode %v, want 0600", info.Mode().Perm())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the key file: %v", err)
	}
	if got := len(strings.TrimSpace(string(raw))); got != sessionKeyBytes*2 {
		t.Errorf("the key file holds %d characters, want %d", got, sessionKeyBytes*2)
	}

	// The second call reads the cached bytes and writes nothing.
	if second := a.sessionSecret(); string(second) != string(first) {
		t.Error("the second call to sessionSecret gave another key")
	}
}

// A key file that a fault damaged is replaced, and the application keeps
// running. The old cookies stop working, which is the correct answer.
func TestDamagedSessionSecretIsReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, sessionSecretFilename)
	if err := os.WriteFile(path, []byte("not a key\n"), 0600); err != nil {
		t.Fatalf("writing the damaged file: %v", err)
	}
	a := &App{StorageDir: dir}
	key := a.sessionSecret()
	if len(key) != sessionKeyBytes {
		t.Fatalf("the replacement key is %d bytes, want %d", len(key), sessionKeyBytes)
	}
	raw, _ := os.ReadFile(path)
	if strings.TrimSpace(string(raw)) == "not a key" {
		t.Error("the damaged file is still on disk")
	}
}

// A cookie survives a restart of the process, because the key is on
// disk. Two App values over one storage directory stand for two starts.
func TestCookieSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	before := &App{StorageDir: dir}
	cookie := sessionCookie(t, before, roleAdmin)

	after := &App{StorageDir: dir}
	if got := after.readSessionRole(sessionReq(t, cookie)); got != roleAdmin {
		t.Errorf("after a restart the cookie gave the role %q, want %q", got, roleAdmin)
	}
}

// The login gives a cookie for the correct password, and nothing for a
// wrong one.
func TestLoginWritesTheTwoCookies(t *testing.T) {
	a := newTestApp(t)
	a.WithConfig(func(c *Config) {
		c.AdminPassword = "the-admin-password"
		c.GuestPassword = "the-guest-password"
	})

	cases := []struct {
		password string
		status   int
		role     string
	}{
		{"the-admin-password", http.StatusOK, roleAdmin},
		{"the-guest-password", http.StatusOK, roleGuest},
		{"wrong", http.StatusUnauthorized, ""},
		{"", http.StatusUnauthorized, ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/login?password="+c.password, nil)
		r.RemoteAddr = "192.168.1.50:5555"
		a.handleLogin(rec, r)

		if rec.Code != c.status {
			t.Errorf("the password %q gave %d, want %d", c.password, rec.Code, c.status)
			continue
		}
		if c.role == "" {
			if len(rec.Result().Cookies()) != 0 {
				t.Errorf("the password %q was refused and still set a cookie", c.password)
			}
			continue
		}

		var signed, hint *http.Cookie
		for _, got := range rec.Result().Cookies() {
			switch got.Name {
			case sessionCookieName:
				signed = got
			case sessionHintCookieName:
				hint = got
			}
		}
		if signed == nil || hint == nil {
			t.Errorf("the password %q did not set both cookies", c.password)
			continue
		}
		if hint.Value != c.role {
			t.Errorf("the hint cookie holds %q, want %q", hint.Value, c.role)
		}
		if got := a.readSessionRole(sessionReq(t, signed)); got != c.role {
			t.Errorf("the signed cookie gave the role %q, want %q", got, c.role)
		}
	}
}

// An empty configured password grants nothing. A person who clears
// admin_password asks for no admin password. The old code read the empty
// value as a match for an empty submission.
func TestEmptyPasswordGrantsNothing(t *testing.T) {
	a := newTestApp(t)
	a.WithConfig(func(c *Config) {
		c.AdminPassword = ""
		c.GuestPassword = ""
	})
	for _, password := range []string{"", "anything"} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/login?password="+password, nil)
		r.RemoteAddr = "192.168.1.50:5555"
		a.handleLogin(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("with no configured password, %q gave %d, want 401", password, rec.Code)
		}
	}
}

// runtimeSupportsFileMode answers whether Perm() reports what os.WriteFile
// asked for. Windows does not carry a Unix mode, and the test skips that
// one check there rather than fail on a platform difference.
func runtimeSupportsFileMode() bool {
	return os.PathSeparator == '/'
}
