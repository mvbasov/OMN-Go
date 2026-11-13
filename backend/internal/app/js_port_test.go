package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
)

// The test of this file runs the real JavaScript through node. It compares
// it with the log filter of a real App, thus it stays in this package.
// internal/gitsync/js_port_test.go compares the lines of a real sync. The
// other JavaScript tests are in internal/repocheck/js_test.go.

// backendDir is the backend/ directory. node runs there, because the scripts
// of the tests load ./frontend/test/page-stub.js.
const backendDir = "../.."

// ----------------------------------------------------------------------
// The log filter, on both sides
// ----------------------------------------------------------------------
//
// One decision has two implementations. logx.Logger.Enabled says
// whether a line reaches stdout. logLinePrints in omn-go-api.js says
// whether the same line reaches the browser console.
//
// Rule 7 of CLAUDE.md section 1 asks for one authority. This pair is an
// exception of the same kind as the fold table. The answer is needed in
// a page that the server does not reach again. The rule therefore asks
// for a test that compares the two.
//
// THE TWO DO NOT TAKE THE SAME INPUT. The Go side takes a level and a
// tag. The JavaScript side takes the whole line and reads both out of it
// with LOG_LINE_RE. The line that it reads is the one that logx.Logger.emit
// writes, thus the test builds the line the same way logx.Logger.emit does.
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

	levels := []logx.Level{logx.LevelDebug, logx.LevelInfo, logx.LevelError}
	tags := []logx.Tag{logx.Sync, logx.Assets}

	// THE FOUR STATES OF Config.LogTags, and each one is reachable.
	//
	// nil is a configuration that never held the key, and
	// config.NormalizeLogTags answers the whole default set for it. An EMPTY
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
						cfg := config.Config{LogDebug: debug, LogInfo: info, LogTags: set}
						a.applyLogFilter(cfg)
						want = append(want, a.log(tag).Enabled(lvl))
						cases = append(cases, jsFilterCase{
							Level: string(lvl), Tag: string(tag),
							Debug: debug, Info: info,
							// EXACTLY what injectRuntimeVars sends to the
							// page. A test that builds this value another
							// way compares the two sides against a state
							// that no browser ever holds.
							Tags: strings.Join(config.NormalizeLogTags(cfg.LogTags), ","),
							// The shape that logx.Logger.emit writes. See internal/logx/hub.go.
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
    run(page, 'omn-go-api.js');
    if (typeof page.logLinePrints !== 'function') {
        process.stdout.write(JSON.stringify({ missing: true }));
        process.exit(0);
    }
    out.push(page.logLinePrints(c.line));
}
process.stdout.write(JSON.stringify({ got: out }));
`
	cmd := exec.Command(node, "-e", script, file)
	cmd.Dir = backendDir
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
		t.Fatal("omn-go-api.js does not export logLinePrints, thus this test " +
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
			"  logx.Logger.Enabled says %v\n"+
			"  logLinePrints in omn-go-api.js says %v\n"+
			"  A person then sees a quiet stdout and a loud console, or the reverse.",
			c.Level, c.Tag, c.Debug, c.Info, c.Tags, want[i], answer.Got[i])
	}
	t.Logf("%d cases, both sides agree", len(cases))
}
