package render

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/noteheader"
)

// ----------------------------------------------------------------------
// The Tags page (OMNGoTags)
// ----------------------------------------------------------------------
//
// OMNGoTags is one generated note at the root. It lists each other note by
// its Tags: header. The page is static HTML: a cloud of links, and one
// section for each tag with relative links to the notes. It thus works
// without JavaScript and from file://.
//
// GenerateTagsPage writes md/OMNGoTags.md. cache.go allows a write to
// md/ only from a save or an edit, and this generated page is the one
// exception. html/OMNGoTags.html comes from RenderAndCache, the same as each
// other page.

// TagSlug makes an HTML id, and a URL fragment, from a tag. The tag pills of
// RenderIndexPage and this generator both call it, thus a "#slug" always
// matches a section id. It keeps each Unicode letter and digit, thus a
// Cyrillic tag works. It changes each other run to one '-', and it trims the
// ends. It keeps the case, thus two tags collide less often. A rare collision
// stays possible.
func TagSlug(tag string) string {
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

// ExtractTitleTags reads the Title and the Tags of a note from its header
// block. CompilePageWithBody calls it too, thus the two read the same values.
// The last "Title:" wins. "Tags:" is a list with commas. The function trims
// each entry and drops an empty one. title is "" when the note has none. tags
// keeps the order and can hold a duplicate. BuildTagIndex removes the
// duplicates.
func ExtractTitleTags(content string) (title string, tags []string) {
	hb := noteheader.Parse(content)
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

// TagPageRef is one note under a tag on the Tags page.
type TagPageRef struct {
	Path  string // page path relative to md root, no extension (e.g. "Hydro/Myrtle")
	Title string
}

// BuildTagIndex walks md/**.md and answers the notes for each tag. It skips
// OMNGoTags itself, the md/local scratch tree, a note with no tag and a file
// that it cannot read. A tag that a note names twice counts one time.
func (rd *Renderer) BuildTagIndex() map[string][]TagPageRef {
	mdRoot := rd.Layout.MD()
	index := map[string][]TagPageRef{}

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
		title, tags := ExtractTitleTags(string(content))
		if title == "" {
			title = pageName
		}
		seen := map[string]bool{}
		for _, t := range tags {
			if seen[t] {
				continue
			}
			seen[t] = true
			index[t] = append(index[t], TagPageRef{Path: pageName, Title: title})
		}
		return nil
	})

	return index
}

// renderTagsMarkdown makes the content of the OMNGoTags note. It holds a
// header, a "do not edit" comment, the cloud of links, and one section for
// each tag. The tags sort without case, and the notes of a tag sort by title
// and then by path. The body is raw HTML, thus each section id is exactly
// TagSlug. A note link is a relative ".html" path, thus it works online and
// from file://. EscapeHTML escapes each tag and title.
func renderTagsMarkdown(index map[string][]TagPageRef) []byte {
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
		fmt.Fprintf(&b, `<a href="#%s" class="omn-taglink"><span class="omn-tagmark">%s</span></a>`+"\n",
			EscapeHTML(TagSlug(t)), EscapeHTML(t))
	}
	b.WriteString("</div>\n\n")

	for _, t := range tagNames {
		refs := index[t]
		sort.Slice(refs, func(i, j int) bool {
			ti, tj := strings.ToLower(refs[i].Title), strings.ToLower(refs[j].Title)
			if ti != tj {
				return ti < tj
			}
			return refs[i].Path < refs[j].Path
		})
		fmt.Fprintf(&b, `<h2 id="%s" class="omn-tags-section">%s (%d)</h2>`+"\n",
			EscapeHTML(TagSlug(t)), EscapeHTML(t), len(refs))
		b.WriteString("<ul>\n")
		for _, r := range refs {
			fmt.Fprintf(&b, `<li><a href="%s.html">%s</a></li>`+"\n",
				EscapeHTML(r.Path), EscapeHTML(r.Title))
		}
		b.WriteString("</ul>\n\n")
	}

	return []byte(b.String())
}

// GenerateTagsPage writes md/OMNGoTags.md again from the tag index and
// compiles html/OMNGoTags.html. It replaces both files, thus a second call
// does no harm. precompileAllPages calls it at start, and serveTagsPage calls
// it when the page is stale.
func (rd *Renderer) GenerateTagsPage(log logx.Logger) error {
	// A rebuild reads each note, and on a large collection that takes time.
	// From serveTagsPage, it runs inside a page navigation, where no progress
	// UI of the page can run. The two log lines show the wait on /api/logs.
	// The reader sees the ProgressBar of WebViewSetup and the delayed overlay
	// of omn-go-nav.js.
	log.Debugf("Rebuilding tags index")
	started := time.Now()
	index := rd.BuildTagIndex()
	content := renderTagsMarkdown(index)
	defer func() {
		log.Infof("Tags index rebuilt: %d tags in %s",
			len(index), time.Since(started).Round(time.Millisecond))
	}()

	mdRoot := rd.Layout.MD()
	if err := os.MkdirAll(mdRoot, 0755); err != nil {
		return fmt.Errorf("tags: mkdir md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(mdRoot, "OMNGoTags.md"), content, 0644); err != nil {
		return fmt.Errorf("tags: write md/OMNGoTags.md: %w", err)
	}
	if _, err := rd.RenderAndCache("OMNGoTags", content); err != nil {
		return fmt.Errorf("tags: cache OMNGoTags.html: %w", err)
	}
	return nil
}

// NewestNoteMtime answers the newest mtime of each md/**.md file AND of each
// directory above it. An add, a delete or a rename changes the mtime of the
// directory, and maybe of no file. It skips OMNGoTags.md and md/local. It
// uses stat only, and it answers the zero time when it cannot walk md/.
func (rd *Renderer) NewestNoteMtime() time.Time {
	mdRoot := rd.Layout.MD()
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

// TagsPageStale reports whether html/OMNGoTags.html needs a rebuild: a
// ?refresh, a missing file, or a note that is newer. serveHTMLPage makes the
// same test with one source. Here the source is each note.
func (rd *Renderer) TagsPageStale(forceRefresh bool) bool {
	if forceRefresh {
		return true
	}
	htmlStat, err := os.Stat(rd.Layout.PageHTML("OMNGoTags"))
	if err != nil {
		return true // missing or unreadable -> (re)generate
	}
	return rd.NewestNoteMtime().After(htmlStat.ModTime())
}
