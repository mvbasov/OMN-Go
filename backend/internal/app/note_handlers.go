package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/noteheader"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

func (a *App) getExternalEditPageBody(fileName string, viewURL string) string {
	view := render.ExternalEditView{
		Cmd:      a.config.Get().DesktopExtCmd,
		FileName: fileName,
		ViewURL:  viewURL,
	}
	return render.RenderExternalEditPage(view)
}

// resolveAndroidEditName makes the name for the omngo://edit intent.
// WebViewSetup picks md/ or html/ by the ".md" suffix alone, thus a page must
// become baseName + ".md". A file that is not a page keeps its name. It has
// no runtime.GOOS test, thus each platform can test it.
func resolveAndroidEditName(name, baseName string, isPage bool) string {
	if isPage {
		return baseName + ".md"
	}
	return name
}

func (a *App) handleEditExternal(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Missing name", http.StatusBadRequest)
		return
	}

	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)
	filePath := htmlPath
	if isPage {
		filePath = mdPath
	} else {
		// This route applies the two rules of serveEditor, because a client
		// can call it directly. First: no editor for a file that is not text.
		if !a.editableFileType(name) {
			a.serveNotEditable(w, r, name)
			return
		}
		// Second: put a shipped html/ asset on disk, or the external editor
		// opens a path with no file.
		a.materializeAsset("/" + filepath.ToSlash(name))
	}

	if runtime.GOOS == "android" {
		editName := resolveAndroidEditName(name, baseName, isPage)
		w.Header().Set("Location", "omngo://edit?name="+url.QueryEscape(editName))
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	var cmd *exec.Cmd
	cmdStr := strings.TrimSpace(a.config.Get().DesktopExtCmd)

	if cmdStr == "" {
		switch runtime.GOOS {
		case "linux":
			cmd = exec.Command("xdg-open", filePath)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
		case "darwin":
			cmd = exec.Command("open", filePath)
		}
	} else {
		parts := strings.Fields(cmdStr)
		if len(parts) > 0 {
			args := append(parts[1:], filePath)
			cmd = exec.Command(parts[0], args...)
		}
	}

	if cmd != nil {
		err := cmd.Start()
		if err != nil {
			a.log(logx.Edit).Errf("Failed to run external editor: %v", err)
		}
	} else {
		a.log(logx.Edit).Errf("Failed to run external editor: no command configured")
	}

	viewURL := name
	if isPage {
		viewURL = baseName + ".html"
	}
	header := fmt.Appendf(nil, "Title: Refresh %s\nDate: %s\nCategory: Action\nStylesheet: css/OMN-Go/omn-go-external.css\n\n",
		name, noteheader.Stamp(time.Now()))
	a.renderPage(w, http.StatusOK, name, header, a.getExternalEditPageBody(name, viewURL))
}

// resolveNewPageTarget resolves a new page name the same way as a link on the
// source page. A bare name is a sibling of source, a leading "/" is absolute,
// and a target with a directory stays. It first trims a stray slash at each
// end of source.
func (a *App) resolveNewPageTarget(source, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return target
	}

	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}

	if strings.Contains(target, "/") {
		return path.Clean(target)
	}

	source = strings.Trim(strings.TrimSpace(source), "/")
	if source == "" {
		return target
	}

	dir := path.Dir(source)
	if dir == "." {
		return target
	}
	return dir + "/" + target
}

// writeNoteFile is the only writer of a note source in package app. It makes
// the directory of md/<baseName>.md and writes content there.
func (a *App) writeNoteFile(baseName string, content []byte) error {
	mdPath := a.layout().MD(filepath.FromSlash(baseName + ".md"))
	if err := os.MkdirAll(filepath.Dir(mdPath), 0755); err != nil {
		return fmt.Errorf("make the directory of %q: %w", baseName, err)
	}
	if err := os.WriteFile(mdPath, content, 0644); err != nil {
		return fmt.Errorf("write %q: %w", baseName, err)
	}
	return nil
}

// writeNoteSource writes a note source and compiles its page. It answers
// the fault of the write. A compile fault goes to the log only: the HTML is
// a cache, and serveHTMLPage compiles the note again at the next view.
func (a *App) writeNoteSource(baseName string, content []byte) error {
	if err := a.writeNoteFile(baseName, content); err != nil {
		return err
	}
	if _, err := a.renderAndCache(baseName, content); err != nil {
		a.log(logx.Page).Errf("compile %q: %v", baseName, err)
	}
	return nil
}

