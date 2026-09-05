package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// The hand-kept copies
// ----------------------------------------------------------------------
//
// Some rules of this application exist two times, in two languages, on
// purpose. The Go side answers for the server, and a copy in JavaScript
// or in Java answers for the page or for the Android layer. CLAUDE.md
// names each pair and asks a person to keep the two the same.
//
// A person cannot do that. Nothing tells a reader of one file that
// another file holds the same rule, and nothing fails when the two move
// apart. One pair had a test before this file. The other five had none,
// and the header pair had ALREADY moved apart. See
// TestHeaderRuleHasAFrontendCopy.
//
// Each test here reads the other language and compares a VALUE, and not
// a whole file. A test that compares a whole file breaks at each
// reformat and teaches a reader to ignore it.
//
// THE SHAPE OF A FAILURE MATTERS. Each message below says which rule
// moved and what a reader must do. A message that says "the files
// differ" sends a reader to read two files and find out why.

// portsJS reads one embedded script of the frontend.
func portsJS(t *testing.T, name string) string {
	t.Helper()
	raw, err := staticFS.ReadFile("frontend/html/js/OMN-Go/" + name)
	if err != nil {
		t.Fatalf("%s is not embedded: %v", name, err)
	}
	return string(raw)
}

// portsJava reads MainActivity.java, which sits outside the Go module.
//
// A tree with no android/ directory skips the test rather than fails it.
// The Docker build and the CI build both hold the full tree, thus the
// rule still has a guard where it counts.
func portsJava(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "android", "app", "src", "main", "java", "net", "basov", "omngo", "MainActivity.java")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("MainActivity.java is not in this tree: %v", err)
	}
	return string(raw)
}

// ----------------------------------------------------------------------
// 1. The header rule
// ----------------------------------------------------------------------

// jsForbiddenFirstCharRe reads the three characters that the JavaScript
// isHeaderFirstLine refuses at the start of a line.
var jsForbiddenFirstCharRe = regexp.MustCompile(`c !== '(.)'`)

// goForbiddenFirstCharRe reads the same three characters out of the Go
// function. Both sides are read from source, thus a change on either
// side breaks this test and not the other one silently.
var goForbiddenFirstCharRe = regexp.MustCompile(`strings\.HasPrefix\(line, "(.)"\)`)

