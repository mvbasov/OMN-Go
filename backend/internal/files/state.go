package files

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/storage"
)

// note records the one fact that a directory row can show: the subtree is all
// shipped, or all made on the device.
func (d *filesDirRow) note(e filesEntry) {
	if e.device == nil {
		d.everyDevice = false
	} else {
		d.anyDevice = true
	}
	if e.ships == nil {
		d.everyShips = false
	} else {
		d.anyShips = true
		d.shipCount++
	}
}

// These are the five color classes. omn-go-core.css holds one token for each
// class, with a value for each theme.
const (
	filesColorApp     = "omn-files-c-app"     // the next version replaces this file
	filesColorAlert   = "omn-files-c-alert"   // ... and the device copy differs
	filesColorKeep    = "omn-files-c-keep"    // yours, kept
	filesColorDerived = "omn-files-c-derived" // made again when needed
	filesColorPlain   = "omn-files-c-plain"   // nothing at stake
)

// filesKindIcon answers the Material Icons ligature for the kind of a file.
// The bundled font css/fonts/material-icons.woff2 has each name here.
// "javascript", "css" and "html" draw as monograms.
func filesKindIcon(name string, isDir bool) string {
	if isDir {
		return "folder"
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "html"
	case ".md":
		return "article"
	case ".js":
		return "javascript"
	case ".css":
		return "css"
	case ".json", ".jsonl":
		return "data_object"
	case ".vcf":
		return "contacts"
	case ".ics", ".vcs":
		return "event"
	case ".txt":
		return "subject"
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp":
		return "image"
	case ".woff", ".woff2", ".ttf", ".otf":
		return "text_fields"
	}
	return "insert_drive_file"
}

// Editable decides whether a row offers an "edit" link. A compiled .html
// page has its own Edit button. A file that is not text gets no link. See
// config.EditableFileType. SVG is an image, thus it gets no link.
func (svc Service) Editable(logical string) bool {
	if strings.HasSuffix(strings.ToLower(logical), ".html") {
		return false
	}
	return config.EditableFileType(svc.MimeTypes, logical)
}

// isVersionDependent reports whether the app owns a path, from
// storage.VersionDependentAssets in internal/storage/assets.go. want is
// relative to the storage directory.
func isVersionDependent(want string) bool {
	for _, v := range storage.VersionDependentAssets {
		if v == want {
			return true
		}
	}
	return false
}

// filesSameBytes answers whether two copies are the same file. It reads the
// bytes only up to filesCompareMax. Above that, checked is false.
func filesSameBytes(embeddedLogical, diskPath string, size int64) (same bool, checked bool) {
	if size > filesCompareMax {
		return false, false
	}
	emb, err := frontend.Static.ReadFile(embeddedLogical)
	if err != nil {
		return false, false
	}
	disk, err := os.ReadFile(diskPath)
	if err != nil {
		return false, false
	}
	return bytes.Equal(emb, disk), true
}

// filesMirrorState describes the .txt pair of internal/storage/note_files.go.
// An equal size and mtime is the answer with no read, because
// storage.CopyFileWithTime copies the mtime. An older copy "waits for restart".
// A newer copy was "edited outside", and one save in the app editor copies it
// back. A pair that agrees says NOTHING.
func filesMirrorState(source, copyOf *indexedFile) (word, color string, extra string) {
	if source == nil || copyOf == nil {
		return "", "", ""
	}
	if source.size == copyOf.size && source.mod.Equal(copyOf.mod) {
		return "", "", ""
	}
	if copyOf.mod.After(source.mod) {
		return "edited outside", filesColorAlert, "save it once in the editor"
	}
	return "waits for restart", filesColorDerived, ""
}

// filesRowFor makes one row of the tree in view.
func (svc Service) filesRowFor(tree string, e filesEntry) filesFileRow {
	name := path.Base(e.path)
	row := filesFileRow{
		Name: name,
		Path: e.path,
		Kind: filesKindIcon(name, false),
		Size: filesSize(e.bytes()),
	}

	switch tree {
	case filesTreeBundled:
		row.AppOwned = isVersionDependent(filesStoragePath(tree, e.path))
		row.OwnerColor = filesColorApp
		// A starter note opens as its PAGE. This tree has no edit link,
		// because an edit changes the copy on the device, which has its own
		// row.
		if md, ok := strings.CutPrefix(e.path, "md/"); ok {
			row.URL = "/" + strings.TrimSuffix(md, ".md") + ".html"
		} else {
			row.URL = "/" + e.path
		}
		return row

	case filesTreeSource:
		row.URL = "/" + strings.TrimSuffix(e.path, ".md") + ".html"
		if !strings.HasSuffix(strings.ToLower(e.path), ".md") {
			row.URL = "/" + e.path
		}
		row.AppOwned = isVersionDependent(filesStoragePath(tree, e.path))
		if svc.Editable(e.path) {
			row.EditURL = row.URL + "?edit=true"
			if strings.HasSuffix(strings.ToLower(e.path), ".md") {
				// The page address opens the editor on the source, the same
				// as the Edit button of the page.
				row.EditURL = "/" + strings.TrimSuffix(e.path, ".md") + ".html?edit=true"
			}
		}
		if storage.IsLocalOnlyPath("md/"+e.path) || strings.HasPrefix(e.path, "local/") {
			row.Extra = append(row.Extra, "local only")
		}

	default: // served
		row.URL = "/" + e.path
		row.AppOwned = isVersionDependent(filesStoragePath(tree, e.path))
		if svc.Editable(e.path) {
			row.EditURL = row.URL + "?edit=true"
		}
	}

	svc.filesState(tree, e, &row)
	return row
}

