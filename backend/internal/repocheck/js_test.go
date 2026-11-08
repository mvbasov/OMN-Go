package repocheck

// ----------------------------------------------------------------------
// The JavaScript, from the Go gate
// ----------------------------------------------------------------------
//
// A Go test that reads a script and compares a VALUE in it is a real guard
// and a narrow one. ports_test.go says the limit out loud:
//
//	"A transcription is not the JavaScript itself. This test can
//	 therefore not find a fault of the transcription."
//
// jsFirstLineAfterHeader in that file is a Go copy of the editor, written
// by hand. It finds a rule that MOVED. It cannot find a copy that was
// wrong the day a person wrote it.
//
// The tests under backend/frontend/test/ load the shipped script and call
// the real function. TestJavaScriptUnitTests below runs them.
//
// THE TEST FILES REACH NO DEVICE. frontend.Static embeds frontend/html and
// frontend/md. frontend/test is neither, thus no byte of it is in the
// binary and no sync carries it. TestFrontendTestsAreNotShipped holds
// that.
//
// THE F-DROID BUILD NEVER RUNS THEM. Node is in the Dockerfile, which
// builds the GitHub artifacts and the local artifacts. The F-Droid recipe
// builds the committed Gradle configuration on its own server and
// installs nothing from those files. The recipe needs no change.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/noteheader"
)

// headerCaseFile is the table that BOTH languages read. See its own
// comment for the contract.
const headerCaseFile = "frontend/test/header-cases.json"

type headerCase struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func readHeaderCases(t *testing.T) []headerCase {
	t.Helper()
	raw, err := readBackendFile(headerCaseFile)
	if err != nil {
		t.Fatalf("%s is missing: %v", headerCaseFile, err)
	}
	var table struct {
		Cases []headerCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("%s is not valid JSON: %v", headerCaseFile, err)
	}
	if len(table.Cases) == 0 {
		t.Fatalf("%s holds no case, thus it proves nothing", headerCaseFile)
	}
	return table.Cases
}

// The Go authority must answer for each shared case without a panic and
// with an offset inside the note.
//
// This is the Go half of the contract. header.test.js is the JavaScript
// half, and TestHeaderPortAgreesWithTheRealJavaScript below compares the
// two answers directly.
func TestHeaderCasesRunThroughTheGoAuthority(t *testing.T) {
	for _, c := range readHeaderCases(t) {
		at := noteheader.Parse(c.Content).BodyOffset
		if at < 0 || at > len(c.Content) {
			t.Errorf("%s: noteheader.Parse gave the offset %d for a note of %d bytes",
				c.Name, at, len(c.Content))
		}
	}
}

// THE TEST THAT ports_test.go COULD NOT WRITE.
//
// It runs the REAL omn-go-editor.js through node and compares each answer
// against noteheader.Parse. A difference means the editor puts the caret
// in one place and the server reads the body from another.
//
// It skips with no node. The build image has one.
func TestHeaderPortAgreesWithTheRealJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on this machine. The build image has one and runs this test.")
	}
	cases := readHeaderCases(t)

	// One node process for the whole table. A process for each case would
	// cost more than the test.
	script := `
const { load } = require('./frontend/test/dom-stub.js');
const fs = require('fs');
const editor = load('omn-go-editor.js');
const table = JSON.parse(fs.readFileSync('./frontend/test/header-cases.json', 'utf8'));
const out = table.cases.map(c => editor.firstLineAfterHeader(c.content));
process.stdout.write(JSON.stringify(out));
`
	cmd := exec.Command(node, "-e", script)
	cmd.Dir = backendDir
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, raw)
	}
	var got []int
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("node did not answer a list of offsets: %v\n%s", err, raw)
	}
	if len(got) != len(cases) {
		t.Fatalf("node answered %d offsets for %d cases", len(got), len(cases))
	}

	for i, c := range cases {
		want := noteheader.Parse(c.Content).BodyOffset
		if got[i] == want {
			continue
		}
		t.Errorf("%s: the body starts at %d in Go and at %d in the editor.\n"+
			"  the server reads the body as %q\n"+
			"  the editor puts the caret at  %q\n"+
			"  Keep isHeaderFirstLine and firstLineAfterHeader in "+
			"omn-go-editor.js the same as internal/noteheader. See CLAUDE.md section 5.",
			c.Name, want, got[i], c.Content[want:], c.Content[min(got[i], len(c.Content)):])
	}
}

// TestJavaScriptUnitTests runs every test file under frontend/test, and it
// measures the lines of each shipped script that the tests ran.
//
// It skips with no node, the same as the Java test skips with no JDK. The
// Docker gate has both, thus the whole set runs before any artifact is
// built.
//
// THE MEASURE. The run has NODE_V8_COVERAGE set, thus each node process
// writes what it ran. frontend/test/coverage.js makes one number for each
// script from these files. See the banner of that file for what counts as
// a line.
func TestJavaScriptUnitTests(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on this machine. The build image has one and runs these tests.")
	}
	files, err := filepath.Glob(backendPath("frontend/test/*.test.js"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no test file under frontend/test: %v", err)
	}

	// The file list and not the directory. The node of Debian bookworm
	// did not accept a directory argument, and a list works with each node.
	coverDir := t.TempDir()
	args := append([]string{"--test"}, files...)
	cmd := exec.Command(node, args...)
	cmd.Env = append(os.Environ(), "NODE_V8_COVERAGE="+coverDir)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Errorf("the JavaScript tests failed: %v\n%s", runErr, out)
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "# pass") || strings.HasPrefix(line, "# fail") {
			t.Log(strings.TrimSpace(line))
		}
	}

	checkJavaScriptLineCoverage(t, node, coverDir)
}