// isHeaderFirstLine decides where a header block starts, and the editor
// needs the same answer as the server. A note whose first line the two
// sides read differently opens with the caret in the wrong place. A save
// can then write a header line into the body.
//
// CLAUDE.md section 5 states the rule. This test holds it.
func TestHeaderRuleHasAFrontendCopy(t *testing.T) {
	js := portsJS(t, "omn-go-editor.js")

	goSrc, err := os.ReadFile("header_block.go")
	if err != nil {
		t.Fatalf("header_block.go: %v", err)
	}

	jsChars := jsForbiddenFirstCharRe.FindAllStringSubmatch(js, -1)
	goChars := goForbiddenFirstCharRe.FindAllStringSubmatch(string(goSrc), -1)
	if len(jsChars) == 0 || len(goChars) == 0 {
		t.Fatal("one side no longer names its forbidden first characters, " +
			"thus this test cannot compare the two rules")
	}

	set := func(m [][]string) map[string]bool {
		out := map[string]bool{}
		for _, g := range m {
			out[g[1]] = true
		}
		return out
	}
	jsSet, goSet := set(jsChars), set(goChars)
	if fmt.Sprint(jsSet) != fmt.Sprint(goSet) {
		t.Errorf("isHeaderFirstLine refuses %v in Go and %v in omn-go-editor.js. "+
			"The editor and the server then disagree about where a header block "+
			"starts. Change both, or change neither.", goSet, jsSet)
	}

	// The colon rule and the trailing CR rule are the other two halves.
	if !strings.Contains(js, "line.indexOf(':') === -1") {
		t.Error("the JavaScript isHeaderFirstLine no longer needs a colon")
	}
	if !strings.Contains(js, `charAt(line.length - 1) === '\r'`) {
		t.Error("the JavaScript isHeaderFirstLine no longer drops a trailing CR, " +
			"thus a note with CRLF line ends classifies differently in the editor")
	}

	// The behavior of the Go side, so a change of the rule itself shows
	// here as well as in the comparison above.
	cases := []struct {
		line string
		want bool
	}{
		{"Title: X", true},
		{"Title: X\r", true},
		{"# Heading: subtitle", false},
		{" indented: value", false},
		{"<script>let x: 1", false},
		{"no colon here", false},
	}
	for _, c := range cases {
		if got := isHeaderFirstLine(c.line); got != c.want {
			t.Errorf("isHeaderFirstLine(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------
// 2. The URI scheme rule
// ----------------------------------------------------------------------

// jsURISchemeRe reads the regular expression that the click interceptor
// of omn-go-core.js uses.
var jsURISchemeRe = regexp.MustCompile(`/\^\[a-zA-Z\]\[[^/]+\*:/`)

// A link that carries a URI scheme is not a page of this application,
// and it must reach the browser as the author wrote it. The server
// decides that in rewriteInternalLink, and the page decides it again in
// setupPreviewLinkInterceptor.
//
// The two must agree. A link that the server leaves alone and the page
// catches opens the wrong thing, and the reverse loses a link.
func TestURISchemeRuleHasAFrontendCopy(t *testing.T) {
	js := portsJS(t, "omn-go-core.js")

	found := jsURISchemeRe.FindString(js)
	if found == "" {
		t.Fatal("omn-go-core.js holds no URI-scheme regular expression. " +
			"setupPreviewLinkInterceptor then treats an external link as a page.")
	}
	// Strip the two slashes of the JavaScript literal.
	jsPattern := strings.TrimSuffix(strings.TrimPrefix(found, "/"), "/")

	if jsPattern != uriSchemeRe.String() {
		t.Errorf("the URI-scheme rule is %q in Go and %q in omn-go-core.js. "+
			"A link then behaves differently on the server and in the page.",
			uriSchemeRe.String(), jsPattern)
	}

	// Both sides must agree about real links, not about the text of a
	// pattern alone.
	for _, c := range []struct {
		href string
		want bool
	}{
		{"https://example.com", true},
		{"mailto:someone@example.com", true},
		{"intent:#Intent;action=x;end;", true},
		{"tel:+1234", true},
		{"Welcome.html", false},
		{"sub/Note", false},
		{"#anchor", false},
		{"9lives:x", false}, // a scheme cannot start with a digit
	} {
		if got := uriSchemeRe.MatchString(c.href); got != c.want {
			t.Errorf("uriSchemeRe.MatchString(%q) = %v, want %v", c.href, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------
// 3. The shape of a log line
// ----------------------------------------------------------------------

// jsLevelStripRe reads the regular expression that applySyncLogLine uses
// to remove the level word.
var jsLevelStripRe = regexp.MustCompile(`replace\(/\^\\\(([^)]+)\\\)\\s\*/, ''\)`)

// The sync progress overlay reads the raw log stream. It finds the
// "[sync]" tag, removes the level word, and matches the rest against a
// table of stages.
//
// Three things must therefore agree with logger.go. Those are the
// brackets of the tag, the parentheses of the level, and the letters that
// a level name can hold. A change of the emitted shape leaves the overlay
// with no stage and no detail, and nothing else reports that.
func TestSyncLogShapeHasAFrontendCopy(t *testing.T) {
	js := portsJS(t, "omn-go-sse.js")

	if !strings.Contains(js, "'[sync]'") {
		t.Errorf("omn-go-sse.js no longer looks for '[sync]'. emitLog writes "+
			"[%s], thus the progress overlay finds no line.", logSync)
	}

	m := jsLevelStripRe.FindStringSubmatch(js)
	if m == nil {
		t.Fatal("applySyncLogLine no longer removes the level word. Each stage " +
			"then fails to match, because the line still starts with (debug).")
	}
	// The captured group is the pattern of a level name, and it carries
	// its own repetition. Compile it as it is, and anchor it.
	levelClass, cErr := regexp.Compile("^" + m[1] + "$")
	if cErr != nil {
		t.Fatalf("the level pattern %q of applySyncLogLine does not compile: %v", m[1], cErr)
	}
	for _, lvl := range []logLevel{levelDebug, levelInfo, levelError} {
		if !levelClass.MatchString(string(lvl)) {
			t.Errorf("the level %q does not match %q, which applySyncLogLine strips. "+
				"The overlay then keeps the level word and matches no stage.", lvl, m[1])
		}
	}

	// The whole shape, end to end. A line that emitLog writes must reduce
	// to its message after the two steps that applySyncLogLine takes.
	for _, lvl := range []logLevel{levelDebug, levelInfo, levelError} {
		line := "[" + string(logSync) + "] (" + string(lvl) + ") Opening repo at /x"
		at := strings.Index(line, "[sync]")
		if at == -1 {
			t.Fatalf("the emitted line %q carries no [sync] tag", line)
		}
		rest := strings.TrimSpace(line[at+len("[sync]"):])
		rest = regexp.MustCompile(`^\([a-z]+\)\s*`).ReplaceAllString(rest, "")
		if rest != "Opening repo at /x" {
			t.Errorf("a %s line reduces to %q, want the message alone", lvl, rest)
		}
	}
}

// ----------------------------------------------------------------------
// 4. The Android fullscreen mode
// ----------------------------------------------------------------------

var javaFullscreenConstRe = regexp.MustCompile(`FULLSCREEN_(OFF|ON|IMMERSIVE)\s*=\s*"([^"]*)"`)

// MainActivity reads android_fullscreen out of config.json itself, and it
// applies its own default. normalizeFullscreen in config.go applies the
// default of the server.
//
// The two must agree, or the Config page shows one mode and the window
// uses another. The default is the half that matters most: it decides
// what each install that predates the setting looks like.
func TestFullscreenModeHasAJavaCopy(t *testing.T) {
	java := portsJava(t)

	found := map[string]string{}
	for _, m := range javaFullscreenConstRe.FindAllStringSubmatch(java, -1) {
		found[m[1]] = m[2]
	}
	want := map[string]string{
		"OFF":       FullscreenOff,
		"ON":        FullscreenOn,
		"IMMERSIVE": FullscreenImmersive,
	}
	for name, value := range want {
		got, ok := found[name]
		if !ok {
			t.Errorf("MainActivity.java holds no FULLSCREEN_%s constant", name)
			continue
		}
		if got != value {
			t.Errorf("FULLSCREEN_%s is %q in Java and %q in Go. The Config page "+
				"and the window then disagree.", name, got, value)
		}
	}

	// The default of readFullscreenMode. The Java falls through to
	// FULLSCREEN_ON, and normalizeFullscreen answers the same for an
	// empty value and for a value it does not know.
	if !strings.Contains(java, "return FULLSCREEN_ON;") {
		t.Error("readFullscreenMode no longer falls back to FULLSCREEN_ON. Each " +
			"install that predates android_fullscreen then changes how it looks.")
	}
	if got := normalizeFullscreen(""); got != FullscreenOn {
		t.Errorf("normalizeFullscreen(\"\") = %q, want %q, which is what the Java "+
			"answers", got, FullscreenOn)
	}
	if got := normalizeFullscreen("nonsense"); got != FullscreenOn {
		t.Errorf("normalizeFullscreen(\"nonsense\") = %q, want %q", got, FullscreenOn)
	}
}

// ----------------------------------------------------------------------
// 5. The upload limit
// ----------------------------------------------------------------------

var javaUploadDefaultRe = regexp.MustCompile(`optInt\("max_upload_size_mb",\s*(\d+)\)`)

// The Android layer writes a shared file itself, thus it reads the limit
// out of config.json without the Go server. It carries its own default
// for a config.json that has none.
//
// A Java default above the Go one lets a file through that the server
// would refuse. A Java default below it refuses a file that the Config
// page says is allowed.
func TestUploadLimitHasAJavaCopy(t *testing.T) {
	java := portsJava(t)

	m := javaUploadDefaultRe.FindStringSubmatch(java)
	if m == nil {
		t.Fatal("MainActivity.java no longer reads max_upload_size_mb with a default")
	}
	want := fmt.Sprint(defaultMaxUploadSizeMB)
	if m[1] != want {
		t.Errorf("the upload default is %s MB in Java and %s MB in Go. The two "+
			"paths then accept different files.", m[1], want)
	}

	// readMaxUploadSizeMB answers the same number three times: for a
	// missing file, for a missing key, and for a fault. Each one must be
	// the Go default.
	inside := java
	if at := strings.Index(java, "private int readMaxUploadSizeMB()"); at != -1 {
		if end := strings.Index(java[at:], "\n    }"); end != -1 {
			inside = java[at : at+end]
		}
	}
	for _, n := range regexp.MustCompile(`return (\d+)`).FindAllStringSubmatch(inside, -1) {
		if n[1] != want {
			t.Errorf("readMaxUploadSizeMB answers %s MB somewhere and the Go default "+
				"is %s MB", n[1], want)
		}
	}
}

// ----------------------------------------------------------------------
// The list itself
// ----------------------------------------------------------------------

// A pair with no test is a pair that moves apart without a sound. This
// test holds the LIST of pairs, so that a new copy of a rule reaches a
// reader of this file.
//
// A new pair needs a test above and a row here. A pair that goes away
// needs the row removed and the reason written in the commit message.
func TestEveryHandKeptCopyHasAGuard(t *testing.T) {
	pairs := []struct{ rule, guard string }{
		{"the fold table", "TestFoldTableHasAFrontendCopy"},
		{"isHeaderFirstLine", "TestHeaderRuleHasAFrontendCopy"},
		{"the URI scheme rule", "TestURISchemeRuleHasAFrontendCopy"},
		{"the shape of a log line", "TestSyncLogShapeHasAFrontendCopy"},
		{"the Android fullscreen mode", "TestFullscreenModeHasAJavaCopy"},
		{"the upload limit", "TestUploadLimitHasAJavaCopy"},
		{"the data-secret attribute", "TestSecretAttributeHasAFrontendReader"},
	}

	// Each named guard must exist in this package. A renamed test that
	// nobody updated here leaves a row that promises a guard and gives
	// none.
	sources := map[string]bool{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(e.Name())
		if err != nil {
			continue
		}
		for _, m := range regexp.MustCompile(`func (Test\w+)`).FindAllStringSubmatch(string(data), -1) {
			sources[m[1]] = true
		}
	}
	for _, p := range pairs {
		if !sources[p.guard] {
			t.Errorf("%s names the guard %s, and no test of that name exists", p.rule, p.guard)
		}
	}
}
