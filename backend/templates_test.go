package backend

import (
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
		AdminPassword: `p"w<d`,
		GuestPassword: "guest",
		Author:        "A & B",
		UseInternalEd: true,
		DesktopExtCmd: "subl",
		GitServers: []gitServerView{
			{Index: 0, Slot: 1, Active: true, Name: `srv "one"`, URL: "git@host:repo.git", SSHKeyData: "-----KEY-----", Password: "s3cret"},
			{Index: 1, Slot: 2, Active: false, Name: "srv two"},
		},
	}
	out := renderConfigPage(v)

	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder left in output:\n%s", out)
	}
	// Stored-XSS regression: attacker-ish values must arrive escaped.
	if !strings.Contains(out, `value="p&quot;w&lt;d"`) {
		t.Error("admin password not HTML-escaped in attribute")
	}
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
		// Exactly one option may be selected.
		if n := strings.Count(out, " selected"); n != 1 {
			t.Errorf("theme=%q: %d options selected, want exactly 1", tc.theme, n)
		}
		if strings.Contains(out, "%%") {
			t.Fatalf("theme=%q: unfilled placeholder left in output", tc.theme)
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
