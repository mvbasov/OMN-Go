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

// ----------------------------------------------------------------------
// The sync progress overlay, against a real sync
// ----------------------------------------------------------------------
//
// Section 3 of CLAUDE.md holds this rule:
//
//	"applySyncLogLine in omn-go-sse.js removes the level word before it
//	 matches a sync stage. Keep the two in agreement, or the progress
//	 overlay loses a stage."
//
// Nothing held it. ports_test.go reads the level pattern out of the
// SOURCE of omn-go-sse.js and compiles it. That proves that the pattern
// exists. It says nothing about the lines that the Go side writes.
//
// THE OVERLAY FAILS QUIETLY. A message that no prefix of SYNC_STAGES
// matches leaves the bar where it was. A person watching a sync sees a
// stage that ended, and no fault reaches any log.
//
// This test runs a WHOLE SYNC and reads the lines that it really wrote.
// The ring of 26.09.38 keeps them, thus the test needs no capture of its
// own. Each line then goes through the REAL JavaScript, in a page whose
// OMNProgress records the stage.
//
// A line of the Go side with no stage in the JavaScript side is the
// failure. That is the drift the rule names.
//
// WHAT IT DOES NOT PROVE. SYNC_STAGES holds prefixes that this scenario
// never reaches, for example the sideband text of a remote over a
// network. A test that asked for every prefix to fire would be wrong,
// and it would grow a list of exceptions. This one asks the other
// question, which is the one that hurts a person.

// jsSyncLines runs a whole life of a sync and answers each [sync] line
// that it wrote.
//
// The ring is package state, thus the test clears it first. Another test
// of this package can write a line during the run, and the filter for
// "[sync]" keeps the answer clean.
func jsSyncLines(t *testing.T) []string {
	t.Helper()
	lgClearHistory(t)

	remote := gsRemote(t)
	gsSeedRemote(t, remote, "first", map[string]string{"md/One.md": "one\n"})
	a := gsApp(t, remote)

	// A first pull, a push, a push that the remote refuses, a conflict,
	// a marked merge, an abort, a force pull and a force push.
	_ = a.SyncRepo("pull", "")
	gsWrite(t, a, "md/Two.md", "two\n")
	_ = a.SyncRepo("push", "add a note")
	gsSeedRemote(t, remote, "other", map[string]string{"md/Other.md": "other\n"})
	gsWrite(t, a, "md/Three.md", "three\n")
	_ = a.SyncRepo("push", "refused")
	_ = a.SyncRepo("pull", "")
	gsWrite(t, a, "md/One.md", "changed here\n")
	gsSeedRemote(t, remote, "third", map[string]string{"md/One.md": "changed there\n"})
	_ = a.SyncRepo("pull", "")
	_ = a.SyncRepo("pull_mark", "")
	_ = a.SyncRepo("pull_abort", "")
	_ = a.SyncRepo("pull_force", "")
	_ = a.SyncRepo("push_force", "take mine")

	var out []string
	for _, line := range logHistorySnapshot() {
		if strings.Contains(line, "[sync]") {
			out = append(out, strings.TrimRight(line, "\n"))
		}
	}
	if len(out) < 20 {
		t.Fatalf("the sync wrote %d lines. The scenario stopped early, thus "+
			"this test proves little.", len(out))
	}
	return out
}

// Each line of a real sync must move the progress overlay.
func TestEverySyncLineReachesTheOverlay(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on this machine. The build image has one and runs this test.")
	}
	lines := jsSyncLines(t)

	raw, err := json.Marshal(lines)
	if err != nil {
		t.Fatalf("cannot encode the lines: %v", err)
	}
	file := filepath.Join(t.TempDir(), "lines.json")
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		t.Fatalf("cannot write the lines: %v", err)
	}

	// One node process for the whole run. It loads the shipped script in
	// a page whose global is the window stub, the same as a <script src>
	// element does. See frontend/test/page-stub.js.
	script := `
const fs = require('fs');
const { newPage, run } = require('./frontend/test/page-stub.js');
const page = newPage();
run(page, 'omn-go-core.js');
run(page, 'omn-go-sse.js');
if (typeof page.applySyncLogLine !== 'function') {
    process.stdout.write(JSON.stringify({ missing: true }));
    process.exit(0);
}
let stages = 0;
const quiet = [];
page.OMNProgress = { show(){}, hide(){}, detail(){}, stage(){ stages++; } };
for (const line of JSON.parse(fs.readFileSync(process.argv[1], 'utf8'))) {
    const before = stages;
    page.applySyncLogLine(line);
    if (stages === before) quiet.push(line);
}
process.stdout.write(JSON.stringify({ quiet: quiet, stages: stages }));
`
	cmd := exec.Command(node, "-e", script, file)
	cmd.Dir = "."
	out, runErr := cmd.Output()
	if runErr != nil {
		t.Fatalf("node failed: %v\n%s", runErr, out)
	}
	var answer struct {
		Missing bool     `json:"missing"`
		Quiet   []string `json:"quiet"`
		Stages  int      `json:"stages"`
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		t.Fatalf("node did not answer JSON: %v\n%s", err, out)
	}
	if answer.Missing {
		t.Fatal("omn-go-sse.js does not export applySyncLogLine. " +
			"omn-go-sync.js then reads a bare name, and 26.09.41 says why " +
			"that is a trap.")
	}
	if len(answer.Quiet) > 0 {
		t.Errorf("%d of %d lines of a real sync moved no stage of the overlay.\n"+
			"  A person then watches a bar that stands still, and no fault is logged.\n"+
			"  Add a prefix to SYNC_STAGES in omn-go-sse.js, or repair the message.\n"+
			"  The first three are:\n    %s",
			len(answer.Quiet), len(lines),
			strings.Join(answer.Quiet[:min(3, len(answer.Quiet))], "\n    "))
	}
	t.Logf("%d lines of a real sync, %d stages", len(lines), answer.Stages)
}

