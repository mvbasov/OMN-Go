package backend

import (
	"embed"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/html frontend/md
var staticFS embed.FS

// ExtractDefaultMarkdown ensures the welcome and initial markdown files exist.
func ExtractDefaultMarkdown() {
	entries, _ := staticFS.ReadDir("frontend/md")
	for _, e := range entries {
		target := filepath.Join(storageDir, "md", e.Name())
		if _, err := os.Stat(target); os.IsNotExist(err) {
			data, _ := staticFS.ReadFile("frontend/md/" + e.Name())
			os.MkdirAll(filepath.Dir(target), 0755)
			os.WriteFile(target, data, 0644)
		}
	}
}

// serveLazyEmbed extracts and serves static HTML/JS/CSS assets lazily.
func serveLazyEmbed(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	path = strings.TrimPrefix(path, "/")
	
	targetPath := filepath.Join(storageDir, "html", path)
	
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		data, err := staticFS.ReadFile("frontend/html/" + path)
		if err == nil {
			os.MkdirAll(filepath.Dir(targetPath), 0755)
			os.WriteFile(targetPath, data, 0644)
		}
	}

	ext := filepath.Ext(targetPath)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		switch ext {
		case ".css": mimeType = "text/css"
		case ".js": mimeType = "application/javascript"
		case ".html": mimeType = "text/html"
		default: mimeType = "text/plain"
		}
	}
	w.Header().Set("Content-Type", mimeType)

	if file, err := os.Open(targetPath); err == nil {
		defer file.Close()
		io.Copy(w, file)
	} else {
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
