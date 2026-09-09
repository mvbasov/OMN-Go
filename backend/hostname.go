package backend

import (
	"os"
	"regexp"
	"strings"
)

// hostnameUnsafeRe strips anything that cannot appear in the filename of a
// database backup. See db_backup.go. The hostname is embedded verbatim in
// <timestamp>_<hostname>.jsonl, thus it shares the same [A-Za-z0-9_-]
// alphabet that the database names already use.
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
// that is usually a useless "localhost". The Hostname field of the Config
// page exists exactly so the user can set a meaningful label, such as
// "pixel7". A device needs that one time.
func defaultHostname() string {
	h, err := os.Hostname()
	if err != nil || sanitizeHostname(h) == "" || strings.EqualFold(h, "localhost") {
		return "device"
	}
	return sanitizeHostname(h)
}

// normalizeHostname repairs the device label of a configuration.
//
// It returns the label that the operating system gives when the stored
// value is empty or holds no usable character. loadConfig calls it, and
// so does the hostname row of configFields. A cleared box on the Config
// page therefore resets the label, which is the rule that this field had
// before the table existed.
//
// A request that does not carry "hostname" leaves the label alone. It
// used to be rewritten to the default instead, thus a note that saved
// one unrelated setting renamed the device. The name of each database
// backup that the device writes carries that label, thus the rename did
// not stay cosmetic.
//
// IT REPAIRS AT LOAD TIME SINCE 26.09.19. The value in config.json was
// left empty until then, and displayHostname made the page show a label
// that the file did not hold. db_backup.go held a second fallback of its
// own for the same reason. One value now answers for all three.
func normalizeHostname(h string) string {
	if s := sanitizeHostname(h); s != "" {
		return s
	}
	return defaultHostname()
}

// normalizePruneDepth repairs the count of backups that one database
// keeps. A count of zero or less keeps no backup at all, which no person
// asks for, thus it becomes the default of three.
//
// The same load-time note as normalizeHostname above applies. This value
// was repaired at display time until 26.09.19, thus config.json could
// hold a 0 while the Config page showed a 3.
func normalizePruneDepth(d int) int {
	if d <= 0 {
		return 3
	}
	return d
}
