package backend

// Tests for the table of settings in config_fields.go.
//
// The table is the one authority for the form side of a setting. These
// tests hold that claim. The apply behavior itself is pinned by the
// existing set in baseline_test.go and handlers_test.go, which post real
// forms and read the result. Nothing here repeats that work.

import (
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// A field of Config with no row in the table, and no place in
// applyGitServerForm, is a setting that no request can write. That is a
// silent fault: the Config page can carry the input, the browser sends
// it, and the value goes nowhere.
//
// A new field therefore fails this test until it has a row or a name in
// the list below. The list is short on purpose, and each entry says why.
func TestEveryConfigFieldIsInTheTable(t *testing.T) {
	// A field that no form writes. Each one needs a reason.
	outside := map[string]string{
		// A one-shot flag that the sync code sets and clears. No page
		// carries it.
		"force_pull_one_time": "written by the force-pull flow, not by a form",
		// An override of builtinMIME. A person edits config.json by hand
		// to use it. See legacyMimeSeeds in config.go.
		"mime_types": "edited by hand in config.json",
		// The git slots and the radio that chooses one of them.
		"active_git_index": "applyGitServerForm",
		"git_servers":      "applyGitServerForm",
	}

	inTable := map[string]bool{}
	for _, f := range configFields {
		inTable[f.Key] = true
	}

	rt := reflect.TypeOf(Config{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			t.Errorf("Config.%s carries no json tag", rt.Field(i).Name)
			continue
		}
		key := strings.Split(tag, ",")[0]
		if inTable[key] || outside[key] != "" {
			continue
		}
		t.Errorf("Config.%s (json %q) has no row in configFields. Add one, "+
			"or add the key to the exception list in this test with a reason.",
			rt.Field(i).Name, key)
	}
}

// The other direction. A row whose key names no field of Config writes
// nothing. The accessor of such a row points at the wrong field.
func TestEveryTableRowNamesAConfigField(t *testing.T) {
	known := map[string]bool{}
	rt := reflect.TypeOf(Config{})
	for i := 0; i < rt.NumField(); i++ {
		known[strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	seen := map[string]bool{}
	for _, f := range configFields {
		if !known[f.Key] {
			t.Errorf("configFields row %q names no field of Config", f.Key)
		}
		if seen[f.Key] {
			t.Errorf("configFields holds two rows for %q", f.Key)
		}
		seen[f.Key] = true
	}
}

// Each row must carry the one accessor that its Kind names. A row with
// the wrong accessor, or with none, panics inside the apply loop on the
// first request that carries the field.
func TestEveryTableRowCarriesItsAccessor(t *testing.T) {
	for _, f := range configFields {
		var have int
		if f.Bool != nil {
			have++
		}
		if f.Int != nil {
			have++
		}
		if f.String != nil {
			have++
		}
		if f.List != nil {
			have++
		}
		if have != 1 {
			t.Errorf("row %q carries %d accessors, want exactly 1", f.Key, have)
			continue
		}
		ok := false
		switch f.Kind {
		case cfBool:
			ok = f.Bool != nil
		case cfInt:
			ok = f.Int != nil
		case cfString:
			ok = f.String != nil
		case cfList:
			ok = f.List != nil
		}
		if !ok {
			t.Errorf("row %q carries an accessor that its Kind does not name", f.Key)
		}
	}
}

// Each accessor must reach a DIFFERENT field of Config. A copied row that
// keeps the accessor of the row above it writes the wrong setting, and no
// compiler catches that.
//
// The test writes a mark through each accessor and reads the whole struct
// back, thus it needs no name of a field.
func TestEveryAccessorReachesItsOwnField(t *testing.T) {
	for _, f := range configFields {
		var c Config
		switch f.Kind {
		case cfBool:
			*f.Bool(&c) = true
		case cfInt:
			*f.Int(&c) = 4242
		case cfString:
			*f.String(&c) = "MARK"
		case cfList:
			*f.List(&c) = []string{"MARK"}
		}
		var touched []string
		rv := reflect.ValueOf(c)
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			if !rv.Field(i).IsZero() {
				touched = append(touched, strings.Split(rt.Field(i).Tag.Get("json"), ",")[0])
			}
		}
		if len(touched) != 1 || touched[0] != f.Key {
			t.Errorf("row %q wrote %v, want only %q", f.Key, touched, f.Key)
		}
	}
}

// The hidden config_fields input must name each checkbox row and no other
// row. A text field or a select always reaches the server. A name of one
// in that list makes a request govern a field that it did not send.
func TestCheckboxFieldsAreTheBoxRows(t *testing.T) {
	got := strings.Split(configCheckboxFields(), ",")
	var want []string
	for _, f := range configFields {
		if f.Kind == cfBool || f.Kind == cfList {
			want = append(want, f.Key)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("configCheckboxFields gave %v, want %v", got, want)
	}
	for _, f := range configFields {
		if f.Kind != cfBool && f.Kind != cfList {
			for _, k := range got {
				if k == f.Key {
					t.Errorf("%q is not a checkbox, but it is in config_fields", k)
				}
			}
		}
	}
}

// normalizeConfig must leave a valid configuration as it is. A repair
// that changes a good value is a repair that fights the person who set
// it, and loadConfig runs this on each start.
func TestNormalizeConfigIsIdempotent(t *testing.T) {
	a := newTestApp(t)
	first := a.GetConfig()
	second := first
	normalizeConfig(&second)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("normalizeConfig changed a fresh configuration:\n first  %+v\n second %+v", first, second)
	}
}

// A row of kind cfInt writes only a positive number. A blank field, a
// word and a zero each leave the setting as it is. This is what stops a
// partial form from setting the upload cap to zero, which would refuse
// every upload.
func TestIntRowsRefuseAnythingButAPositiveNumber(t *testing.T) {
	for _, bad := range []string{"", "0", "-1", "abc", " "} {
		for _, f := range configFields {
			if f.Kind != cfInt {
				continue
			}
			c := Config{}
			*f.Int(&c) = 77
			r := httptest.NewRequest("POST",
				"/api/config?"+url.Values{f.Key: {bad}}.Encode(), nil)
			applyConfigForm(&c, r, func(string) bool { return true })
			// backup_prune_depth and max_upload_size_mb carry a
			// Normalize, and 77 survives it.
			if got := *f.Int(&c); got != 77 {
				t.Errorf("%s=%q changed the value to %d, want the old 77", f.Key, bad, got)
			}
		}
	}
}

// ----------------------------------------------------------------------
// The work that a saved change starts
// ----------------------------------------------------------------------

// A rebuild of the global index reads each note, thus a save must start
// one only when the index is really wrong. The condition was one line of
// three clauses inside handleConfig until 26.09.20, and no test read it.
//
// The caller tests SearchEnabled before it calls this, thus each row
// below has search on in the new configuration.
func TestSearchIndexNeedsRebuild(t *testing.T) {
	on := func(f func(*Config)) Config {
		c := Config{SearchEnabled: true, SearchKinds: []string{"md", "bookmarks"}}
		if f != nil {
			f(&c)
		}
		return c
	}
	for _, tt := range []struct {
		what string
		prev Config
		next Config
		want bool
	}{
		{"search was off", on(func(c *Config) { c.SearchEnabled = false }), on(nil), true},
		{"nothing changed", on(nil), on(nil), false},
		{"the kinds changed", on(nil), on(func(c *Config) { c.SearchKinds = []string{"md"} }), true},
		{"the bundled switch changed", on(nil), on(func(c *Config) { c.SearchBundled = true }), true},
		{
			// A nil list and the default list are the same set. A save
			// that writes the default over a nil must not rebuild.
			"nil against the default list",
			on(func(c *Config) { c.SearchKinds = nil }),
			on(func(c *Config) { c.SearchKinds = normalizeSearchKinds(nil) }),
			false,
		},
		{
			// A change that no part of the index reads.
			"an unrelated field changed",
			on(nil), on(func(c *Config) { c.Author = "Ann" }), false,
		},
	} {
		if got := searchIndexNeedsRebuild(tt.prev, tt.next); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.what, got, tt.want)
		}
	}
}

