package backend

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// ----------------------------------------------------------------------
// Whole-database JSONL backups for user SQLite databases
// ----------------------------------------------------------------------
//
// Replaces the per-table db_json/ mirror (formerly sqlite_backup.go).
// That mechanism was automatic (export at push, mtime-triggered restore
// on open) and file-per-object, which made cross-device sync fragile:
// stale caches could be re-exported over freshly pulled data, fresh
// devices never restored at all, and dropped tables resurrected from
// orphaned files. This design is the opposite on every axis:
//
//   - MANUAL: backups are created and restored only from the /db_backups
//     page (or its /api/db/* endpoints). The one exception is bootstrap:
//     opening a database that has backups but no .sqlite file at all
//     restores the newest backup, because there is no local state a
//     restore could destroy.
//   - WHOLE-DATABASE: one backup = one self-contained .jsonl file
//     holding the header, every schema object (tables, indexes, views,
//     triggers), sqlite_sequence values and every row. A backup is
//     internally consistent by construction.
//   - IMMUTABLE: a backup file is written once (timestamped filename,
//     unique per device via the config Hostname) and never modified -
//     only pruned. Git therefore only ever sees clean adds and deletes;
//     two devices cannot conflict on the same path.
//   - FULL REPLACE on restore: the backup is loaded into a temporary
//     .sqlite file which is then atomically renamed over the real one
//     (after evicting the cached handle - the same eviction machinery
//     /api/sql already uses to self-heal from SQLITE_READONLY_DBMOVED).
//     Nothing from the old database can survive by accident.
//
// Layout: <StorageDir>/html/db_backup/<db>/<UTCtimestamp>_<hostname>.jsonl
// Living under html/ means backups are served (download links work in
// any real browser) and staged for git like every other tracked file;
// databases named local-* are kept out of git by a single .gitignore
// glob (see ensureGitignore in git_helper.go).
//
// File format (version 2), one JSON object per line:
//
//	{"format":"omn-db-backup","version":2,"database":"mydata",
//	 "created":"2026-07-14T10:30:00Z","hostname":"pixel7",
//	 "objects":4,"rows":123}
//	{"kind":"table","name":"t","sql":"CREATE TABLE ...","columns":["a","b"]}
//	{"kind":"index","name":"i1","sql":"CREATE INDEX ..."}
//	{"kind":"view","name":"v1","sql":"CREATE VIEW ..."}
//	{"kind":"seq","table":"t","value":41}
//	{"kind":"row","table":"t","v":[1,"x"]}
//	{"kind":"trigger","name":"tr1","table":"t","sql":"CREATE TRIGGER ..."}
//
// Values: numbers are written exactly (int64 survives via json.Number on
// read-back); BLOBs - and TEXT that isn't valid UTF-8 - are tagged as
// {"b64":"..."}; everything else is the natural JSON type. One line per
// row keeps git diffs minimal, and a git conflict marker anywhere in the
// file fails JSON parsing on a precise line instead of half-applying.

const (
	backupFormatName    = "omn-db-backup"
	backupFormatVersion = 2
	backupMaxLineBytes  = 10 << 20 // same 10MB row cap the old loader used
)

