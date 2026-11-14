package repocheck

// ----------------------------------------------------------------------
// The stylesheets of OMN-Go
// ----------------------------------------------------------------------
//
// Section 1 of omn-go-core.css holds the palette: each color value of the
// application. The theme maps give each token a palette value. A color
// scheme of the user sets palette values, thus a color value in another
// rule is a color that no scheme can change.

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
)

var (
	cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	// cssColorRe finds a color value: a hex value, rgb(), rgba(), hsl(),
	// hsla(), or a color name. "white-space" is not a color.
	cssColorRe = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?)\(|` +
		`(?:^|[\s:,(])(?:white|black|red|green|blue|yellow|aqua|orange|gray|grey|silver|purple|pink|navy|teal|maroon|lime|fuchsia|olive|ivory|gold|brown|cyan|magenta|beige|indigo|violet)(?:$|[\s;,)!])`)
	// codeColorRe is the check of a script or a template. It finds a hex value
	// or a color function. A color name is too common in prose there.
	codeColorRe  = regexp.MustCompile(`['":\s]#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})(?:$|[^\w-])|\b(?:rgba?|hsla?)\(`)
	cssPaletteRe = regexp.MustCompile(`(?s):root \{(\s*--omn-light-.*?)\n\}`)
	cssDarkRe    = regexp.MustCompile(`(?s):root\[data-theme="dark"\] \{(.*?)\n\}`)
	cssAutoRe    = regexp.MustCompile(`(?s):root\[data-theme="auto"\] \{(.*?)\n    \}`)
	cssLightRe   = regexp.MustCompile(`(?s):root \{\s*color-scheme: light;(.*?)\n\}`)
	cssDeclRe    = regexp.MustCompile(`(--[\w-]+)\s*:\s*([^;]+);`)
)

// appStylesheets answers the path and the text of each stylesheet that
// OMN-Go writes. A vendored .min.css file is not ours.
func appStylesheets(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(frontend.Static, "html/css", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != ".css" || strings.Contains(p, ".min.") {
			return err
		}
		data, rerr := frontend.Static.ReadFile(p)
		out[p] = string(data)
		return rerr
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 5 {
		t.Fatalf("only %d stylesheets were read, thus this test proves little", len(out))
	}
	return out
}

// No stylesheet, template or application script holds a color value
// outside the palette. omn-go-compat.js is the one exception: it runs in a
// WebView that is too old for CSS custom properties.
func TestColorValuesLiveOnlyInThePalette(t *testing.T) {
	check := func(name, text string) {
		re := cssColorRe
		if !strings.HasSuffix(name, ".css") {
			re = codeColorRe
		}
		for i, line := range strings.Split(text, "\n") {
			if m := re.FindString(line); m != "" {
				t.Errorf("%s:%d holds the color %q. Add a token to the palette of omn-go-core.css, and use it.",
					name, i+1, strings.TrimSpace(m))
			}
		}
	}
	for name, text := range appStylesheets(t) {
		text = cssCommentRe.ReplaceAllStringFunc(text, func(c string) string {
			return strings.Repeat("\n", strings.Count(c, "\n"))
		})
		if strings.HasSuffix(name, "/omn-go-core.css") {
			m := cssPaletteRe.FindStringSubmatchIndex(text)
			if m == nil {
				t.Fatalf("%s has no palette block", name)
			}
			text = text[:m[2]] + strings.Repeat("\n", strings.Count(text[m[2]:m[3]], "\n")) + text[m[3]:]
		}
		check(name, text)
	}
	templates, err := frontend.Templates.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range templates {
		data, _ := frontend.Templates.ReadFile("templates/" + e.Name())
		check("templates/"+e.Name(), strings.ReplaceAll(string(data), "&#", ""))
	}
	scripts, err := frontend.Static.ReadDir("html/js/OMN-Go")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range scripts {
		if strings.Contains(e.Name(), ".min.") || e.Name() == "omn-go-compat.js" {
			continue
		}
		data, _ := frontend.Static.ReadFile("html/js/OMN-Go/" + e.Name())
		check("html/js/OMN-Go/"+e.Name(), string(data))
	}
}

// Each token has a light palette value and a dark palette value. The light
// map and the two dark maps name each token, and the two dark maps are the
// same. A token that is absent from a dark map keeps its light color in the
// dark theme.
func TestDarkMapsAreTheSame(t *testing.T) {
	src, err := frontend.Static.ReadFile("html/css/OMN-Go/omn-go-core.css")
	if err != nil {
		t.Fatal(err)
	}
	text := cssCommentRe.ReplaceAllString(string(src), "")
	decls := func(re *regexp.Regexp, what string) map[string]string {
		m := re.FindStringSubmatch(text)
		if m == nil {
			t.Fatalf("omn-go-core.css has no %s", what)
		}
		out := map[string]string{}
		for _, d := range cssDeclRe.FindAllStringSubmatch(m[1], -1) {
			out[d[1]] = strings.TrimSpace(d[2])
		}
		return out
	}
	palette := decls(cssPaletteRe, "palette")
	light := decls(cssLightRe, "light map")
	dark := decls(cssDarkRe, "dark map")
	auto := decls(cssAutoRe, "auto map")

	if len(light) < 40 {
		t.Fatalf("the light map has %d tokens", len(light))
	}
	for tok, v := range light {
		name := strings.TrimPrefix(tok, "--omn-")
		if v != "var(--omn-light-"+name+")" {
			t.Errorf("the light map gives %s the value %q", tok, v)
		}
		if dark[tok] != "var(--omn-dark-"+name+")" {
			t.Errorf("the dark map gives %s the value %q", tok, dark[tok])
		}
		if auto[tok] != dark[tok] {
			t.Errorf("the auto map gives %s %q, and the dark map gives %q", tok, auto[tok], dark[tok])
		}
		for _, mode := range []string{"light", "dark"} {
			if palette["--omn-"+mode+"-"+name] == "" {
				t.Errorf("the palette has no --omn-%s-%s", mode, name)
			}
		}
	}
	if len(dark) != len(light) || len(auto) != len(light) || len(palette) != 2*len(light) {
		t.Errorf("the light map has %d tokens, the dark maps %d and %d, the palette %d values",
			len(light), len(dark), len(auto), len(palette))
	}
}

// ----------------------------------------------------------------------
// Each class, id and token of OMN-Go has the prefix "omn-"
// ----------------------------------------------------------------------
//
// A note puts its own markup, styles and scripts into the page. The prefix
// keeps the names of the application apart from the names of a note.
// The classes of a vendored library (Material Icons, highlight.js, KaTeX)
// keep the names of that library.

var (
	cssPreludeRe = regexp.MustCompile(`([^{};]+)\{`)
	cssNameRe    = regexp.MustCompile(`([.#])([A-Za-z_][\w-]*)`)
	cssTokenRe   = regexp.MustCompile(`--[A-Za-z][\w-]*`)
	htmlClassRe  = regexp.MustCompile(`\bclass="([^"]*)"`)
	htmlIDRe     = regexp.MustCompile(`\bid="([^"]*)"`)
	// slotRe finds a part of a value that the server or a script fills: a
	// %%SLOT%% or a JavaScript concatenation such as ' + st.cls + '.
	slotRe = regexp.MustCompile(`%%\w+%%|'\s*\+[^+]*\+\s*'`)
)

// vendoredClass tells if a class belongs to a vendored library.
func vendoredClass(c string) bool {
	return c == "material-icons" || strings.HasPrefix(c, "hljs") || strings.HasPrefix(c, "katex")
}

func TestEachNameInAStylesheetHasThePrefix(t *testing.T) {
	for p, text := range appStylesheets(t) {
		text = cssCommentRe.ReplaceAllString(text, "")
		for _, m := range cssPreludeRe.FindAllStringSubmatch(text, -1) {
			sel := strings.TrimSpace(m[1])
			if strings.HasPrefix(sel, "@") {
				continue
			}
			for _, n := range cssNameRe.FindAllStringSubmatch(sel, -1) {
				if !strings.HasPrefix(n[2], "omn-") && !vendoredClass(n[2]) {
					t.Errorf("%s: the selector %q holds %s%s, which has no prefix \"omn-\"", p, sel, n[1], n[2])
				}
			}
		}
		for _, tok := range cssTokenRe.FindAllString(text, -1) {
			if !strings.HasPrefix(tok, "--omn-") {
				t.Errorf("%s: the token %s has no prefix \"--omn-\"", p, tok)
			}
		}
	}
}

func TestEachNameInATemplateHasThePrefix(t *testing.T) {
	entries, err := fs.ReadDir(frontend.Templates, "templates")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		p := "templates/" + e.Name()
		data, err := frontend.Templates.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, m := range htmlClassRe.FindAllStringSubmatch(text, -1) {
			for _, c := range strings.Fields(slotRe.ReplaceAllString(m[1], " ")) {
				checked++
				if !strings.HasPrefix(c, "omn-") && !vendoredClass(c) {
					t.Errorf("%s: the class %q has no prefix \"omn-\"", p, c)
				}
			}
		}
		for _, m := range htmlIDRe.FindAllStringSubmatch(text, -1) {
			checked++
			if id := slotRe.ReplaceAllString(m[1], ""); !strings.HasPrefix(id, "omn-") {
				t.Errorf("%s: the id %q has no prefix \"omn-\"", p, m[1])
			}
		}
	}
	if checked < 100 {
		t.Fatalf("only %d names were read, thus this test proves little", checked)
	}
}

// ----------------------------------------------------------------------
// The layers of the page
// ----------------------------------------------------------------------

// cssLayersRe finds the list of layers in section 6 of omn-go-core.css.
var cssLayersRe = regexp.MustCompile(`:root\s*\{((?:\s*--omn-layer-[a-z-]+:\s*\d+;)+)\s*\}`)

// cssZIndexRe finds each z-index declaration of a stylesheet.
var cssZIndexRe = regexp.MustCompile(`z-index:\s*([^;}]+)`)

// cssLayerVarRe reads the name of a layer from a z-index value.
var cssLayerVarRe = regexp.MustCompile(`^var\((--omn-layer-[a-z-]+)\)$`)

// Each z-index of a stylesheet of OMN-Go is a layer of the list in
// omn-go-core.css, or 0 or 1. The values 0 and 1 give an order inside one
// element, for example the tag list inside the bookmark panel. A number
// such as 60 is a layer outside the list, and nothing tells which layer it
// covers. Each layer of the list is in use.
func TestEachZIndexIsALayer(t *testing.T) {
	core, err := frontend.Static.ReadFile("html/css/OMN-Go/omn-go-core.css")
	if err != nil {
		t.Fatal(err)
	}
	m := cssLayersRe.FindStringSubmatch(cssCommentRe.ReplaceAllString(string(core), ""))
	if m == nil {
		t.Fatal("omn-go-core.css has no :root block with the --omn-layer- tokens")
	}
	layers := map[string]bool{}
	for _, d := range cssDeclRe.FindAllStringSubmatch(m[1], -1) {
		layers[d[1]] = false
	}
	if len(layers) < 5 {
		t.Fatalf("the list of layers has %d tokens", len(layers))
	}
	seen := 0
	for name, src := range appStylesheets(t) {
		text := cssCommentRe.ReplaceAllString(src, "")
		for _, z := range cssZIndexRe.FindAllStringSubmatch(text, -1) {
			seen++
			value := strings.TrimSpace(z[1])
			if value == "0" || value == "1" {
				continue
			}
			v := cssLayerVarRe.FindStringSubmatch(value)
			if v == nil {
				t.Errorf("%s has z-index: %s. Use a layer of the list in omn-go-core.css, "+
					"or 0 or 1 for an order inside one element.", name, value)
				continue
			}
			if _, ok := layers[v[1]]; !ok {
				t.Errorf("%s uses %s, which is not in the list of layers", name, v[1])
				continue
			}
			layers[v[1]] = true
		}
	}
	if seen < 5 {
		t.Fatalf("the scan found %d z-index declarations", seen)
	}
	for layer, used := range layers {
		if !used {
			t.Errorf("no stylesheet uses the layer %s. Remove it from the list.", layer)
		}
	}
}
