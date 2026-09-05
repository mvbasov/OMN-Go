package backend

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readRepoFile reads a file of the repository by a path from the root.
// The tests run in backend/, thus each path starts one level up.
func readRepoFile(rel string) (string, error) {
	raw, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
	return string(raw), err
}

// ----------------------------------------------------------------------
// The settings key of the Bookmarks page
// ----------------------------------------------------------------------
//
// Bookmarker.js keeps the settings of the Bookmarks page in localStorage
// under one name. The bookmarks themselves live in md/Bookmarks.md and
// are not at risk here. The settings are the sort order, the open state
// of the tag cloud and the other display choices.
//
// The name must not change. localStorage has no migration, thus a new
// name reads as an empty store and each reader goes back to the default
// display. That is a small loss, and it is a silent one, which is what
// makes it worth a test.
//
// 26.09.15 removed three lines above the name that made a prefix from an
// application id. See the banner of Bookmarker.js for why those lines
// never ran and why the prefix answered no question.

// bookmarkerJS reads the embedded script.
func bookmarkerJS(t *testing.T) string {
	t.Helper()
	raw, err := staticFS.ReadFile("frontend/html/js/OMN-Go/Bookmarker.js")
	if err != nil {
		t.Fatalf("Bookmarker.js is not embedded: %v", err)
	}
	return string(raw)
}

// jsWithoutComments drops each line that holds a comment and nothing
// else. A test of what the CODE names must not read the banner. The
// banner of Bookmarker.js names each of the three faults that 26.09.15
// removed.
//
// The rule takes a whole line, thus a line of code keeps each byte. A
// comment that follows code on one line stays, which is safe here: no
// such comment names any of the words below.
func jsWithoutComments(js string) string {
	var out []string
	inBlock := false
	for _, line := range strings.Split(js, "\n") {
		trimmed := strings.TrimSpace(line)
		if inBlock {
			if strings.Contains(trimmed, "*/") {
				inBlock = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			if !strings.Contains(trimmed, "*/") {
				inBlock = true
			}
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// bookmarkerKeyRe reads the value that configKey holds.
var bookmarkerKeyRe = regexp.MustCompile(`const configKey = '([^']*)';`)

// The name is one literal, it is assigned one time, and it is the name
// that each existing install already holds.
func TestBookmarkerConfigKeyIsWellFormed(t *testing.T) {
	js := bookmarkerJS(t)

	m := bookmarkerKeyRe.FindStringSubmatch(js)
	if m == nil {
		t.Fatal("Bookmarker.js no longer declares configKey as one const literal")
	}
	if m[1] != "OMNBookmarkerConfigG" {
		t.Errorf("the settings key is %q, and each install holds "+
			"OMNBookmarkerConfigG. A change of the name loses the display "+
			"settings of the Bookmarks page for each reader, with no message.", m[1])
	}

	// One assignment. A second one is the fault that 26.09.15 removed,
	// and a const cannot carry it.
	if n := strings.Count(jsWithoutComments(js), "configKey ="); n != 1 {
		t.Errorf("configKey is assigned %d times, want 1. A const that a second "+
			"line assigns throws a TypeError and stops the whole page.", n)
	}
}

// The three faults of the removed lines must not come back. Each one is
// silent: the branch never started, thus no reader and no console ever
// reported it.
func TestBookmarkerHasNoDeadApplicationIdPrefix(t *testing.T) {
	// The CODE, and not the banner. See jsWithoutComments.
	js := jsWithoutComments(bookmarkerJS(t))

	for _, bad := range []struct{ text, why string }{
		{"packgeName", "a misspelled name that exists nowhere, thus the test is always false"},
		{"packageName", "a name that the frontend never defines, thus the body throws a ReferenceError"},
	} {
		if strings.Contains(js, bad.text) {
			t.Errorf("Bookmarker.js names %q again. That is %s.", bad.text, bad.why)
		}
	}

	// A prefix from PackageName would answer nothing either. The value is
	// the same literal for both flavors. See the banner of Bookmarker.js.
	if strings.Contains(js, "PackageName") {
		t.Error("Bookmarker.js builds the settings key from PackageName. That value " +
			"is the literal net.basov.omngo for each build, thus the prefix is the " +
			"same on the standard flavor and on the F-Droid flavor.")
	}
}

// The reasoning of the banner rests on two facts of the build. A change
// of either one makes the banner wrong, and this test says so.
func TestBookmarkerKeyReasoningStillHolds(t *testing.T) {
	// Fact one: the two flavors serve different ports, thus localStorage
	// is separate without any prefix.
	gradle, err := readRepoFile("android/app/build.gradle")
	if err != nil {
		t.Skipf("android/app/build.gradle is not in this tree: %v", err)
	}
	ports := regexp.MustCompile(`buildConfigField "int", "DEFAULT_SERVER_PORT", "(\d+)"`).
		FindAllStringSubmatch(gradle, -1)
	if len(ports) < 2 {
		t.Fatal("build.gradle no longer names a port for each flavor")
	}
	if ports[0][1] == ports[1][1] {
		t.Errorf("both flavors serve port %s. localStorage then belongs to one "+
			"origin for both, and the Bookmarks settings of the two builds mix. "+
			"Read the banner of Bookmarker.js before you change this.", ports[0][1])
	}

	// Fact two: PackageName is one literal, thus it cannot tell the two
	// flavors apart.
	md, err := readRepoFile("backend/markdown.go")
	if err != nil {
		t.Fatalf("markdown.go: %v", err)
	}
	if !strings.Contains(md, `PackageName: "net.basov.omngo"`) {
		t.Error("PackageName is no longer one literal in compilePageWithBody. The " +
			"banner of Bookmarker.js says that it is, thus one of the two needs a " +
			"change.")
	}
}
