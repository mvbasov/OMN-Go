package backend

// ----------------------------------------------------------------------
// The session cookie
// ----------------------------------------------------------------------
//
// Until this file existed, /login wrote the cookie "session_role=admin"
// and hasRole read that value back and trusted it. Nothing signed the
// value, thus nothing tied it to a password.
//
// A client on the network could therefore set that cookie itself and get
// the admin role with no password at all. One line in a browser console
// was enough. The admin role opens /api/sql, /api/upload,
// /api/import/note and /api/restart. A GET of /api/config answers with
// the admin password and each git password in cleartext.
//
// Two conditions limited the exposure, and neither one closed it. The
// listener binds 127.0.0.1 while share_lan is off, thus a remote client
// cannot connect. A local connection is always the owner, thus the
// device itself never read a cookie. The gate still failed for the one
// case it exists for: a person who turns LAN sharing on.
//
// THE RULE NOW: the server writes the role, the time it stops, and an
// HMAC of the two. It accepts a cookie only when it can make the same
// HMAC with the key of this install. A client cannot make that HMAC,
// because it does not hold the key.
//
// The key is 32 random bytes in <StorageDir>/session_secret, beside
// config.json. It is NOT a field of Config, and the reason is exact.
// GET /api/config marshals the whole Config struct. The Config page
// renders from the same struct. A secret in that struct reaches both.
// The file is in gitignorePatterns, thus a sync never carries the key of
// one device to another.
//
// WHY THE COOKIE IS NOT HttpOnly, AND WHY A SECOND COOKIE EXISTS.
// checkRole in omn-go-sse.js reads document.cookie to find out whether
// the reader is a guest. It then disables each control that carries the
// class admin-only. An HttpOnly cookie is invisible to that code, thus
// a guest would see each admin control.
//
// The signed cookie is HttpOnly, and a second cookie carries the role
// for the page alone:
//
//	session_role       HttpOnly, signed, the only thing the server reads
//	session_role_hint  readable, NOT signed, display only
//
// The hint decides nothing. readSessionRole never looks at it, thus a
// client that changes the hint changes what its own page shows and
// nothing more. HttpOnly is worth the second cookie here: goldmark runs
// with html.WithUnsafe(), thus a note can hold a script. A script that
// can read the cookie can send it to another machine, and that machine
// then holds the role for 30 days. A script that cannot read the cookie
// must use the browser of the reader for each request it makes.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The two roles. handleLogin writes one of these, and hasRole compares
// against them. An empty string means "no role", and it is never written
// into a cookie.
const (
	roleAdmin = "admin"
	roleGuest = "guest"
)

const (
	// sessionCookieName holds the signed value that the server reads.
	sessionCookieName = "session_role"

	// sessionHintCookieName holds the plain role that the page reads. See
	// the banner above. The server never reads this cookie.
	sessionHintCookieName = "session_role_hint"

	// sessionSecretFilename holds the HMAC key. It sits in StorageDir,
	// beside config.json and assets_version, and NOT under html/, where
	// the server would serve it.
	sessionSecretFilename = "session_secret"

	// sessionKeyBytes is the length of the HMAC key. 32 bytes is the
	// block output of SHA-256, and more key than that adds nothing.
	sessionKeyBytes = 32

	// sessionTTL is how long a login lasts. A person on the network logs
	// in one time each 30 days. A local connection needs no login at all,
	// thus this value never applies to the device itself.
	sessionTTL = 30 * 24 * time.Hour
)

