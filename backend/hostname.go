package backend

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// hostnameUnsafeRe strips anything that can't appear in a database backup
// filename (see db_backup.go): the hostname is embedded verbatim in
// <timestamp>_<hostname>.jsonl, so it shares the same [A-Za-z0-9_-]
// alphabet the db names already use.
var hostnameUnsafeRe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// sanitizeHostname maps an arbitrary user-supplied device label to a
// filename-safe string, capped at 64 chars. Empty input stays empty so
// callers can detect "not set" and fall back to defaultHostname.
func sanitizeHostname(s string) string {
	s = hostnameUnsafeRe.ReplaceAllString(strings.TrimSpace(s), "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Trim(s, "_")
}

// defaultHostname derives a device label from the OS hostname. On Android
// this is usually a useless "localhost" - the Config page's Hostname
// field exists precisely so the user can set a meaningful label
// ("pixel7") once per device.
func defaultHostname() string {
	h, err := os.Hostname()
	if err != nil || sanitizeHostname(h) == "" || strings.EqualFold(h, "localhost") {
		return "device"
	}
	return sanitizeHostname(h)
}

// handleConfigExt wraps handleConfig to persist the fields this feature
// adds WITHOUT editing handleConfig's body: on POST the new form values
// are applied to the live config first, then handleConfig runs as
// always - its own read-modify-write snapshot marshals the full Config
// struct (including these fields) and writes config.json. r.FormValue
// parses and caches the form, so handleConfig's later FormValue calls
// see the same data. Registered on /api/config in place of handleConfig.
func (a *App) handleConfigExt(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		host := sanitizeHostname(r.FormValue("hostname"))
		if host == "" {
			// Clearing the field resets to the OS-derived default.
			host = defaultHostname()
		}
		var depth int
		fmt.Sscanf(r.FormValue("backup_prune_depth"), "%d", &depth)
		a.WithConfig(func(c *Config) {
			c.Hostname = host
			if depth > 0 {
				c.BackupPruneDepth = depth
			}
		})
	}
	a.handleConfig(w, r)
}

// displayHostname / displayPruneDepth back-fill the on-page defaults for
// installations whose config.json predates these fields (zero values), so
// what the Config page shows always matches the effective runtime
// fallbacks - no config-file migration needed.
func displayHostname(h string) string {
	if s := sanitizeHostname(h); s != "" {
		return s
	}
	return defaultHostname()
}

func displayPruneDepth(d int) int {
	if d <= 0 {
		return 3
	}
	return d
}
