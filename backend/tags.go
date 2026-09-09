package backend

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ----------------------------------------------------------------------
// Auto-generated Tags page (OMNGoTags)
// ----------------------------------------------------------------------
//
// OMNGoTags is a single, root-level, generated note. It indexes every other
// note by its Tags: header. It is Format A, which is "prepared links". The
// page is static HTML. It holds a cloud of jump-links, and one section per tag
// with relative links to the tagged pages. It thus works with JavaScript
// disabled, and when the compiled html/ tree is opened offline (file://). See
// claude/tags-page-plan.md for the full design.
//
// generateTagsPage is a sanctioned writer of md/OMNGoTags.md. The general
// cache contract in render_cache.go reserves an md write for the save and edit
// paths. This generated page is the documented exception. generateTagsPage
// produces html/OMNGoTags.html through renderAndCache, like any other page.

// tagSlug turns a tag into an HTML id, which is also a URL fragment. It is the
// single source of the anchor contract that the tag pills (renderIndexPage)
// and this generator share. A "#slug" of a pill thus always matches the id of
// a section. Both are server-side Go, thus they can never drift. Unicode
// letters and digits are kept, thus a Cyrillic tag slugs sanely too. Every
// other run collapses to a single '-', trimmed at the ends. Case is preserved
// to minimize the chance that two distinct tags collide to one slug. That is
// an accepted and rare v1 limitation.
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

// extractTitleTags reads the Title and the Tags of a note out of its header
// block. It uses exactly the same rules as compilePageWithBody, which now
// calls this too, thus the two cannot drift. The last "Title:" wins. "Tags:"
// is a comma-separated list, trimmed, with each empty entry dropped. title is
// "" when the note has none, and the caller decides the fallback. tags keeps
// the order and, like the pill path, is NOT de-duplicated here. The tags-page
// generator de-dupes per page itself.
func extractTitleTags(content string) (title string, tags []string) {
	hb := parseHeaderBlock(content)
	if !hb.HasHeader {
		return "", nil
	}
	for _, h := range strings.Split(hb.Header, "\n") {
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

// renderTagsMarkdown builds the OMNGoTags note content from a tag index. The
// content is a header, a "do not edit" comment, a cloud of jump-links, and one
// section per tag. Tags are ordered case-insensitively. Pages within a tag are
// ordered by title, and then by path. The body is raw HTML, because a note
// renders with html.WithUnsafe(). The section ids thus match tagSlug exactly.
// A page link is a relative ".html" path, which resolves from the root-level
// page both online and offline. Every tag and title is HTML-escaped for its
// context.
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
	// A rebuild reads and parses every note. On a large collection that is a
	// real wait. When it happens lazily, it is inside a page navigation
	// (serveTagsPage), where no in-page progress UI can run. A log of the
	// start and the end at least surfaces the wait on the /api/logs stream,
	// and in the JS console. The indicator that the reader sees for the
	// navigation itself is the Android ProgressBar
	// (MainActivity.onPageStarted), and the delayed overlay in
	// omn-go-core.js.
	a.logDebugf(logTags, "Rebuilding tags index")
	started := time.Now()
	index := a.buildTagIndex()
	content := renderTagsMarkdown(index)
	defer func() {
		a.logInfof(logTags, "Tags index rebuilt: %d tags in %s",
			len(index), time.Since(started).Round(time.Millisecond))
	}()

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

// newestNoteMtime returns the most recent modification time among the note
// sources that the Tags page is built from. Those are every md/**.md file AND
// its containing directories. A directory mtime is included on purpose. A file
// added, deleted or renamed bumps the mtime of its directory, and not
// necessarily the mtime of any surviving file. A scan of files alone would
// thus miss those. The generated OMNGoTags.md, which is a derived file, and
// the md/local scratch tree are excluded. It is a stat-only walk, with no
// parsing. It returns the zero time when md/ cannot be walked.
func (a *App) newestNoteMtime() time.Time {
	mdRoot := filepath.Join(a.StorageDir, "md")
	var newest time.Time
	consider := func(d fs.DirEntry) {
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	_ = filepath.WalkDir(mdRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if p == filepath.Join(mdRoot, "local") {
				return fs.SkipDir
			}
			consider(d)
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(mdRoot, p)
		if err != nil {
			return nil
		}
		if strings.TrimSuffix(filepath.ToSlash(rel), ".md") == "OMNGoTags" {
			return nil // never let the derived file drive its own staleness
		}
		consider(d)
		return nil
	})
	return newest
}

// tagsPageStale reports whether html/OMNGoTags.html needs regenerating: forced
// (?refresh), missing/unreadable, or older than the newest note source. This is
// the OMNGoTags analogue of serveHTMLPage's single-source mtime check, except
// its "source" is every note rather than one .md.
func (a *App) tagsPageStale(forceRefresh bool) bool {
	if forceRefresh {
		return true
	}
	htmlStat, err := os.Stat(a.pageHTMLPath("OMNGoTags"))
	if err != nil {
		return true // missing or unreadable -> (re)generate
	}
	return a.newestNoteMtime().After(htmlStat.ModTime())
}

// serveTagsPage serves the generated Tags page, regenerating it first when
// stale (lazy + mtime-invalidated). Special-cased from serveHTMLPage so it uses
// the all-notes staleness above instead of the normal one-source check. Serving
// itself mirrors serveHTMLPage's tail (injectRuntimeVars over the cached html).
func (a *App) serveTagsPage(w http.ResponseWriter, r *http.Request) {
	forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
	if a.tagsPageStale(forceRefresh) {
		if err := a.generateTagsPage(); err != nil {
			a.logErrf(logTags, "serveTagsPage: %v", err)
		}
	}
	htmlPath := a.pageHTMLPath("OMNGoTags")
	writeHTMLHeader(w)
	data, err := os.ReadFile(htmlPath)
	if err == nil {
		w.Write(a.injectRuntimeVars(data))
	} else {
		http.ServeFile(w, r, htmlPath)
	}
}
