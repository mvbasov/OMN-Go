package backend

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestEscapeHTML(t *testing.T) {
	tests := []struct{ in, want string }{
		{`plain`, `plain`},
		{`a & b`, `a &amp; b`},
		{`<script>`, `&lt;script&gt;`},
		{`say "hi"`, `say &quot;hi&quot;`},
		{`it's`, `it&#39;s`},
		// & must be escaped first or the others get double-escaped
		{`&lt;`, `&amp;lt;`},
	}
	for _, tt := range tests {
		if got := escapeHTML(tt.in); got != tt.want {
			t.Errorf("escapeHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEscapeJS(t *testing.T) {
	tests := []struct{ in, want string }{
		{`plain`, `plain`},
		{`back\slash`, `back\\slash`},
		{`single'quote`, `single\'quote`},
		{`double"quote`, `double\"quote`},
		{"new\nline", `new\nline`},
		{"carriage\rreturn", `carriage\rreturn`},
		// critical: no value may ever assemble a literal "</script>"
		{`</script>`, `\x3c/script\x3e`},
		{`a&b`, `a\x26b`},
		{"line\u2028sep", `line\u2028sep`},
		{"para\u2029sep", `para\u2029sep`},
	}
	for _, tt := range tests {
		if got := escapeJS(tt.in); got != tt.want {
			t.Errorf("escapeJS(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFill(t *testing.T) {
	got := fill("a %%X%% b %%Y%% c %%X%%", map[string]string{"X": "1", "Y": "2"})
	want := "a 1 b 2 c 1"
	if got != want {
		t.Errorf("fill = %q, want %q", got, want)
	}
	// Unknown placeholders are left alone (they indicate a template/render
	// mismatch and should be visible, not silently vanish).
	got = fill("keep %%UNKNOWN%%", map[string]string{"X": "1"})
	if got != "keep %%UNKNOWN%%" {
		t.Errorf("fill with unknown placeholder = %q", got)
	}
}

func TestRenderIndexPageEscaping(t *testing.T) {
	v := indexPageView{
		Title:       `My "Quoted" <Title> & Co`,
		PackageName: "net.basov.omngo",
		PageName:    `Weird'Page"Name`,
		PageExt:     ".md",
		IsMarkdown:  true,
		MetaTags:    []metaTagView{{Name: "author", Value: `A "quoted" <author>`}},
		Tags:        []string{`tag<1>`, "plain"},
		PreviewHTML: "<p>trusted <strong>html</strong></p>",
	}
	out := renderIndexPage(v)

	// No placeholder may survive rendering.
	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder left in output:\n%s", out)
	}
	// HTML contexts escaped.
	if !strings.Contains(out, "My &quot;Quoted&quot; &lt;Title&gt; &amp; Co") {
		t.Error("title not HTML-escaped in output")
	}
	if !strings.Contains(out, `content="A &quot;quoted&quot; &lt;author&gt;"`) {
		t.Error("meta tag value not HTML-escaped")
	}
	if !strings.Contains(out, "tag&lt;1&gt;") {
		t.Error("tag pill not HTML-escaped")
	}
	// The rendered view page must NOT carry a copy of its own source: the
	// editor textarea (and the old %%RAW_MD_HTML%% placeholder) is gone,
	// editing is a separate page. Guard against the doubled content
	// regressing.
	if strings.Contains(out, "<textarea id=\"editor\"") {
		t.Error("rendered view page still embeds an #editor textarea (doubled content)")
	}
	// Trusted preview HTML is spliced unescaped.
	if !strings.Contains(out, "<p>trusted <strong>html</strong></p>") {
		t.Error("preview HTML was escaped or lost")
	}
	// JS string contexts escaped.
	if !strings.Contains(out, `var PageName = 'Weird\'Page\"Name';`) {
		t.Error("PageName not JS-escaped in inline script")
	}
	// currentNote moved from an end-of-body script into the <head> page
	// variables block (declared with var, single-quoted like its siblings)
	// so classic note scripts that execute during body parsing can see it.
	if !strings.Contains(out, `var currentNote = 'Weird\'Page\"Name';`) {
		t.Error("currentNote not JS-escaped in inline script")
	}
	if !strings.Contains(out, "var IS_MARKDOWN = true;") {
		t.Error("IS_MARKDOWN script missing for markdown page")
	}
	// Runtime-vars marker must survive rendering so injectRuntimeVars can
	// find it later (this is the regression where an HTML-comment marker
	// was silently stripped).
	if !strings.Contains(out, runtimeVarsMarker) {
		t.Error("runtime vars marker missing from rendered page")
	}
	// Generator meta injected by compilePageWithBody is not part of this
	// view; here we only assert what we passed in came through.
}

// The user files must be LAST: the stylesheet after every stylesheet of
// the application, the script after every script of it. That order is the
// whole feature - it is what lets a user rule win and a user function
// replace an application function. The editor page must NOT load them, so
// a bad rule or a bad line can never keep the user out of the editor that
// repairs it.
func TestRenderIndexPageLoadsCustomAssetsLast(t *testing.T) {
	out := renderIndexPage(indexPageView{
		Title:       "T",
		PageName:    "T",
		PageExt:     ".md",
		IsMarkdown:  true,
		AssetPrefix: "/",
		PreviewHTML: "<p>x</p>",
	})

	customCSS := strings.Index(out, "css/omn-go-custom.css")
	if customCSS < 0 {
		t.Fatal("page does not load css/omn-go-custom.css")
	}
	for _, sheet := range []string{"css/omn-go-core.css", "css/highlight.default.min.css", "css/katex.min.css"} {
		if i := strings.Index(out, sheet); i < 0 || i > customCSS {
			t.Errorf("%s must load BEFORE css/omn-go-custom.css", sheet)
		}
	}

	customJS := strings.Index(out, "js/omn-go-custom.js")
	if customJS < 0 {
		t.Fatal("page does not load js/omn-go-custom.js")
	}
	for _, script := range []string{
		"js/omn-go-core.js", "js/omn-go-sse.js", "js/highlight.min.js",
		"js/katex.min.js", "js/auto-render.min.js",
	} {
		if i := strings.Index(out, script); i < 0 || i > customJS {
			t.Errorf("%s must load BEFORE js/omn-go-custom.js", script)
		}
	}

	editor := renderEditorPage(editorPageView{Title: "T", Name: "T", PageExt: ".md", ViewURL: "/T.html"})
	if strings.Contains(editor, "omn-go-custom") {
		t.Error("the editor page must not load the user CSS or the user script")
	}
}

func TestRenderEditorPage(t *testing.T) {
	out := renderEditorPage(editorPageView{
		Title:   `Weird'Page"Name`,
		Name:    `Weird'Page"Name`,
		PageExt: ".md",
		ViewURL: "/Weird'Page\"Name.html",
	})

	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder in editor page:\n%s", out)
	}
	// The source is fetched at runtime, never baked in.
	if strings.Contains(out, "OMN_EDIT_SOURCE") || strings.Contains(out, "textarea>Weird") {
		t.Error("editor page must not embed note source")
	}
	// The editor fetches from /api/note and loads its own script.
	if !strings.Contains(out, "/js/omn-go-editor.js") {
		t.Error("editor page does not load omn-go-editor.js")
	}
	// Name is JS-escaped in the OMN_EDIT_NAME string literal.
	if !strings.Contains(out, `var OMN_EDIT_NAME = 'Weird\'Page\"Name';`) {
		t.Error("OMN_EDIT_NAME not JS-escaped")
	}
	// Title is HTML-escaped where it appears in text.
	if !strings.Contains(out, "Weird&#39;Page&quot;Name") {
		t.Error("editor title not HTML-escaped")
	}
	// Runtime marker present so the theme is injected (no flash).
	if !strings.Contains(out, runtimeVarsMarker) {
		t.Error("editor page missing runtime-vars marker for theme injection")
	}
}

func TestRenderConfigPage(t *testing.T) {
	v := configPageView{
		ServerPort:    8080,
		Author:        "A & B",
		UseInternalEd: true,
		DesktopExtCmd: "subl",
		GitServers: []gitServerView{
			{Index: 0, Slot: 1, Active: true, Name: `srv "one"`, URL: "git@host:repo.git"},
			{Index: 1, Slot: 2, Active: false, Name: "srv two"},
		},
	}
	out := renderConfigPage(v)

	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder left in output:\n%s", out)
	}
	// Stored-XSS regression: attacker-ish values must arrive escaped.
	// The view holds no password since 26.09.7, thus the git server name
	// carries this check now. See TestConfigPageCarriesNoSecret.
	if !strings.Contains(out, "srv &quot;one&quot;") {
		t.Error("git server name not HTML-escaped")
	}
	if !strings.Contains(out, `value="A &amp; B"`) {
		t.Error("author not HTML-escaped")
	}
	// Exactly one card is the active radio.
	if strings.Count(out, `value="0" checked`) != 1 {
		t.Error("active git server slot 0 not marked checked exactly once")
	}
	if strings.Contains(out, `value="1" checked`) {
		t.Error("inactive slot wrongly marked checked")
	}
	// Both cards rendered, indices intact.
	for _, want := range []string{"git_name_0", "git_name_1", "Slot 1", "Slot 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in config page output", want)
		}
	}
	// Internal editor checkbox honored.
	if !strings.Contains(out, `name="use_internal_editor" value="true" checked`) {
		t.Error("use_internal_editor checkbox not checked")
	}
}

func TestRenderConfigPageAndroidToggles(t *testing.T) {
	// Off (zero value): neither Android checkbox is checked, but both
	// placeholders are still filled (no leftover %%...%%).
	off := renderConfigPage(configPageView{})
	if strings.Contains(off, "%%INTENT_URI_CHECKED%%") || strings.Contains(off, "%%TERMUX_INTENT_CHECKED%%") {
		t.Fatalf("Android toggle placeholder left unfilled:\n%s", off)
	}
	if strings.Contains(off, `name="enable_intent_uri" value="true" checked`) {
		t.Error("enable_intent_uri wrongly checked when EnableIntentURI is false")
	}
	if strings.Contains(off, `name="enable_termux_intent" value="true" checked`) {
		t.Error("enable_termux_intent wrongly checked when EnableTermuxIntent is false")
	}

	// On: both checkboxes render checked.
	on := renderConfigPage(configPageView{EnableIntentURI: true, EnableTermuxIntent: true})
	if !strings.Contains(on, `name="enable_intent_uri" value="true" checked`) {
		t.Error("enable_intent_uri checkbox not checked when EnableIntentURI is true")
	}
	if !strings.Contains(on, `name="enable_termux_intent" value="true" checked`) {
		t.Error("enable_termux_intent checkbox not checked when EnableTermuxIntent is true")
	}
}

func TestRenderExternalEditPage(t *testing.T) {
	v := externalEditView{
		Cmd:      "subl",
		FileName: `note "x".md`,
		// hostile ViewURL trying to break out of the JS string and the
		// onclick attribute at once
		ViewURL: `x');alert("pwn`,
	}
	out := renderExternalEditPage(v)

	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder left in output:\n%s", out)
	}
	if !strings.Contains(out, "note &quot;x&quot;.md") {
		t.Error("file name not HTML-escaped")
	}
	// The raw payload must not survive into the attribute.
	if strings.Contains(out, `x');alert("pwn`) {
		t.Error("hostile ViewURL not escaped in onclick attribute")
	}
	// JS-escaped then HTML-escaped: ' -> \' -> \&#39;
	if !strings.Contains(out, `x\&#39;)`) {
		t.Errorf("ViewURL escaping unexpected, got:\n%s", out)
	}
}

