package backend

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ----------------------------------------------------------------------
// Git-tracked JSON backup for user SQLite databases
// ----------------------------------------------------------------------
//
// The .sqlite files under <StorageDir>/db/ are binary and gitignored - git
// diffs on them are meaningless and merging two devices' changes is not
// possible. This file adds a git-friendly JSON mirror that IS meant to be
// tracked, following the same source/cache split this app already uses
// for pages: .md is the tracked source, .html is a regenerable cache
// rebuilt whenever the source is newer (serveHTMLPage's mtime check).
// Here the roles are the mirror image: JSON is the tracked source, the
// .sqlite file is the regenerable cache.
//
// Layout: <StorageDir>/html/db_json/<db>_<object>_sch.json (schema; every
// table, view and trigger gets one) and ..._data.json (JSONL, one compact
// JSON array per line, tables only). Living under html/ means these files
// are served and staged for git exactly like everything else already
// under html/ - no new sync/staging plumbing needed.
//
// A database whose name starts with "local-" is treated identically by
// every function in this file; what makes it local-only is a single
// .gitignore glob (see ensureGitignore in git_helper.go) matching its
// exported filenames, so it's still backed up on disk but never staged
// or pushed. No special-case code here - one glob line does the whole
// job, the same way config.json's protection is a name check while
// git-ignored files rely purely on the matcher.
//
// BLOB columns are NOT supported: JSON has no binary type, and this layer
// intentionally does not invent a base64 convention - a note script that
// needs BLOB round-tripping can base64-encode/decode at the JS level and
// store the result in a TEXT column, which this layer handles natively.
// A table containing any BLOB column is skipped (schema and data), logged
// as an error, and export continues with the rest of that database's
// objects - one unsupported table should not block backing up everything
// else.
//
// Object-name collisions after filename sanitization (two objects whose
// sanitized names collide) are also logged and skipped, same philosophy.
//
// Restore is always a full replace, never a merge: DROP + CREATE + bulk
// INSERT inside one transaction. Row-level merging was considered and
// rejected - not every table has a clean primary key, and a silent
// partial merge is worse than an honest full replace (the same risk
// profile Force Pull already has for notes, and this restore surfaces it
// the same way: as a clearly-stated tradeoff, not hidden behind clever
// logic that fails unpredictably).
//
// Whole-database restore is a flat, fixed-order pass - CREATE TABLE (all)
// -> CREATE VIEW (all) -> data load (all tables) -> CREATE TRIGGER (all) -
// not a dependency graph. Triggers must load after data or a bulk INSERT
// fires every AFTER INSERT trigger as if it were live traffic; a view
// that depends on another view (not a table) can still fail if the two
// land in the "wrong" relative order within the view phase - a known v1
// limitation, not a bug.
//
// Single-object restore (?table=<name>) is STRICT: it restores exactly
// that one table, view or trigger and nothing else. A restored table's
// triggers are not automatically re-created - if a table and its triggers
// must move together, restore the whole database (omit ?table=).

// sanitizeObjectName maps an arbitrary SQL identifier to a filename-safe
// string. Used only for object names (table/view/trigger); database names
// are already filename-safe by construction (dbNameRe).
var unsafeFilenameCharsRe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func sanitizeObjectName(s string) string {
	return unsafeFilenameCharsRe.ReplaceAllString(s, "_")
}

// quoteIdent safely embeds a SQL identifier (table/column/etc. name) in a
// statement that can't use a placeholder for it (DDL, PRAGMA). Identifiers
// here always originate from sqlite_master or PRAGMA table_info - i.e.
// from CREATE statements SQLite itself already accepted - so this is
// defense in depth, not the primary trust boundary.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func dbJSONDir(a *App) string {
	return filepath.Join(a.StorageDir, "html", "db_json")
}

// relStoragePath converts an absolute path under StorageDir into the
// slash-separated, StorageDir-relative form git status/gitignore matching
// use throughout this codebase (see e.g. storage.go's precompileAllPages).
func (a *App) relStoragePath(full string) string {
	rel, err := filepath.Rel(a.StorageDir, full)
	if err != nil {
		return full
	}
	return filepath.ToSlash(rel)
}

// ---------------------------------------------------------------------
// Schema file model
// ---------------------------------------------------------------------

type schemaFile struct {
	Version  int      `json:"version"`
	Database string   `json:"database"`
	Object   string   `json:"object"`
	Kind     string   `json:"kind"`            // "table" | "view" | "trigger"
	Table    string   `json:"table,omitempty"` // triggers only: the table they're on
	SQL      string   `json:"sql"`
	Columns  []string `json:"columns,omitempty"` // tables only
}

