package backend

import (
	"fmt"
	"net/http"
	"strings"
)

// ----------------------------------------------------------------------
// One descriptor for each setting
// ----------------------------------------------------------------------
//
// A setting of OMN-Go used to need an edit in many separate places. The
// POST handler had a branch of its own. The loader had a repair line of
// its own. The Config page carried the name of each checkbox in a
// hand-written list. Nothing tied the three together, thus one of them
// could go out of step with no test to say so.
//
// The table below is the one authority for the form side of a setting.
// See rule 7 of CLAUDE.md section 1. Each row says how a request writes
// the field, and how a value from an older version is repaired.
//
// WHAT THE TABLE DRIVES:
//
//  1. applyConfigForm, which the POST handler calls in place of a chain
//     of 150 lines.
//  2. configCheckboxFields, which fills the hidden config_fields input of
//     the Config page. That list was hand-written until 26.09.19.
//  3. normalizeConfig, which loadConfig calls in place of six lines.
//
// WHAT THE TABLE DOES NOT DRIVE. The page view and the Status page keep
// their own typed structs. Each one is a documented shape that a reader
// of the code can follow to the markup or to doc/API.md. A generated map
// would answer the same values and hide where they go.
//
// WHAT STAYS OUTSIDE THE TABLE. The five git server slots, and the
// active_git_index radio that chooses between them. They are an array of
// structs with a form shape of their own. A row for each of 21 fields
// would read worse than the loop. applyGitServerForm holds them.
//
// TestEveryConfigFieldIsInTheTable reflects over Config and fails when a
// field has neither a row nor a place in that function.

// configFieldKind says how a request writes one field.
//
// There is no separate kind for an enumeration. An enumeration is a
// cfString row with a Normalize function, and that function is the
// whitelist. normalizeTheme is the example.
type configFieldKind int

const (
	// cfBool is a checkbox. The value "true" is on, and each other value
	// is off. A browser sends nothing for an unticked box, thus a row of
	// this kind must reach the page through configCheckboxFields.
	cfBool configFieldKind = iota

	// cfInt parses the value and writes it only when it is positive. A
	// blank field, a word, and a zero each leave the setting alone. No
	// setting of this kind has a meaning at zero or below.
	cfInt

	// cfString writes the value as it arrives, an empty value included.
	// Clearing the author name has to work.
	cfString

	// cfList is a set of checkboxes that share one name. The request
	// carries the whole new set, and an empty set is a valid answer.
	cfList
)

// configField is the descriptor of one setting.
//
// Exactly one of Bool, Int, String and List is set, and it agrees with
// Kind. Each one returns a pointer into the Config that the caller
// holds, thus the apply loop needs no reflection.
type configField struct {
	// Key is the name of the form field and of the JSON key. The two are
	// the same for each setting, and TestEveryConfigFieldIsInTheTable
	// holds that.
	Key  string
	Kind configFieldKind

	// Secret marks a value that the Config page never renders. A person
	// reads it with GET /api/config after a press of "Show passwords".
	// See the banner of gitServerView in templates.go.
	Secret bool

	Bool   func(*Config) *bool
	Int    func(*Config) *int
	String func(*Config) *string
	List   func(*Config) *[]string

	// Normalize repairs the value. loadConfig calls it for each row, thus
	// a configuration that an older version wrote arrives repaired. The
	// apply loop calls it again after a write, thus a request cannot
	// store a value that the loader would refuse.
	Normalize func(*Config)
}