func TestInjectRuntimeVars(t *testing.T) {
	a := &App{}
	a.Config.UseInternalEd = true

	page := []byte("<head>" + runtimeVarsMarker + "</head>")
	out := string(a.injectRuntimeVars(page))

	if strings.Contains(out, runtimeVarsMarker) {
		t.Error("marker not replaced")
	}
	if !strings.Contains(out, `var APP_VERSION = "`+APP_VERSION+`";`) {
		t.Error("APP_VERSION not injected")
	}
	if !strings.Contains(out, "var USE_INTERNAL_ED = true;") {
		t.Error("USE_INTERNAL_ED not injected")
	}

	// A page without the marker passes through unchanged.
	plain := []byte("<head>no marker</head>")
	if got := string(a.injectRuntimeVars(plain)); got != string(plain) {
		t.Errorf("page without marker was modified: %q", got)
	}
}

// End-to-end guard: a page rendered through renderIndexPage carries the
// marker, and injectRuntimeVars finds it - the exact pair that broke when
// the marker was an HTML comment.
func TestRenderedPageAcceptsRuntimeVars(t *testing.T) {
	a := &App{}
	out := a.injectRuntimeVars([]byte(renderIndexPage(indexPageView{Title: "T", PageName: "T"})))
	if !strings.Contains(string(out), "var APP_VERSION") {
		t.Error("rendered index page did not accept runtime vars injection")
	}
}