// failSave logs the fault of a save and answers 500 with a general text. A
// caller on the LAN can read the answer, thus it holds no path.
func (a *App) failSave(w http.ResponseWriter, where string, err error) {
	a.log(logx.Page).Errf("%s: %v", where, err)
	http.Error(w, "Failed to save", http.StatusInternalServerError)
}

func (a *App) handleQuickNote(w http.ResponseWriter, r *http.Request) {
	note := r.FormValue("note")
	if note == "" {
		return
	}
	path := a.layout().MD("QuickNotes.md")
	// An absent note is correct: the first quick note makes it.
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		a.failSave(w, "handleQuickNote", err)
		return
	}
	lines := strings.Split(string(data), "\n")

	insertIdx := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" { // Find first blank line ending Pelican header
			insertIdx = i + 1
			break
		}
	}

	timestamp := noteheader.Stamp(time.Now())
	entry := fmt.Sprintf("\n---\n##### %s\n%s\n", timestamp, note)

	newContent := append(lines[:insertIdx], append([]string{entry}, lines[insertIdx:]...)...)
	fullMarkdown := strings.Join(newContent, "\n")
	fullMarkdown = a.ensureHeaderModified(fullMarkdown, "Quick Notes")
	if err := a.writeNoteSource("QuickNotes", []byte(fullMarkdown)); err != nil {
		a.failSave(w, "handleQuickNote", err)
		return
	}
	w.Write([]byte("Saved"))
}

