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
		`(?:^|[\s:,(])(?:white|black|red|green|blue|yellow|aqua|orange|gray|grey|silver|purple|pink|navy|teal|maroon|lime|fuchsia|olive)(?:$|[\s;,)!])`)
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
		name := strings.TrimPrefix(tok, "--")
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
