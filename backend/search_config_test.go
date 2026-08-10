package backend

// Tests for the search configuration and the gating it drives.
//
// The property under nearly all of them is the same one: **the setting governs
// global search and nothing else**. Page search has to keep working with the
// config off, with every kind unticked, and with a config file written before
// any of these fields existed - because that is what makes it safe to leave the
// search button in the header of a device that will never turn the index on.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeSearchKinds(t *testing.T) {
	// Absent (nil) and empty are NOT the same thing, and the difference is
	// what stands between "this config predates the feature" and "the user
	// unticked everything on purpose".
	if got := normalizeSearchKinds(nil); strings.Join(got, ",") != "md,bookmarks" {
		t.Errorf("nil -> %v, want the default md,bookmarks", got)
	}
	if got := normalizeSearchKinds([]string{}); len(got) != 0 {
		t.Errorf("explicitly empty -> %v, want it to stay empty", got)
	}

	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"md"}, "md"},
		{[]string{"MD", " js "}, "md,js"},                           // folded and trimmed
		{[]string{"md", "md", "js"}, "md,js"},                       // de-duplicated
		{[]string{"md", "nonsense", "js"}, "md,js"},                 // unknown dropped
		{[]string{"nonsense"}, ""},                                  // ... even to nothing
		{[]string{"user_json", "bookmarks"}, "user_json,bookmarks"}, // order kept
	}
	for _, c := range cases {
		if got := strings.Join(normalizeSearchKinds(c.in), ","); got != c.want {
			t.Errorf("normalizeSearchKinds(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeSearchScope(t *testing.T) {
	for in, want := range map[string]string{
		"page": SearchScopePage,
		"PAGE": SearchScopePage,
		"all":  SearchScopeAll,
		"":     SearchScopeAll, // every config written before this field
		"junk": SearchScopeAll,
	} {
		if got := normalizeSearchScope(in); got != want {
			t.Errorf("normalizeSearchScope(%q) = %q, want %q", in, got, want)
		}
	}
}

// A config.json from before this feature must load with global search off and
// the default kinds - not with search silently enabled, and not with an empty
// kind list that would index nothing once enabled.
func TestLoadConfig_PreSearchConfigFile(t *testing.T) {
	a := &App{StorageDir: t.TempDir()}
	old := `{"server_port":8080,"author":"Ann","theme":"dark"}`
	if err := os.WriteFile(filepath.Join(a.StorageDir, "config.json"), []byte(old), 0644); err != nil {
		t.Fatal(err)
	}

	a.loadConfig(a.StorageDir)
	cfg := a.GetConfig()

	if cfg.SearchEnabled {
		t.Error("global search enabled itself on an upgrade; it must be opt-in")
	}
	if got := strings.Join(cfg.SearchKinds, ","); got != "md,bookmarks" {
		t.Errorf("SearchKinds = %q, want the default md,bookmarks", got)
	}
	if cfg.SearchScope != SearchScopeAll {
		t.Errorf("SearchScope = %q, want all", cfg.SearchScope)
	}
	if cfg.Author != "Ann" || cfg.Theme != ThemeDark {
		t.Errorf("existing settings were disturbed: %+v", cfg)
	}
}

func TestLoadConfig_FreshInstallDefaults(t *testing.T) {
	a := &App{StorageDir: t.TempDir()}
	a.loadConfig(a.StorageDir)

	cfg := a.GetConfig()
	if cfg.SearchEnabled {
		t.Error("a fresh install must not enable the index")
	}
	if strings.Join(cfg.SearchKinds, ",") != "md,bookmarks" {
		t.Errorf("SearchKinds = %v", cfg.SearchKinds)
	}

	// And it round-trips through the file it just wrote.
	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk Config
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.SearchEnabled || strings.Join(onDisk.SearchKinds, ",") != "md,bookmarks" {
		t.Errorf("config.json disagrees: %+v", onDisk)
	}
}

// Unticking every kind must survive a save. This is the case the nil/empty
// distinction exists for: a POST with no search_kinds values means "none",
// and if that were stored as nil the next load would helpfully restore the
// default the user had just removed.
func TestConfigPost_SearchKinds(t *testing.T) {
	a := newTestApp(t)
	a.WithConfig(func(c *Config) {
		c.GitServers = make([]GitServerConfig, maxGitServers)
		c.SearchKinds = []string{SearchKindMD, SearchKindBookmarks}
	})

	postForm(t, a.handleConfig, "/api/config", url.Values{
		"search_enabled": {"true"},
		"search_kinds":   {"md", "js"},
		"search_scope":   {"page"},
	})
	cfg := a.GetConfig()
	if !cfg.SearchEnabled {
		t.Error("search_enabled did not stick")
	}
	if got := strings.Join(cfg.SearchKinds, ","); got != "md,js" {
		t.Errorf("SearchKinds = %q, want md,js", got)
	}
	if cfg.SearchScope != SearchScopePage {
		t.Errorf("SearchScope = %q", cfg.SearchScope)
	}

	// Now untick everything: the form DECLARES search_kinds (config_fields),
	// so no value at all means none, not "reset to default" - and it has to
	// persist that way.
	postForm(t, a.handleConfig, "/api/config", url.Values{
		"config_fields":  {configFormFields},
		"search_enabled": {"true"},
	})
	if got := a.GetConfig().SearchKinds; len(got) != 0 {
		t.Errorf("SearchKinds = %v, want empty after unticking every box", got)
	}
	if a.GetConfig().SearchEnabled != true {
		t.Error("search_enabled was cleared by a form that set it")
	}

	data, err := os.ReadFile(filepath.Join(a.StorageDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk Config
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.SearchKinds == nil {
		t.Error("an empty kind list was written as null, which reads back as 'use the default'")
	}
	if len(onDisk.SearchKinds) != 0 {
		t.Errorf("config.json SearchKinds = %v, want empty", onDisk.SearchKinds)
	}

	// A DECLARED checkbox that sends no value clears the boolean, which is
	// what unticking it on the Config page does. An undeclared one is left
	// alone - see TestBaseline_ConfigPostSemantics.
	postForm(t, a.handleConfig, "/api/config", url.Values{"config_fields": {configFormFields}})
	if a.GetConfig().SearchEnabled {
		t.Error("a declared, unticked search_enabled did not clear")
	}
}

// The gate: global scope answers 503 with a reason, and the two reasons are
// different because they ask different things of the user.
func TestSearchGating_GlobalScope(t *testing.T) {
	a := newTestApp(t)

	rec, resp := searchReq(t, a, url.Values{"q": {"x"}, "scope": {"all"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", rec.Code)
	}
	if resp.Status != "disabled" {
		t.Errorf("status = %q, want disabled when the setting is off", resp.Status)
	}
	if !strings.Contains(resp.Error, "Search") {
		t.Errorf("error %q should point at where to turn it on", resp.Error)
	}

	// Turned on, but there is still no index: a different answer, because
	// there is nothing for the user to do about this one.
	a.WithConfig(func(c *Config) { c.SearchEnabled = true })
	rec, resp = searchReq(t, a, url.Values{"q": {"x"}, "scope": {"all"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", rec.Code)
	}
	if resp.Status != "unavailable" {
		t.Errorf("status = %q, want unavailable when enabled but not built", resp.Status)
	}
}

// Page search is not gated by anything. This is the phase's central claim, so
// it is asserted against every setting that could plausibly leak into it.
func TestSearchGating_PageScopeIgnoresConfig(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nneedle here\n")

	states := []struct {
		name string
		set  func(c *Config)
	}{
		{"all defaults", func(c *Config) {}},
		{"global search off", func(c *Config) { c.SearchEnabled = false }},
		{"global search on", func(c *Config) { c.SearchEnabled = true }},
		{"no kinds indexed", func(c *Config) { c.SearchKinds = []string{} }},
		{"notes excluded from the index", func(c *Config) { c.SearchKinds = []string{SearchKindJS} }},
		{"scope defaults to all", func(c *Config) { c.SearchScope = SearchScopeAll }},
	}
	for _, st := range states {
		a.WithConfig(st.set)
		_, resp := searchReq(t, a, url.Values{
			"q": {"needle"}, "scope": {"page"}, "on": {"Note"},
		})
		if len(resp.Results) != 1 {
			t.Errorf("%s: page search returned %d results, want 1 - it must not depend on the index settings",
				st.name, len(resp.Results))
		}
	}
}

// An unscoped request must not default into a scope that can only fail.
func TestDefaultSearchScope(t *testing.T) {
	a := newTestApp(t)
	writeSearchNote(t, a, "Note.md", "Title: A Note\n\nneedle here\n")

	// Config says "all", but global search cannot answer, so an unscoped
	// query falls back to the page rather than 503-ing a caller who
	// expressed no preference.
	a.WithConfig(func(c *Config) { c.SearchScope = SearchScopeAll; c.SearchEnabled = true })
	rec, resp := searchReq(t, a, url.Values{"q": {"needle"}, "on": {"Note"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if resp.Scope != SearchScopePage {
		t.Errorf("scope = %q, want page while global search is unavailable", resp.Scope)
	}
	if len(resp.Results) != 1 {
		t.Errorf("got %d results, want 1", len(resp.Results))
	}

	// An EXPLICIT scope=all is still refused - falling back there would hide
	// the fact that the caller asked for something this server cannot do.
	rec, _ = searchReq(t, a, url.Values{"q": {"needle"}, "scope": {"all"}, "on": {"Note"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("explicit scope=all: status %d, want 503", rec.Code)
	}
}

// OMN_SEARCH_GLOBAL means "the dialog may offer All notes", which is the
// config flag AND an index to answer from - never the flag alone, or the
// dialog would present a scope that fails.
func TestGlobalSearchAvailableRuntimeVar(t *testing.T) {
	a := newTestApp(t)

	if a.globalSearchAvailable() {
		t.Error("available with search off")
	}
	a.WithConfig(func(c *Config) { c.SearchEnabled = true })
	if a.globalSearchAvailable() {
		t.Error("available with the setting on but no index built")
	}

	page := string(a.injectRuntimeVars([]byte(runtimeVarsMarker)))
	if !strings.Contains(page, "var OMN_SEARCH_GLOBAL = false;") {
		t.Errorf("runtime var not injected as false: %s", page)
	}
}

func TestConfigPageSearchScreen(t *testing.T) {
	a := newTestApp(t)
	a.WithConfig(func(c *Config) {
		c.SearchEnabled = true
		c.SearchKinds = []string{SearchKindMD, SearchKindJS}
		c.SearchScope = SearchScopePage
	})

	body := a.getConfigPageBody()

	if strings.Contains(body, "%%") {
		t.Error("the Search screen left an unfilled placeholder")
	}
	if !strings.Contains(body, `data-goto="search"`) {
		t.Error("no Search entry on the config menu")
	}
	if !strings.Contains(body, `data-screen="search"`) {
		t.Error("no Search screen")
	}
	// The ticked boxes reflect the config, and the unticked ones do not.
	for _, want := range []string{
		`name="search_enabled" value="true" checked`,
		`name="search_kinds" value="md" checked`,
		`name="search_kinds" value="js" checked`,
		`name="search_scope" value="page" checked`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in the rendered screen", want)
		}
	}
	if strings.Contains(body, `name="search_kinds" value="bookmarks" checked`) {
		t.Error("bookmarks is not in the configured kinds but rendered as checked")
	}
	if strings.Contains(body, `name="search_scope" value="all" checked`) {
		t.Error("both scope radios rendered as checked")
	}
	// The memory cost is stated where the switch is, not buried in a manual.
	if !strings.Contains(strings.ToLower(body), "memory") {
		t.Error("the enable checkbox does not mention what it costs")
	}
}
