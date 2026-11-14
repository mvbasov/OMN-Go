package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"net.basov.omngo/backend/internal/logx"
)

// MaxGitServers is the fixed number of git server slots. The loader,
// fields.go and getConfigPageBody read it, thus a new count is a
// change of one line.
const MaxGitServers = 5

// These are the theme values of Config.Theme. ThemeAuto follows the dark mode
// of the system, through the prefers-color-scheme media query of the CSS.
const (
	ThemeAuto  = "auto"
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// NormalizeTheme maps each input to a valid theme. An unknown or empty value,
// also a missing key, becomes ThemeAuto. The loader, the POST handler and the
// renderer all use it, thus each reader after them gets one of the three
// constants.
func NormalizeTheme(s string) string {
	switch s {
	case ThemeLight, ThemeDark:
		return s
	default:
		return ThemeAuto
	}
}

// These are the Android system bar modes of Config.AndroidFullscreen. The
// default is FullscreenOn, and NOT the zero value. AndroidManifest.xml sets
// Theme.NoTitleBar.Fullscreen, thus the app hides the status bar when
// config.json has no value. With a bool, a missing key would mean false, and
// an old install would change its look.
const (
	FullscreenOff       = "off"        // status and navigation bars visible
	FullscreenOn        = "fullscreen" // status bar hidden (the default)
	FullscreenImmersive = "immersive"  // status AND navigation bars hidden
)

// NormalizeFullscreen maps each input to a valid mode. An unknown or empty
// value, also a missing key, becomes FullscreenOn.
// MainActivity.readFullscreenMode reads config.json itself, and it must apply
// the same default.
func NormalizeFullscreen(s string) string {
	switch s {
	case FullscreenOff, FullscreenImmersive:
		return s
	default:
		return FullscreenOn
	}
}

// These are the search kinds of Config.SearchKinds. They tell what the GLOBAL
// index covers. Page search ignores this setting, and it reads the open file.
// The default is notes and bookmarks. Scripts, JSON and each tree of
// UserFileTrees are optional, because each kind costs memory for the life of
// the process.
const (
	SearchKindMD        = "md"
	SearchKindBookmarks = "bookmarks"
	SearchKindJS        = "js"
	SearchKindJSON      = "json"
)

// SearchKindsAll ends with one kind for each tree of UserFileTrees.
var SearchKindsAll = append([]string{
	SearchKindMD, SearchKindBookmarks, SearchKindJS, SearchKindJSON,
}, userFileKinds()...)

var searchKindsDefault = []string{SearchKindMD, SearchKindBookmarks}

// These are the scope values of Config.SearchScope. They tell where a search
// STARTS. The dialog can change the scope of one query.
const (
	SearchScopeAll  = "all"
	SearchScopePage = "page"
)

// NormalizeSearchKinds keeps only known kinds, with no duplicate, in their
// order. nil and empty differ on purpose. A config.json with NO search_kinds
// key gives nil, and nil gets the default. A person who clears each box gets
// an empty list, which means "index nothing".
func NormalizeSearchKinds(kinds []string) []string {
	if kinds == nil {
		return append([]string(nil), searchKindsDefault...)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, k := range kinds {
		k = strings.ToLower(strings.TrimSpace(k))
		if seen[k] {
			continue
		}
		for _, known := range SearchKindsAll {
			if k == known {
				seen[k] = true
				out = append(out, k)
				break
			}
		}
	}
	return out
}

// LogTagsDefault holds each tag of logx.AllTags. A fresh install checks each
// tag, because the two level switches are the control
// that a reader finds first. The tag list narrows a level that is on.
var LogTagsDefault = func() []string {
	out := make([]string, 0, len(logx.AllTags))
	for _, t := range logx.AllTags {
		out = append(out, string(t))
	}
	return out
}()

// NormalizeLogTags keeps only known tags, with no duplicate, in the order of
// logx.AllTags. As in NormalizeSearchKinds, nil gets each tag, and an empty
// list means "no debug or info line".
func NormalizeLogTags(tags []string) []string {
	if tags == nil {
		return append([]string(nil), LogTagsDefault...)
	}
	want := map[string]bool{}
	for _, t := range tags {
		want[strings.ToLower(strings.TrimSpace(t))] = true
	}
	out := []string{}
	for _, known := range LogTagsDefault {
		if want[known] {
			out = append(out, known)
		}
	}
	return out
}

// NormalizeSearchScope maps each unknown or empty value to SearchScopeAll.
func NormalizeSearchScope(s string) string {
	if strings.ToLower(strings.TrimSpace(s)) == SearchScopePage {
		return SearchScopePage
	}
	return SearchScopeAll
}

// ----------------------------------------------------------------------
// The mime_types map of config.json
// ----------------------------------------------------------------------
//
// Config.MimeTypes OVERRIDES BuiltinMIME in content_types.go, and
// resolveContentType reads it first. BuiltinMIME is the one authority for a
// content type. A FRESH INSTALL WRITES NO MAP, because each row of a map
// hides the table and has no charset. See
// doc/decisions/0003-use-one-table-for-each-content-type.md.
//
// LegacyMimeSeeds holds the two maps that an older version wrote.
// dropLegacyMimeSeed removes a map only when it is EXACTLY one of them. A map
// with one changed row is a choice of the user, and it stays.
var LegacyMimeSeeds = []map[string]string{
	{
		".css":   "text/css",
		".js":    "application/javascript",
		".json":  "application/json",
		".html":  "text/html",
		".md":    "text/markdown",
		".svg":   "image/svg+xml",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".woff2": "font/woff2",
	},
	{
		".css":   "text/css",
		".js":    "application/javascript",
		".json":  "application/json",
		".woff2": "font/woff2",
	},
}

// dropLegacyMimeSeed removes a mime_types map that an older version wrote,
// and it reports whether it changed something. The caller then writes
// config.json. A nil map is the state of a fresh install, thus the function
// answers false.
func dropLegacyMimeSeed(log logx.Logger, c *Config) bool {
	if c.MimeTypes == nil {
		return false
	}
	for _, seed := range LegacyMimeSeeds {
		if !sameStringMap(c.MimeTypes, seed) {
			continue
		}
		c.MimeTypes = nil
		log.Infof("removed the mime_types map that an older version wrote, " +
			"thus the content types of this build answer again")
		return true
	}
	return false
}

// sameStringMap answers whether two maps hold the same keys and values.
func sameStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

type GitServer struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	SSHKeyData string `json:"ssh_key_data"`
	Password   string `json:"password"`
}

// DefaultMaxUploadSizeMB is the default limit, in MB, for an uploaded image
// or a file of UserFileTrees. saveUploadedFile in upload_handlers.go applies it. The Android
// "share to OMN-Go" path writes the file without the Go server, thus
// MainActivity.java reads the same value from config.json. See
// Config.MaxUploadSizeMB.
const DefaultMaxUploadSizeMB = 3

type Config struct {
	ForcePullOneTime bool   `json:"force_pull_one_time"`
	ServerPort       int    `json:"server_port"`
	AdminPassword    string `json:"admin_password"`
	Author           string `json:"author"`
	UseInternalEd    bool   `json:"use_internal_editor"`
	DesktopExtCmd    string `json:"desktop_ext_cmd"`
	Theme            string `json:"theme"` // "auto" | "light" | "dark", see NormalizeTheme
	// ShareLAN sets the listen address. False, the default, binds 127.0.0.1,
	// and only this device can connect. True binds 0.0.0.0, and each write
	// needs the admin password. The socket binds one time, thus
	// a change applies at the next start. See
	// doc/decisions/0002-bind-the-loopback-address-when-lan-sharing-is-off.md.
	ShareLAN         bool              `json:"share_lan"`
	Hostname         string            `json:"hostname"`
	BackupPruneDepth int               `json:"backup_prune_depth"`
	MimeTypes        map[string]string `json:"mime_types"`
	ActiveGitIndex   int               `json:"active_git_index"`
	GitServers       []GitServer       `json:"git_servers"`
	// SearchEnabled turns on GLOBAL search, which builds and keeps an index.
	// The default is FALSE. The index uses about half the size of the indexed
	// text, for the life of the process. Page search does not need it.
	SearchEnabled bool `json:"search_enabled"`
	// SearchKinds is what the global index covers. See NormalizeSearchKinds
	// for why absent and empty differ.
	SearchKinds []string `json:"search_kinds"`
	// SearchBundled also indexes the scripts that OMN-Go ships, from the
	// storage.VersionDependentAssets list. It is off by default, because they are
	// larger than a typical note collection and rarely the target of a
	// search.
	SearchBundled bool `json:"search_bundled"`
	// SearchScope is where a search starts: "all" or "page".
	SearchScope string `json:"search_scope"`
	// MaxUploadSizeMB limits an uploaded image or user file, in MB. See
	// DefaultMaxUploadSizeMB.
	MaxUploadSizeMB int `json:"max_upload_size_mb"`
	// EnableIntentURI is the main switch for an Android "intent:" link in a
	// note, for example
	// [Wi-Fi](intent:#Intent;action=android.settings.WIRELESS_SETTINGS;end;).
	// The default is false, and IntentBridge.handleIntentUri then
	// refuses each intent URI. MainActivity reads the value from config.json
	// at each tap, thus a change needs no restart. The desktop ignores it.
	EnableIntentURI bool `json:"enable_intent_uri"`
	// EnableTermuxIntent also allows the Termux RUN_COMMAND path: a note that
	// runs a shell command through com.termux/.app.RunCommandService. The
	// default is false, and it needs EnableIntentURI too. Termux must be
	// installed, with its RUN_COMMAND permission, and each tap needs a
	// confirmation. The Android side enforces each rule, and it reads the
	// value from config.json at each tap.
	EnableTermuxIntent bool `json:"enable_termux_intent"`
	// AndroidFullscreen selects the system bars that the Android app hides.
	// See NormalizeFullscreen. MainActivity reads it from config.json on
	// resume and after each page load, thus a change needs no restart. The
	// desktop ignores it.
	AndroidFullscreen string `json:"android_fullscreen"`
	// LogDebug and LogInfo turn on the two quiet log levels. Both are false
	// on a fresh install, because each open page copies the log into the
	// browser console. The error level has no switch: a person who asks for
	// less noise never asks for fewer faults. See internal/logx/levels.go.
	LogDebug bool `json:"log_debug"`
	LogInfo  bool `json:"log_info"`
	// LogTags is the second axis. A debug or info line prints when its level
	// is on AND its tag is in this list. An error line ignores the list. See
	// NormalizeLogTags.
	LogTags []string `json:"log_tags"`
}

// Load fills c from the config.json at configPath. The caller holds the write
// lock. fallbackPort is the port for a config.json with none.
func Load(c *Config, configPath string, fallbackPort int, log logx.Logger) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		*c = Config{
			// Do not use a literal 8080. The fdroid flavor of Android passes
			// 8081, because the two flavors can run side by side. The loader
			// writes the port to config.json, thus each later reader sees it.
			ServerPort:      fallbackPort,
			AdminPassword:   "admin_secret_changeme",
			Author:          "Anonymous",
			UseInternalEd:   true,
			DesktopExtCmd:   "subl",
			Theme:           ThemeAuto,
			MaxUploadSizeMB: DefaultMaxUploadSizeMB,

			// Global search is off on a fresh install. When a person turns it
			// on, it covers notes and bookmarks.
			SearchEnabled: false,
			SearchKinds:   append([]string(nil), searchKindsDefault...),
			SearchScope:   SearchScopeAll,

			// Both quiet levels are off, and each tag is checked. A fresh
			// install thus writes faults and nothing else.
			LogDebug: false,
			LogInfo:  false,
			LogTags:  append([]string(nil), LogTagsDefault...),
			// This is the same as Theme.NoTitleBar.Fullscreen in the
			// manifest.
			AndroidFullscreen: FullscreenOn,

			// Hostname labels this device in the file names of database
			// backups. See internal/db/backup.go. BackupPruneDepth is the number of
			// backups that each database keeps.
			Hostname:         DefaultHostname(),
			BackupPruneDepth: 3,

			// Write NO MimeTypes MAP. A fresh install overrides nothing. See
			// LegacyMimeSeeds.
		}
		if err := Save(configPath, *c); err != nil {
			log.Errf("loadConfig: %v", err)
		}
	} else {
		data, readErr := os.ReadFile(configPath)
		if readErr != nil {
			// The loader cannot read an existing config.json. Leave c
			// at its zero value, and write an error line. Do not run with an
			// empty config in silence.
			log.Errf("loadConfig: failed to read %s: %v", configPath, readErr)
		} else if err := json.Unmarshal(data, c); err != nil {
			// A config.json that does not parse leaves c partly zero.
			// The error line explains why the passwords and settings seem to
			// reset.
			log.Errf("loadConfig: failed to parse %s (using defaults for any unparsed fields): %v", configPath, err)
		}
	}
	// A config.json with no server_port, or a value below 1, gets the
	// fallback port.
	if c.ServerPort <= 0 {
		c.ServerPort = fallbackPort
	}
	// normalizeConfig applies each other repair from the table in
	// fields.go. An old config.json can have an empty theme and no
	// log_tags key. The next save of config.json writes the repair.
	normalizeConfig(c)
	// The slot array always holds MaxGitServers rows. A config.json with
	// fewer rows, or with "git_servers": null, gets the missing rows here.
	// getConfigPageBody holds a second guard for its snapshot.
	for len(c.GitServers) < MaxGitServers {
		c.GitServers = append(c.GitServers, GitServer{Name: fmt.Sprintf("Server %d", len(c.GitServers)+1)})
	}

	// Remove a map that an older version wrote, thus the table of the build
	// answers again. See dropLegacyMimeSeed.
	if dropLegacyMimeSeed(log, c) {
		if err := Save(configPath, *c); err != nil {
			log.Errf("loadConfig: %v", err)
		}
	}
}

// Save writes c to configPath as indented JSON. Only Save writes config.json.
func Save(configPath string, c Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal the configuration: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}
	return nil
}

// LogFilter answers the log switches of c. See applyLogFilter of package
// backend.
func LogFilter(c Config) logx.Filter {
	f := logx.Filter{
		Debug: c.LogDebug,
		Info:  c.LogInfo,
		Tags:  make(map[logx.Tag]bool, len(logx.AllTags)),
	}
	for _, t := range NormalizeLogTags(c.LogTags) {
		f.Tags[logx.Tag(t)] = true
	}
	return f
}

// MaxUploadBytes converts MaxUploadSizeMB to bytes. load always sets a
// positive value, thus the fallback below is a guard only.
func MaxUploadBytes(c Config) int64 {
	mb := c.MaxUploadSizeMB
	if mb <= 0 {
		mb = DefaultMaxUploadSizeMB
	}
	return int64(mb) * 1024 * 1024
}