func (a *App) handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")

	path := a.layout().MD("Bookmarks.md")
	timestamp := noteheader.Stamp(time.Now())

	tagsList := []string{}
	for t := range strings.SplitSeq(tags, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tagsList = append(tagsList, trimmed)
		}
	}
	// A semicolon separates notes, and a comma separates tags, the same as in
	// OMN. json.MarshalIndent below escapes '<', '>' and '&', thus a note
	// cannot close the <script> of Bookmarks.md.
	notesList := []string{}
	for n := range strings.SplitSeq(notes, ";") {
		if trimmed := strings.TrimSpace(n); trimmed != "" {
			notesList = append(notesList, trimmed)
		}
	}

	type BM struct {
		Date  string   `json:"date"`
		Url   string   `json:"url"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
		Notes []string `json:"notes"`
	}
	bm := BM{Date: timestamp, Url: url, Title: title, Tags: tagsList, Notes: notesList}
	bmJson, _ := json.MarshalIndent(bm, "  ", "  ")
	entry := "  " + string(bmJson) + ",\n"

	data, err := os.ReadFile(path)
	if err != nil {
		a.failSave(w, "handleBookmark", err)
		return
	}
	content := string(data)
	marker := "<!-- Don't edit body below this line -->"
	if !strings.Contains(content, marker) {
		a.failSave(w, "handleBookmark", fmt.Errorf("the note Bookmarks has no marker line %q", marker))
		return
	}
	newContent := strings.Replace(content, marker, marker+"\n"+entry, 1)
	newContent = a.ensureHeaderModified(newContent, "Incoming bookmarks")
	if err := a.writeNoteSource("Bookmarks", []byte(newContent)); err != nil {
		a.failSave(w, "handleBookmark", err)
		return
	}
	w.Write([]byte("Saved"))
}

func (a *App) handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Welcome"
	}

	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)

	if !isPage {
		// The editor reads its text here. Refuse each file that serveEditor
		// refuses, because a save through a textarea damages binary content.
		if !a.editableFileType(name) {
			a.serveNotEditable(w, r, name)
			return
		}
		data, err := os.ReadFile(htmlPath)
		if err != nil {
			// The file is not on disk yet. Extract it first. A 404 would open
			// an EMPTY editor, and the first Save would erase the shipped
			// content.
			if physPath, ok := a.materializeAsset("/" + strings.TrimPrefix(filepath.ToSlash(name), "/")); ok {
				data, err = os.ReadFile(physPath)
			}
		}
		if err != nil {
			// The file is not on disk and not embedded. Answer a 404 in plain
			// text. The editor shows it, and a save then makes the file. The
			// User Manual documents that use.
			a.serveNotFound(w, r)
			return
		}
		w.Write(data)
		return
	}

	data, err := os.ReadFile(mdPath)
	if err == nil {
		w.Write(data)
		return
	}

	// Not on disk yet. Use the embedded default or an empty page, and write
	// it, thus this runs one time for each page. Log a failed write. The data
	// in memory is still correct.
	embedPath := "md/" + baseName + ".md"
	if embedData, embedErr := frontend.Static.ReadFile(embedPath); embedErr == nil {
		data = embedData
	} else {
		data = []byte(noteheader.New(baseName, a.config.Get().Author, time.Now()))
	}

	if err := a.writeNoteFile(baseName, data); err != nil {
		a.log(logx.Page).Errf("handleGetNote: %v", err)
	}

	w.Write(data)
}

func (a *App) handleNewPage(w http.ResponseWriter, r *http.Request) {
	source := r.FormValue("source")
	target := r.FormValue("target")
	title := r.FormValue("title")

	if target == "" || title == "" {
		http.Error(w, "Missing fields", http.StatusBadRequest)
		return
	}

	// A bare target resolves relative to the directory of source, the same as
	// a bare relative link on that page. "test" from "local/local" is
	// "local/test".
	rawTarget := target
	// storage.ContainedName keeps each name in the md directory. See
	// internal/storage/paths.go.
	target = storage.ContainedName(a.resolveNewPageTarget(source, target))
	source = storage.ContainedName(source)

	// storage.FileExists and not os.Stat: a fault of the path is not a page.
	if !storage.FileExists(a.layout().MD(target + ".md")) {
		header := noteheader.New(title, a.config.Get().Author, time.Now())
		if err := a.writeNoteFile(target, []byte(header)); err != nil {
			a.log(logx.Page).Errf("handleNewPage: %v", err)
			http.Error(w, "Failed to create the page", http.StatusInternalServerError)
			return
		}
	}

	if source != "" {
		sourceMdPath := a.layout().MD(source + ".md")
		sourceData, err := os.ReadFile(sourceMdPath)
		if err == nil {
			content := string(sourceData)

			// The link in source must resolve to the new page. The browser
			// resolves a bare name relative to source, thus a bare rawTarget
			// stays as it is. A rawTarget with its own directory, or an
			// absolute one, gets a leading "/".
			linkHref := strings.TrimSpace(rawTarget)
			if strings.Contains(linkHref, "/") {
				linkHref = "/" + target
			}
			linkStr := fmt.Sprintf("* [%s](%s.html)", title, linkHref)

			// The new link goes below the header block, or at the top of a
			// note with no header. See package noteheader.
			hb := noteheader.Parse(content)
			if hb.HasHeader {
				if hb.Body != "" {
					content = hb.Header + "\n\n" + linkStr + "\n" + hb.Body
				} else {
					content = hb.Header + "\n\n" + linkStr + "\n"
				}
			} else {
				content = linkStr + "\n\n" + content
			}

			content = a.ensureHeaderModified(content, source)
			// The new page exists already. A link that the server cannot
			// write is a fault for the log, and not a failed request.
			if err := a.writeNoteSource(source, []byte(content)); err != nil {
				a.log(logx.Page).Errf("handleNewPage: %v", err)
			}
		}
	}

	w.Write([]byte(target))
}

func (a *App) handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		http.Error(w, "Missing name", http.StatusBadRequest)
		return
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")

	_, htmlPath, baseName, isPage := a.resolvePageName(name)

	if !isPage {
		// The last of the four guards, and the one that matters, because
		// this is the write. A crafted request can pass each guard above it.
		if !a.editableFileType(name) {
			a.serveNotEditable(w, r, name)
			return
		}
		if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
			a.log(logx.Page).Errf("handleSaveNote: mkdir failed for %q: %v", name, err)
			http.Error(w, "Failed to save", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(htmlPath, []byte(content), 0644); err != nil {
			a.log(logx.Page).Errf("handleSaveNote: write failed for %q: %v", name, err)
			http.Error(w, "Failed to save", http.StatusInternalServerError)
			return
		}
		// A text file beside a note also goes back to md/. Git sync carries
		// the md/ copy, and it must not go stale. See internal/storage/note_files.go.
		a.syncNoteFileToMD(htmlPath)
		w.Write([]byte("Saved"))
		return
	}

	content = a.ensureHeaderModified(content, baseName)

	if err := a.writeNoteSource(baseName, []byte(content)); err != nil {
		a.failSave(w, "handleSaveNote", err)
		return
	}
	w.Write([]byte("Saved"))
}
