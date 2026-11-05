package search

import (
	"fmt"
	"strings"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/render"
)

// --- The search result page (search_page.html) ---

var searchPageTmpl = render.LoadTemplate("search_page.html")

// searchPageView holds each value of renderSearchPage. Query comes from a
// URL, and the function escapes it. renderSnippetHTML escapes the snippets.
type searchPageView struct {
	Query        string
	Results      []searchResult
	Total        int
	Truncated    bool
	IndexedKinds []string // what the index currently covers, for the empty state
	Highlight    []string // query terms, hung off every result link as ?hl=
	Disabled     bool     // global search is switched off: explain, do not search
}

// searchKindLabel answers the name of a kind for the group heading.
func searchKindLabel(kind string) string {
	switch kind {
	case config.SearchKindMD:
		return "Notes"
	case config.SearchKindBookmarks:
		return "Bookmarks"
	case config.SearchKindJS:
		return "Scripts"
	case config.SearchKindJSON:
		return "JSON"
	}
	for _, tree := range config.UserFileTrees {
		if kind == tree.Dir {
			return tree.Label
		}
	}
	return kind
}

// renderSnippetHTML puts <mark> around each span. The spans are RUNE offsets,
// thus it walks the text as []rune. It escapes each segment.
func renderSnippetHTML(text string, spans [][2]int) string {
	runes := []rune(text)
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		start, length := sp[0], sp[1]
		if start < at || length <= 0 || start+length > len(runes) {
			continue
		}
		if start > at {
			b.WriteString(render.EscapeHTML(string(runes[at:start])))
		}
		b.WriteString(`<mark class="omn-search-hit">`)
		b.WriteString(render.EscapeHTML(string(runes[start : start+length])))
		b.WriteString(`</mark>`)
		at = start + length
	}
	if at < len(runes) {
		b.WriteString(render.EscapeHTML(string(runes[at:])))
	}
	return b.String()
}

// searchDisabledNotice is the page text when global search is off. A note can
// link here, thus it is a page and not a 404. Its markup is static.
const searchDisabledNotice = `<div class="omn-search-page-notice">` +
	`<h2>Global search is off</h2>` +
	`<p>Searching every note at once needs an index, and the index is held in ` +
	`memory for as long as the app runs - roughly a third of the size of the ` +
	`text it covers. That is a real cost on a small device, so it is off until ` +
	`you ask for it.</p>` +
	`<p><a class="omn-search-page-cta" href="/Config.html#cfg-search">` +
	`Turn on global search in Settings</a></p>` +
	`<p class="omn-search-page-note">There, <em>Enable global search</em> switches ` +
	`it on and the checkboxes under it choose what gets indexed - notes and ` +
	`bookmarks to begin with. It applies immediately; no restart.</p>` +
	`<p class="omn-search-page-note">Searching the note you have open needs none of ` +
	`this and always works: the magnifier in the page header, or ` +
	`<kbd>Ctrl</kbd>+<kbd>K</kbd>.</p>` +
	`</div>`

