package backend

// ----------------------------------------------------------------------
// The JavaScript, from the Go gate
// ----------------------------------------------------------------------
//
// No JavaScript of this project ran under a test until 26.09.27. Two Go
// tests read a script and compared a VALUE in it, which is a real guard
// and a narrow one. ports_test.go says the limit out loud:
//
//	"A transcription is not the JavaScript itself. This test can
//	 therefore not find a fault of the transcription."
//
// jsFirstLineAfterHeader in that file is a Go copy of the editor, written
// by hand. It finds a rule that MOVED. It cannot find a copy that was
// wrong the day a person wrote it. The header pair had already moved
// apart when E1 first tested it, and 26.09.14 repaired it.
//
// The tests under backend/frontend/test/ load the shipped script and call
// the real function. TestJavaScriptUnitTests below runs them.
//
// THE TEST FILES REACH NO DEVICE. staticFS embeds frontend/html and
// frontend/md. frontend/test is neither, thus no byte of it is in the
// binary and no sync carries it. TestFrontendTestsAreNotShipped holds
// that.
//
// THE F-DROID BUILD NEVER RUNS THEM. Node is in Dockerfile.base and
// Dockerfile.ci, which build the GitHub artifacts. The F-Droid recipe
// builds the committed Gradle configuration on its own server and
// installs nothing from those files. The recipe needs no change.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	raw, err := os.ReadFile(filepath.FromSlash(headerCaseFile))
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
		at := parseHeaderBlock(c.Content).BodyOffset
		if at < 0 || at > len(c.Content) {
			t.Errorf("%s: parseHeaderBlock gave the offset %d for a note of %d bytes",
				c.Name, at, len(c.Content))
		}
	}
}

// THE TEST THAT ports_test.go COULD NOT WRITE.
//
// It runs the REAL omn-go-editor.js through node and compares each answer
// against parseHeaderBlock. A difference means the editor puts the caret
// in one place and the server reads the body from another. That is the
// fault that 26.09.14 repaired, in four of eight note shapes.
//
// It skips with no node. The build image has one since 26.09.27.
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
	cmd.Dir = "."
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
		want := parseHeaderBlock(c.Content).BodyOffset
		if got[i] == want {
			continue
		}
		t.Errorf("%s: the body starts at %d in Go and at %d in the editor.\n"+
			"  the server reads the body as %q\n"+
			"  the editor puts the caret at  %q\n"+
			"  Keep isHeaderFirstLine and firstLineAfterHeader in "+
			"omn-go-editor.js the same as header_block.go. See CLAUDE.md section 5.",
			c.Name, want, got[i], c.Content[want:], c.Content[min(got[i], len(c.Content)):])
	}
}

// TestJavaScriptUnitTests runs every test file under frontend/test.
//
// It skips with no node, the same as the Java test skips with no JDK. The
// Docker gate has both, thus the whole set runs before any artifact is
// built.
func TestJavaScriptUnitTests(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on this machine. The build image has one and runs these tests.")
	}
	files, err := filepath.Glob(filepath.FromSlash("frontend/test/*.test.js"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no test file under frontend/test: %v", err)
	}

	// The file list and not the directory. A directory argument needs a
	// newer node than Debian bookworm carries.
	args := append([]string{"--test"}, files...)
	out, runErr := exec.Command(node, args...).CombinedOutput()
	if runErr != nil {
		t.Errorf("the JavaScript tests failed: %v\n%s", runErr, out)
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "# pass") || strings.HasPrefix(line, "# fail") {
			t.Log(strings.TrimSpace(line))
		}
	}
}

// No test file may reach a device.
//
// staticFS embeds frontend/html and frontend/md. A test file under either
// one would go into the binary, onto the storage of each device, and into
// each git sync. frontend/test is outside both, and this test says so
// rather than leaving it to a reader of assets.go.
func TestFrontendTestsAreNotShipped(t *testing.T) {
	entries, err := os.ReadDir(filepath.FromSlash("frontend/test"))
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
		for _, tree := range []string{"frontend/html/", "frontend/md/", "frontend/templates/"} {
			if _, err := staticFS.ReadFile(tree + e.Name()); err == nil {
				t.Errorf("%s is embedded under %s. A test file must reach no device.",
					e.Name(), tree)
			}
		}
	}
	// And the whole directory must be absent from the embedded tree.
	if _, err := staticFS.ReadDir("frontend/test"); err == nil {
		t.Error("staticFS embeds frontend/test. Each test file would then reach " +
			"every device and every git sync.")
	}
}
