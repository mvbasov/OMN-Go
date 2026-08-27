package backend

// ---------------------------------------------------------------------
// The log transport and the level rule
//
// Two things are pinned here. The shape of a line, because the browser
// parses it. And the ban on log.Printf, because a line that skips
// a.logDebugf, a.logInfof or a.logErrf carries no level, and therefore
// escapes every filter a person sets on the Config page.
// ---------------------------------------------------------------------

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// logPrintfAllowed names the only two files that may call log.Printf.
// No *App can reach either call site. loadTemplate in templates.go runs at
// package init. logAnchorsOff and addBookmarks in search_sections.go run
// from a package-level function inside a sync.Once, and from a method on
// searchDocument, which has no application.
//
// Each of those lines is a fault, and a fault always prints, so the missing
// level costs the reader nothing. They write "(error)" in the text by hand,
// which the second half of this test checks.
var logPrintfAllowed = map[string]bool{
	"templates.go":       true,
	"search_sections.go": true,
}

// handWrittenLevelRe matches the shape those two files must produce:
// a bracketed tag, then "(error)", then the message.
var handWrittenLevelRe = regexp.MustCompile(`^log\.Printf\("\[[a-z0-9-]+\] \(error\) `)

// TestNoDirectLogPrintf exists because a log.Printf line reaches stdout and
// the browser with no tag and no level. The Config page can then never
// switch it off, and the person who asked for less noise still gets it.
func TestNoDirectLogPrintf(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		// A test file does not ship to a device. This file names the banned
		// call as a string, which a scan cannot tell from a real call.
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "log.Printf(") {
				continue
			}
			if !logPrintfAllowed[name] {
				t.Errorf("%s:%d calls log.Printf. Use a.logDebugf, a.logInfof "+
					"or a.logErrf with a tag from log_levels.go. A line with no "+
					"level cannot be filtered, and the reader has no way to "+
					"switch it off.", name, i+1)
				continue
			}
			if !handWrittenLevelRe.MatchString(trimmed) {
				t.Errorf("%s:%d is an allowed log.Printf, but its text does not "+
					"start with \"[tag] (error) \". The browser reads that shape "+
					"to decide what to print.", name, i+1)
			}
		}
	}
}

// TestEmitLogLineShape pins the text the browser parses. omn-go-sse.js reads
// the tag and the level out of each line, and applySyncLogLine skips the
// level word before it matches a sync stage. A change here breaks both.
func TestEmitLogLineShape(t *testing.T) {
	a := newTestApp(t)

	ch := make(chan string, 4)
	logMutex.Lock()
	logClients = append(logClients, ch)
	logMutex.Unlock()
	defer func() {
		logMutex.Lock()
		for i, c := range logClients {
			if c == ch {
				logClients = append(logClients[:i], logClients[i+1:]...)
				break
			}
		}
		logMutex.Unlock()
	}()

	a.logDebugf(logSync, "Staging file: %s", "Note.md")
	a.logInfof(logAssets, "%d asset(s) refreshed", 3)
	a.logErrf(logEdit, "cannot run %q", "subl")

	want := []string{
		"[sync] (debug) Staging file: Note.md\n",
		"[assets] (info) 3 asset(s) refreshed\n",
		"[edit] (error) cannot run \"subl\"\n",
	}
	for _, w := range want {
		got := <-ch
		if !strings.HasSuffix(got, w) {
			t.Errorf("log line %q does not end with %q", got, w)
		}
		// The stamp is the standard log package layout, so stdout and the
		// stream look the same whatever wrote the line.
		if len(got) != len(w)+len(logTimeLayout) {
			t.Errorf("log line %q does not carry a %d-character time stamp",
				got, len(logTimeLayout))
		}
	}
}

// TestAllLogTagsIsComplete exists because the Config page builds its
// checkbox list from allLogTags. A tag that is absent from that slice can
// never be switched off, and normalizeLogTags would drop it from
// config.json on the next save.
func TestAllLogTagsIsComplete(t *testing.T) {
	src, err := os.ReadFile("log_levels.go")
	if err != nil {
		t.Fatal(err)
	}
	declared := regexp.MustCompile(`(?m)^\tlog[A-Za-z0-9]+\s+logTag = "([a-z0-9-]+)"`).
		FindAllStringSubmatch(string(src), -1)
	if len(declared) == 0 {
		t.Fatal("no logTag constant found - has the constant block changed shape?")
	}
	listed := map[logTag]bool{}
	for _, tag := range allLogTags {
		listed[tag] = true
	}
	for _, m := range declared {
		if !listed[logTag(m[1])] {
			t.Errorf("tag %q has a constant but is missing from allLogTags. "+
				"Add a new tag to both in the same commit.", m[1])
		}
	}
	if len(declared) != len(allLogTags) {
		t.Errorf("%d tag constants, %d entries in allLogTags", len(declared), len(allLogTags))
	}
}
