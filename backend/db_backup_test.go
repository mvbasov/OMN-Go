package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Replaces sqlite_backup_test.go: that suite tested the deleted per-table
// db_json mirror; this one covers the whole-database JSONL engine in
// db_backup.go with the same intents (round trip, full replace, trigger
// safety, strictness/validation, endpoints) plus the new format's
// specific guarantees (indexes, sqlite_sequence, BLOBs, int64 fidelity,
// prune, fresh-device bootstrap, damaged-file rejection).
//
// All helpers here are dbb-prefixed to avoid colliding with helpers in
// the package's other _test.go files.

func dbbApp(t *testing.T) *App {
	t.Helper()
	return &App{StorageDir: t.TempDir()}
}

func dbbExec(t *testing.T, a *App, db, stmt string, args ...interface{}) {
	t.Helper()
	h, err := a.openUserDB(db)
	if err != nil {
		t.Fatalf("open %s: %v", db, err)
	}
	if _, err := h.Exec(stmt, args...); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

func dbbQueryInt(t *testing.T, a *App, db, query string) int64 {
	t.Helper()
	h, err := a.openUserDB(db)
	if err != nil {
		t.Fatalf("open %s: %v", db, err)
	}
	var n int64
	if err := h.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

// dbbBackup creates a backup and returns the bare backup filename.
func dbbBackup(t *testing.T, a *App, db string) string {
	t.Helper()
	rel, _, err := a.createDBBackup(db)
	if err != nil {
		t.Fatalf("createDBBackup(%s): %v", db, err)
	}
	return filepath.Base(rel)
}

func dbbRestore(t *testing.T, a *App, db, file string) {
	t.Helper()
	a.dbRestoreMu.Lock()
	err := a.restoreDBFromBackup(db, file)
	a.dbRestoreMu.Unlock()
	if err != nil {
		t.Fatalf("restoreDBFromBackup(%s, %s): %v", db, file, err)
	}
}

func TestDBBackupRoundTripSchemaAndData(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE items(id INTEGER PRIMARY KEY, txt TEXT, num REAL)`)
	dbbExec(t, a, "t1", `CREATE INDEX idx_items_txt ON items(txt)`)
	dbbExec(t, a, "t1", `CREATE VIEW v_items AS SELECT txt FROM items WHERE num > 1`)
	dbbExec(t, a, "t1", `INSERT INTO items VALUES (1, 'hello; "world"', 2.5)`)
	dbbExec(t, a, "t1", `INSERT INTO items VALUES (2, ?, 0.5)`, "unicode: привет ✓\nsecond line")

	file := dbbBackup(t, a, "t1")

	// Wreck the live data, then restore.
	dbbExec(t, a, "t1", `DELETE FROM items`)
	dbbExec(t, a, "t1", `DROP INDEX idx_items_txt`)
	dbbRestore(t, a, "t1", file)

	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM items`); n != 2 {
		t.Fatalf("row count after restore = %d, want 2", n)
	}
	h, _ := a.openUserDB("t1")
	var txt string
	if err := h.QueryRow(`SELECT txt FROM items WHERE id = 2`).Scan(&txt); err != nil {
		t.Fatalf("read restored row: %v", err)
	}
	if txt != "unicode: привет ✓\nsecond line" {
		t.Fatalf("restored text mismatch: %q", txt)
	}
	// Index and view must be recreated (the old engine silently lost indexes).
	if n := dbbQueryInt(t, a, "t1",
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_items_txt'`); n != 1 {
		t.Fatalf("index not restored")
	}
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM v_items`); n != 1 {
		t.Fatalf("view not restored or wrong content: %d", n)
	}
}

func TestDBBackupRestoreIsFullReplace(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE keep(a)`)
	file := dbbBackup(t, a, "t1")

	// Created AFTER the backup - must not survive a restore.
	dbbExec(t, a, "t1", `CREATE TABLE extra(b)`)
	dbbRestore(t, a, "t1", file)

	if n := dbbQueryInt(t, a, "t1",
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='extra'`); n != 0 {
		t.Fatalf("table created after backup survived the restore")
	}
	if n := dbbQueryInt(t, a, "t1",
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='keep'`); n != 1 {
		t.Fatalf("backed-up table missing after restore")
	}
}

