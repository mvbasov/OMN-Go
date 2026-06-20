package backend

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func handleQuickNote(w http.ResponseWriter, r *http.Request) {
	entry := r.FormValue("entry")
	if entry == "" {
		return
	}
	path := filepath.Join(storageDir, "md", "QuickNotes.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(content), "\n")
	var newContent []string
	now := time.Now().Format("2006-01-02 15:04:05")
	inserted := false
	for _, l := range lines {
		if strings.HasPrefix(l, "## ") && !inserted {
			newContent = append(newContent, fmt.Sprintf("## %s", now))
			newContent = append(newContent, entry)
			newContent = append(newContent, "")
			inserted = true
		}
		newContent = append(newContent, l)
	}
	fullMarkdown := strings.Join(newContent, "\n")
	fullMarkdown = ensureHeaderModified(fullMarkdown, "Quick Notes")
	os.WriteFile(path, []byte(fullMarkdown), 0644)
	http.Redirect(w, r, "/QuickNotes.html", http.StatusSeeOther)
}

func handleBookmark(w http.ResponseWriter, r *http.Request) {
	entry := r.FormValue("entry")
	if entry == "" {
		return
	}
	path := filepath.Join(storageDir, "md", "Bookmarks.md")
	data, err := os.ReadFile(path)
	if err == nil {
		content := string(data)
		marker := "<!-- BOOKMARKS_START -->"
		if strings.Contains(content, marker) {
			newContent := strings.Replace(content, marker, marker+"\n"+entry, 1)
			newContent = ensureHeaderModified(newContent, "Incoming bookmarks")
			os.WriteFile(path, []byte(newContent), 0644)
		}
	}
	http.Redirect(w, r, "/Bookmarks.html", http.StatusSeeOther)
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()
	os.MkdirAll(filepath.Join(storageDir, "images"), 0755)
	dest, _ := os.Create(filepath.Join(storageDir, "images", header.Filename))
	defer dest.Close()
	io.Copy(dest, file)
	w.Write([]byte("/images/" + header.Filename))
}

func handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()
	os.MkdirAll(filepath.Join(storageDir, "user_json"), 0755)
	dest, _ := os.Create(filepath.Join(storageDir, "user_json", header.Filename))
	defer dest.Close()
	io.Copy(dest, file)
	w.Write([]byte("/user_json/" + header.Filename))
}

func handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		return
	}
	if name == "Config" {
		w.Write([]byte("Configuration is handled in the UI."))
		return
	}
	path := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))
	content, err := os.ReadFile(path)
	if err != nil {
		humanTitle := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		authorLine := ""
		if appConfig.Author != "" {
			authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
		}
		defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n", humanTitle, timestamp, authorLine)
		w.Write([]byte(defaultContent))
		return
	}
	w.Write(content)
}

func handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		return
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")

	var path string
	if !strings.Contains(name, ".") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {
		cleanName := strings.TrimSuffix(name, ".html")
		if !strings.HasSuffix(cleanName, ".md") {
			cleanName += ".md"
		}
		path = filepath.Join(storageDir, "md", filepath.Clean(cleanName))

		humanTitle := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSuffix(cleanName, ".md"), "-", " "), "_", " ")
		content = ensureHeaderModified(content, humanTitle)

		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)

		htmlPath := filepath.Join(storageDir, "html", strings.TrimSuffix(cleanName, ".md")+".html")
		os.MkdirAll(filepath.Dir(htmlPath), 0755)
		compiled := compilePage(strings.TrimSuffix(cleanName, ".md"), []byte(content))
		os.WriteFile(htmlPath, compiled, 0644)

	} else {
		path = filepath.Join(storageDir, "html", filepath.Clean(name))
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)
	}

	w.Write([]byte("Saved"))
}

func handleNewPage(w http.ResponseWriter, r *http.Request) {
	source := r.FormValue("source")
	target := r.FormValue("target")
	title := r.FormValue("title")

	if target == "" || title == "" {
		http.Error(w, "Missing fields", http.StatusBadRequest)
		return
	}

	targetMdPath := filepath.Join(storageDir, "md", target+".md")
	if _, err := os.Stat(targetMdPath); os.IsNotExist(err) {
		defaultContent := "<!-- OMN_GO_RAW_MD -->\n\n"
		os.MkdirAll(filepath.Dir(targetMdPath), 0755)
		os.WriteFile(targetMdPath, []byte(defaultContent), 0644)
	}

	if source != "" {
		sourceMdPath := filepath.Join(storageDir, "md", source+".md")
		sourceData, err := os.ReadFile(sourceMdPath)
		if err == nil {
			content := string(sourceData)
			linkStr := fmt.Sprintf("* [%s](%s)", title, target)
			parts := strings.SplitN(content, "\n\n", 2)

			isHeader := false
			if len(parts) > 0 && strings.Contains(parts[0], ":") {
				firstLine := strings.Split(parts[0], "\n")[0]
				if strings.Contains(firstLine, ":") && !strings.HasPrefix(firstLine, " ") && !strings.HasPrefix(firstLine, "#") {
					isHeader = true
				}
			}

			if isHeader {
				if len(parts) > 1 {
					content = parts[0] + "\n\n" + linkStr + "\n" + parts[1]
				} else {
					content = parts[0] + "\n\n" + linkStr + "\n"
				}
			} else {
				content = linkStr + "\n\n" + content
			}

			content = ensureHeaderModified(content, source)
			os.WriteFile(sourceMdPath, []byte(content), 0644)
			
			// Recompile Source HTML immediately
			htmlPath := filepath.Join(storageDir, "html", source+".html")
			compiled := compilePage(source, []byte(content))
			os.MkdirAll(filepath.Dir(htmlPath), 0755)
			os.WriteFile(htmlPath, compiled, 0644)
		}
	}

	w.Write([]byte("Created"))
}

func handleEditExternal(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		return
	}
	path := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))

	err := OpenExternalEditor(path)
	if err != nil {
		// Log silently or handle if necessary
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`
		<html>
		<head>
			<meta name="viewport" content="width=device-width, initial-scale=1.0">
			<style>
				body { font-family: sans-serif; display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; background: #f4f4f4; margin: 0; }
				button { padding: 15px 30px; font-size: 18px; background: #007bff; color: white; border: none; border-radius: 8px; cursor: pointer; }
			</style>
		</head>
		<body>
			<h2>Waiting for External Editor...</h2>
			<p>When you are finished saving, click below to return.</p>
			<button onclick="window.location.replace('/%s.html')">Return to OMN-Go</button>
		</body>
		</html>
	`, name)))
}