func schemaFileName(db, object string) string {
	return db + "_" + sanitizeObjectName(object) + "_sch.json"
}
func dataFileName(db, object string) string {
	return db + "_" + sanitizeObjectName(object) + "_data.json"
}

// ---------------------------------------------------------------------
// Discovering database objects
// ---------------------------------------------------------------------

type dbObject struct {
	Kind  string // "table" | "view" | "trigger"
	Name  string
	Table string // trigger's owning table (sqlite_master.tbl_name); "" otherwise
	SQL   string
}

func listDatabaseObjects(db *sql.DB) ([]dbObject, error) {
	rows, err := db.Query(`
		SELECT type, name, tbl_name, sql FROM sqlite_master
		WHERE type IN ('table','view','trigger')
		  AND name NOT LIKE 'sqlite_%'
		ORDER BY type, name`)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	defer rows.Close()

	var out []dbObject
	for rows.Next() {
		var o dbObject
		var sqlText sql.NullString
		if err := rows.Scan(&o.Kind, &o.Name, &o.Table, &sqlText); err != nil {
			return nil, err
		}
		o.SQL = sqlText.String // NULL for implicit rowid-table indexes etc.; harmless here since we filter type above
		if o.Kind != "trigger" {
			o.Table = ""
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// tableColumns returns column names in declaration order, and whether any
// column is declared BLOB (case-insensitively, matching "BLOB" anywhere in
// the declared type, same convention SQLite's own type affinity rules
// use).
func tableColumns(db *sql.DB, table string) (cols []string, hasBlob bool, err error) {
	rows, err := db.Query(`PRAGMA table_info(` + quoteIdent(table) + `)`)
	if err != nil {
		return nil, false, fmt.Errorf("table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, false, err
		}
		cols = append(cols, name)
		if strings.Contains(strings.ToUpper(ctype), "BLOB") {
			hasBlob = true
		}
	}
	return cols, hasBlob, rows.Err()
}

// ---------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------

// writeIfChanged compares data against what's already on disk at path and
// writes it (atomically, via temp file + rename) only when write is true
// AND the content actually differs. Returns whether it differs (would
// change), independent of write - this is what makes a dry run (write=
// false) side-effect-free while still reporting accurate "would change"
// results, and what makes "commit canceled -> JSON unchanged" free: a
// canceled push never calls this with write=true, so nothing was ever
// touched to begin with.
func writeIfChanged(path string, data []byte, write bool) bool {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return false
	}
	if !write {
		return true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("[db-export] mkdir for %s: %v", path, err)
		return true
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		log.Printf("[db-export] write %s: %v", tmp, err)
		return true
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("[db-export] rename %s -> %s: %v", tmp, path, err)
	}
	return true
}

// buildTableDataJSONL queries every row of table (in cols order) and
// returns one compact JSON array per line - deliberately not one big
// multi-line array, so a git diff after a single row change touches one
// line instead of rewriting the whole file.
func buildTableDataJSONL(db *sql.DB, table string, cols []string) ([]byte, error) {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}
	rows, err := db.Query(`SELECT ` + strings.Join(quoted, ",") + ` FROM ` + quoteIdent(table))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", table, err)
	}
	defer rows.Close()

	var buf bytes.Buffer
	for rows.Next() {
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		// TEXT columns come back as []byte from the driver same as BLOB
		// would - safe to always stringify here since tables containing
		// an actual BLOB column were already excluded by the caller.
		for i, v := range raw {
			if b, ok := v.([]byte); ok {
				raw[i] = string(b)
			}
		}
		line, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), rows.Err()
}

