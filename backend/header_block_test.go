package backend

import "testing"

// TestParseHeaderBlock pins the behavior of the single header-block parser
// introduced in Phase 1 (header_block.go), now the one authority every Go
// caller shares. The interesting rows are the ones that used to be handled
// differently by the three old copies: a '#'/'<'-prefixed first line is
// body, not a header; a header-only note has an empty body; CRLF is handled.
func TestParseHeaderBlock(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		hasHeader  bool
		header     string
		body       string
		bodyOffset int
	}{
		{
			name:       "header and body",
			in:         "Title: T\nDate: D\n\nBody",
			hasHeader:  true,
			header:     "Title: T\nDate: D",
			body:       "Body",
			bodyOffset: 18,
		},
		{
			name:       "header only, no trailing blank",
			in:         "Title: T",
			hasHeader:  true,
			header:     "Title: T",
			body:       "",
			bodyOffset: 8,
		},
		{
			name:       "header only, trailing blank",
			in:         "Title: T\n\n",
			hasHeader:  true,
			header:     "Title: T",
			body:       "",
			bodyOffset: 10,
		},
		{
			name:       "plain body, no colon",
			in:         "hello world\n\nmore",
			hasHeader:  false,
			header:     "",
			body:       "hello world\n\nmore",
			bodyOffset: 0,
		},
		{
			name:       "markdown heading with colon is body",
			in:         "# Head: x\n\nBody",
			hasHeader:  false,
			header:     "",
			body:       "# Head: x\n\nBody",
			bodyOffset: 0,
		},
		{
			name:       "html first line with colon is body",
			in:         "<script>a: 1</script>\n\nx",
			hasHeader:  false,
			header:     "",
			body:       "<script>a: 1</script>\n\nx",
			bodyOffset: 0,
		},
		{
			name:       "indented first line with colon is body",
			in:         " Indented: x\n\ny",
			hasHeader:  false,
			header:     "",
			body:       " Indented: x\n\ny",
			bodyOffset: 0,
		},
		{
			name:       "any colon first line is a header (unchanged from all old impls)",
			in:         "Just: kidding\n\nreal",
			hasHeader:  true,
			header:     "Just: kidding",
			body:       "real",
			bodyOffset: 15,
		},
		{
			name:       "crlf header and body",
			in:         "Title: T\r\nDate: D\r\n\r\nBody",
			hasHeader:  true,
			header:     "Title: T\r\nDate: D\r",
			body:       "Body",
			bodyOffset: 21,
		},
		{
			// Regression: the separator line carries stray spaces (real
			// notes have this - see GeminiSvgComponentEditor). It must still
			// terminate the header instead of letting it swallow the
			// "<style>" block and its "--var: #hex;" lines as metadata.
			name:       "whitespace-only separator line",
			in:         "Title: X\nTags: AI\n    \n<style>foo",
			hasHeader:  true,
			header:     "Title: X\nTags: AI",
			body:       "<style>foo",
			bodyOffset: 23,
		},
		{
			// Regression: header immediately followed by content with no
			// blank line at all - the first non-"Key: Value" line ("<style>")
			// is the body, not more header.
			name:       "no separator, header then non-header line",
			in:         "Title: X\n<style>foo",
			hasHeader:  true,
			header:     "Title: X",
			body:       "<style>foo",
			bodyOffset: 9,
		},
		{
			// A prose line (no colon) right after the header starts the body.
			name:       "header then prose without colon",
			in:         "Title: X\nJust prose.\n\nmore",
			hasHeader:  true,
			header:     "Title: X",
			body:       "Just prose.\n\nmore",
			bodyOffset: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hb := parseHeaderBlock(tt.in)
			if hb.HasHeader != tt.hasHeader {
				t.Errorf("HasHeader = %v, want %v", hb.HasHeader, tt.hasHeader)
			}
			if hb.Header != tt.header {
				t.Errorf("Header = %q, want %q", hb.Header, tt.header)
			}
			if hb.Body != tt.body {
				t.Errorf("Body = %q, want %q", hb.Body, tt.body)
			}
			if hb.BodyOffset != tt.bodyOffset {
				t.Errorf("BodyOffset = %d, want %d", hb.BodyOffset, tt.bodyOffset)
			}
			// BodyOffset must index the real start of Body in the original
			// content (the property the editor caret relies on).
			if tt.hasHeader && hb.Body != "" {
				if got := tt.in[hb.BodyOffset:]; got != hb.Body {
					t.Errorf("in[BodyOffset:] = %q, want Body %q", got, hb.Body)
				}
			}
		})
	}
}

// TestIsHeaderFirstLine documents the first-line rule the whole system
// shares (and that the editor JS ports verbatim).
func TestIsHeaderFirstLine(t *testing.T) {
	yes := []string{"Title: X", "Just: kidding", "Date: 2026-01-01 00:00:00", "A:b"}
	no := []string{"no colon here", "# Head: x", "<script>x: 1", " indented: x", "", "plain"}
	for _, s := range yes {
		if !isHeaderFirstLine(s) {
			t.Errorf("isHeaderFirstLine(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isHeaderFirstLine(s) {
			t.Errorf("isHeaderFirstLine(%q) = true, want false", s)
		}
	}
}