// selectBlock returns the markup of the <select name="..."> element in html,
// so a test can assert about one dropdown without being perturbed by any
// other. The page carries more than one select (theme, android_fullscreen),
// which is why "count the selected attributes in the whole page" is not a
// safe assertion.
func selectBlock(t *testing.T, html, name string) string {
	t.Helper()
	i := strings.Index(html, `name="`+name+`"`)
	if i == -1 {
		t.Fatalf("no <select name=%q> in rendered page", name)
	}
	j := strings.Index(html[i:], "</select>")
	if j == -1 {
		t.Fatalf("unterminated <select name=%q> in rendered page", name)
	}
	return html[i : i+j]
}

func TestRenderConfigPageThemeSelection(t *testing.T) {
	cases := []struct {
		theme        string
		wantSelected string
	}{
		{"dark", `value="dark" selected`},
		{"light", `value="light" selected`},
		{"auto", `value="auto" selected`},
		// pre-theme configs (empty) and garbage both normalize to auto
		{"", `value="auto" selected`},
		{"purple", `value="auto" selected`},
	}
	for _, tc := range cases {
		out := renderConfigPage(configPageView{Theme: tc.theme})
		if !strings.Contains(out, tc.wantSelected) {
			t.Errorf("theme=%q: expected %q in output", tc.theme, tc.wantSelected)
		}
		// Exactly one option may be selected - within the theme select.
		if n := strings.Count(selectBlock(t, out, "theme"), " selected"); n != 1 {
			t.Errorf("theme=%q: %d theme options selected, want exactly 1", tc.theme, n)
		}
		if strings.Contains(out, "%%") {
			t.Fatalf("theme=%q: unfilled placeholder left in output", tc.theme)
		}
	}
}