// filesStoragePath maps a logical path of one tree to the form of
// storage.VersionDependentAssets.
func filesStoragePath(tree, logical string) string {
	switch tree {
	case filesTreeSource:
		return "md/" + logical
	case filesTreeBundled:
		if strings.HasPrefix(logical, "md/") {
			return logical
		}
		return "html/" + logical
	}
	return "html/" + logical
}

// filesEmbeddedPath maps a logical path to its path in frontend.Static.
func filesEmbeddedPath(tree, logical string) string {
	if tree == filesTreeSource {
		return "md/" + logical
	}
	if strings.HasPrefix(logical, "md/") {
		return logical
	}
	return "html/" + logical
}

// filesState sets the word of the first line, its color and the other facts.
// Here most rows get no word at all.
func (svc Service) filesState(tree string, e filesEntry, row *filesFileRow) {
	if e.device != nil {
		row.Mod = e.device.mod.UTC().Format("2006-01-02")
		row.ModFull = e.device.mod.UTC().Format("2006-01-02 15:04 UTC")
	}
	row.OwnerColor = filesColorApp

	switch {
	case e.ships != nil && e.device == nil:
		// The file ships, and no request asked for it yet.
		row.State, row.StateColor = "not extracted", filesColorPlain
		if row.AppOwned {
			row.StateColor = filesColorApp
		}
		row.Size = filesSize(e.ships.size)
		row.Mod, row.ModFull = "", ""

	case e.ships != nil && e.device != nil:
		same, checked := true, true
		if e.ships.size != e.device.size {
			same = false
		} else {
			same, checked = filesSameBytes(filesEmbeddedPath(tree, e.path), svc.filesDiskPath(tree, e.path), e.device.size)
		}
		switch {
		case !checked && e.ships.size == e.device.size:
			row.State, row.StateColor = "same size", filesColorPlain
		case same:
			// The row stays SILENT, because the copy here equals the build.
			// "app-owned" on the second line still tells what the next
			// version does to the file.
		default:
			row.State, row.StateColor = "changed here", filesColorKeep
			if row.AppOwned {
				row.StateColor = filesColorAlert
				row.OwnerColor = filesColorAlert
			}
			// Show both sizes only when they READ differently. Two files that
			// differ by one line can both show "68 KB".
			if from, to := filesSize(e.ships.size), filesSize(e.device.size); from != to {
				row.Size = from + " → " + to
			}
		}

	default:
		// A file only on the device says NOTHING. The .txt pair of
		// internal/storage/note_files.go is the exception, when the two copies
		// differ.
		if !storage.IsSyncedNoteFile(e.path) {
			return
		}
		var source, copyOf *indexedFile
		switch tree {
		case filesTreeServed:
			source, copyOf = svc.filesStat("md", e.path), e.device
		case filesTreeSource:
			source, copyOf = e.device, svc.filesStat("html", e.path)
		}
		word, color, extra := filesMirrorState(source, copyOf)
		if word == "" {
			return
		}
		row.State, row.StateColor = word, color
		if extra != "" {
			row.Extra = append(row.Extra, extra)
		}
	}
}

// filesDiskPath answers the disk path of a logical path in the tree in view.
func (svc Service) filesDiskPath(tree, logical string) string {
	sub := "html"
	if tree == filesTreeSource {
		sub = "md"
	}
	return svc.Layout.File(sub, filepath.FromSlash(logical))
}

// filesStat reads one file of the storage tree for the .txt comparison.
func (svc Service) filesStat(sub, logical string) *indexedFile {
	st, err := os.Stat(svc.Layout.File(sub, filepath.FromSlash(logical)))
	if err != nil || st.IsDir() {
		return nil
	}
	return &indexedFile{path: logical, size: st.Size(), mod: st.ModTime()}
}

// filesFromTheApp is the end of the word that a directory row can show.
// filesLegend must find the same text, thus it is a constant.
const filesFromTheApp = "from the app"
