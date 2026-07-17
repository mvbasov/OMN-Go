package backend

import (
	"strings"
	"testing"
)

func TestRewriteInternalLink(t *testing.T) {
	a := &App{}
	tests := []struct{ in, want string }{
		// bare page names get .html
		{"Page", "Page.html"},
		{"./Page", "./Page.html"},
		{"../Page", "../Page.html"},
		{"/Page", "/Page.html"},
		{"dir/Page", "dir/Page.html"},
		// .md becomes .html
		{"Page.md", "Page.html"},
		{"dir/Page.md", "dir/Page.html"},
		// concrete extensions untouched
		{"img.png", "img.png"},
		{"style.css", "style.css"},
		{"Page.html", "Page.html"},
		// external / special schemes untouched
		{"http://example.com/x", "http://example.com/x"},
		{"https://example.com/x", "https://example.com/x"},
		{"//cdn.example.com/x", "//cdn.example.com/x"},
		{"mailto:a@b.c", "mailto:a@b.c"},
		{"tel:+123", "tel:+123"},
		{"javascript:void(0)", "javascript:void(0)"},
		{"data:text/plain,x", "data:text/plain,x"},
		// Android intent URIs pass through byte-identical - the bare
		// "intent:#Intent;...;end" form must NOT be split at its "#" and
		// have ".html" appended to the "intent:" segment (would produce the
		// broken "intent:.html#Intent;..."), and the "intent://" form is
		// likewise left alone. See MainActivity.shouldOverrideUrlLoading.
		{"intent:#Intent;action=android.settings.WIRELESS_SETTINGS;end;", "intent:#Intent;action=android.settings.WIRELESS_SETTINGS;end;"},
		{"intent:#Intent;action=android.settings.DEVICE_INFO_SETTINGS;end;", "intent:#Intent;action=android.settings.DEVICE_INFO_SETTINGS;end;"},
		{"intent://scan/#Intent;scheme=zxing;package=com.google.zxing.client.android;end", "intent://scan/#Intent;scheme=zxing;package=com.google.zxing.client.android;end"},
		// pure anchor / query untouched
		{"#section", "#section"},
		{"?x=1", "?x=1"},
		// anchor/query suffixes preserved AFTER the .html rewrite
		{"Page#section", "Page.html#section"},
		{"Page?x=1", "Page.html?x=1"},
		{"Page.md#section", "Page.html#section"},
		// directory-only references untouched
		{".", "."},
		{"..", ".."},
		{"dir/", "dir/"},
		// empty
		{"", ""},
	}
	for _, tt := range tests {
		if got := a.rewriteInternalLink(tt.in); got != tt.want {
			t.Errorf("rewriteInternalLink(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderMarkdownToHTMLMathProtection(t *testing.T) {
	a := &App{}
	// Underscores inside math would be corrupted into <em> by the markdown
	// parser if the $$...$$ / $...$ protection didn't hold.
	md := "Before\n\n$$x_1 + y_2$$\n\nand inline $a_b$ end"
	out := a.renderMarkdownToHTML([]byte(md))

	if !strings.Contains(out, "$$x_1 + y_2$$") {
		t.Errorf("block math not preserved verbatim, got:\n%s", out)
	}
	if !strings.Contains(out, "$a_b$") {
		t.Errorf("inline math not preserved verbatim, got:\n%s", out)
	}
	if strings.Contains(out, "x<em>") || strings.Contains(out, "a<em>") {
		t.Error("markdown emphasis corrupted math content")
	}
}

// TestRenderMarkdownRawNoPlaceholderLeak guards the bug where documentation
// pages that mention "<script>" inside inline code and inside fenced code
// blocks (like Database.md) leaked "OMN_RAW_n_END" placeholder tokens into
// the rendered HTML. The old five-sequential-pass shielding matched the
// literal "<script>" in a code span and paired it with a real "</script>"
// in a later fenced example, producing placeholders whose stored text
// contained other placeholders; restoring them in Go's randomized
// map-iteration order left some unrestored (so it surfaced on some devices
// only). The combined single-pass scan must never leak, deterministically.
func TestRenderMarkdownRawNoPlaceholderLeak(t *testing.T) {
	a := &App{}
	md := strings.Join([]string{
		"A note's own `<script>` block can use it.",
		"",
		"```html",
		"<script>",
		"const t = `total ${a + b}`;",
		"console.log('$5 and $10');",
		"</script>",
		"```",
		"",
		"Then more `<script>` prose and a second example:",
		"",
		"```js",
		"const x = `${y}`;",
		"</script>", // a stray close inside a fence, as docs sometimes have
		"```",
		"",
		"<script>",
		"document.title = `x ${1 + 2}`;",
		"</script>",
	}, "\n")

	// Render repeatedly: the historical bug was nondeterministic (map order).
	first := a.renderMarkdownToHTML([]byte(md))
	for i := 0; i < 40; i++ {
		out := a.renderMarkdownToHTML([]byte(md))
		if strings.Contains(out, "OMN_RAW_") {
			t.Fatalf("raw placeholder leaked into output:\n%s", out)
		}
		if strings.Contains(out, "OMN_MATH_") {
			t.Fatalf("math placeholder leaked into output:\n%s", out)
		}
		if out != first {
			t.Fatalf("rendering is not deterministic across runs")
		}
	}
	// The fenced examples must survive as code, and a real top-level <script>
	// (with '$' template literals) must pass through unescaped.
	if !strings.Contains(first, "<pre><code") {
		t.Error("fenced code block was not rendered as a code block")
	}
	if !strings.Contains(first, "document.title = `x ${1 + 2}`;") {
		t.Error("real top-level <script> with template literal was not preserved")
	}
}

func TestRenderMarkdownToHTMLLinkRewrite(t *testing.T) {
	a := &App{}
	out := a.renderMarkdownToHTML([]byte("[a](Other) [b](Other.md) [c](img.png) [d](https://x.y/z)"))

	for _, want := range []string{`href="Other.html"`, `href="img.png"`, `href="https://x.y/z"`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %s in rendered output:\n%s", want, out)
		}
	}
	if strings.Contains(out, `href="Other.md"`) {
		t.Error(".md link not rewritten to .html")
	}
}

// A markdown-authored Android intent link must reach the WebView unmangled:
// no ".html" appended, no split at the "#", href byte-identical to source.
func TestRenderMarkdownToHTMLIntentLinkUntouched(t *testing.T) {
	a := &App{}
	const intentHref = "intent:#Intent;action=android.settings.WIRELESS_SETTINGS;end;"
	out := a.renderMarkdownToHTML([]byte("[Wi-Fi](" + intentHref + ")"))

	if !strings.Contains(out, `href="`+intentHref+`"`) {
		t.Errorf("intent link was rewritten; expected href=%q in:\n%s", intentHref, out)
	}
	if strings.Contains(out, "intent:.html") {
		t.Errorf("intent link mangled to intent:.html in:\n%s", out)
	}
}

func TestCompilePageWithBodyHeaders(t *testing.T) {
	a := &App{}
	md := "Title: My \"Quoted\" Page\nTags: alpha, beta\nAuthor: Someone\n\n# Heading\n\nBody **bold** text."
	out := string(a.compilePage("TestPage", []byte(md)))

	if strings.Contains(out, "%%") {
		t.Fatalf("unfilled placeholder in compiled page:\n%s", out)
	}
	// Title extracted from header and HTML-escaped.
	if !strings.Contains(out, "My &quot;Quoted&quot; Page") {
		t.Error("title from header not used/escaped")
	}
	// Header lines become meta tags.
	if !strings.Contains(out, `<meta name="author" content="Someone"`) {
		t.Error("author header not emitted as meta tag")
	}
	// Generator meta always appended.
	if !strings.Contains(out, `content="OMN-Go `+APP_VERSION+`"`) {
		t.Error("generator meta tag missing")
	}
	// Tag pills for both tags.
	for _, tag := range []string{"alpha", "beta"} {
		if !strings.Contains(out, `Tags.html#`+tag) {
			t.Errorf("tag pill for %q missing", tag)
		}
	}
	// Markdown body rendered (header block excluded from body).
	if !strings.Contains(out, "<strong>bold</strong>") {
		t.Error("markdown body not rendered")
	}
	if strings.Contains(out, `<meta name="title" content="My`) == false {
		t.Error("title header not emitted as meta tag")
	}
	// Bare page: markdown flag set.
	if !strings.Contains(out, "var IS_MARKDOWN = true;") {
		t.Error("IS_MARKDOWN missing for markdown page")
	}
	// The rendered page must NOT carry its own source in an editor
	// textarea (editing is a separate page); only the rendered body.
	if strings.Contains(out, "<textarea id=\"editor\"") {
		t.Error("compiled page still embeds an #editor textarea (doubled content)")
	}
	if !strings.Contains(out, "<strong>bold</strong>") {
		t.Error("rendered body missing")
	}
}

// TestModalsInjectedAtServeTime pins Phase 5c: the server-only modals are
// NOT baked into the cached/exported page (which carries only the empty
// slot), and injectRuntimeVars splices them in when the backend serves it.
func TestModalsInjectedAtServeTime(t *testing.T) {
	a := &App{}

	compiled := string(a.compilePage("Welcome", []byte("Title: W\n\nBody")))
	if !strings.Contains(compiled, `<div id="omn-go-modals-slot"></div>`) {
		t.Fatalf("compiled/cached page missing the modals slot marker:\n%s", compiled)
	}
	for _, modal := range []string{`id="loginOverlay"`, `id="quickPanel"`, `id="conflict-modal"`} {
		if strings.Contains(compiled, modal) {
			t.Errorf("modal %s was baked into the cached page (should be serve-time only)", modal)
		}
	}

	served := string(a.injectRuntimeVars([]byte(compiled)))
	if strings.Contains(served, `<div id="omn-go-modals-slot"></div>`) {
		t.Error("modals slot was not replaced at serve time")
	}
	for _, want := range []string{`id="loginOverlay"`, `id="quickPanel"`, `id="bmPanel"`, `id="commitModal"`, `id="conflict-modal"`} {
		if !strings.Contains(served, want) {
			t.Errorf("served page missing injected modal %s", want)
		}
	}
}

func TestRelPrefix(t *testing.T) {
	cases := map[string]string{
		"Welcome":                     "",
		"QuickNotes":                  "",
		"local/Note":                  "../",
		"AI/GeminiSvgComponentEditor": "../",
		"a/b/c":                       "../../",
	}
	for name, want := range cases {
		if got := relPrefix(name); got != want {
			t.Errorf("relPrefix(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestCompilePageAssetPrefix pins Phase 5b: a cached markdown page gets
// depth-relative chrome-asset paths (so it loads when opened directly from
// disk as well as over HTTP), while a dynamic custom-body page keeps
// absolute paths (it is only ever served online, at a URL whose depth does
// not track the page name).
func TestCompilePageAssetPrefix(t *testing.T) {
	a := &App{}

	// Root markdown page -> bare relative paths, no leading slash.
	root := string(a.compilePage("Welcome", []byte("Title: W\n\nBody")))
	for _, want := range []string{`href="css/omn-go-core.css"`, `src="js/omn-go-core.js"`, `href="Welcome.html"`} {
		if !strings.Contains(root, want) {
			t.Errorf("root page missing %q", want)
		}
	}
	if strings.Contains(root, `href="/css/`) || strings.Contains(root, `src="/js/`) {
		t.Errorf("root page still carries absolute asset paths:\n%s", root)
	}

	// Two-deep markdown page -> "../../" prefix.
	nested := string(a.compilePage("a/b/Note", []byte("Title: N\n\nBody")))
	for _, want := range []string{`href="../../css/omn-go-core.css"`, `src="../../js/omn-go-core.js"`, `href="../../Welcome.html"`} {
		if !strings.Contains(nested, want) {
			t.Errorf("nested page missing %q", want)
		}
	}

	// Dynamic custom-body page -> absolute paths.
	dyn := string(a.compilePageWithBody("Config", []byte("Title: Config\n\n"), "<p>dashboard</p>"))
	for _, want := range []string{`href="/css/omn-go-core.css"`, `src="/js/omn-go-core.js"`, `href="/Welcome.html"`} {
		if !strings.Contains(dyn, want) {
			t.Errorf("custom-body page missing absolute %q:\n%s", want, dyn)
		}
	}
}

func TestCompilePageWithBodyCustomBody(t *testing.T) {
	a := &App{}
	raw := []byte("console.log('x');")
	// A non-markdown asset rendered with a custom body (e.g. the Config
	// dashboard / external-edit wait page path): IS_MARKDOWN off, PAGE_EXT
	// derived from the name, custom body used verbatim as the content.
	out := string(a.compilePageWithBody("app.js", raw, "<pre>console.log('x');</pre>"))

	if strings.Contains(out, "var IS_MARKDOWN = true;") {
		t.Error("IS_MARKDOWN set for a .js custom-body view")
	}
	if !strings.Contains(out, "var PAGE_EXT = '.js';") {
		t.Error("PAGE_EXT not derived from filename")
	}
	if !strings.Contains(out, "<pre>console.log('x');</pre>") {
		t.Error("custom body not used as preview")
	}
}

func TestEnsureHeaderModifiedUpdatesExisting(t *testing.T) {
	a := &App{}
	in := "Title: T\nDate: 2026-01-01 00:00:00\nModified: 2026-01-01 00:00:00\n\nBody"
	out := a.ensureHeaderModified(in, "T")

	if strings.Contains(out, "Modified: 2026-01-01 00:00:00") {
		t.Error("stale Modified timestamp not replaced")
	}
	if strings.Count(out, "Modified:") != 1 {
		t.Errorf("expected exactly one Modified line, got %d", strings.Count(out, "Modified:"))
	}
	if !strings.HasSuffix(out, "Body") {
		t.Errorf("body lost or altered: %q", out)
	}
	if !strings.Contains(out, "Title: T\n") {
		t.Error("existing header lines lost")
	}
}

func TestEnsureHeaderModifiedAddsMissingModified(t *testing.T) {
	a := &App{}
	out := a.ensureHeaderModified("Title: T\nDate: 2026-01-01\n\nBody", "T")
	if strings.Count(out, "Modified:") != 1 {
		t.Errorf("Modified line not added exactly once:\n%s", out)
	}
}

func TestEnsureHeaderModifiedSynthesizesHeader(t *testing.T) {
	a := &App{}
	a.Config.Author = "Tester"
	out := a.ensureHeaderModified("Just body text", "NewPage")

	for _, want := range []string{"Title: NewPage", "Date: ", "Modified: ", "Author: Tester"} {
		if !strings.Contains(out, want) {
			t.Errorf("synthesized header missing %q:\n%s", want, out)
		}
	}
	if !strings.HasSuffix(out, "Just body text") {
		t.Error("body not preserved after synthesized header")
	}
}

// TestCompilePageNoSpuriousMetaFromCSSBody is a regression test for the
// GeminiSvgComponentEditor bug: a note whose header/body separator line
// carried stray spaces ("    ") caused the front-matter parser to run the
// header on through the "<style>" block, turning every "--var: #hex;" CSS
// line into a bogus <meta> tag (which the metadata panel then dumped, and
// whose leaked SVG markup blew up innerHTML). The header must stop at the
// whitespace-only separator, so the CSS renders as body and never becomes a
// meta tag.
func TestCompilePageNoSpuriousMetaFromCSSBody(t *testing.T) {
	a := &App{}
	md := "Title: Editor\nTags: AI\n    \n<style>\n#app { --bg-color: #E2DCD2; }\nhtml, body { margin: 0; }\n</style>\n\nBody."
	out := string(a.compilePage("AI/Editor", []byte(md)))

	// Legitimate header fields still become meta tags.
	if !strings.Contains(out, `name="title" content="Editor"`) {
		t.Errorf("real Title header lost:\n%s", out)
	}
	// CSS custom properties must NEVER appear as meta tags.
	for _, bogus := range []string{`name="--bg-color"`, `name="margin"`, `content="#E2DCD2`} {
		if strings.Contains(out, bogus) {
			t.Errorf("CSS leaked into a meta tag (%s):\n%s", bogus, out)
		}
	}
	// The <style> block must survive into the rendered body (html.WithUnsafe).
	if !strings.Contains(out, "--bg-color: #E2DCD2") {
		t.Error("the <style> block was dropped from the body")
	}
}

// Content starting with markdown or HTML must NOT be mistaken for a
// key:value header block (a "#" heading or "<script>" can contain ":").
func TestEnsureHeaderModifiedNonHeaderFirstLine(t *testing.T) {
	a := &App{}
	for _, in := range []string{"# Head: line\n\nBody", "<script>x: 1</script>\n\nBody"} {
		out := a.ensureHeaderModified(in, "P")
		if !strings.Contains(out, "Title: P") {
			t.Errorf("input %q: expected synthesized header, got:\n%s", in, out)
		}
		if !strings.Contains(out, in) {
			t.Errorf("input %q: original content not preserved verbatim", in)
		}
	}
}
