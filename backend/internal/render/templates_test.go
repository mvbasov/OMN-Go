package render

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/config"
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
		if got := EscapeHTML(tt.in); got != tt.want {
			t.Errorf("EscapeHTML(%q) = %q, want %q", tt.in, got, tt.want)
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
		if got := EscapeJS(tt.in); got != tt.want {
			t.Errorf("EscapeJS(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFill(t *testing.T) {
	got := Fill("a %%X%% b %%Y%% c %%X%%", map[string]string{"X": "1", "Y": "2"})
	want := "a 1 b 2 c 1"
	if got != want {
		t.Errorf("fill = %q, want %q", got, want)
	}
	// Unknown placeholders are left alone (they indicate a template/render
	// mismatch and should be visible, not silently vanish).
	got = Fill("keep %%UNKNOWN%%", map[string]string{"X": "1"})
	if got != "keep %%UNKNOWN%%" {
		t.Errorf("fill with unknown placeholder = %q", got)
	}
}

func TestRenderIndexPageEscaping(t *testing.T) {
	v := IndexPageView{
		Title:       `My "Quoted" <Title> & Co`,
		PackageName: "net.basov.omngo",
		PageName:    `Weird'Page"Name`,
		PageExt:     ".md",
		IsMarkdown:  true,
		MetaTags:    []MetaTagView{{Name: "author", Value: `A "quoted" <author>`}},
		Tags:        []string{`tag<1>`, "plain"},
		PreviewHTML: "<p>trusted <strong>html</strong></p>",
	}
	out := RenderIndexPage(v)

	// No placeholder may stay in the page.
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
	// The rendered view page must NOT carry a copy of its own source. The
	// editor textarea is gone, and so is the old %%RAW_MD_HTML%%
	// placeholder. An edit is a separate page. Guard against a return of
	// the doubled content.
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
	// currentNote moved from an end-of-body script into the page variables
	// block of the <head>. It is declared with var and single-quoted, like
	// its siblings. A classic note script that runs during the body parse
	// can thus see it.
	if !strings.Contains(out, `var currentNote = 'Weird\'Page\"Name';`) {
		t.Error("currentNote not JS-escaped in inline script")
	}
	if !strings.Contains(out, "var IS_MARKDOWN = true;") {
		t.Error("IS_MARKDOWN script missing for markdown page")
	}
	// The page must keep the marker of the runtime values, thus
	// InjectRuntimeVars can find it later. An older marker was an HTML
	// comment, and the render removed it with no message.
	if !strings.Contains(out, RuntimeVarsMarker) {
		t.Error("runtime vars marker missing from rendered page")
	}
	// The Generator meta that CompilePageWithBody injects is not part of
	// this view. Here we assert only that what we passed in came through.
}

// The user files must be LAST. The stylesheet loads after each stylesheet
// of the application, and the script after each script of it. That order
// is the whole feature. It is what lets a user rule win, and what lets a
// user function replace an application function. The editor page must NOT
// load them. A bad rule or a bad line can then never keep the user out of
// the editor that repairs it.
func TestRenderIndexPageLoadsCustomAssetsLast(t *testing.T) {
	out := RenderIndexPage(IndexPageView{
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
	for _, sheet := range []string{"css/OMN-Go/omn-go-core.css", "css/OMN-Go/highlight.default.min.css", "css/OMN-Go/katex.min.css"} {
		if i := strings.Index(out, sheet); i < 0 || i > customCSS {
			t.Errorf("%s must load BEFORE css/omn-go-custom.css", sheet)
		}
	}

	customJS := strings.Index(out, "js/omn-go-custom.js")
	if customJS < 0 {
		t.Fatal("page does not load js/omn-go-custom.js")
	}
	for _, script := range []string{
		"js/OMN-Go/omn-go-console.js", "js/OMN-Go/omn-go-core.js",
		"js/OMN-Go/omn-go-highlight.js", "js/OMN-Go/omn-go-nav.js",
		"js/OMN-Go/omn-go-share.js", "js/OMN-Go/omn-go-api.js",
		"js/OMN-Go/highlight.min.js", "js/OMN-Go/katex.min.js", "js/OMN-Go/auto-render.min.js",
	} {
		if i := strings.Index(out, script); i < 0 || i > customJS {
			t.Errorf("%s must load BEFORE js/omn-go-custom.js", script)
		}
	}

	editor := RenderEditorPage(EditorPageView{Title: "T", Name: "T", PageExt: ".md", ViewURL: "/T.html"})
	if strings.Contains(editor, "omn-go-custom") {
		t.Error("the editor page must not load the user CSS or the user script")
	}
}

// A Stylesheet: line of the header block loads a page stylesheet. The link
// is in the head and before omn-go-custom.css, thus a rule of the user still
// wins. The link uses the asset prefix, thus a note at any depth and an
// exported page find the file. The line makes no meta tag.
func TestStylesheetHeaderLoadsAPageStylesheet(t *testing.T) {
	rd := Renderer{Generator: "g"}
	page := string(rd.CompilePageWithBody("Status", PageHeader("Status", "System", "css/OMN-Go/omn-go-status.css"), "<p>x</p>"))
	link := `<link rel="stylesheet" href="/css/OMN-Go/omn-go-status.css">`
	at, custom, head := strings.Index(page, link), strings.Index(page, "css/omn-go-custom.css"), strings.Index(page, "</head>")
	if at < 0 || at > custom || custom > head {
		t.Errorf("the page stylesheet is not in the head before omn-go-custom.css:\n%s", page[:head])
	}
	if strings.Contains(page, `name="stylesheet"`) {
		t.Error("the Stylesheet: line made a meta tag")
	}

	note := string(rd.CompilePage("dir/Note", []byte("Title: N\nStylesheet: /css/mine.css\n\nbody")))
	if !strings.Contains(note, `<link rel="stylesheet" href="../css/mine.css">`) {
		t.Error("a note in a directory does not reach its stylesheet with a relative path")
	}
	plain := string(rd.CompilePage("Plain", []byte("Title: P\n\nbody")))
	if strings.Count(plain, `rel="stylesheet"`) != strings.Count(string(rd.CompilePage("Plain", []byte("body"))), `rel="stylesheet"`) {
		t.Error("a note with no Stylesheet: line got a page stylesheet")
	}
}

func TestRenderEditorPage(t *testing.T) {
	out := RenderEditorPage(EditorPageView{
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
	if !strings.Contains(out, "/js/OMN-Go/omn-go-editor.js") {
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
	if !strings.Contains(out, RuntimeVarsMarker) {
		t.Error("editor page missing runtime-vars marker for theme injection")
	}
}

func TestRenderExternalEditPage(t *testing.T) {
	v := ExternalEditView{
		Cmd:      "subl",
		FileName: `note "x".md`,
		// A hostile ViewURL that tries to leave the data-arg attribute and
		// to start an attribute of its own.
		ViewURL: `x" onclick="alert('pwn')`,
	}
	out := RenderExternalEditPage(v)

	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder left in output:\n%s", out)
	}
	if !strings.Contains(out, "note &quot;x&quot;.md") {
		t.Error("file name not HTML-escaped")
	}
	// The raw payload must not survive into the attribute.
	if strings.Contains(out, `x" onclick=`) {
		t.Error("hostile ViewURL not escaped in the data-arg attribute")
	}
	if !strings.Contains(out, `data-arg="/x&quot; onclick=&quot;alert(&#39;pwn&#39;)"`) {
		t.Errorf("ViewURL escaping unexpected, got:\n%s", out)
	}
	if strings.Contains(out, `onclick="`) {
		t.Errorf("the page holds an inline handler:\n%s", out)
	}
}

// templates.go holds only the helpers of each page. The view and the render
// function of a page go in the file of that page.
func TestTemplatesGoHoldsOnlyTheHelpers(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "templates.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			got = append(got, d.Name.Name)
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					got = append(got, s.Name.Name)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						got = append(got, n.Name)
					}
				}
			}
		}
	}
	slices.Sort(got)
	want := []string{"EscapeHTML", "EscapeJS", "Fill", "LoadTemplate"}
	if !slices.Equal(got, want) {
		t.Errorf("templates.go declares %v, want %v. Put the view and the "+
			"render function of a page in the file of that page.", got, want)
	}
}

func TestInjectRuntimeVars(t *testing.T) {
	a := &testApp{}
	a.config.Update(func(c *config.Config) { c.UseInternalEd = true })

	page := []byte("<head>" + RuntimeVarsMarker + "</head>")
	out := string(a.injectRuntimeVars(page))

	if strings.Contains(out, RuntimeVarsMarker) {
		t.Error("marker not replaced")
	}
	if !strings.Contains(out, `var APP_VERSION = "`+version+`";`) {
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

// End-to-end guard. A page rendered through RenderIndexPage carries the
// marker, and injectRuntimeVars finds it. That is the exact pair that broke
// when the marker was an HTML comment.
func TestRenderedPageAcceptsRuntimeVars(t *testing.T) {
	a := &testApp{}
	out := a.injectRuntimeVars([]byte(RenderIndexPage(IndexPageView{Title: "T", PageName: "T"})))
	if !strings.Contains(string(out), "var APP_VERSION") {
		t.Error("rendered index page did not accept runtime vars injection")
	}
}

func TestInjectRuntimeVarsTheme(t *testing.T) {
	page := []byte("<head>" + RuntimeVarsMarker + "</head>")

	// Explicit theme delivered verbatim, and applied to <html> from the
	// injected head script (before first paint).
	a := &testApp{}
	a.config.Update(func(c *config.Config) { c.Theme = config.ThemeDark })
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
		b := &testApp{}
		b.config.Update(func(c *config.Config) { c.Theme = raw })
		got := string(b.injectRuntimeVars(page))
		if !strings.Contains(got, `var OMN_THEME = "auto";`) {
			t.Errorf("theme=%q: expected auto in injection, got:\n%s", raw, got)
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
// The notice is a file of its own, and not an inline block of index.html.
// An inline copy would go into the compiled page of every note. A <script
// src> element is its own parse unit, thus a SyntaxError in omn-go-core.js
// cannot stop it. That holds while two rules
// hold, and this test is the whole guarantee of both:
//
//   - the file itself is ES5, thus the old parser accepts it.
//   - index.html loads it FIRST, thus nothing throws before it runs.
//
// See the banner of omn-go-compat.js for the version number and where that
// number comes from.
// ---------------------------------------------------------------------

// compatCommentRe removes a block comment and a line comment. The prose of
// the banner names "async/await" and "arrow functions". It thus cannot look
// like code to the scan below.
var compatCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`)

// compatBannedES6 are tokens that an ES5 parser rejects. Each one is
// written so that it cannot match ordinary prose.
var compatBannedES6 = []string{"=>", "`", "const ", "let ", "async ", "await ", "class ", "?.", "??", "..."}

func TestCompatScriptIsFirstAndES5(t *testing.T) {
	// 1. index.html loads it before every other script, and before the
	//    stylesheet links, which would delay it for no reason.
	scripts := regexp.MustCompile(`<script[^>]*>`).FindAllString(IndexPageTmpl, -1)
	if len(scripts) == 0 {
		t.Fatal("index.html loads no script at all")
	}
	if !strings.Contains(scripts[0], "js/OMN-Go/omn-go-compat.js") {
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
	at := strings.Index(IndexPageTmpl, "omn-go-compat.js")
	if css := strings.Index(IndexPageTmpl, `<link rel="stylesheet"`); css >= 0 && at > css {
		t.Error("the compat script is after the stylesheet link, which delays it for no reason")
	}

	raw, err := frontend.Static.ReadFile("html/js/OMN-Go/omn-go-compat.js")
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
		t.Error("the notice does not name 85 as the minimum. When you change it on " +
			"purpose, change it here too, and give the reason in the banner.")
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
// copied into html/<name>.html for EVERY note. A new inline block here
// costs the same bytes again, on disk and in every
// git sync, multiplied by the note count.
func TestCompiledPageShellStaysSmall(t *testing.T) {
	const maxShellBytes = 5000
	if n := len(IndexPageTmpl); n > maxShellBytes {
		t.Errorf("index.html is %d bytes, over the %d-byte guard. Put the new "+
			"code in an asset under frontend/html/ and load it with a src, or "+
			"raise this number on purpose.", n, maxShellBytes)
	}
}

// The one value that WriteHTMLHeader writes. It carries the charset, and
// it keeps the prefix that pageCacheWriter reads.
func TestPageContentTypeCarriesTheCharset(t *testing.T) {
	if HTMLContentType != "text/html; charset=utf-8" {
		t.Fatalf("the page content type is %q", HTMLContentType)
	}
	// The value must start with the prefix that pageCacheWriter reads.
	if !strings.HasPrefix(HTMLContentType, "text/html") {
		t.Error("pageCacheWriter reads the prefix text/html to make a page no-store")
	}
}