// ----------------------------------------------------------------------
// The git slot array
// ----------------------------------------------------------------------

// loadConfig must always leave maxGitServers slots, whatever the file
// holds. Each renderer and each handler indexes that array by number, and
// a short array is an out-of-range panic waiting for a save.
//
// The loop that does this stood two times in loadConfig until 26.09.21.
// The second copy covered both branches, thus the first one was dead. A
// test of the shape below is what makes the deletion safe.
func TestConfigWithFewGitServersIsPadded(t *testing.T) {
	for _, tt := range []struct{ what, file string }{
		{"no key at all", `{"author":"Ann"}`},
		{"an explicit null", `{"git_servers":null}`},
		{"an empty array", `{"git_servers":[]}`},
		{"one slot", `{"git_servers":[{"name":"mine","url":"git@host:r.git"}]}`},
		{"a full array", `{"git_servers":[{},{},{},{},{}]}`},
	} {
		a := newUnconfiguredApp(t)
		writeConfigJSON(t, a, tt.file)
		a.loadConfig(a.StorageDir)

		cfg := a.GetConfig()
		if len(cfg.GitServers) != maxGitServers {
			t.Errorf("%s: %d slots, want %d", tt.what, len(cfg.GitServers), maxGitServers)
			continue
		}
		// A slot that the file carried keeps its values. Padding must
		// add rows and never rewrite one.
		if tt.what == "one slot" {
			if cfg.GitServers[0].Name != "mine" || cfg.GitServers[0].URL != "git@host:r.git" {
				t.Errorf("padding overwrote the slot the file carried: %+v", cfg.GitServers[0])
			}
			// Each added row carries the label that the Config page shows
			// for an empty slot.
			if cfg.GitServers[4].Name != "Server 5" {
				t.Errorf("added slot 5 is named %q, want %q", cfg.GitServers[4].Name, "Server 5")
			}
		}
	}
}

// A fresh install gets the same array. The branch that writes the default
// configuration does not pad, thus the one loop after both branches is
// what covers it.
func TestFreshInstallHasEveryGitSlot(t *testing.T) {
	a := newUnconfiguredApp(t)
	a.loadConfig(a.StorageDir)
	if got := len(a.GetConfig().GitServers); got != maxGitServers {
		t.Errorf("a fresh install has %d slots, want %d", got, maxGitServers)
	}
}