func TestDBBackupRestoreDoesNotFireTriggers(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE items(id INTEGER PRIMARY KEY, v TEXT)`)
	dbbExec(t, a, "t1", `CREATE TABLE audit(n INTEGER)`)
	dbbExec(t, a, "t1", `CREATE TRIGGER tr AFTER INSERT ON items BEGIN INSERT INTO audit VALUES (new.id); END`)
	dbbExec(t, a, "t1", `INSERT INTO items(v) VALUES ('a'), ('b')`)

	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM audit`); n != 2 {
		t.Fatalf("precondition: audit = %d, want 2", n)
	}

	file := dbbBackup(t, a, "t1")
	dbbRestore(t, a, "t1", file)

	// Bulk data load must not have fired the trigger again: audit rows
	// come only from the backup's own data.
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM audit`); n != 2 {
		t.Fatalf("audit after restore = %d, want 2 (trigger fired during data load?)", n)
	}
	// ... but the trigger itself must work again after the restore.
	dbbExec(t, a, "t1", `INSERT INTO items(v) VALUES ('c')`)
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM audit`); n != 3 {
		t.Fatalf("trigger not functional after restore: audit = %d, want 3", n)
	}
}

func TestDBBackupPreservesSequenceAndBigIntsAndBlobs(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE s(id INTEGER PRIMARY KEY AUTOINCREMENT, v TEXT)`)
	dbbExec(t, a, "t1", `INSERT INTO s(v) VALUES ('a'), ('b'), ('c')`)
	dbbExec(t, a, "t1", `DELETE FROM s WHERE id = 3`)
	dbbExec(t, a, "t1", `CREATE TABLE nums(big INTEGER)`)
	dbbExec(t, a, "t1", `INSERT INTO nums VALUES (9007199254740993)`) // 2^53 + 1: dies in float64
	blob := []byte{0x00, 0x01, 0xFF, 0xFE, 0x80}
	dbbExec(t, a, "t1", `CREATE TABLE bin(data BLOB)`)
	dbbExec(t, a, "t1", `INSERT INTO bin VALUES (?)`, blob)

	file := dbbBackup(t, a, "t1")
	dbbRestore(t, a, "t1", file)

	// AUTOINCREMENT sequence preserved: next id must be 4, never a reuse of 3.
	dbbExec(t, a, "t1", `INSERT INTO s(v) VALUES ('d')`)
	if got := dbbQueryInt(t, a, "t1", `SELECT MAX(id) FROM s`); got != 4 {
		t.Fatalf("AUTOINCREMENT id after restore = %d, want 4 (sequence lost)", got)
	}
	if got := dbbQueryInt(t, a, "t1", `SELECT big FROM nums`); got != 9007199254740993 {
		t.Fatalf("big integer mangled by restore: %d", got)
	}
	h, _ := a.openUserDB("t1")
	var back []byte
	if err := h.QueryRow(`SELECT data FROM bin`).Scan(&back); err != nil {
		t.Fatalf("read blob: %v", err)
	}
	if !bytes.Equal(back, blob) {
		t.Fatalf("blob mangled by restore: % x", back)
	}
}

func TestDBBackupPruneKeepsNewest(t *testing.T) {
	a := dbbApp(t)
	a.WithConfig(func(c *Config) { c.BackupPruneDepth = 2 })
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)

	var files []string
	for i := 0; i < 3; i++ {
		dbbExec(t, a, "t1", `INSERT INTO x VALUES (?)`, i) // content change per backup
		files = append(files, dbbBackup(t, a, "t1"))
		time.Sleep(1100 * time.Millisecond) // distinct timestamps
	}

	left, err := a.listBackupFiles("t1")
	if err != nil {
		t.Fatalf("listBackupFiles: %v", err)
	}
	if len(left) != 2 {
		t.Fatalf("after 3 backups with depth 2, %d files remain: %v", len(left), left)
	}
	if left[0] != files[2] || left[1] != files[1] {
		t.Fatalf("prune kept wrong files: have %v, want [%s %s]", left, files[2], files[1])
	}
	if _, err := os.Stat(filepath.Join(a.dbBackupDir("t1"), files[0])); !os.IsNotExist(err) {
		t.Fatalf("oldest backup %s not pruned", files[0])
	}
}

func TestDBBackupBootstrapRestoresMissingDatabase(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (41), (42)`)
	dbbBackup(t, a, "t1")

	// Simulate a fresh device: backups exist, the .sqlite cache does not.
	a.evictUserDB("t1")
	if err := os.Remove(a.userDBPath("t1")); err != nil {
		t.Fatalf("remove sqlite: %v", err)
	}

	// The very first open must transparently restore the newest backup.
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM x`); n != 2 {
		t.Fatalf("bootstrap restore missing: count = %d, want 2", n)
	}
}

func TestDBRestoreRejectsDamagedAndForeignFiles(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (1)`)
	good := dbbBackup(t, a, "t1")

	// A copy with a git-conflict-marker line must be rejected whole, and
	// the live database must stay untouched.
	raw, err := os.ReadFile(filepath.Join(a.dbBackupDir("t1"), good))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	damagedName := "99991231T235959Z_corrupt.jsonl"
	damaged := append(append([]byte{}, raw...), []byte("<<<<<<< HEAD\n")...)
	if err := os.WriteFile(filepath.Join(a.dbBackupDir("t1"), damagedName), damaged, 0644); err != nil {
		t.Fatalf("write damaged copy: %v", err)
	}
	a.dbRestoreMu.Lock()
	err = a.restoreDBFromBackup("t1", damagedName)
	a.dbRestoreMu.Unlock()
	if err == nil {
		t.Fatalf("damaged backup accepted")
	}
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM x`); n != 1 {
		t.Fatalf("failed restore must not touch the database: count = %d", n)
	}

	// A backup whose header names another database must be rejected.
	dbbExec(t, a, "other", `CREATE TABLE y(b)`)
	otherFile := dbbBackup(t, a, "other")
	src := filepath.Join(a.dbBackupDir("other"), otherFile)
	dst := filepath.Join(a.dbBackupDir("t1"), "99991231T235958Z_foreign.jsonl")
	data, _ := os.ReadFile(src)
	os.WriteFile(dst, data, 0644)
	a.dbRestoreMu.Lock()
	err = a.restoreDBFromBackup("t1", "99991231T235958Z_foreign.jsonl")
	a.dbRestoreMu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "other") {
		t.Fatalf("foreign-database backup not rejected properly: %v", err)
	}

	// Path traversal / invalid names never reach the filesystem.
	a.dbRestoreMu.Lock()
	err = a.restoreDBFromBackup("t1", "../../../etc/passwd")
	a.dbRestoreMu.Unlock()
	if err == nil {
		t.Fatalf("invalid backup filename accepted")
	}
}

func TestDBBackupEndpoints(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (7)`)

	// POST /api/db/backup?db=t1
	rec := httptest.NewRecorder()
	a.handleDBBackupCreate(rec, httptest.NewRequest(http.MethodPost, "/api/db/backup?db=t1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("backup endpoint: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Status string `json:"status"`
		File   string `json:"file"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Status != "success" {
		t.Fatalf("backup endpoint response: %s (%v)", rec.Body.String(), err)
	}

	// GET /api/db/backups
	rec = httptest.NewRecorder()
	a.handleDBBackupList(rec, httptest.NewRequest(http.MethodGet, "/api/db/backups", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list endpoint: %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Status    string `json:"status"`
		Databases []struct {
			Name    string `json:"name"`
			State   string `json:"state"`
			Backups []struct {
				File  string `json:"file"`
				Valid bool   `json:"valid"`
				Rows  int    `json:"rows"`
			} `json:"backups"`
		} `json:"databases"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("list endpoint JSON: %v", err)
	}
	var file string
	for _, d := range listed.Databases {
		if d.Name != "t1" {
			continue
		}
		if len(d.Backups) != 1 || !d.Backups[0].Valid || d.Backups[0].Rows != 1 {
			t.Fatalf("list endpoint content: %+v", d)
		}
		if d.State != "insync" {
			t.Fatalf("state right after backup = %q, want insync", d.State)
		}
		file = d.Backups[0].File
	}
	if file == "" {
		t.Fatalf("database t1 missing from list: %s", rec.Body.String())
	}

	// Change data, then POST /api/db/restore?db=t1&file=...
	dbbExec(t, a, "t1", `DELETE FROM x`)
	rec = httptest.NewRecorder()
	a.handleDBRestore(rec, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/db/restore?db=t1&file=%s", file), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("restore endpoint: %d %s", rec.Code, rec.Body.String())
	}
	if n := dbbQueryInt(t, a, "t1", `SELECT COUNT(*) FROM x`); n != 1 {
		t.Fatalf("restore via endpoint did not bring data back: %d", n)
	}

	// Method and parameter validation.
	rec = httptest.NewRecorder()
	a.handleDBBackupCreate(rec, httptest.NewRequest(http.MethodGet, "/api/db/backup?db=t1", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("backup GET not rejected: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.handleDBBackupCreate(rec, httptest.NewRequest(http.MethodPost, "/api/db/backup?db=../evil", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid db name not rejected: %d", rec.Code)
	}
}

func TestDBBackupHeaderIsFirstLineWithCounts(t *testing.T) {
	a := dbbApp(t)
	dbbExec(t, a, "t1", `CREATE TABLE x(a)`)
	dbbExec(t, a, "t1", `INSERT INTO x VALUES (1), (2), (3)`)
	file := dbbBackup(t, a, "t1")

	h, err := readBackupHeader(filepath.Join(a.dbBackupDir("t1"), file))
	if err != nil {
		t.Fatalf("readBackupHeader: %v", err)
	}
	if h.Format != backupFormatName || h.Version != backupFormatVersion {
		t.Fatalf("header format/version: %+v", h)
	}
	if h.Database != "t1" || h.Rows != 3 || h.Objects != 1 {
		t.Fatalf("header counts: %+v", h)
	}
	if h.Hostname == "" {
		t.Fatalf("header hostname empty")
	}
}
