package db

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"net.basov.omngo/backend/internal/render"
)

// ----------------------------------------------------------------------
// The HTTP endpoints and the page
// ----------------------------------------------------------------------

var dbBackupsPageTmpl = render.LoadTemplate("db_backups.html")

// ServeBackupsPage renders the Database Backups page. The button at the top
// of the Config page opens it. The page gets its data from GET
// /api/db/backups, thus the template needs no render.Fill().
func (svc Service) ServeBackupsPage(w http.ResponseWriter, r *http.Request) {
	svc.RenderPage(w, http.StatusOK, "DB_Backups", render.PageHeader("Database Backups", "Settings", "css/OMN-Go/omn-go-db.css"), dbBackupsPageTmpl)
}

// HandleBackupCreate answers POST /api/db/backup?db=NAME.
func (svc Service) HandleBackupCreate(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("db")
	if !dbNameRe.MatchString(name) {
		svc.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid db name %q", name))
		return
	}
	file, pruned, err := svc.CreateBackup(name)
	if err != nil {
		svc.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	svc.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"file":   file,
		"pruned": pruned,
	})
}

// HandleRestore answers POST /api/db/restore?db=NAME&file=FILENAME.
func (svc Service) HandleRestore(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("db")
	fileName := r.URL.Query().Get("file")
	if !dbNameRe.MatchString(name) {
		svc.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid db name %q", name))
		return
	}
	if !backupFileRe.MatchString(fileName) {
		svc.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid backup filename %q", fileName))
		return
	}
	if err := svc.Restore(name, fileName); err != nil {
		svc.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	svc.writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

type backupFileView struct {
	File     string `json:"file"`
	Size     int64  `json:"size"`
	MTime    string `json:"mtime"`
	Created  string `json:"created"`
	Hostname string `json:"hostname"`
	Objects  int    `json:"objects"`
	Rows     int    `json:"rows"`
	Valid    bool   `json:"valid"`
	Error    string `json:"error,omitempty"`
}

type backupDBView struct {
	Name         string           `json:"name"`
	SQLiteExists bool             `json:"sqlite_exists"`
	Size         int64            `json:"size"`
	MTime        string           `json:"mtime"`
	State        string           `json:"state"`
	Backups      []backupFileView `json:"backups"`
}

// HandleBackupList answers GET /api/db/backups with each value of the
// /db_backups page. It never opens a database, because an open can start the
// bootstrap restore, and a listing must change nothing.
func (svc Service) HandleBackupList(w http.ResponseWriter, r *http.Request) {

	// Take each database that has a .sqlite file or only backups, as on a
	// fresh device before the first open.
	names := map[string]bool{}
	if entries, err := os.ReadDir(svc.Layout.DB()); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sqlite") {
				n := strings.TrimSuffix(e.Name(), ".sqlite")
				if dbNameRe.MatchString(n) {
					names[n] = true
				}
			}
		}
	}
	if entries, err := os.ReadDir(svc.backupRoot()); err == nil {
		for _, e := range entries {
			if e.IsDir() && dbNameRe.MatchString(e.Name()) {
				names[e.Name()] = true
			}
		}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	depth := svc.PruneDepth
	if depth <= 0 {
		depth = 3
	}

	dbs := make([]backupDBView, 0, len(sorted))
	for _, name := range sorted {
		v := backupDBView{Name: name}
		// The raw mtime is for backupState below.
		var dbMTime time.Time
		if info, err := os.Stat(svc.UserDBPath(name)); err == nil && info.Size() > 0 {
			v.SQLiteExists = true
			v.Size = info.Size()
			dbMTime = info.ModTime()
			v.MTime = dbMTime.UTC().Format(time.RFC3339)
		}

		files, _ := svc.ListBackupFiles(name)
		var newestMTime time.Time
		var newestHeader backupHeader
		newestValid := false
		for i, fn := range files {
			full := filepath.Join(svc.BackupDir(name), fn)
			bv := backupFileView{File: fn}
			if info, err := os.Stat(full); err == nil {
				bv.Size = info.Size()
				bv.MTime = info.ModTime().UTC().Format(time.RFC3339)
				if i == 0 {
					newestMTime = info.ModTime()
				}
			}
			if h, err := ReadBackupHeader(full); err == nil && h.Database == name {
				bv.Valid = true
				bv.Created = h.Created
				bv.Hostname = h.Hostname
				bv.Objects = h.Objects
				bv.Rows = h.Rows
				if i == 0 {
					newestValid = true
					newestHeader = h
				}
			} else if err != nil {
				bv.Error = err.Error()
			} else {
				bv.Error = fmt.Sprintf("header names database %q", h.Database)
			}
			v.Backups = append(v.Backups, bv)
		}
		if v.Backups == nil {
			// A database with no backup must give "backups":[]. A nil slice
			// gives JSON null, and the page script fails on .length.
			v.Backups = []backupFileView{}
		}

		switch {
		case len(files) == 0:
			v.State = "none"
		case !newestValid:
			v.State = "invalid"
		case !v.SQLiteExists:
			v.State = "missing"
		default:
			v.State = backupState(newestHeader, newestMTime, dbMTime)
		}
		dbs = append(dbs, v)
	}

	svc.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "success",
		"hostname":    svc.Hostname,
		"prune_depth": depth,
		"databases":   dbs,
	})
}
