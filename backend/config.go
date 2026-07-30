package backend

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// maxGitServers is the fixed number of git-server config slots the UI
// exposes. It used to be a literal "5" repeated in four different places
// (this file, handleConfig's POST handler, and getConfigPageBody) that all
// had to be kept in sync by hand; centralizing it here means changing the
// slot count is a one-line change.
const maxGitServers = 5

// UI theme values accepted in Config.Theme. ThemeAuto means "follow the
// OS/browser dark-mode setting" (implemented purely in CSS via a
// prefers-color-scheme media query - no JS needed for the auto case).
const (
	ThemeAuto  = "auto"
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// normalizeTheme maps any input to a valid theme value. Unknown or empty
// values (including configs written before the theme field existed)
// become ThemeAuto. Centralized here so the config loader, the config
// POST handler and the page renderer can't disagree on what's valid -
// everything downstream (injectRuntimeVars, renderConfigPage) may safely
// assume the value is one of the three constants.
func normalizeTheme(s string) string {
	switch s {
	case ThemeLight, ThemeDark:
		return s
	default:
		return ThemeAuto
	}
}

// Android system-bar modes accepted in Config.AndroidFullscreen.
//
// Note the default is FullscreenOn, NOT the zero value: the app has always
// shipped Theme.NoTitleBar.Fullscreen in AndroidManifest.xml, so every
// existing install already runs with the status bar hidden. A plain bool
// would have made "field absent" mean false and silently changed how every
// upgraded install looks; a string enum lets normalizeFullscreen map absent
// onto the behaviour those installs already have, exactly as normalizeTheme
// maps absent onto auto.
const (
	FullscreenOff       = "off"        // status and navigation bars visible
	FullscreenOn        = "fullscreen" // status bar hidden (historic behaviour)
	FullscreenImmersive = "immersive"  // status AND navigation bars hidden
)

// normalizeFullscreen maps any input to a valid Android fullscreen mode.
// Unknown or empty values (including every config.json written before this
// field existed) become FullscreenOn. Centralized for the same reason as
// normalizeTheme - and deliberately mirrored in
// MainActivity.readFullscreenMode(), which reads config.json natively and
// must apply the identical default.
func normalizeFullscreen(s string) string {
	switch s {
	case FullscreenOff, FullscreenImmersive:
		return s
	default:
		return FullscreenOn
	}
}

// Search kinds accepted in Config.SearchKinds - the content the GLOBAL index
// covers. Page search ignores this entirely: it reads whatever file is open,
// which is what lets it work with no configuration at all.
//
// The default is notes plus bookmarks. Scripts and JSON are opt-in because
// most people's notes are prose, and every kind added is memory held for as
// long as the process lives.
const (
	SearchKindMD        = "md"
	SearchKindBookmarks = "bookmarks"
	SearchKindJS        = "js"
	SearchKindJSON      = "json"
	SearchKindUserJSON  = "user_json"
)

var searchKindsAll = []string{
	SearchKindMD, SearchKindBookmarks, SearchKindJS, SearchKindJSON, SearchKindUserJSON,
}

var searchKindsDefault = []string{SearchKindMD, SearchKindBookmarks}

// Scope values accepted in Config.SearchScope: where a search STARTS. The
// dialog can still re-aim one query without changing this.
const (
	SearchScopeAll  = "all"
	SearchScopePage = "page"
)

// normalizeSearchKinds whitelists and de-duplicates, preserving order.
//
// The nil/empty distinction is load-bearing and deliberate: a config written
// before this feature existed has NO search_kinds key, unmarshals to nil, and
// must get the default - whereas someone who unticks every box gets a real
// empty list, which means "index nothing". Same shape as normalizeTheme
// otherwise: the loader, the POST handler and the renderer all go through
// here, so none of them can disagree about what is valid.
func normalizeSearchKinds(kinds []string) []string {
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
		for _, known := range searchKindsAll {
			if k == known {
				seen[k] = true
				out = append(out, k)
				break
			}
		}
	}
	return out
}

// normalizeSearchScope maps anything unrecognised - including the "" in every
// config written before this field existed - onto SearchScopeAll.
func normalizeSearchScope(s string) string {
	if strings.ToLower(strings.TrimSpace(s)) == SearchScopePage {
		return SearchScopePage
	}
	return SearchScopeAll
}

type GitServerConfig struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	SSHKeyData string `json:"ssh_key_data"`
	Password   string `json:"password"`
}

