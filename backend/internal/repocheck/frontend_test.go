package repocheck

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
)

// ---------------------------------------------------------------------
// The clipboard and the documented API
// ---------------------------------------------------------------------

// TestClipboardHasOneAuthority exists because a second clipboard path
// gives a second chance to get the Android WebView wrong. Two paths can
// then behave differently: one copy works on Android 6 and the other does
// not.
//
// Only omn-go-share.js can call execCommand('copy'). That file holds
// omnGoCopyText, which each other caller uses. It also holds the
// quick-note-copy action, which stays direct. The text of that action is
// already in a textarea, and the focus must stay there for the typing that
// follows.
func TestClipboardHasOneAuthority(t *testing.T) {
	const authority = "html/js/OMN-Go/omn-go-share.js"
	for _, tree := range []struct {
		name string
		fs   fs.FS
		root string
	}{
		{"frontend.Static", frontend.Static, "html"},
		{"frontend.Templates", frontend.Templates, "templates"},
	} {
		err := fs.WalkDir(tree.fs, tree.root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(p, ".js") && !strings.HasSuffix(p, ".html") {
				return nil
			}
			src, rerr := fs.ReadFile(tree.fs, p)
			if rerr != nil {
				return rerr
			}
			if !strings.Contains(string(src), `execCommand('copy')`) {
				return nil
			}
			if p != authority {
				t.Errorf("%s calls execCommand('copy'). Call "+
					"window.omnGoCopyText in its place. That function is the "+
					"only clipboard writer. It holds what this project knows "+
					"about the Android WebView.", p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", tree.name, err)
		}
	}
}

// documentedCoreAPI names each frontend function that the "Useful
// functions" section of the User Manual gives to a note script. A
// documented name is a promise. Each name thus has an explicit window
// export, and not a bare declaration that becomes global by accident.
var documentedCoreAPI = []string{
	"OMNProgress",
	"omnClearHighlights",
	"omnGoCopyText",
	"omnGoOnServerLog",
	"omnGoOpenDatabase",
	"omnGoPageLink",
	"omnGoPageTitle",
	"omnGoRenderMath",
	"omnHighlightTerms",
	"omnSearchOpen",
}

// TestDocumentedCoreAPIIsExported exists because the manual sends a reader
// to these names. A rename takes a documented name away, and an IIFE
// around one file does the same. There is no other sign of the loss. The
// note that used the name then fails in the browser of the reader, and
// nowhere else.
func TestDocumentedCoreAPIIsExported(t *testing.T) {
	// It reads EVERY shipped script and not a named list. A script can
	// split into several files, and a fixed list would have to grow with
	// each such move. A name that no file exports is the fault
	// this test looks for, and the file that holds it does not matter.
	entries, err := frontend.Static.ReadDir("html/js/OMN-Go")
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	var read int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		src, rErr := frontend.Static.ReadFile("html/js/OMN-Go/" + e.Name())
		if rErr != nil {
			t.Fatalf("%s is not embedded: %v", e.Name(), rErr)
		}
		all.Write(src)
		read++
	}
	if read == 0 {
		t.Fatal("no script was read, thus this test proves nothing")
	}
	src := all.String()
	for _, name := range documentedCoreAPI {
		if !strings.Contains(src, "window."+name+" =") {
			t.Errorf("the User Manual documents window.%s. No frontend file "+
				"exports that name. Keep the name, or change the manual in "+
				"the same commit.", name)
		}
	}
}

// ----------------------------------------------------------------------
// A modal that hides by class must have a rule that shows it again
// ----------------------------------------------------------------------
//
// dialogClassRe finds the class omn-overlay or omn-modal as a whole word.
// The class omn-modal-buttons-row of a panel does not match.
var dialogClassRe = regexp.MustCompile(`(^|[\s"'])omn-(overlay|modal)([\s"']|$)`)

// TestNoDialogMarkupOutsideOmnDialog keeps each dialog in omnDialog of
// omn-go-dialog.js. A template or a script that builds an overlay of its
// own is a second way to make a dialog. Such a dialog misses the queue, the
// keys and the Back key of omnDialog.
//
// The test reads each template and each script of OMN-Go. A line with the
// class omn-overlay or omn-modal fails, unless it is in omn-go-dialog.js.
func TestNoDialogMarkupOutsideOmnDialog(t *testing.T) {
	type source struct{ dir, glob string }
	scanned := 0
	for _, src := range []source{{"templates", "templates/*.html"}, {"html/js/OMN-Go", "html/js/OMN-Go/*.js"}} {
		fsys := frontend.Templates
		if src.dir != "templates" {
			fsys = frontend.Static
		}
		names, err := fs.Glob(fsys, src.glob)
		if err != nil || len(names) == 0 {
			t.Fatalf("no file matches %s: %v", src.glob, err)
		}
		for _, name := range names {
			if strings.HasSuffix(name, ".min.js") || strings.HasSuffix(name, "/omn-go-dialog.js") {
				continue
			}
			data, err := fs.ReadFile(fsys, name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			scanned++
			for i, line := range strings.Split(string(data), "\n") {
				if !dialogClassRe.MatchString(line) {
					continue
				}
				t.Errorf("%s:%d builds a dialog of its own. Use omnDialog of "+
					"omn-go-dialog.js: %s", name, i+1, strings.TrimSpace(line))
			}
		}
	}
	if scanned < 5 {
		t.Fatalf("the scan read %d files. The globs no longer match the tree.", scanned)
	}
}

// ----------------------------------------------------------------------
// A control names its work in data-action
// ----------------------------------------------------------------------
//
// A control of a template has no inline handler such as onclick. It holds
// data-action="name", and OMN.action("name", fn) in a script gives the name
// its function. One click listener of omn-go-core.js joins the two.
//
// A name with no function is a dead button. Nothing throws, and the only
// report is one console warning at the click. The first test below finds
// such a name in the source.

// inlineHandlerRe finds an inline event handler in markup: a space, then
// "on", a name and "=". The space keeps "content=" out.
var inlineHandlerRe = regexp.MustCompile(`\son[a-z]+\s*=\s*["']`)

// dataActionRe finds the name of a data-action attribute.
var dataActionRe = regexp.MustCompile(`data-action="([^"]+)"`)

// actionCallRe finds the name in a call of OMN.action or of its local name
// action.
var actionCallRe = regexp.MustCompile(`\baction\('([a-z0-9-]+)'`)

// Each data-action of a template has a function. A shipped script gives it,
// or the script block of a template does. A script can also build a control
// in its markup, for example the list of omn-go-db.js. Such a control
// is a use of its action, the same as a control of a template.
func TestEachDataActionHasAFunction(t *testing.T) {
	registered := map[string]bool{}
	used := map[string]bool{}
	// builtBy names the script that builds a control of each action.
	builtBy := map[string]string{}
	scripts, err := frontend.Static.ReadDir("html/js/OMN-Go")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range scripts {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "omn-go-") {
			continue
		}
		src, rerr := frontend.Static.ReadFile("html/js/OMN-Go/" + e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, m := range actionCallRe.FindAllStringSubmatch(string(src), -1) {
			registered[m[1]] = true
		}
		for _, m := range dataActionRe.FindAllStringSubmatch(string(src), -1) {
			used[m[1]] = true
			builtBy[m[1]] = e.Name()
		}
	}
	if len(registered) == 0 {
		t.Fatal("the scan found no OMN.action call, thus this test proves nothing")
	}

	templates, err := frontend.Templates.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range templates {
		src, rerr := frontend.Templates.ReadFile("templates/" + e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, m := range actionCallRe.FindAllStringSubmatch(string(src), -1) {
			registered[m[1]] = true
		}
	}
	for _, e := range templates {
		src, rerr := frontend.Templates.ReadFile("templates/" + e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, m := range dataActionRe.FindAllStringSubmatch(string(src), -1) {
			used[m[1]] = true
			if !registered[m[1]] {
				t.Errorf("templates/%s has data-action=%q, and no script calls "+
					"OMN.action('%s', ...). The control does nothing.", e.Name(), m[1], m[1])
			}
		}
	}
	if len(used) == 0 {
		t.Fatal("the scan found no data-action in the templates, thus this test proves nothing")
	}
	for name, file := range builtBy {
		if !registered[name] {
			t.Errorf("%s builds a control with data-action=%q, and no script calls "+
				"OMN.action('%s', ...). The control does nothing.", file, name, name)
		}
	}
	for name := range registered {
		if !used[name] {
			t.Errorf("a script calls OMN.action('%s', ...), and no template has a "+
				"control with that data-action. Remove the action.", name)
		}
	}
}

// No template holds an inline handler. This is also true for the markup
// that the script block of a template writes.
func TestTemplatesHoldNoInlineHandler(t *testing.T) {
	templates, err := frontend.Templates.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatal("no template was read, thus this test proves nothing")
	}
	for _, e := range templates {
		name := "templates/" + e.Name()
		src, rerr := frontend.Templates.ReadFile(name)
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, found := range inlineHandlerRe.FindAllString(string(src), -1) {
			t.Errorf("%s holds the inline handler %q. Give the control a "+
				"data-action, and call OMN.action in a script.", name, strings.TrimSpace(found))
		}
	}
}

// A template holds no style block. A style block of a template goes to each
// page that the template makes, and no other page can use it. Put the rules
// in a stylesheet of the page, and name it with a Stylesheet: line or a link.
func TestTemplatesHoldNoStyleBlock(t *testing.T) {
	templates, err := frontend.Templates.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range templates {
		name := "templates/" + e.Name()
		src, rerr := frontend.Templates.ReadFile(name)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if strings.Contains(strings.ToLower(string(src)), "<style") {
			t.Errorf("%s holds a style block. Move the rules to a stylesheet.", name)
		}
	}
}

// browserDialogRe finds a call of a dialog of the browser: alert(),
// confirm() or prompt(), also with "window." in front. omnAlert and the
// other names of omn-go-dialog.js do not match.
var browserDialogRe = regexp.MustCompile(`(^|[^\w.]|window\.)(alert|confirm|prompt)\s*\(`)

// The application shows each message and each question with the dialogs of
// omn-go-dialog.js: omnAlert, omnConfirm, omnPrompt and omnDialog. A dialog
// of the browser has the look of the browser and not the look of the
// application. On Android it is a native dialog of the WebView. The test
// reads each script of the application and each template. It skips a line
// that is a comment, and the vendored libraries.
func TestNoBrowserDialog(t *testing.T) {
	var files []string
	scripts, err := frontend.Static.ReadDir("html/js/OMN-Go")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range scripts {
		if strings.HasSuffix(e.Name(), ".js") && !strings.HasSuffix(e.Name(), ".min.js") {
			files = append(files, "html/js/OMN-Go/"+e.Name())
		}
	}
	templates, err := frontend.Templates.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range templates {
		files = append(files, "templates/"+e.Name())
	}
	if len(files) < 10 {
		t.Fatalf("only %d files were read, thus this test proves nothing", len(files))
	}
	for _, name := range files {
		var src []byte
		if strings.HasPrefix(name, "templates/") {
			src, err = frontend.Templates.ReadFile(name)
		} else {
			src, err = frontend.Static.ReadFile(name)
		}
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") ||
				strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "<!--") {
				continue
			}
			if browserDialogRe.MatchString(line) {
				t.Errorf("%s:%d calls a dialog of the browser: %s\n"+
					"Use omnAlert, omnConfirm, omnPrompt or omnDialog of omn-go-dialog.js.",
					name, i+1, trimmed)
			}
		}
	}
}