func renderSearchPage(v searchPageView) string {
	if v.Disabled {
		// Each other slot stays empty. There is no form, because a submit
		// comes back here. There is no result section.
		return render.Fill(searchPageTmpl, map[string]string{
			"DISABLED": " omn-is-disabled",
			"NOTICE":   searchDisabledNotice,
			"QUERY":    "",
			"SUMMARY":  "",
			"GROUPS":   "",
			"EMPTY":    "",
		})
	}

	var groups strings.Builder

	// Group by kind, in a fixed order, thus the page has a stable shape.
	// Inside a group, the order of the server stays.
	for _, kind := range config.SearchKindsAll {
		var inKind []searchResult
		for _, r := range v.Results {
			if r.Kind == kind {
				inKind = append(inKind, r)
			}
		}
		if len(inKind) == 0 {
			continue
		}
		fmt.Fprintf(&groups, "<h2 class=\"omn-search-group\">%s <span class=\"omn-search-group-count\">%d</span></h2>\n",
			render.EscapeHTML(searchKindLabel(kind)), len(inKind))

		for _, r := range inKind {
			groups.WriteString("<div class=\"omn-search-result\">\n")
			title := r.Title
			if title == "" {
				title = r.Name
			}
			// The link carries the query as ?hl=. The note then marks and
			// scrolls to the match. The client removes the parameters after
			// it uses them, thus a copied URL is plain.
			fmt.Fprintf(&groups, "  <a class=\"omn-search-result-title\" href=\"%s\">%s</a>\n",
				render.EscapeHTML(highlightURL(r.URL, v.Highlight)), render.EscapeHTML(title))
			fmt.Fprintf(&groups, "  <div class=\"omn-search-result-path\">%s</div>\n", render.EscapeHTML(r.Name))

			if len(r.Tags) > 0 {
				groups.WriteString("  <div class=\"omn-search-result-tags\">")
				for _, t := range r.Tags {
					// Use the same pill markup and the same anchor as the
					// page header. See render.RenderIndexPage.
					fmt.Fprintf(&groups, "<a href=\"/OMNGoTags.html#%s\" class=\"omn-taglink\"><span class=\"omn-tagmark\">%s</span></a>",
						render.EscapeHTML(render.TagSlug(t)), render.EscapeHTML(t))
				}
				groups.WriteString("</div>\n")
			}

			lastSection := ""
			for _, m := range r.Matches {
				// Show the section heading one time for each run of hits in
				// that section. Several matches in one bookmark are one
				// place.
				if m.Section != nil && m.Section.Label != "" && m.Section.Label != lastSection {
					lastSection = m.Section.Label
					groups.WriteString("  <div class=\"omn-search-section\">")
					if m.Section.ID != "" {
						// r.URL already ends with the anchor of the BEST hit.
						// This link needs the anchor of THIS section, thus
						// cut the anchor off first.
						base := r.URL
						if at := strings.IndexByte(base, '#'); at >= 0 {
							base = base[:at]
						}
						fmt.Fprintf(&groups, "<a href=\"%s#%s\">%s</a>",
							render.EscapeHTML(highlightURL(base, v.Highlight)),
							render.EscapeHTML(m.Section.ID), render.EscapeHTML(m.Section.Label))
					} else {
						groups.WriteString(render.EscapeHTML(m.Section.Label))
					}
					groups.WriteString("</div>\n")
				} else if m.Section == nil {
					lastSection = ""
				}
				// Each snippet line opens the note AT that line. The text of
				// the line gets percent-encoding in snippetURL and an HTML
				// escape here, because the text of a note can come from
				// another person.
				fmt.Fprintf(&groups, "  <a class=\"omn-search-snippet\" href=\"%s\">",
					render.EscapeHTML(snippetURL(r.URL, v.Highlight, m)))
				fmt.Fprintf(&groups, "<span class=\"omn-search-snippet-line\">%d</span>", m.Line)
				if m.Context != "" {
					where := "inside a code block"
					if m.Context == "script" {
						where = "inside a <script> block"
					}
					fmt.Fprintf(&groups, "<span class=\"omn-search-snippet-ctx\" title=\"%s\">&lsaquo;/&rsaquo;</span>",
						render.EscapeHTML(where))
				}
				fmt.Fprintf(&groups, "<span class=\"omn-search-snippet-text\">%s</span>",
					renderSnippetHTML(m.Text, m.Spans))
				groups.WriteString("</a>\n")
			}
			if r.Truncated {
				groups.WriteString("  <div class=\"omn-search-result-note\">only the first 500 KiB of this file was searched</div>\n")
			}
			groups.WriteString("</div>\n")
		}
	}

	summary := ""
	empty := ""
	switch {
	case v.Query == "":
		summary = ""
	case v.Total == 0:
		// Name the kinds that the search covered. "No results" from a setting
		// that the reader forgot is a trap.
		var kinds []string
		for _, k := range v.IndexedKinds {
			kinds = append(kinds, searchKindLabel(k))
		}
		covered := "nothing"
		if len(kinds) > 0 {
			covered = strings.Join(kinds, ", ")
		}
		empty = fmt.Sprintf(`<div class="omn-search-empty">`+
			`<p>No matches for <strong>%s</strong>.</p>`+
			`<p class="omn-search-empty-hint">The index currently covers: %s. `+
			`<a href="/Config.html#cfg-search">Change what is searched</a>.</p>`+
			`</div>`, render.EscapeHTML(v.Query), render.EscapeHTML(covered))
	default:
		word := "results"
		if v.Total == 1 {
			word = "result"
		}
		summary = fmt.Sprintf("%d %s for <strong>%s</strong>", v.Total, word, render.EscapeHTML(v.Query))
		if v.Truncated && len(v.Results) < v.Total {
			summary += fmt.Sprintf(" <span class=\"omn-search-page-note\">(showing the first %d)</span>", len(v.Results))
		}
	}

	return render.Fill(searchPageTmpl, map[string]string{
		"DISABLED": "",
		"NOTICE":   "",
		"QUERY":    render.EscapeHTML(v.Query),
		"SUMMARY":  summary,
		"GROUPS":   groups.String(),
		"EMPTY":    empty,
	})
}