// defaultMaxUploadSizeMB is the out-of-the-box cap on how large an
// uploaded image or JSON file may be, in megabytes - enforced by
// saveUploadedFile (backend/handlers.go) for both the editor's
// drag-and-drop upload and Android's "share to OMN-Go" file handoff
// (android/.../MainActivity.java, which reads this same value out of
// config.json natively since that path never goes through the Go HTTP
// server for the file write itself). Overridable per-install via the
// Config page; see Config.MaxUploadSizeMB below.
const defaultMaxUploadSizeMB = 3

type Config struct {
	ForcePullOneTime bool   `json:"force_pull_one_time"`
	ServerPort       int    `json:"server_port"`
	AdminPassword    string `json:"admin_password"`
	GuestPassword    string `json:"guest_password"`
	Author           string `json:"author"`
	UseInternalEd    bool   `json:"use_internal_editor"`
	DesktopExtCmd    string `json:"desktop_ext_cmd"`
	Theme            string `json:"theme"` // "auto" | "light" | "dark", see normalizeTheme
	// ShareLAN controls the listen address: false (default) binds
	// 127.0.0.1 so only this device can reach the server; true binds
	// 0.0.0.0 so other devices on the network can connect (protected by
	// the admin/guest passwords via authMiddleware). Changing it takes
	// effect on the next application start - the socket is bound once.
	ShareLAN         bool              `json:"share_lan"`
	Hostname         string            `json:"hostname"`
	BackupPruneDepth int               `json:"backup_prune_depth"`
	MimeTypes        map[string]string `json:"mime_types"`
	ActiveGitIndex   int               `json:"active_git_index"`
	GitServers       []GitServerConfig `json:"git_servers"`
	// SearchEnabled turns on GLOBAL search - the part that builds and holds
	// an index. Default FALSE: the index is the first standing memory cost
	// this app has (roughly half the size of the indexed text, held for the
	// life of the process), and on a device with little to spare the right
	// amount of it is none. Page search is unaffected and always available.
	SearchEnabled bool `json:"search_enabled"`
	// SearchKinds is what the global index covers; see normalizeSearchKinds
	// for why absent and empty mean different things.
	SearchKinds []string `json:"search_kinds"`
	// SearchBundled additionally indexes OMN-Go's own shipped scripts
	// (omn-go-*.js, *.min.js and friends - the versionDependentAssets list).
	// Off by default: they are several times the size of a typical note
	// collection and rarely what anyone is looking for.
	SearchBundled bool `json:"search_bundled"`
	// SearchScope is where a search starts, "all" or "page".
	SearchScope string `json:"search_scope"`
	// MaxUploadSizeMB caps uploaded image/JSON file size (megabytes).
	// Enforced in saveUploadedFile; see defaultMaxUploadSizeMB above for
	// where the default and the Android-native duplicate of this value
	// come from.
	MaxUploadSizeMB int `json:"max_upload_size_mb"`
	// EnableIntentURI is the master switch for launching Android "intent:"
	// URIs (e.g. [Wi-Fi](intent:#Intent;action=android.settings.WIRELESS_SETTINGS;end;))
	// from taps inside the WebView. Default false. When false,
	// MainActivity.shouldOverrideUrlLoading refuses to dispatch intent
	// URIs at all. Like MaxUploadSizeMB, the Android layer reads this value
	// straight out of config.json at tap time (see MainActivity), not
	// through the Go HTTP server, so a change applies without an app
	// restart. Purely an Android-client concern: the desktop/LAN server
	// ignores it (an intent link is dead in a normal browser regardless).
	EnableIntentURI bool `json:"enable_intent_uri"`
	// EnableTermuxIntent additionally permits the Termux RUN_COMMAND path
	// (a note running a shell command on the device via
	// com.termux/.app.RunCommandService). Default false, and gated behind
	// EnableIntentURI as well - both must be true, mirroring old OMN's
	// pk_enable_intent_uri + pk_enable_termux_intent pair - plus Termux
	// installed, its RUN_COMMAND permission granted, and a per-tap
	// confirmation (all enforced Android-side). Also read natively from
	// config.json at tap time.
	EnableTermuxIntent bool `json:"enable_termux_intent"`
	// AndroidFullscreen selects which system bars the Android app hides:
	// FullscreenOff (none), FullscreenOn (status bar - the historic and
	// default behaviour) or FullscreenImmersive (status and navigation
	// bars, revealed by a swipe). See normalizeFullscreen above for why
	// this is a string rather than a bool. Like the two intent toggles it
	// is read natively by MainActivity out of config.json rather than
	// through the Go HTTP server, and re-read on resume and after each page
	// load, so a change applies without an app restart. Purely an
	// Android-client concern; the desktop/LAN server ignores it.
	AndroidFullscreen string `json:"android_fullscreen"`
}

