package backend

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// The secrets of the Config page
// ----------------------------------------------------------------------
//
// 26.09.7 took each password and each SSH key out of the HTML of the
// Config page. An empty box now means "keep the stored value". These
// tests hold the two halves of that rule: the page carries no secret,
// and a request that omits a secret changes none.

// secretValues are the values that the tests below plant in the
// configuration. Each one is long and unusual, thus a match in the page
// is a real leak and not a coincidence.
var secretValues = map[string]string{
	"admin":  "ADMIN-SECRET-8f2a1c",
	"guest":  "GUEST-SECRET-4b7e90",
	"sshKey": "-----BEGIN OPENSSH PRIVATE KEY-----KEY-SECRET-11ff22",
	"keyPwd": "KEYPASS-SECRET-73dd10",
}

// secretsApp builds an application whose configuration holds each value
// of secretValues, in the admin password, the guest password and git
// slot 0.
func secretsApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t)
	a.WithConfig(func(c *Config) {
		c.AdminPassword = secretValues["admin"]
		c.GuestPassword = secretValues["guest"]
		c.GitServers = make([]GitServerConfig, maxGitServers)
		c.GitServers[0].Name = "primary"
		c.GitServers[0].URL = "git@host:notes.git"
		c.GitServers[0].SSHKeyData = secretValues["sshKey"]
		c.GitServers[0].Password = secretValues["keyPwd"]
	})
	return a
}