// ----------------------------------------------------------------------
// The log filter, on both sides
// ----------------------------------------------------------------------
//
// One decision has two implementations. logLineEnabled in logger.go says
// whether a line reaches stdout. logLinePrints in omn-go-sse.js says
// whether the same line reaches the browser console.
//
// Rule 7 of CLAUDE.md section 1 asks for one authority. This pair is an
// exception of the same kind as the fold table. The answer is needed in
// a page that the server does not reach again. The rule therefore asks
// for a test that compares the two.
//
// THE TWO DO NOT TAKE THE SAME INPUT. The Go side takes a level and a
// tag. The JavaScript side takes the whole line and reads both out of it
// with LOG_LINE_RE. The line that it reads is the one that emitLog
// writes, thus the test builds the line the same way emitLog does.
//
// A DIFFERENCE IS NOT COSMETIC. A person turns a level off, sees a quiet
// stdout and a loud console, and cannot tell which one lies.

// jsFilterCase is one row of the table that both sides answer.
type jsFilterCase struct {
	Level string `json:"level"`
	Tag   string `json:"tag"`
	Debug bool   `json:"debug"`
	Info  bool   `json:"info"`
	Tags  string `json:"tags"`
	Line  string `json:"line"`
}

func TestLogFilterPortAgreesWithTheRealJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on this machine. The build image has one and runs this test.")
	}

	levels := []logLevel{levelDebug, levelInfo, levelError}
	tags := []logTag{logSync, logAssets}

	// THE FOUR STATES OF Config.LogTags, and each one is reachable.
	//
	// nil is a configuration that never held the key, and
	// normalizeLogTags answers the whole default set for it. An EMPTY
	// slice is a person who unticked every box on the Config page, and
	// it answers an empty set. The two are different, and the first
	// draft of this test used nil where it meant empty. It then read a
	// disagreement that no page can meet.
	tagSets := [][]string{
		nil,
		{},
		{"sync"},
		{"sync", "assets"},
		{"assets"},
	}

	var cases []jsFilterCase
	var want []bool
	a := newTestApp(t)
	for _, lvl := range levels {
		for _, tag := range tags {
			for _, set := range tagSets {
				for _, debug := range []bool{true, false} {
					for _, info := range []bool{true, false} {
						cfg := Config{LogDebug: debug, LogInfo: info, LogTags: set}
						a.applyLogFilter(cfg)
						want = append(want, a.logLineEnabled(lvl, tag))
						cases = append(cases, jsFilterCase{
							Level: string(lvl), Tag: string(tag),
							Debug: debug, Info: info,
							// EXACTLY what injectRuntimeVars sends to the
							// page. A test that builds this value another
							// way compares the two sides against a state
							// that no browser ever holds.
							Tags: strings.Join(normalizeLogTags(cfg.LogTags), ","),
							// The shape that emitLog writes. See logger.go.
							Line: "2026/09/06 12:00:00 [" + string(tag) + "] (" +
								string(lvl) + ") a message",
						})
					}
				}
			}
		}
	}

	raw, err := json.Marshal(cases)
	if err != nil {
		t.Fatalf("cannot encode the cases: %v", err)
	}
	file := filepath.Join(t.TempDir(), "cases.json")
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		t.Fatalf("cannot write the cases: %v", err)
	}

	script := `
const fs = require('fs');
const { newPage, run } = require('./frontend/test/page-stub.js');
const out = [];
for (const c of JSON.parse(fs.readFileSync(process.argv[1], 'utf8'))) {
    const page = newPage();
    page.OMN_LOG_DEBUG = c.debug;
    page.OMN_LOG_INFO = c.info;
    page.OMN_LOG_TAGS = c.tags;
    run(page, 'omn-go-core.js');
    run(page, 'omn-go-sse.js');
    if (typeof page.logLinePrints !== 'function') {
        process.stdout.write(JSON.stringify({ missing: true }));
        process.exit(0);
    }
    out.push(page.logLinePrints(c.line));
}
process.stdout.write(JSON.stringify({ got: out }));
`
	cmd := exec.Command(node, "-e", script, file)
	cmd.Dir = "."
	out, runErr := cmd.Output()
	if runErr != nil {
		t.Fatalf("node failed: %v\n%s", runErr, out)
	}
	var answer struct {
		Missing bool   `json:"missing"`
		Got     []bool `json:"got"`
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		t.Fatalf("node did not answer JSON: %v\n%s", err, out)
	}
	if answer.Missing {
		t.Fatal("omn-go-sse.js does not export logLinePrints, thus this test " +
			"cannot compare the two implementations.")
	}
	if len(answer.Got) != len(want) {
		t.Fatalf("node answered %d values for %d cases", len(answer.Got), len(want))
	}

	for i, c := range cases {
		if answer.Got[i] == want[i] {
			continue
		}
		t.Errorf("the two sides disagree for level %q, tag %q, debug=%v, info=%v, tags=%q.\n"+
			"  logLineEnabled in logger.go says %v\n"+
			"  logLinePrints in omn-go-sse.js says %v\n"+
			"  A person then sees a quiet stdout and a loud console, or the reverse.",
			c.Level, c.Tag, c.Debug, c.Info, c.Tags, want[i], answer.Got[i])
	}
	t.Logf("%d cases, both sides agree", len(cases))
}