// configFields is the table. The order is the order of the Config page.
// configCheckboxFields reads that order, thus a move of a row changes the
// hidden input of that page.
var configFields = []configField{
	{
		Key: "server_port", Kind: cfInt,
		Int: func(c *Config) *int { return &c.ServerPort },
		// No Normalize. The repair of a missing port needs the port that
		// the caller of StartServer supplied, which is a field of App and
		// not of Config. loadConfig holds that one line.
	},
	{
		Key: "admin_password", Kind: cfString, Secret: true,
		String: func(c *Config) *string { return &c.AdminPassword },
	},
	{
		Key: "guest_password", Kind: cfString, Secret: true,
		String: func(c *Config) *string { return &c.GuestPassword },
	},
	{
		Key: "author", Kind: cfString,
		String: func(c *Config) *string { return &c.Author },
	},
	{
		Key: "use_internal_editor", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.UseInternalEd },
	},
	{
		Key: "desktop_ext_cmd", Kind: cfString,
		String: func(c *Config) *string { return &c.DesktopExtCmd },
	},
	{
		// An enumeration. normalizeTheme is the whitelist, and a value
		// that is not light or dark becomes auto.
		Key: "theme", Kind: cfString,
		String:    func(c *Config) *string { return &c.Theme },
		Normalize: func(c *Config) { c.Theme = normalizeTheme(c.Theme) },
	},
	{
		// The listen socket is bound one time at the start, thus this
		// switch takes effect at the next start. handleConfig answers
		// "RestartRequired" when the value changes.
		Key: "share_lan", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.ShareLAN },
	},
	{
		// The device label of a database backup file name. A cleared box
		// resets it to the label that the operating system gives.
		// normalizeHostname holds both halves of that rule.
		Key: "hostname", Kind: cfString,
		String:    func(c *Config) *string { return &c.Hostname },
		Normalize: func(c *Config) { c.Hostname = normalizeHostname(c.Hostname) },
	},
	{
		Key: "backup_prune_depth", Kind: cfInt,
		Int:       func(c *Config) *int { return &c.BackupPruneDepth },
		Normalize: func(c *Config) { c.BackupPruneDepth = normalizePruneDepth(c.BackupPruneDepth) },
	},
	{
		Key: "max_upload_size_mb", Kind: cfInt,
		Int: func(c *Config) *int { return &c.MaxUploadSizeMB },
		Normalize: func(c *Config) {
			if c.MaxUploadSizeMB <= 0 {
				c.MaxUploadSizeMB = defaultMaxUploadSizeMB
			}
		},
	},
	{
		// MainActivity reads this value out of config.json at the time of
		// a tap. The server on a desktop ignores it.
		Key: "enable_intent_uri", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.EnableIntentURI },
	},
	{
		Key: "enable_termux_intent", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.EnableTermuxIntent },
	},
	{
		// An enumeration, the same shape as theme above. A value that
		// this build does not know becomes FullscreenOn, which is the
		// behavior that each older install already has.
		Key: "android_fullscreen", Kind: cfString,
		String:    func(c *Config) *string { return &c.AndroidFullscreen },
		Normalize: func(c *Config) { c.AndroidFullscreen = normalizeFullscreen(c.AndroidFullscreen) },
	},
	{
		Key: "search_enabled", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.SearchEnabled },
	},
	{
		Key: "search_bundled", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.SearchBundled },
	},
	{
		// A set of checkboxes. An empty set means "index nothing", and a
		// nil slice means "this install recorded no answer". The two are
		// different, and normalizeSearchKinds keeps them apart.
		Key: "search_kinds", Kind: cfList,
		List:      func(c *Config) *[]string { return &c.SearchKinds },
		Normalize: func(c *Config) { c.SearchKinds = normalizeSearchKinds(c.SearchKinds) },
	},
	{
		// A pair of radio buttons. A browser always sends one of a radio
		// group, thus this key is not a checkbox key.
		Key: "search_scope", Kind: cfString,
		String:    func(c *Config) *string { return &c.SearchScope },
		Normalize: func(c *Config) { c.SearchScope = normalizeSearchScope(c.SearchScope) },
	},
	{
		Key: "log_debug", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.LogDebug },
	},
	{
		Key: "log_info", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.LogInfo },
	},
	{
		// A set of checkboxes, the same shape as search_kinds above and
		// with the same reason for the non-nil slice. An empty set means
		// "no debug or info line from any subsystem".
		Key: "log_tags", Kind: cfList,
		List:      func(c *Config) *[]string { return &c.LogTags },
		Normalize: func(c *Config) { c.LogTags = normalizeLogTags(c.LogTags) },
	},
}

