package backend

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// ----------------------------------------------------------------------
// Auto-generated Tags page (OMNGoTags)
// ----------------------------------------------------------------------
//
// OMNGoTags is a single, root-level, generated note that indexes every other
// note by its Tags: header. It is Format A - "prepared links": the page is
// static HTML (a cloud of jump-links plus one section per tag with relative
// links to the tagged pages), so it works with JavaScript disabled and when the
// compiled html/ tree is opened offline (file://). See
// claude/tags-page-plan.md for the full design.
//
// generateTagsPage is a sanctioned writer of md/OMNGoTags.md (the general cache
// contract in render_cache.go reserves md writes for the save/edit paths; this
// generated page is the documented exception) and produces html/OMNGoTags.html
// through renderAndCache like any other page.

// tagSlug turns a tag into an HTML id / URL-fragment. It is the single source
// of the anchor contract shared by the tag pills (renderIndexPage) and this
// generator, so a pill's "#slug" always matches a section's id - both are
// server-side Go, so they can never drift. Unicode letters and digits are
// kept (so Cyrillic tags slug sanely too); every other run collapses to a
// single '-', trimmed at the ends. Case is preserved to minimise the chance of
// two distinct tags colliding to one slug (an accepted, rare v1 limitation).
func tagSlug(tag string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range tag {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// extractTitleTags reads a note's Title and Tags out of its front-matter
// header, using the exact same rules as compilePageWithBody (which now calls
// this too, so the two can't drift): the last "Title:" wins; "Tags:" is a
// comma-separated list, trimmed, empties dropped. title is "" when absent (the
// caller decides the fallback); tags preserves order and, like the pill path,
// is NOT de-duplicated here - the tags-page generator de-dupes per page itself.
func extractTitleTags(content string) (title string, tags []string) {
	fm := splitFrontMatter(content)
	if !fm.HasHeader {
		return "", nil
	}
	for _, h := range strings.Split(fm.Header, "\n") {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		if k == "title" {
			title = v
		} else if k == "tags" {
			for _, t := range strings.Split(v, ",") {
				if t = strings.TrimSpace(t); t != "" {
					tags = append(tags, t)
				}
			}
		}
	}
	return title, tags
}

// tagPageRef is one tagged note as listed under a tag on the Tags page.
type tagPageRef struct {
	path  string // page path relative to md root, no extension (e.g. "Hydro/Myrtle")
	title string
}

// buildTagIndex walks md/**.md and returns tag -> pages. It excludes the
// generated page itself (OMNGoTags) and the gitignored md/local/ scratch tree,
// skips untagged notes, and de-dupes a tag repeated within one note. Unreadable
// entries are skipped rather than aborting the whole scan.
func (a *App) buildTagIndex() map[string][]tagPageRef {
	mdRoot := filepath.Join(a.StorageDir, "md")
	index := map[string][]tagPageRef{}

	_ = filepath.WalkDir(mdRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if p == filepath.Join(mdRoot, "local") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(mdRoot, p)
		if err != nil {
			return nil
		}
		pageName := strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		if pageName == "OMNGoTags" {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		title, tags := extractTitleTags(string(content))
		if title == "" {
			title = pageName
		}
		seen := map[string]bool{}
		for _, t := range tags {
			if seen[t] {
				continue
			}
			seen[t] = true
			index[t] = append(index[t], tagPageRef{path: pageName, title: title})
		}
		return nil
	})

	return index
}

// renderTagsMarkdown builds the OMNGoTags note content from a tag index: a
// header, a "don't edit" comment, a cloud of jump-links, and one section per
// tag. Tags are ordered case-insensitively; pages within a tag by title then
// path. The body is raw HTML (notes render with html.WithUnsafe()) so the
// section ids exactly match tagSlug; page links are relative ".html" paths that
// resolve from the root-level page both online and offline. Every tag/title is
// HTML-escaped for its context.
func renderTagsMarkdown(index map[string][]tagPageRef) []byte {
	tagNames := make([]string, 0, len(index))
	for t := range index {
		tagNames = append(tagNames, t)
	}
	sort.Slice(tagNames, func(i, j int) bool {
		li, lj := strings.ToLower(tagNames[i]), strings.ToLower(tagNames[j])
		if li != lj {
			return li < lj
		}
		return tagNames[i] < tagNames[j]
	})

	var b strings.Builder
	b.WriteString("Title: Tags\nCategory: System\n\n")
	b.WriteString("<!--\n")
	b.WriteString("  Generated automatically by OMN-Go from every note's Tags: header.\n")
	b.WriteString("  Do not edit - your changes are overwritten on the next regeneration.\n")
	b.WriteString("-->\n\n")

	b.WriteString(`<div class="omn-tags-cloud">` + "\n")
	for _, t := range tagNames {
		fmt.Fprintf(&b, `<a href="#%s" class="taglink"><span class="tagmark">%s</span></a>`+"\n",
			escapeHTML(tagSlug(t)), escapeHTML(t))
	}
	b.WriteString("</div>\n\n")

	for _, t := range tagNames {
		refs := index[t]
		sort.Slice(refs, func(i, j int) bool {
			ti, tj := strings.ToLower(refs[i].title), strings.ToLower(refs[j].title)
			if ti != tj {
				return ti < tj
			}
			return refs[i].path < refs[j].path
		})
		fmt.Fprintf(&b, `<h2 id="%s" class="omn-tags-section">%s (%d)</h2>`+"\n",
			escapeHTML(tagSlug(t)), escapeHTML(t), len(refs))
		b.WriteString("<ul>\n")
		for _, r := range refs {
			fmt.Fprintf(&b, `<li><a href="%s.html">%s</a></li>`+"\n",
				escapeHTML(r.path), escapeHTML(r.title))
		}
		b.WriteString("</ul>\n\n")
	}

	return []byte(b.String())
}

// generateTagsPage rebuilds md/OMNGoTags.md from the current tag index and
// compiles it to html/OMNGoTags.html. Safe to call repeatedly (it fully
// replaces both files). Wiring - when it runs (startup, and lazily on a stale
// view) - is Phase T2; this is the generator itself.
func (a *App) generateTagsPage() error {
	content := renderTagsMarkdown(a.buildTagIndex())

	mdRoot := filepath.Join(a.StorageDir, "md")
	if err := os.MkdirAll(mdRoot, 0755); err != nil {
		return fmt.Errorf("tags: mkdir md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(mdRoot, "OMNGoTags.md"), content, 0644); err != nil {
		return fmt.Errorf("tags: write md/OMNGoTags.md: %w", err)
	}
	if _, err := a.renderAndCache("OMNGoTags", content); err != nil {
		return fmt.Errorf("tags: cache OMNGoTags.html: %w", err)
	}
	return nil
}
