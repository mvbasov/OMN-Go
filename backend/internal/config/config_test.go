package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"net.basov.omngo/backend/internal/logx"
)

func TestNormalizeFullscreen(t *testing.T) {
	cases := map[string]string{
		"off":        FullscreenOff,
		"fullscreen": FullscreenOn,
		"immersive":  FullscreenImmersive,
		// Empty is the important one: it is what a config.json with no
		// android_fullscreen key gives.
		"":          FullscreenOn,
		"sideways":  FullscreenOn,
		"OFF":       FullscreenOn, // case-sensitive whitelist, as NormalizeTheme
		"Immersive": FullscreenOn,
	}
	for in, want := range cases {
		if got := NormalizeFullscreen(in); got != want {
			t.Errorf("NormalizeFullscreen(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeTheme(t *testing.T) {
	cases := map[string]string{
		"auto":   ThemeAuto,
		"light":  ThemeLight,
		"dark":   ThemeDark,
		"":       ThemeAuto,
		"purple": ThemeAuto,
		"DARK":   ThemeAuto, // case-sensitive whitelist by design
	}
	for in, want := range cases {
		if got := NormalizeTheme(in); got != want {
			t.Errorf("NormalizeTheme(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNormalizeLogTags pins the nil rule. An install that upgrades to this
// version has no log_tags key in config.json, and it must get every tag. A
// person who unticks every box gets an empty list, which is a different
// thing and must survive a save.
func TestNormalizeLogTags(t *testing.T) {
	if got := NormalizeLogTags(nil); len(got) != len(logx.AllTags) {
		t.Errorf("nil gave %d tags, want every one of the %d", len(got), len(logx.AllTags))
	}
	if got := NormalizeLogTags([]string{}); len(got) != 0 {
		t.Errorf("an empty list gave %v, want an empty list - unticking every box "+
			"is not the same as an upgrade with no key", got)
	}
	got := NormalizeLogTags([]string{"SYNC", " sync ", "not-a-tag", "assets"})
	want := []string{"assets", "sync"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("NormalizeLogTags gave %v, want %v - it lowercases, trims, "+
			"drops an unknown tag, and keeps the order of logx.AllTags", got, want)
	}
}

func TestNormalizeSearchKinds(t *testing.T) {
	// Absent (nil) and empty are NOT the same thing. The difference is what
	// stands between "this config predates the feature" and "the user
	// unticked everything on purpose".
	if got := NormalizeSearchKinds(nil); strings.Join(got, ",") != "md,bookmarks" {
		t.Errorf("nil -> %v, want the default md,bookmarks", got)
	}
	if got := NormalizeSearchKinds([]string{}); len(got) != 0 {
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
		if got := strings.Join(NormalizeSearchKinds(c.in), ","); got != c.want {
			t.Errorf("NormalizeSearchKinds(%v) = %q, want %q", c.in, got, c.want)
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
		if got := NormalizeSearchScope(in); got != want {
			t.Errorf("NormalizeSearchScope(%q) = %q, want %q", in, got, want)
		}
	}
}

// Save and Load agree. A field that Save drops or renames comes back empty
// after the next start, and the person loses the setting.
func TestSaveThenLoadGivesTheSameConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := Save(p, Config{ServerPort: 9090, Theme: "dark"}); err != nil {
		t.Fatal(err)
	}
	var got Config
	Load(&got, p, 8080, logx.New(logx.Config, &atomic.Value{}, &logx.Hub{}))
	if got.ServerPort != 9090 || got.Theme != "dark" {
		t.Errorf("Load gave port %d and theme %q, want 9090 and dark", got.ServerPort, got.Theme)
	}
}

// A write that fails answers an error. persistConfig then answers 500, and
// the page does not say "Saved". The path here is a directory.
func TestSaveReportsAWriteFault(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.Mkdir(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Save(p, Config{}); err == nil {
		t.Error("Save over a directory answered no error")
	}
}