// configCheckboxFields returns the keys that the Config page must name in
// its hidden config_fields input, as one comma-separated value.
//
// A browser sends nothing at all for an unticked checkbox, thus
// "unticked" and "not my business" arrive the same. The form declares
// what it governs, and a name in that list counts as sent. See
// configFieldSent in handlers.go for the whole rule.
//
// The list was written by hand in config_page.html until 26.09.19. A new
// checkbox that a person forgot to add to it could not be cleared. Only a
// test that reads the markup found such a fault.
func configCheckboxFields() string {
	var keys []string
	for _, f := range configFields {
		if f.Kind == cfBool || f.Kind == cfList {
			keys = append(keys, f.Key)
		}
	}
	return strings.Join(keys, ",")
}

// normalizeConfig repairs each field that has a Normalize function.
//
// loadConfig calls it one time, after it reads config.json. A field of a
// configuration that an older version wrote is then valid before any
// other code reads it.
func normalizeConfig(c *Config) {
	for _, f := range configFields {
		if f.Normalize != nil {
			f.Normalize(c)
		}
	}
}

// applyConfigForm writes each field that the request carries into c.
//
// THE RULE: a field that the request does not carry is left as it is.
// sent reports whether the request carries one field, and it counts a
// name in config_fields as carried. See configFieldSent for why an
// unticked checkbox needs that list.
//
// A cfInt row has no sent test, and it needs none. The value is written
// only when it parses to a positive number, thus an absent field and a
// blank field both leave the setting alone.
//
// Normalize runs after a write and not before it. A request therefore
// cannot store a value that loadConfig would refuse at the next start.
func applyConfigForm(c *Config, r *http.Request, sent func(string) bool) {
	for _, f := range configFields {
		if !applyConfigField(f, c, r, sent) {
			continue
		}
		if f.Normalize != nil {
			f.Normalize(c)
		}
	}
}

// applyConfigField writes one field and reports whether it wrote it.
func applyConfigField(f configField, c *Config, r *http.Request, sent func(string) bool) bool {
	switch f.Kind {
	case cfBool:
		if !sent(f.Key) {
			return false
		}
		*f.Bool(c) = r.FormValue(f.Key) == "true"
		return true

	case cfInt:
		var n int
		fmt.Sscanf(r.FormValue(f.Key), "%d", &n)
		if n <= 0 {
			return false
		}
		*f.Int(c) = n
		return true

	case cfString:
		if !sent(f.Key) {
			return false
		}
		*f.String(c) = r.FormValue(f.Key)
		return true

	case cfList:
		if !sent(f.Key) {
			return false
		}
		// The empty slice is not nil, and the difference carries a
		// meaning. See the search_kinds row above.
		values := []string{}
		values = append(values, r.Form[f.Key]...)
		*f.List(c) = values
		return true
	}
	return false
}

// applyGitServerForm writes the git server slots and the active slot.
//
// These fields stay outside the table above. Each slot is a struct of
// four fields with an index in its form name. A row for each one would
// give 21 rows that say the same thing.
//
// Each field follows the same sent rule as a field of the table. Until
// 26.09.7 the loop read the four fields of a slot and wrote each one
// when a minimum of one was not empty. That rule needed a page that
// carries the SSH key and the key password. The Config page carries
// neither since 26.09.7, thus a save that changed the name alone wrote
// an empty key over the real one.
func applyGitServerForm(c *Config, r *http.Request, sent func(string) bool) {
	// The active slot is an index and not a count. Zero is a valid
	// answer, thus this field cannot be a cfInt row of the table.
	if idxStr := r.FormValue("active_git_index"); idxStr != "" {
		var idx int
		fmt.Sscanf(idxStr, "%d", &idx)
		if idx >= 0 && idx < len(c.GitServers) {
			c.ActiveGitIndex = idx
		}
	}
	for i := 0; i < maxGitServers && i < len(c.GitServers); i++ {
		if f := fmt.Sprintf("git_name_%d", i); sent(f) {
			c.GitServers[i].Name = r.FormValue(f)
		}
		if f := fmt.Sprintf("git_url_%d", i); sent(f) {
			c.GitServers[i].URL = r.FormValue(f)
		}
		if f := fmt.Sprintf("git_key_%d", i); sent(f) {
			c.GitServers[i].SSHKeyData = r.FormValue(f)
		}
		if f := fmt.Sprintf("git_pass_%d", i); sent(f) {
			c.GitServers[i].Password = r.FormValue(f)
		}
	}
}