// sessionSecret returns the HMAC key of this install.
//
// The key is read one time and kept for the life of the process. A first
// start makes it and writes the file with mode 0600. A file that holds
// something other than 64 hexadecimal characters is replaced, and the
// replacement invalidates each cookie that the old key signed. That is
// the correct answer to a damaged key file: a person logs in again.
//
// A failure to WRITE the file is not a failure to run. The key stays in
// memory, thus each session ends at the next start of the process. The
// line says so.
func (a *App) sessionSecret() []byte {
	a.sessionOnce.Do(func() {
		path := filepath.Join(a.StorageDir, sessionSecretFilename)

		if data, err := os.ReadFile(path); err == nil {
			key, decErr := hex.DecodeString(strings.TrimSpace(string(data)))
			if decErr == nil && len(key) == sessionKeyBytes {
				a.sessionKey = key
				return
			}
			a.logErrf(logSession, "%s does not hold a valid key, a new key replaces it", path)
		}

		key := make([]byte, sessionKeyBytes)
		if _, err := rand.Read(key); err != nil {
			// crypto/rand does not fail on a platform this application
			// runs on. If it does, write no key at all. newSessionCookie
			// then refuses to make a cookie, and each remote request gets
			// 401. A guessable key would be worse than no login.
			a.logErrf(logSession, "no random source for the session key: %v", err)
			return
		}
		if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0600); err != nil {
			a.logErrf(logSession, "failed to write %s, each session ends at the next start: %v", path, err)
		}
		a.sessionKey = key
	})
	return a.sessionKey
}

// sessionMAC makes the HMAC of one role and one expiry time.
//
// The signed text is "role.expiry", which is the cookie value without its
// third part. The separator is inside the signed text on purpose. A role
// and an expiry hold no dot of their own. No pair of them can therefore
// move text across the separator to make a second valid value.
func (a *App) sessionMAC(role string, expiry int64) string {
	key := a.sessionSecret()
	if key == nil {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(role + "." + strconv.FormatInt(expiry, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// newSessionCookies makes the two cookies of one login. See the banner
// for why there are two.
//
// The first return value is nil when the install has no key. A caller
// must test it, and must answer the request with a fault.
func (a *App) newSessionCookies(role string) (signed, hint *http.Cookie) {
	expiry := time.Now().Add(sessionTTL).Unix()
	sig := a.sessionMAC(role, expiry)
	if sig == "" {
		return nil, nil
	}
	expires := time.Unix(expiry, 0)
	value := role + "." + strconv.FormatInt(expiry, 10) + "." + sig

	signed = &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		// Lax and not Strict. A link from another application to a note
		// of this server is a normal way to arrive. Strict would show
		// the login page for such a link.
		//
		// Secure is absent because this server speaks HTTP. A Secure
		// cookie on a plain connection is a cookie the browser never
		// sends.
		SameSite: http.SameSiteLaxMode,
	}
	hint = &http.Cookie{
		Name:     sessionHintCookieName,
		Value:    role,
		Path:     "/",
		Expires:  expires,
		SameSite: http.SameSiteLaxMode,
	}
	return signed, hint
}

// readSessionRole returns the role that the request carries, or "" when
// it carries none that this install signed.
//
// It is the ONE reader of the session cookie. hasRole calls it, and
// nothing else does. See CLAUDE.md section 1, rule 7.
//
// Each of these gives "":
//
//   - No cookie.
//   - A value that is not three parts.
//   - A role that is neither admin nor guest.
//   - An expiry that is not a number, or one that passed.
//   - An HMAC that this key does not make.
//   - A value in the old, unsigned form, for example the bare word admin.
func (a *App) readSessionRole(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 {
		return ""
	}
	role, expiryText, sig := parts[0], parts[1], parts[2]

	if role != roleAdmin && role != roleGuest {
		return ""
	}
	expiry, err := strconv.ParseInt(expiryText, 10, 64)
	if err != nil {
		return ""
	}

	// The MAC is tested before the clock. A value that this install did
	// not sign is refused for that reason alone, whatever time it names.
	want := a.sessionMAC(role, expiry)
	if want == "" || !hmac.Equal([]byte(sig), []byte(want)) {
		return ""
	}
	if time.Now().Unix() >= expiry {
		return ""
	}
	return role
}