// exportDatabase writes (write=true) or dry-runs (write=false) the JSON
// mirror for one database. object, if non-empty, restricts export to
// that single object (matching /api/db/export?table=); empty exports
// everything sqlite_master reports. Returns the StorageDir-relative paths
// of every file that changed or (dry run) would change.
func (a *App) exportDatabase(db *sql.DB, name, object string, write bool) ([]string, error) {
	objs, err := listDatabaseObjects(db)
	if err != nil {
		return nil, err
	}

	dir := dbJSONDir(a)
	var changed []string
	seenBase := map[string]string{} // sanitized base -> first object name using it, for collision detection

	for _, o := range objs {
		if object != "" && o.Name != object {
			continue
		}

		base := sanitizeObjectName(o.Name)
		if prev, dup := seenBase[base]; dup {
			log.Printf("[db-export] %s: %q and %q both sanitize to %q - skipping %q",
				name, prev, o.Name, base, o.Name)
			continue
		}
		seenBase[base] = o.Name

		sf := schemaFile{Version: 1, Database: name, Object: o.Name, Kind: o.Kind, SQL: o.SQL}
		if o.Kind == "trigger" {
			sf.Table = o.Table
		}

		if o.Kind == "table" {
			cols, hasBlob, err := tableColumns(db, o.Name)
			if err != nil {
				return nil, err
			}
			if hasBlob {
				log.Printf("[db-export] %s.%s: table has a BLOB column, which JSON export does not support - skipping (encode BLOBs to TEXT at the JS level if you need them backed up)", name, o.Name)
				continue
			}
			sf.Columns = cols
		}

		schBytes, err := json.Marshal(sf)
		if err != nil {
			return nil, err
		}
		schPath := filepath.Join(dir, schemaFileName(name, o.Name))
		if writeIfChanged(schPath, schBytes, write) {
			changed = append(changed, a.relStoragePath(schPath))
		}

		if o.Kind != "table" {
			continue
		}
		dataBytes, err := buildTableDataJSONL(db, o.Name, sf.Columns)
		if err != nil {
			return nil, err
		}
		dataPath := filepath.Join(dir, dataFileName(name, o.Name))
		if writeIfChanged(dataPath, dataBytes, write) {
			changed = append(changed, a.relStoragePath(dataPath))
		}
	}
	return changed, nil
}

// allDatabaseNames lists every database that currently has a .sqlite file
// on disk (not just ones already opened this process), so export-at-push
// picks up databases from a previous run too.
func (a *App) allDatabaseNames() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(a.StorageDir, "db"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sqlite") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".sqlite"))
	}
	return names, nil
}

// exportAllDatabases exports every database found on disk. One database
// failing to open or export is logged and skipped rather than aborting
// the rest - called from syncPush, where a single bad database must not
// block pushing everything else.
func (a *App) exportAllDatabases(write bool) ([]string, error) {
	names, err := a.allDatabaseNames()
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, name := range names {
		db, err := a.openUserDBLocked(name)
		if err != nil {
			log.Printf("[db-export] open %s: %v", name, err)
			continue
		}
		files, err := a.exportDatabase(db, name, "", write)
		if err != nil {
			log.Printf("[db-export] %s: %v", name, err)
			continue
		}
		changed = append(changed, files...)
	}
	return changed, nil
}

// ---------------------------------------------------------------------
// Restore
// ---------------------------------------------------------------------

// loadTableDataJSONL reads a JSONL data file and INSERTs every row into
// table within tx. A missing data file is not an error (a table can
// legitimately have zero rows and therefore no meaningful data file).
func loadTableDataJSONL(tx *sql.Tx, table string, cols []string, path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteIdent(table), strings.Join(quoted, ","), placeholders)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10<<20) // allow rows up to 10MB
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var row []interface{}
		if err := json.Unmarshal(line, &row); err != nil {
			return fmt.Errorf("%s line %d: %w", filepath.Base(path), lineNum, err)
		}
		if len(row) != len(cols) {
			return fmt.Errorf("%s line %d: %d values, expected %d", filepath.Base(path), lineNum, len(row), len(cols))
		}
		if _, err := tx.Exec(insertSQL, row...); err != nil {
			return fmt.Errorf("%s line %d: insert into %s: %w", filepath.Base(path), lineNum, table, err)
		}
	}
	return scanner.Err()
}

// readSchemaFilesForDB loads every *_sch.json in db_json/ that belongs to
// name (and, if object is non-empty, matches that object exactly - see
// the file-top comment on why single-object restore is strict).
func (a *App) readSchemaFilesForDB(name, object string) ([]schemaFile, error) {
	dir := dbJSONDir(a)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	prefix := name + "_"
	var out []schemaFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), "_sch.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var sf schemaFile
		if err := json.Unmarshal(raw, &sf); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		if sf.Database != name {
			continue // defensive; the filename prefix should already guarantee this
		}
		if object != "" && sf.Object != object {
			continue
		}
		out = append(out, sf)
	}
	return out, nil
}