// backupFileRe validates backup filenames handed to the restore endpoint
// (they are used to build a path, so this is the traversal guard) and
// filters directory listings. Lexicographic order of matching names is
// chronological order because the timestamp leads.
var backupFileRe = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z(_[0-9]+)?_[A-Za-z0-9_-]{1,64}\.jsonl$`)

func dbBackupRoot(a *App) string {
	return filepath.Join(a.StorageDir, "html", "db_backup")
}
func (a *App) dbBackupDir(name string) string {
	return filepath.Join(dbBackupRoot(a), name)
}
func (a *App) userDBPath(name string) string {
	return filepath.Join(a.StorageDir, "db", name+".sqlite")
}

// relStoragePath converts an absolute path under StorageDir into the
// slash-separated, StorageDir-relative form git status/gitignore matching
// use throughout this codebase. (Carried over from the removed
// sqlite_backup.go - other code may rely on it.)
func (a *App) relStoragePath(full string) string {
	rel, err := filepath.Rel(a.StorageDir, full)
	if err != nil {
		return full
	}
	return filepath.ToSlash(rel)
}

// quoteIdent safely embeds a SQL identifier in a statement that can't use
// a placeholder for it (DDL, PRAGMA). Identifiers here always originate
// from sqlite_master or PRAGMA table_info - i.e. from CREATE statements
// SQLite itself already accepted - so this is defense in depth, not the
// primary trust boundary. (Carried over from the removed sqlite_backup.go.)
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// ---------------------------------------------------------------------
// File format model
// ---------------------------------------------------------------------

type backupHeader struct {
	Format   string `json:"format"`
	Version  int    `json:"version"`
	Database string `json:"database"`
	Created  string `json:"created"`
	Hostname string `json:"hostname"`
	Objects  int    `json:"objects"`
	Rows     int    `json:"rows"`
}

type backupLine struct {
	Kind    string        `json:"kind"`
	Name    string        `json:"name,omitempty"`
	Table   string        `json:"table,omitempty"`
	SQL     string        `json:"sql,omitempty"`
	Columns []string      `json:"columns,omitempty"`
	Value   int64         `json:"value,omitempty"`
	V       []interface{} `json:"v,omitempty"`
}

// ---------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------

// backupTableInfo returns column names in declaration order and, per
// column, whether it is declared BLOB (matching "BLOB" anywhere in the
// declared type, the same convention SQLite's affinity rules use).
func backupTableInfo(tx *sql.Tx, table string) (cols []string, blob []bool, err error) {
	rows, err := tx.Query(`PRAGMA table_info(` + quoteIdent(table) + `)`)
	if err != nil {
		return nil, nil, fmt.Errorf("table_info(%s): %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, nil, err
		}
		cols = append(cols, name)
		blob = append(blob, strings.Contains(strings.ToUpper(ctype), "BLOB"))
	}
	return cols, blob, rows.Err()
}

// encodeBackupValue maps a scanned SQLite value to its JSON form. []byte
// becomes {"b64":...} when the column is declared BLOB or the bytes are
// not valid UTF-8 (JSON cannot carry raw bytes); otherwise it is written
// as a plain string. Everything else passes through as its natural type.
func encodeBackupValue(v interface{}, blobCol bool) interface{} {
	b, ok := v.([]byte)
	if !ok {
		return v
	}
	if blobCol || !utf8.Valid(b) {
		return map[string]string{"b64": base64.StdEncoding.EncodeToString(b)}
	}
	return string(b)
}

// createDBBackup snapshots database name into a new timestamped backup
// file and prunes old backups beyond the configured depth. Returns the
// StorageDir-relative path of the new file and of every pruned file.
// The whole read runs inside one transaction, so the snapshot is
// consistent even while note scripts keep writing.
func (a *App) createDBBackup(name string) (created string, pruned []string, err error) {
	db, err := a.openUserDB(name)
	if err != nil {
		return "", nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return "", nil, fmt.Errorf("begin snapshot: %w", err)
	}
	defer tx.Rollback() // read-only tx; rollback is the cheap way out

	objRows, err := tx.Query(`
		SELECT type, name, tbl_name, sql FROM sqlite_master
		WHERE type IN ('table','view','trigger','index')
		  AND name NOT LIKE 'sqlite_%'
		ORDER BY type, name`)
	if err != nil {
		return "", nil, fmt.Errorf("list objects: %w", err)
	}
	type dbObj struct{ kind, name, table, sqlText string }
	var objs []dbObj
	for objRows.Next() {
		var o dbObj
		var sqlText sql.NullString
		if err := objRows.Scan(&o.kind, &o.name, &o.table, &sqlText); err != nil {
			objRows.Close()
			return "", nil, err
		}
		o.sqlText = sqlText.String
		if o.sqlText == "" {
			continue // implicit index (PK/UNIQUE) - recreated by CREATE TABLE itself
		}
		objs = append(objs, o)
	}
	objRows.Close()
	if err := objRows.Err(); err != nil {
		return "", nil, err
	}

	var body bytes.Buffer
	writeLine := func(v interface{}) error {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		body.Write(data)
		body.WriteByte('\n')
		return nil
	}

	objects, totalRows := 0, 0
	tables := map[string][]string{} // name -> columns, for the row pass
	blobs := map[string][]bool{}

	// Schema first: tables, then indexes, views, triggers. The restore
	// side regroups by kind anyway; this order just keeps the file
	// readable top-down.
	for _, phase := range []string{"table", "index", "view", "trigger"} {
		for _, o := range objs {
			if o.kind != phase {
				continue
			}
			line := backupLine{Kind: o.kind, Name: o.name, SQL: o.sqlText}
			if o.kind == "trigger" {
				line.Table = o.table
			}
			if o.kind == "table" {
				cols, blob, err := backupTableInfo(tx, o.name)
				if err != nil {
					return "", nil, err
				}
				line.Columns = cols
				tables[o.name] = cols
				blobs[o.name] = blob
			}
			if err := writeLine(line); err != nil {
				return "", nil, err
			}
			objects++
		}
	}

	// sqlite_sequence: present only if some table uses AUTOINCREMENT.
	// Preserving it means a restored database can never re-issue an id
	// that an older row (deleted or not) already used.
	if seqRows, err := tx.Query(`SELECT name, seq FROM sqlite_sequence`); err == nil {
		for seqRows.Next() {
			var tbl string
			var seq int64
			if err := seqRows.Scan(&tbl, &seq); err != nil {
				seqRows.Close()
				return "", nil, err
			}
			if err := writeLine(backupLine{Kind: "seq", Table: tbl, Value: seq}); err != nil {
				seqRows.Close()
				return "", nil, err
			}
		}
		seqRows.Close()
	}

	// Data: one line per row, in each table's natural (rowid) order.
	for _, o := range objs {
		if o.kind != "table" {
			continue
		}
		cols := tables[o.name]
		blob := blobs[o.name]
		quoted := make([]string, len(cols))
		for i, c := range cols {
			quoted[i] = quoteIdent(c)
		}
		dataRows, err := tx.Query(`SELECT ` + strings.Join(quoted, ",") + ` FROM ` + quoteIdent(o.name))
		if err != nil {
			return "", nil, fmt.Errorf("read %s: %w", o.name, err)
		}
		for dataRows.Next() {
			raw := make([]interface{}, len(cols))
			ptrs := make([]interface{}, len(cols))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if err := dataRows.Scan(ptrs...); err != nil {
				dataRows.Close()
				return "", nil, err
			}
			for i, v := range raw {
				raw[i] = encodeBackupValue(v, blob[i])
			}
			if err := writeLine(backupLine{Kind: "row", Table: o.name, V: raw}); err != nil {
				dataRows.Close()
				return "", nil, err
			}
			totalRows++
		}
		if err := dataRows.Err(); err != nil {
			dataRows.Close()
			return "", nil, err
		}
		dataRows.Close()
	}

	// Header goes first in the file but is built last, because it carries
	// the counts. Keeping it on line 1 lets the list endpoint show backup
	// metadata by reading a single line.
	cfg := a.GetConfig()
	host := sanitizeHostname(cfg.Hostname)
	if host == "" {
		host = defaultHostname()
	}
	header := backupHeader{
		Format:   backupFormatName,
		Version:  backupFormatVersion,
		Database: name,
		Created:  time.Now().UTC().Format(time.RFC3339),
		Hostname: host,
		Objects:  objects,
		Rows:     totalRows,
	}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", nil, err
	}

	dir := a.dbBackupDir(name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", nil, fmt.Errorf("create backup directory: %w", err)
	}

	// Timestamped name; on a same-second collision (two backups within
	// one second, or two devices sharing a hostname) insert a counter
	// rather than overwrite - backups are immutable by contract.
	stamp := time.Now().UTC().Format("20060102T150405") + "Z"
	fileName := stamp + "_" + host + ".jsonl"
	target := filepath.Join(dir, fileName)
	for n := 2; ; n++ {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			break
		}
		fileName = fmt.Sprintf("%s_%d_%s.jsonl", stamp, n, host)
		target = filepath.Join(dir, fileName)
	}

	tmp := target + ".tmp"
	out := append(append(headerBytes, '\n'), body.Bytes()...)
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return "", nil, fmt.Errorf("write backup: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", nil, fmt.Errorf("finalize backup: %w", err)
	}

	// The database now matches this backup exactly. Align the .sqlite
	// mtime with the backup file's so the page's state dot reads
	// "in sync" instead of inventing a phantom difference.
	if info, err := os.Stat(target); err == nil {
		if err := os.Chtimes(a.userDBPath(name), info.ModTime(), info.ModTime()); err != nil && !os.IsNotExist(err) {
			log.Printf("[db-backup] touch %s.sqlite: %v", name, err)
		}
	}

	pruned, err = a.pruneDBBackups(name)
	if err != nil {
		// The backup itself succeeded; a prune hiccup is not worth
		// failing the request over.
		log.Printf("[db-backup] prune %s: %v", name, err)
		err = nil
	}
	return a.relStoragePath(target), pruned, nil
}

// listBackupFiles returns the backup filenames for name, newest first.
func (a *App) listBackupFiles(name string) ([]string, error) {
	entries, err := os.ReadDir(a.dbBackupDir(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !backupFileRe.MatchString(e.Name()) {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files))) // timestamp prefix => lexicographic == chronological
	return files, nil
}

// pruneDBBackups removes backups beyond the configured depth (newest
// kept). Returns the StorageDir-relative paths of removed files - for
// tracked databases these deletions travel through git like any other
// removed file; for local-* databases they are final.
func (a *App) pruneDBBackups(name string) ([]string, error) {
	depth := a.GetConfig().BackupPruneDepth
	if depth <= 0 {
		depth = 3
	}
	files, err := a.listBackupFiles(name)
	if err != nil {
		return nil, err
	}
	var removed []string
	for i := depth; i < len(files); i++ {
		full := filepath.Join(a.dbBackupDir(name), files[i])
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			log.Printf("[db-backup] prune %s: %v", files[i], err)
			continue
		}
		log.Printf("[db-backup] %s: pruned %s", name, files[i])
		removed = append(removed, a.relStoragePath(full))
	}
	return removed, nil
}

// ---------------------------------------------------------------------
// Restore
// ---------------------------------------------------------------------

// readBackupHeader reads and validates only the first line of a backup
// file - cheap enough to run for every file on every page load.
func readBackupHeader(path string) (backupHeader, error) {
	var h backupHeader
	f, err := os.Open(path)
	if err != nil {
		return h, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64*1024)
	line, err := r.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return h, fmt.Errorf("empty backup file")
	}
	if err := json.Unmarshal(bytes.TrimSpace(line), &h); err != nil {
		return h, fmt.Errorf("bad header: %w", err)
	}
	if h.Format != backupFormatName || h.Version != backupFormatVersion {
		return h, fmt.Errorf("unsupported format %q version %d", h.Format, h.Version)
	}
	return h, nil
}

// decodeBackupValue reverses encodeBackupValue for one JSON value decoded
// with UseNumber: json.Number becomes int64 when exact (float64
// otherwise), {"b64":...} becomes []byte, everything else passes through.
func decodeBackupValue(v interface{}) (interface{}, error) {
	switch t := v.(type) {
	case json.Number:
		if iv, err := t.Int64(); err == nil {
			return iv, nil
		}
		if fv, err := t.Float64(); err == nil {
			return fv, nil
		}
		return t.String(), nil
	case map[string]interface{}:
		enc, ok := t["b64"].(string)
		if !ok || len(t) != 1 {
			return nil, fmt.Errorf("unrecognized tagged value %v", t)
		}
		return base64.StdEncoding.DecodeString(enc)
	default:
		return v, nil
	}
}

// restoreDBFromBackup replaces database name with the contents of backup
// fileName (a bare filename inside the database's backup directory). The
// backup is loaded into a temporary .sqlite file first and atomically
// renamed over the real one only after every statement succeeded - a
// half-parsed file (e.g. one damaged by git conflict markers) therefore
// changes nothing at all.
//
// Callers must hold a.dbRestoreMu (bootstrapIfMissing and handleDBRestore
// do); this function itself must not take it, so bootstrap can call it
// while already holding the lock.
func (a *App) restoreDBFromBackup(name, fileName string) error {
	if !dbNameRe.MatchString(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	if !backupFileRe.MatchString(fileName) {
		return fmt.Errorf("invalid backup filename %q", fileName)
	}
	backupPath := filepath.Join(a.dbBackupDir(name), fileName)

	f, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), backupMaxLineBytes)

	if !scanner.Scan() {
		return fmt.Errorf("empty backup file")
	}
	var header backupHeader
	if err := json.Unmarshal(bytes.TrimSpace(scanner.Bytes()), &header); err != nil {
		return fmt.Errorf("bad header: %w", err)
	}
	if header.Format != backupFormatName || header.Version != backupFormatVersion {
		return fmt.Errorf("unsupported format %q version %d", header.Format, header.Version)
	}
	if header.Database != name {
		return fmt.Errorf("backup is for database %q, not %q", header.Database, name)
	}

	// Parse everything up front (grouped by kind) before touching any
	// database file: a parse error on line N aborts with nothing built.
	var tables, indexes, views, triggers, seqs []backupLine
	type tableRows struct {
		cols []string
		rows [][]interface{}
	}
	data := map[string]*tableRows{}
	order := []string{}
	lineNum := 1
	for scanner.Scan() {
		lineNum++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var line backupLine
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&line); err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}
		switch line.Kind {
		case "table":
			tables = append(tables, line)
			data[line.Name] = &tableRows{cols: line.Columns}
			order = append(order, line.Name)
		case "index":
			indexes = append(indexes, line)
		case "view":
			views = append(views, line)
		case "trigger":
			triggers = append(triggers, line)
		case "seq":
			seqs = append(seqs, line)
		case "row":
			tr, ok := data[line.Table]
			if !ok {
				return fmt.Errorf("line %d: row for unknown table %q", lineNum, line.Table)
			}
			if len(line.V) != len(tr.cols) {
				return fmt.Errorf("line %d: %d values, expected %d", lineNum, len(line.V), len(tr.cols))
			}
			vals := make([]interface{}, len(line.V))
			for i, v := range line.V {
				dv, err := decodeBackupValue(v)
				if err != nil {
					return fmt.Errorf("line %d: %w", lineNum, err)
				}
				vals[i] = dv
			}
			tr.rows = append(tr.rows, vals)
		default:
			return fmt.Errorf("line %d: unknown kind %q", lineNum, line.Kind)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Build the replacement database in a temp file next to the real one.
	finalPath := a.userDBPath(name)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}
	tmpPath := finalPath + ".restoretmp"
	os.Remove(tmpPath)
	tmpDB, err := sql.Open("sqlite",
		"file:"+tmpPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(TRUNCATE)")
	if err != nil {
		return fmt.Errorf("open temp database: %w", err)
	}
	tmpDB.SetMaxOpenConns(1)
	cleanup := func() {
		tmpDB.Close()
		os.Remove(tmpPath)
	}

	tx, err := tmpDB.Begin()
	if err != nil {
		cleanup()
		return fmt.Errorf("begin restore: %w", err)
	}

	apply := func(kind string, lines []backupLine) error {
		for _, l := range lines {
			if _, err := tx.Exec(l.SQL); err != nil {
				return fmt.Errorf("create %s %s: %w", kind, l.Name, err)
			}
		}
		return nil
	}
	fail := func(err error) error {
		tx.Rollback()
		cleanup()
		return err
	}

	if err := apply("table", tables); err != nil {
		return fail(err)
	}
	if err := apply("view", views); err != nil {
		return fail(err)
	}
	for _, tbl := range order { // data before indexes (bulk-load faster) and triggers (must not fire)
		tr := data[tbl]
		if len(tr.rows) == 0 {
			continue
		}
		quoted := make([]string, len(tr.cols))
		for i, c := range tr.cols {
			quoted[i] = quoteIdent(c)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(tr.cols)), ",")
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			quoteIdent(tbl), strings.Join(quoted, ","), placeholders)
		for _, vals := range tr.rows {
			if _, err := tx.Exec(insertSQL, vals...); err != nil {
				return fail(fmt.Errorf("insert into %s: %w", tbl, err))
			}
		}
	}
	if err := apply("index", indexes); err != nil {
		return fail(err)
	}
	for _, s := range seqs {
		// sqlite_sequence rows already exist for AUTOINCREMENT tables
		// that received data above; overwrite with the saved counter so
		// deleted-then-never-reused ids stay never-reused.
		res, err := tx.Exec(`UPDATE sqlite_sequence SET seq = ? WHERE name = ?`, s.Value, s.Table)
		if err != nil {
			return fail(fmt.Errorf("restore sequence for %s: %w", s.Table, err))
		}
		if n, _ := res.RowsAffected(); n == 0 {
			if _, err := tx.Exec(`INSERT INTO sqlite_sequence(name, seq) VALUES (?, ?)`, s.Table, s.Value); err != nil {
				// No AUTOINCREMENT table -> no sqlite_sequence: the saved
				// counter has nothing to attach to; skip rather than fail.
				log.Printf("[db-restore] %s: sequence for %s not restorable: %v", name, s.Table, err)
			}
		}
	}
	if err := apply("trigger", triggers); err != nil {
		return fail(err)
	}

	if err := tx.Commit(); err != nil {
		cleanup()
		return fmt.Errorf("commit restore: %w", err)
	}
	if err := tmpDB.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp database: %w", err)
	}

	// Swap: evict the cached handle first so no open connection keeps
	// the doomed file's identity, then rename. Any /api/sql batch racing
	// this self-heals via its existing stale-handle retry.
	a.evictUserDB(name)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("swap database: %w", err)
	}

	// Align the .sqlite mtime with the restored backup's: the two are
	// content-identical right now, and the page's state dot reads exactly
	// that (older backups than the newest one will correctly show as
	// "newer backup exists").
	if info, err := os.Stat(backupPath); err == nil {
		os.Chtimes(finalPath, info.ModTime(), info.ModTime())
	}

	log.Printf("[db-restore] %s: restored from %s (%d objects, %d rows)",
		name, fileName, header.Objects, header.Rows)
	return nil
}

// bootstrapIfMissing is the single automatic exception to manual-only
// backups: when a database has at least one backup but no .sqlite file
// at all (fresh device right after a pull), the newest backup IS the
// database, and restoring it cannot destroy anything because there is no
// local state yet. Returns a fresh handle when a restore happened (the
// caller's handle was evicted by the swap), nil otherwise.
func (a *App) bootstrapIfMissing(name string) (*sql.DB, error) {
	a.dbRestoreMu.Lock()
	defer a.dbRestoreMu.Unlock()

	if info, err := os.Stat(a.userDBPath(name)); err == nil && info.Size() > 0 {
		return nil, nil
	}
	files, err := a.listBackupFiles(name)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	log.Printf("[db-bootstrap] %s: no database file yet, restoring newest backup %s", name, files[0])
	if err := a.restoreDBFromBackup(name, files[0]); err != nil {
		return nil, err
	}
	return a.openUserDBLocked(name)
}

// ---------------------------------------------------------------------
// HTTP endpoints + page
// ---------------------------------------------------------------------

var dbBackupsPageTmpl = loadTemplate("db_backups.html")

// serveDBBackupsPage renders the Database Backups admin page, reached
// from the button at the top of the Config page. All dynamic data comes
// from GET /api/db/backups client-side, so the template needs no fill().
func (a *App) serveDBBackupsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	compiled := a.compilePageWithBody("DB_Backups", []byte("Title: Database Backups\nCategory: Settings\n\n"), dbBackupsPageTmpl)
	w.Write(a.injectRuntimeVars(compiled))
}

func writeBackupJSON(w http.ResponseWriter, httpStatus int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[db-backup] encode response: %v", err)
	}
}

func backupErr(w http.ResponseWriter, httpStatus int, err error) {
	writeBackupJSON(w, httpStatus, map[string]string{"status": "error", "message": err.Error()})
}

// handleDBBackupCreate: POST /api/db/backup?db=NAME
func (a *App) handleDBBackupCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		backupErr(w, http.StatusMethodNotAllowed, fmt.Errorf("POST only"))
		return
	}
	name := r.URL.Query().Get("db")
	if !dbNameRe.MatchString(name) {
		backupErr(w, http.StatusBadRequest, fmt.Errorf("invalid db name %q", name))
		return
	}
	file, pruned, err := a.createDBBackup(name)
	if err != nil {
		backupErr(w, http.StatusInternalServerError, err)
		return
	}
	writeBackupJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"file":   file,
		"pruned": pruned,
	})
}

// handleDBRestore: POST /api/db/restore?db=NAME&file=FILENAME
func (a *App) handleDBRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		backupErr(w, http.StatusMethodNotAllowed, fmt.Errorf("POST only"))
		return
	}
	name := r.URL.Query().Get("db")
	fileName := r.URL.Query().Get("file")
	if !dbNameRe.MatchString(name) {
		backupErr(w, http.StatusBadRequest, fmt.Errorf("invalid db name %q", name))
		return
	}
	if !backupFileRe.MatchString(fileName) {
		backupErr(w, http.StatusBadRequest, fmt.Errorf("invalid backup filename %q", fileName))
		return
	}
	a.dbRestoreMu.Lock()
	err := a.restoreDBFromBackup(name, fileName)
	a.dbRestoreMu.Unlock()
	if err != nil {
		backupErr(w, http.StatusInternalServerError, err)
		return
	}
	writeBackupJSON(w, http.StatusOK, map[string]string{"status": "success"})
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

// handleDBBackupList: GET /api/db/backups - everything the /db_backups
// page needs, in one call. Deliberately read-only: it never opens a
// database (opening would trigger the bootstrap restore, and a listing
// must not mutate anything).
func (a *App) handleDBBackupList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		backupErr(w, http.StatusMethodNotAllowed, fmt.Errorf("GET only"))
		return
	}

	// Union of databases that have a .sqlite file and databases that only
	// have backups (fresh device before first open).
	names := map[string]bool{}
	if entries, err := os.ReadDir(filepath.Join(a.StorageDir, "db")); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sqlite") {
				n := strings.TrimSuffix(e.Name(), ".sqlite")
				if dbNameRe.MatchString(n) {
					names[n] = true
				}
			}
		}
	}
	if entries, err := os.ReadDir(dbBackupRoot(a)); err == nil {
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

	depth := a.GetConfig().BackupPruneDepth
	if depth <= 0 {
		depth = 3
	}

	dbs := make([]backupDBView, 0, len(sorted))
	for _, name := range sorted {
		v := backupDBView{Name: name}
		// Keep the raw mtime for the state comparison below: the RFC3339
		// string in v.MTime is second-precision, and comparing a re-parsed
		// (truncated) value against the backup file's nanosecond mtime
		// would misreport "backup newer" right after a backup, where
		// createDBBackup made the two mtimes exactly equal.
		var dbMTime time.Time
		if info, err := os.Stat(a.userDBPath(name)); err == nil && info.Size() > 0 {
			v.SQLiteExists = true
			v.Size = info.Size()
			dbMTime = info.ModTime()
			v.MTime = dbMTime.UTC().Format(time.RFC3339)
		}

		files, _ := a.listBackupFiles(name)
		var newestMTime time.Time
		newestValid := false
		for i, fn := range files {
			full := filepath.Join(a.dbBackupDir(name), fn)
			bv := backupFileView{File: fn}
			if info, err := os.Stat(full); err == nil {
				bv.Size = info.Size()
				bv.MTime = info.ModTime().UTC().Format(time.RFC3339)
				if i == 0 {
					newestMTime = info.ModTime()
				}
			}
			if h, err := readBackupHeader(full); err == nil && h.Database == name {
				bv.Valid = true
				bv.Created = h.Created
				bv.Hostname = h.Hostname
				bv.Objects = h.Objects
				bv.Rows = h.Rows
				if i == 0 {
					newestValid = true
				}
			} else if err != nil {
				bv.Error = err.Error()
			} else {
				bv.Error = fmt.Sprintf("header names database %q", h.Database)
			}
			v.Backups = append(v.Backups, bv)
		}

		switch {
		case len(files) == 0:
			v.State = "none"
		case !newestValid:
			v.State = "invalid"
		case !v.SQLiteExists:
			v.State = "missing"
		default:
			switch {
			case newestMTime.After(dbMTime):
				v.State = "backup_newer"
			case dbMTime.After(newestMTime):
				v.State = "dirty"
			default:
				v.State = "insync"
			}
		}
		dbs = append(dbs, v)
	}

	writeBackupJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "success",
		"hostname":    a.GetConfig().Hostname,
		"prune_depth": depth,
		"databases":   dbs,
	})
}