func (a *App) loadConfig(storageDir string) {
	a.ConfigMutex.Lock()
	defer a.ConfigMutex.Unlock()

	configPath := filepath.Join(a.StorageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		a.Config = Config{
			ServerPort:      8080,
			AdminPassword:   "admin_secret_changeme",
			GuestPassword:   "guest_secret_changeme",
			Author:          "Anonymous",
			UseInternalEd:   true,
			DesktopExtCmd:   "subl",
			Theme:           ThemeAuto,
			MaxUploadSizeMB: defaultMaxUploadSizeMB,

			// Global search is off on a fresh install; when it is switched
			// on, it starts with notes and bookmarks.
			SearchEnabled: false,
			SearchKinds:   append([]string(nil), searchKindsDefault...),
			SearchScope:   SearchScopeAll,
			// Matches the manifest's Theme.NoTitleBar.Fullscreen, so a
			// fresh install looks the same as every existing one.
			AndroidFullscreen: FullscreenOn,

			// Hostname labels this device in database backup filenames
			// (see db_backup.go); BackupPruneDepth is how many backups
			// to keep per database before the oldest is pruned.
			Hostname:         defaultHostname(),
			BackupPruneDepth: 3,

			MimeTypes: map[string]string{
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
		}
		data, err := json.MarshalIndent(a.Config, "", "  ")
		if err != nil {
			log.Printf("loadConfig: failed to marshal default config: %v", err)
		} else if err := os.WriteFile(configPath, data, 0644); err != nil {
			log.Printf("loadConfig: failed to write default config.json: %v", err)
		}
	} else {
		data, readErr := os.ReadFile(configPath)
		if readErr != nil {
			// Can't read an existing config.json - leave a.Config at its
			// zero value and say so loudly, rather than silently running
			// with an empty/broken config that looks intentional.
			log.Printf("loadConfig: failed to read %s: %v", configPath, readErr)
		} else if err := json.Unmarshal(data, &a.Config); err != nil {
			// A corrupt config.json used to be swallowed here, leaving
			// a.Config partially or fully zeroed with no indication why.
			// Log it clearly so a bad file is obvious instead of looking
			// like passwords/settings mysteriously reset themselves.
			log.Printf("loadConfig: failed to parse %s (using defaults for any unparsed fields): %v", configPath, err)
		}
		// [OMN-Go 1.5.21] Absolute Array Lock: Prevents the JSON 'null' wipe bug forever
		for len(a.Config.GitServers) < maxGitServers {
			a.Config.GitServers = append(a.Config.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(a.Config.GitServers)+1)})
		}

	}
	if a.Config.ServerPort == 0 {
		a.Config.ServerPort = 8080
	}
	// Configs written before max_upload_size_mb existed (or one explicitly
	// saved as 0/negative, which isn't a sane limit) fall back to the
	// default here - same reasoning as the ServerPort fixup just above.
	// Not persisted immediately: it'll be written out next time config.json
	// is saved for any other reason, same as the theme normalization below.
	if a.Config.MaxUploadSizeMB <= 0 {
		a.Config.MaxUploadSizeMB = defaultMaxUploadSizeMB
	}
	// Configs written before the theme field existed carry "" here;
	// normalize once at load so the rest of the code never sees an
	// invalid value.
	a.Config.Theme = normalizeTheme(a.Config.Theme)
	// Same story for android_fullscreen: configs written before the field
	// existed carry "", which normalizes to FullscreenOn - i.e. exactly the
	// status-bar-hidden behaviour those installs already had.
	a.Config.AndroidFullscreen = normalizeFullscreen(a.Config.AndroidFullscreen)
	// And the search settings: a config predating them has no search_kinds
	// key at all (nil -> the default) and an empty search_scope (-> "all").
	a.Config.SearchKinds = normalizeSearchKinds(a.Config.SearchKinds)
	a.Config.SearchScope = normalizeSearchScope(a.Config.SearchScope)
	// [OMN-Go 1.5.16] Enforce maxGitServers empty slots natively
	for len(a.Config.GitServers) < maxGitServers {
		a.Config.GitServers = append(a.Config.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(a.Config.GitServers)+1)})
	}

	if a.Config.MimeTypes == nil {
		a.Config.MimeTypes = map[string]string{
			".css":   "text/css",
			".js":    "application/javascript",
			".json":  "application/json",
			".woff2": "font/woff2",
		}
		data, err := json.MarshalIndent(a.Config, "", "  ")
		if err != nil {
			log.Printf("loadConfig: failed to marshal config after mime-type fixup: %v", err)
		} else if err := os.WriteFile(configPath, data, 0644); err != nil {
			log.Printf("loadConfig: failed to write config.json after mime-type fixup: %v", err)
		}
	}

}