func TestRenderConfigPageFullscreenSelection(t *testing.T) {
	cases := []struct {
		mode         string
		wantSelected string
	}{
		{"off", `value="off" selected`},
		{"fullscreen", `value="fullscreen" selected`},
		{"immersive", `value="immersive" selected`},
		// A config.json written before android_fullscreen existed carries
		// "" and MUST land on fullscreen, not off - that is what keeps an
		// upgraded install looking the way it always has. Garbage lands
		// there too.
		{"", `value="fullscreen" selected`},
		{"sideways", `value="fullscreen" selected`},
	}
	for _, tc := range cases {
		out := renderConfigPage(configPageView{AndroidFullscreen: tc.mode})
		if !strings.Contains(out, tc.wantSelected) {
			t.Errorf("fullscreen=%q: expected %q in output", tc.mode, tc.wantSelected)
		}
		if n := strings.Count(selectBlock(t, out, "android_fullscreen"), " selected"); n != 1 {
			t.Errorf("fullscreen=%q: %d fullscreen options selected, want exactly 1", tc.mode, n)
		}
		if strings.Contains(out, "%%") {
			t.Fatalf("fullscreen=%q: unfilled placeholder left in output", tc.mode)
		}
	}
}

func TestNormalizeFullscreen(t *testing.T) {
	cases := map[string]string{
		"off":        FullscreenOff,
		"fullscreen": FullscreenOn,
		"immersive":  FullscreenImmersive,
		// Empty is the important one: it is what every config.json written
		// before this field existed contains.
		"":          FullscreenOn,
		"sideways":  FullscreenOn,
		"OFF":       FullscreenOn, // case-sensitive whitelist, as normalizeTheme
		"Immersive": FullscreenOn,
	}
	for in, want := range cases {
		if got := normalizeFullscreen(in); got != want {
			t.Errorf("normalizeFullscreen(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInjectRuntimeVarsTheme(t *testing.T) {
	page := []byte("<head>" + runtimeVarsMarker + "</head>")

	// Explicit theme delivered verbatim, and applied to <html> from the
	// injected head script (before first paint).
	a := &App{}
	a.Config.Theme = ThemeDark
	out := string(a.injectRuntimeVars(page))
	if !strings.Contains(out, `var OMN_THEME = "dark";`) {
		t.Error("dark theme not injected")
	}
	if !strings.Contains(out, `document.documentElement.setAttribute('data-theme', OMN_THEME);`) {
		t.Error("data-theme application script missing")
	}

	// Unset / invalid themes normalize to auto at the injection point too
	// (belt and braces on top of loadConfig's normalization).
	for _, raw := range []string{"", "purple"} {
		b := &App{}
		b.Config.Theme = raw
		got := string(b.injectRuntimeVars(page))
		if !strings.Contains(got, `var OMN_THEME = "auto";`) {
			t.Errorf("theme=%q: expected auto in injection, got:\n%s", raw, got)
		}
	}
}

func TestNormalizeTheme(t *testing.T) {
	cases := map[string]string{
		"auto":   ThemeAuto,
		"light":  ThemeLight,
		"dark":   ThemeDark,
		"":       ThemeAuto,
		"purple": ThemeAuto,
		"DARK":   ThemeAuto, // case-sensitive whitelist by design
	}
	for in, want := range cases {
		if got := normalizeTheme(in); got != want {
			t.Errorf("normalizeTheme(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------
// The compat script
//
// omn-go-compat.js tells a person with an old WebView why the page is
// blank. Every other script uses async/await and arrow functions, and a
// parser that cannot read those drops the WHOLE file. A notice written in
// the style of its neighbors would be the one thing that does not run when
// it is needed.
//
// The notice was an inline block of index.html until 26.08.73. It moved to
// its own file, because the inline copy went into the compiled page of
// every note. A <script src> element is its own parse unit, thus a
// SyntaxError in omn-go-core.js cannot stop it. That holds while two rules
// hold, and this test is the whole guarantee of both:
//
//   - the file itself is ES5, thus the old parser accepts it.
//   - index.html loads it FIRST, thus nothing throws before it runs.
//
// See the banner of omn-go-compat.js for the version number and where that
// number comes from.
// ---------------------------------------------------------------------

// compatCommentRe removes a block comment and a line comment, so that the
// prose of the banner, which names "async/await" and "arrow functions",
// cannot look like code to the scan below.
var compatCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`)

// compatBannedES6 are tokens that an ES5 parser rejects. Each one is
// written so that it cannot match ordinary prose.
var compatBannedES6 = []string{"=>", "`", "const ", "let ", "async ", "await ", "class ", "?.", "??", "..."}

func TestCompatScriptIsFirstAndES5(t *testing.T) {
	// 1. index.html loads it before every other script, and before the
	//    stylesheet links, which would delay it for no reason.
	scripts := regexp.MustCompile(`<script[^>]*>`).FindAllString(indexPageTmpl, -1)
	if len(scripts) == 0 {
		t.Fatal("index.html loads no script at all")
	}
	if !strings.Contains(scripts[0], "js/omn-go-compat.js") {
		t.Errorf("the first script of index.html is %q, want omn-go-compat.js. "+
			"A script above it that a WebView cannot parse throws before the "+
			"notice runs, and the reader sees a blank page with no reason.",
			scripts[0])
	}
	if strings.Contains(scripts[0], "defer") || strings.Contains(scripts[0], " async") {
		t.Errorf("the compat script carries defer or async: %q. Either one "+
			"delays it past the modern scripts, which is the order this test "+
			"exists to protect.", scripts[0])
	}
	at := strings.Index(indexPageTmpl, "omn-go-compat.js")
	if css := strings.Index(indexPageTmpl, `<link rel="stylesheet"`); css >= 0 && at > css {
		t.Error("the compat script is after the stylesheet link, which delays it for no reason")
	}

	raw, err := staticFS.ReadFile("frontend/html/js/omn-go-compat.js")
	if err != nil {
		t.Fatalf("omn-go-compat.js is not embedded: %v", err)
	}
	code := compatCommentRe.ReplaceAllString(string(raw), "")

	// 2. The file parses on the oldest WebView this build supports.
	for _, es6 := range compatBannedES6 {
		if strings.Contains(code, es6) {
			t.Errorf("omn-go-compat.js uses %q, which an old WebView cannot parse - "+
				"it must stay ES5, or it is the one script that fails when it is needed", es6)
		}
	}

	// 3. The number it reports has to be a number, and one this application
	//    can justify: 85 is String.replaceAll, the highest requirement the
	//    frontend really has.
	if !strings.Contains(code, "var MIN = 85;") {
		t.Error("the notice no longer names 85 as the minimum; if that changed on purpose, " +
			"change it here too and say why in the banner")
	}

	// 4. Every byte stays ASCII. The server sends this file as
	//    application/javascript with no charset, thus a literal multi-byte
	//    character can arrive misdecoded.
	for i, c := range raw {
		if c > 127 {
			t.Errorf("omn-go-compat.js byte %d is not ASCII. Write a \\uXXXX "+
				"escape instead of the character.", i)
			break
		}
	}
}

// TestCompiledPageShellStaysSmall exists because the shell of index.html is
// copied into html/<name>.html for EVERY note. 26.08.73 moved 3.3 KB of
// inline script and 133 bytes of inline style out of it for that reason.
// A new inline block here costs the same bytes again, on disk and in every
// git sync, multiplied by the note count.
func TestCompiledPageShellStaysSmall(t *testing.T) {
	const maxShellBytes = 5000
	if n := len(indexPageTmpl); n > maxShellBytes {
		t.Errorf("index.html is %d bytes, over the %d-byte guard. Put the new "+
			"code in an asset under frontend/html/ and load it with a src, or "+
			"raise this number on purpose.", n, maxShellBytes)
	}
}

// ---------------------------------------------------------------------
// The clipboard and the documented API
// ---------------------------------------------------------------------

// TestClipboardHasOneAuthority exists because a second clipboard path
// gives a second chance to get the Android WebView wrong. That is what
// happened. Before 26.08.74 the Status page had its own textarea and its
// own execCommand call. omnGoCopyText took the Clipboard API and returned
// before its own second way. The copy on the Status page worked on
// Android 6. The copy in the metadata panel did not.
//
// Only omn-go-core.js can call execCommand('copy'). That file holds
// omnGoCopyText, which each other caller uses. It also holds
// copyQuickNote, which stays direct. The text of copyQuickNote is already
// in a textarea, and the focus must stay there for the typing that
// follows.
func TestClipboardHasOneAuthority(t *testing.T) {
	const authority = "frontend/html/js/omn-go-core.js"
	for _, tree := range []struct {
		name string
		fs   fs.FS
		root string
	}{
		{"staticFS", staticFS, "frontend/html"},
		{"templatesFS", templatesFS, "frontend/templates"},
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
	var all strings.Builder
	for _, f := range []string{
		"frontend/html/js/omn-go-core.js",
		"frontend/html/js/omn-go-sse.js",
	} {
		src, err := staticFS.ReadFile(f)
		if err != nil {
			t.Fatalf("%s is not embedded: %v", f, err)
		}
		all.Write(src)
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