// jsLineCoverageTarget is the share of the code lines of each script that
// the tests must run, in percent. It is the target of step 6.4 of the plan.
const jsLineCoverageTarget = 60.0

// jsLineCoverageFloor holds the scripts that are below the target, with the
// share that each one has today. THE LIST ONLY SHRINKS, AND A NUMBER ONLY
// RISES. A script that reaches the target leaves the list. A script that is
// not in the list must be at the target or above it.
//
// The floor is the measured number, rounded down. A change that takes a
// test away, or that adds code with no test, fails here.
//
// THE LIST IS EMPTY. Each shipped script is at the target. Do not put a
// row here for a new script. Write the tests of that script.

var jsLineCoverageFloor = map[string]float64{}

// jsCoverageSlack is the count of percent points that a script can be above
// its floor before the test asks for a higher floor. Without it a floor
// stays low after new tests, and it then guards nothing.
const jsCoverageSlack = 5.0

// checkJavaScriptLineCoverage compares the measure of each shipped script
// with the target and with its floor.
func checkJavaScriptLineCoverage(t *testing.T, node, coverDir string) {
	t.Helper()
	out, err := exec.Command(node, backendPath("frontend/test/coverage.js"), coverDir).Output()
	if err != nil {
		t.Fatalf("frontend/test/coverage.js failed: %v", err)
	}
	var report map[string]struct {
		Lines   int     `json:"lines"`
		Covered int     `json:"covered"`
		Percent float64 `json:"percent"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("the answer of coverage.js is not JSON: %v\n%s", err, out)
	}
	if len(report) == 0 {
		t.Fatal("coverage.js found no script, thus this test proves nothing")
	}

	names := make([]string, 0, len(report))
	measured := 0
	for name, r := range report {
		names = append(names, name)
		if r.Covered > 0 {
			measured++
		}
	}
	sort.Strings(names)
	// A node that writes no coverage file gives 0 for each script. That is
	// a fault of the measure, and not of the tests.
	if measured == 0 {
		t.Fatal("no script has a line that ran. NODE_V8_COVERAGE gave no data.")
	}

	for _, name := range names {
		r := report[name]
		t.Logf("%-22s %5.1f%%  %4d of %4d lines", name, r.Percent, r.Covered, r.Lines)
		floor, listed := jsLineCoverageFloor[name]
		switch {
		case !listed && r.Percent < jsLineCoverageTarget:
			t.Errorf("the tests run %.1f%% of the lines of %s, and the target is %.0f%%. "+
				"Add a test under frontend/test.", r.Percent, name, jsLineCoverageTarget)
		case listed && r.Percent < floor:
			t.Errorf("the tests run %.1f%% of the lines of %s, and its floor is %.0f%%. "+
				"A test went away, or new code has no test.", r.Percent, name, floor)
		case listed && r.Percent >= jsLineCoverageTarget:
			t.Errorf("%s is at %.1f%%, which is the target. Remove its row from "+
				"jsLineCoverageFloor.", name, r.Percent)
		case listed && r.Percent >= floor+jsCoverageSlack:
			t.Errorf("%s is at %.1f%%, and its floor is %.0f%%. Raise the floor in "+
				"jsLineCoverageFloor.", name, r.Percent, floor)
		}
	}
	for name := range jsLineCoverageFloor {
		if _, ok := report[name]; !ok {
			t.Errorf("jsLineCoverageFloor names %s, and no such script exists", name)
		}
	}
}

// No test file may reach a device.
//
// frontend.Static embeds frontend/html and frontend/md. A test file under either
// one would go into the binary, onto the storage of each device, and into
// each git sync. frontend/test is outside both. This test holds that rule,
// thus a reader of internal/storage/assets.go does not have to check it.
func TestFrontendTestsAreNotShipped(t *testing.T) {
	entries, err := os.ReadDir(backendPath("frontend/test"))
	if err != nil {
		t.Fatalf("frontend/test is missing: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("frontend/test is empty")
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		for _, tree := range []string{"html/", "md/", "templates/"} {
			if _, err := frontend.Static.ReadFile(tree + e.Name()); err == nil {
				t.Errorf("%s is embedded under %s. A test file must reach no device.",
					e.Name(), tree)
			}
		}
	}
	// And the whole directory must be absent from the embedded tree.
	if _, err := frontend.Static.ReadDir("test"); err == nil {
		t.Error("frontend.Static embeds frontend/test. Each test file would then reach " +
			"every device and every git sync.")
	}
}