// The compiled Config page is a file on disk in the storage directory,
// and each reader of the device can open it. It must therefore hold no
// password and no SSH key. Before 26.09.7 it held all four.
func TestConfigPageCarriesNoSecret(t *testing.T) {
	a := secretsApp(t)
	page := a.getConfigPageBody()

	for name, value := range secretValues {
		if strings.Contains(page, value) {
			t.Errorf("the Config page carries the %s value in its HTML", name)
		}
	}
	// The name and the URL of a slot are not secrets, and the page needs
	// them. A page with no name box would say that the boxes are gone
	// rather than that the secrets are gone.
	for _, want := range []string{"primary", "git@host:notes.git", `name="admin_password"`, `name="git_key_0"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the Config page lost %q", want)
		}
	}
}

// Each secret box carries data-secret. omn-go-sse.js reads that attribute
// to find the boxes that it must remove from a save. A box that loses the
// attribute silently clears its value at the next save.
func TestEverySecretBoxIsMarked(t *testing.T) {
	a := secretsApp(t)
	page := a.getConfigPageBody()

	want := []string{"admin_password", "guest_password"}
	for i := 0; i < maxGitServers; i++ {
		want = append(want, "git_key_"+itoa(i), "git_pass_"+itoa(i))
	}
	for _, name := range want {
		if !strings.Contains(page, `data-secret="`+name+`"`) {
			t.Errorf("the box %q carries no data-secret attribute", name)
		}
	}

	// No secret box may carry a value attribute that is not empty.
	valueRe := regexp.MustCompile(`data-secret="[^"]*"[^>]*value="([^"]+)"`)
	if m := valueRe.FindStringSubmatch(page); m != nil {
		t.Errorf("a secret box carries the value %q", m[1])
	}
}

// THIS IS THE TEST FOR THE TRAP THAT 26.09.7 REMOVES. The old loop wrote
// each of the four fields of a slot when a minimum of one was not empty.
// With the key box empty in the page, a save that changed the name alone
// wrote an empty key over the real one. The key password went the same
// way.
func TestConfigPostKeepsAnUnsentGitSecret(t *testing.T) {
	a := secretsApp(t)

	// Exactly what the Config page sends after the change: the name and
	// the URL of each slot, and no key and no password.
	form := url.Values{
		"git_name_0": {"renamed"},
		"git_url_0":  {"git@host:notes.git"},
	}
	postForm(t, a.handleConfigExt, "/api/config", form)

	cfg := a.GetConfig()
	if cfg.GitServers[0].Name != "renamed" {
		t.Errorf("the name is %q, want renamed", cfg.GitServers[0].Name)
	}
	if cfg.GitServers[0].SSHKeyData != secretValues["sshKey"] {
		t.Errorf("the SSH key changed to %q", cfg.GitServers[0].SSHKeyData)
	}
	if cfg.GitServers[0].Password != secretValues["keyPwd"] {
		t.Errorf("the key password changed to %q", cfg.GitServers[0].Password)
	}
}

// A reader who empties a revealed box asks for an empty value. The box is
// then dirty, thus omn-go-sse.js sends it, thus the server must write it.
func TestConfigPostClearsASentGitSecret(t *testing.T) {
	a := secretsApp(t)

	postForm(t, a.handleConfigExt, "/api/config", url.Values{
		"git_key_0":  {""},
		"git_pass_0": {""},
	})

	cfg := a.GetConfig()
	if cfg.GitServers[0].SSHKeyData != "" {
		t.Errorf("a sent and empty git_key_0 did not clear the key: %q", cfg.GitServers[0].SSHKeyData)
	}
	if cfg.GitServers[0].Password != "" {
		t.Errorf("a sent and empty git_pass_0 did not clear the password: %q", cfg.GitServers[0].Password)
	}
	// The name and the URL were not sent, thus they stay.
	if cfg.GitServers[0].Name != "primary" {
		t.Errorf("the name changed to %q", cfg.GitServers[0].Name)
	}
}

// A new key reaches the slot, and it reaches that slot alone.
func TestConfigPostWritesANewGitKey(t *testing.T) {
	a := secretsApp(t)
	a.WithConfig(func(c *Config) { c.GitServers[1].SSHKeyData = "SLOT-ONE-KEY" })

	postForm(t, a.handleConfigExt, "/api/config", url.Values{
		"git_key_0": {"A-NEW-KEY"},
	})

	cfg := a.GetConfig()
	if cfg.GitServers[0].SSHKeyData != "A-NEW-KEY" {
		t.Errorf("slot 0 holds %q, want the new key", cfg.GitServers[0].SSHKeyData)
	}
	if cfg.GitServers[1].SSHKeyData != "SLOT-ONE-KEY" {
		t.Errorf("slot 1 changed to %q", cfg.GitServers[1].SSHKeyData)
	}
}

// The same rule covers the two passwords of the Network screen. An
// omitted field keeps the stored password, and a sent and empty field
// clears it. The second half is what a person does to remove a password.
func TestConfigPostPasswordFollowsTheSentRule(t *testing.T) {
	a := secretsApp(t)
	postForm(t, a.handleConfigExt, "/api/config", url.Values{"author": {"Ann"}})
	if got := a.GetConfig().AdminPassword; got != secretValues["admin"] {
		t.Errorf("an omitted admin_password changed to %q", got)
	}

	postForm(t, a.handleConfigExt, "/api/config", url.Values{"admin_password": {""}})
	if got := a.GetConfig().AdminPassword; got != "" {
		t.Errorf("a sent and empty admin_password did not clear it: %q", got)
	}
	if got := a.GetConfig().GuestPassword; got != secretValues["guest"] {
		t.Errorf("the guest password changed to %q", got)
	}
}

// omn-go-sse.js removes each box that carries data-secret and no
// data-dirty from the FormData. The page and the script must therefore
// agree on the attribute name. This test reads both files and compares
// them, the same as TestFoldTableHasAFrontendCopy.
func TestSecretAttributeHasAFrontendReader(t *testing.T) {
	raw, err := staticFS.ReadFile("frontend/html/js/OMN-Go/omn-go-sse.js")
	if err != nil {
		t.Fatalf("omn-go-sse.js is not embedded: %v", err)
	}
	script := string(raw)

	for _, want := range []string{"[data-secret]", "dataset.dirty", "fd.delete("} {
		if !strings.Contains(script, want) {
			t.Errorf("omn-go-sse.js no longer holds %q. The Config page then sends "+
				"an empty password on each save, and each save clears it.", want)
		}
	}
	if !strings.Contains(configPageTmpl, "omnGoRevealSecrets") {
		t.Error("the Config page has no button that reads the passwords back")
	}
	if !strings.Contains(script, "window.omnGoRevealSecrets") {
		t.Error("omn-go-sse.js exports no omnGoRevealSecrets, thus the button does nothing")
	}
}