// restoreDatabase replaces (never merges) database objects from their
// JSON backup. object == "" restores everything found (flat, fixed order:
// tables, views, data, triggers - see file-top comment); a non-empty
// object restores exactly that one table/view/trigger and nothing else.
func (a *App) restoreDatabase(db *sql.DB, name, object string) error {
	schemas, err := a.readSchemaFilesForDB(name, object)
	if err != nil {
		return err
	}
	if len(schemas) == 0 {
		if object != "" {
			return fmt.Errorf("no backup found for %q in database %q", object, name)
		}
		return fmt.Errorf("no backup found for database %q", name)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op once Commit has succeeded

	for _, sf := range schemas { // 1: tables
		if sf.Kind != "table" {
			continue
		}
		if _, err := tx.Exec(`DROP TABLE IF EXISTS ` + quoteIdent(sf.Object)); err != nil {
			return fmt.Errorf("drop table %s: %w", sf.Object, err)
		}
		if _, err := tx.Exec(sf.SQL); err != nil {
			return fmt.Errorf("create table %s: %w", sf.Object, err)
		}
	}
	for _, sf := range schemas { // 2: views
		if sf.Kind != "view" {
			continue
		}
		if _, err := tx.Exec(`DROP VIEW IF EXISTS ` + quoteIdent(sf.Object)); err != nil {
			return fmt.Errorf("drop view %s: %w", sf.Object, err)
		}
		if _, err := tx.Exec(sf.SQL); err != nil {
			return fmt.Errorf("create view %s: %w", sf.Object, err)
		}
	}
	for _, sf := range schemas { // 3: data (after tables/views, before triggers)
		if sf.Kind != "table" {
			continue
		}
		dataPath := filepath.Join(dbJSONDir(a), dataFileName(name, sf.Object))
		if err := loadTableDataJSONL(tx, sf.Object, sf.Columns, dataPath); err != nil {
			return fmt.Errorf("load data for %s: %w", sf.Object, err)
		}
	}
	for _, sf := range schemas { // 4: triggers (last, so bulk data load above doesn't fire them)
		if sf.Kind != "trigger" {
			continue
		}
		if _, err := tx.Exec(`DROP TRIGGER IF EXISTS ` + quoteIdent(sf.Object)); err != nil {
			return fmt.Errorf("drop trigger %s: %w", sf.Object, err)
		}
		if _, err := tx.Exec(sf.SQL); err != nil {
			return fmt.Errorf("create trigger %s: %w", sf.Object, err)
		}
	}

	return tx.Commit()
}

// restoreIfStale restores name from its JSON backup when the backup is
// newer than the on-disk .sqlite file - the mirror image of
// serveHTMLPage's .md-newer-than-.html check. Called from openUserDB
// after its lock is released (restoreDatabase runs a transaction, and
// must never run while a.sqlMu is held).
func (a *App) restoreIfStale(db *sql.DB, name string) error {
	sqliteInfo, err := os.Stat(filepath.Join(a.StorageDir, "db", name+".sqlite"))
	if err != nil {
		return nil // no .sqlite file yet - nothing to compare against
	}

	entries, err := os.ReadDir(dbJSONDir(a))
	if err != nil {
		return nil // no backups directory at all - nothing to restore from
	}

	prefix := name + "_"
	newer := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(sqliteInfo.ModTime()) {
			newer = true
			break
		}
	}
	if !newer {
		return nil
	}

	log.Printf("[db-restore] %s: backup JSON is newer than the database, restoring", name)
	return a.restoreDatabase(db, name, "")
}

// ---------------------------------------------------------------------
// Manual force endpoints: POST /api/db/export|restore?db=NAME[&table=OBJ]
// ---------------------------------------------------------------------
//
// Not exposed anywhere in the UI by design (per the feature's requirements) -
// callable from a note's own <script> block (see db.exportBackup() /
// db.restoreBackup() in omn-go-sse.js) to add a manual backup/restore
// button to a specific page, or via a bare fetch()/curl for scripting.
// Admin-gated like /api/sql: this reads or replaces database content.

func dbAndObjectParams(r *http.Request) (name, object string, err error) {
	name = r.URL.Query().Get("db")
	if name == "" {
		return "", "", fmt.Errorf("missing db parameter")
	}
	if !dbNameRe.MatchString(name) {
		return "", "", fmt.Errorf("invalid db name %q", name)
	}
	return name, r.URL.Query().Get("table"), nil
}

func (a *App) handleDBExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	name, object, err := dbAndObjectParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db, err := a.openUserDBLocked(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	changed, err := a.exportDatabase(db, name, object, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "success",
		"changed_files": changed,
	})
}

func (a *App) handleDBRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	name, object, err := dbAndObjectParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db, err := a.openUserDBLocked(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.restoreDatabase(db, name, object); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
