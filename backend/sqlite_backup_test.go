package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupBackupDB(t *testing.T, a *App, name string) {
	t.Helper()
	_, resp := postSQL(t, a, `{"db":"`+name+`","statements":[
		{"sql":"CREATE TABLE items(id INTEGER PRIMARY KEY, name TEXT)"},
		{"sql":"CREATE TABLE log(id INTEGER PRIMARY KEY, msg TEXT)"},
		{"sql":"CREATE VIEW item_names AS SELECT name FROM items"},
		{"sql":"CREATE TRIGGER items_ai AFTER INSERT ON items BEGIN INSERT INTO log(msg) VALUES('inserted'); END"},
		{"sql":"INSERT INTO items(name) VALUES('a')"},
		{"sql":"INSERT INTO items(name) VALUES('b')"}
	]}`)
	if resp.Status != "success" {
		t.Fatalf("setup failed: %+v", resp)
	}
}

func postAdmin(t *testing.T, a *App, path string, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestExportDatabaseWritesExpectedFiles(t *testing.T) {
	a := newTestApp(t)
	setupBackupDB(t, a, "notes")

	db, err := a.openUserDBLocked("notes")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := a.exportDatabase(db, "notes", "", true)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	want := []string{
		"html/db_json/notes_items_sch.json",
		"html/db_json/notes_items_data.json",
		"html/db_json/notes_log_sch.json",
		"html/db_json/notes_log_data.json",
		"html/db_json/notes_item_names_sch.json",
		"html/db_json/notes_items_ai_sch.json",
	}
	for _, w := range want {
		found := false
		for _, c := range changed {
			if c == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in changed files, got %v", w, changed)
		}
	}
	// The insert trigger fired twice during setup (two inserts into
	// items), so log has data - and the view has no data file at all.
	logData, err := os.ReadFile(filepath.Join(a.StorageDir, "html", "db_json", "notes_log_data.json"))
	if err != nil {
		t.Fatalf("log data file missing: %v", err)
	}
	if strings.Count(string(logData), "\n") != 2 {
		t.Errorf("expected 2 log rows from trigger firing during setup, got:\n%s", logData)
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "db_json", "notes_item_names_data.json")); err == nil {
		t.Error("a view must not get a _data.json file")
	}

	// items_data.json is JSONL: one compact array per line, in column order.
	itemsData, err := os.ReadFile(filepath.Join(a.StorageDir, "html", "db_json", "notes_items_data.json"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(itemsData), "\n"), "\n")
	if len(lines) != 2 || lines[0] != `[1,"a"]` || lines[1] != `[2,"b"]` {
		t.Errorf("unexpected items_data.json content: %q", lines)
	}
}

func TestExportDryRunWritesNothing(t *testing.T) {
	a := newTestApp(t)
	setupBackupDB(t, a, "notes")
	db, _ := a.openUserDBLocked("notes")

	changed, err := a.exportDatabase(db, "notes", "", false)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(changed) == 0 {
		t.Fatal("dry run reported no changes on a fresh database - export logic broken")
	}
	entries, _ := os.ReadDir(filepath.Join(a.StorageDir, "html", "db_json"))
	if len(entries) != 0 {
		t.Errorf("dry run wrote files: %v", entries)
	}

	// Second dry run after a real write reports nothing changed.
	if _, err := a.exportDatabase(db, "notes", "", true); err != nil {
		t.Fatal(err)
	}
	changed, err = a.exportDatabase(db, "notes", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Errorf("dry run after a real export reported changes: %v", changed)
	}
}

func TestExportSkipsBlobTable(t *testing.T) {
	a := newTestApp(t)
	_, resp := postSQL(t, a, `{"db":"notes","statements":[
		{"sql":"CREATE TABLE blobby(id INTEGER PRIMARY KEY, data BLOB)"},
		{"sql":"CREATE TABLE fine(id INTEGER PRIMARY KEY, name TEXT)"}
	]}`)
	if resp.Status != "success" {
		t.Fatalf("setup: %+v", resp)
	}
	db, _ := a.openUserDBLocked("notes")

	changed, err := a.exportDatabase(db, "notes", "", true)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, c := range changed {
		if strings.Contains(c, "blobby") {
			t.Errorf("BLOB table was exported: %v", changed)
		}
	}
	found := false
	for _, c := range changed {
		if strings.Contains(c, "fine") {
			found = true
		}
	}
	if !found {
		t.Error("non-BLOB table in the same database was not exported")
	}
}

func TestRestoreDatabaseFullReplace(t *testing.T) {
	a := newTestApp(t)
	setupBackupDB(t, a, "notes")
	db, _ := a.openUserDBLocked("notes")
	if _, err := a.exportDatabase(db, "notes", "", true); err != nil {
		t.Fatal(err)
	}

	// Diverge local state from the backup.
	if _, resp := postSQL(t, a, `{"db":"notes","statements":[
		{"sql":"INSERT INTO items(name) VALUES('local-only')"},
		{"sql":"DROP TABLE log"}
	]}`); resp.Status != "success" {
		t.Fatalf("divergence setup: %+v", resp)
	}

	if err := a.restoreDatabase(db, "notes", ""); err != nil {
		t.Fatalf("restore: %v", err)
	}

	_, resp := postSQL(t, a, `{"db":"notes","statements":[{"sql":"SELECT name FROM items ORDER BY id"}]}`)
	if resp.Status != "success" {
		t.Fatalf("verify query: %+v", resp)
	}
	rows := resp.Results[0].Rows
	if len(rows) != 2 || rows[0][0] != "a" || rows[1][0] != "b" {
		t.Errorf("items after restore = %v, want exactly [a, b] (local-only row must be gone)", rows)
	}

	// The dropped "log" table must be back (restore recreates schema).
	_, resp = postSQL(t, a, `{"db":"notes","statements":[{"sql":"SELECT COUNT(*) FROM log"}]}`)
	if resp.Status != "success" {
		t.Errorf("log table not restored: %+v", resp)
	}
}

// The core ordering guarantee: bulk-loading table data during restore
// must NOT fire AFTER INSERT triggers (they run last, after data load).
func TestRestoreDoesNotFireTriggersDuringDataLoad(t *testing.T) {
	a := newTestApp(t)
	setupBackupDB(t, a, "notes") // trigger already fired twice here (2 inserts)
	db, _ := a.openUserDBLocked("notes")
	if _, err := a.exportDatabase(db, "notes", "", true); err != nil {
		t.Fatal(err)
	}

	// Wipe everything, including the log rows the trigger already wrote.
	if _, resp := postSQL(t, a, `{"db":"notes","statements":[
		{"sql":"DROP TABLE items"}, {"sql":"DROP TABLE log"}, {"sql":"DROP VIEW item_names"}
	]}`); resp.Status != "success" {
		t.Fatalf("wipe: %+v", resp)
	}

	if err := a.restoreDatabase(db, "notes", ""); err != nil {
		t.Fatalf("restore: %v", err)
	}

	// If the trigger had (wrongly) fired during the 2-row bulk load of
	// "items", log would have 2 EXTRA rows on top of the 2 restored from
	// its own backup - i.e. 4 instead of 2.
	_, resp := postSQL(t, a, `{"db":"notes","statements":[{"sql":"SELECT COUNT(*) AS n FROM log"}]}`)
	if resp.Status != "success" {
		t.Fatalf("verify: %+v", resp)
	}
	if n, _ := resp.Results[0].Rows[0][0].(float64); n != 2 {
		t.Errorf("log row count after restore = %v, want 2 (trigger must not fire during data load)", n)
	}
}

func TestRestoreSingleObjectIsStrict(t *testing.T) {
	a := newTestApp(t)
	setupBackupDB(t, a, "notes")
	db, _ := a.openUserDBLocked("notes")
	if _, err := a.exportDatabase(db, "notes", "", true); err != nil {
		t.Fatal(err)
	}

	// Diverge both "items" and "log". Note: inserting into "items" fires
	// items_ai (AFTER INSERT ON items), which itself adds a log row - so
	// this single INSERT contributes to BOTH tables' divergence, on top
	// of the explicit log INSERT right after it.
	if _, resp := postSQL(t, a, `{"db":"notes","statements":[
		{"sql":"INSERT INTO items(name) VALUES('extra')"},
		{"sql":"INSERT INTO log(msg) VALUES('extra')"}
	]}`); resp.Status != "success" {
		t.Fatalf("divergence: %+v", resp)
	}

	// Restore ONLY items.
	if err := a.restoreDatabase(db, "notes", "items"); err != nil {
		t.Fatalf("restore: %v", err)
	}

	_, resp := postSQL(t, a, `{"db":"notes","statements":[
		{"sql":"SELECT COUNT(*) FROM items"},
		{"sql":"SELECT COUNT(*) FROM log"}
	]}`)
	if resp.Status != "success" {
		t.Fatalf("verify: %+v", resp)
	}
	if n, _ := resp.Results[0].Rows[0][0].(float64); n != 2 {
		t.Errorf("items count = %v, want 2 (restored)", n)
	}
	// log started this test at 2 (from setupBackupDB's own two trigger
	// firings), gained a 3rd row from items_ai firing on the 'extra'
	// items insert above, and a 4th from the explicit log insert - all of
	// which must survive untouched, since single-object restore of
	// "items" must never touch "log".
	if n, _ := resp.Results[1].Rows[0][0].(float64); n != 4 {
		t.Errorf("log count = %v, want 4 (untouched by items-only restore)", n)
	}
}

func TestRestoreNoBackupIsAnError(t *testing.T) {
	a := newTestApp(t)
	setupBackupDB(t, a, "notes") // no export called
	db, _ := a.openUserDBLocked("notes")

	if err := a.restoreDatabase(db, "notes", ""); err == nil {
		t.Error("expected an error restoring a database with no backup, got nil")
	}
}

// Per the agreed design ("log error about collision"): the FIRST object to
// claim a given sanitized filename exports normally; only SECOND-and-later
// objects colliding on that same name are skipped. Silently dropping every
// colliding object (including the first, uncontested claimant) would lose
// legitimate data for no reason.
func TestSanitizeObjectNameCollisionKeepsFirstSkipsRest(t *testing.T) {
	a := newTestApp(t)
	// "a.b" and "a b" both sanitize to "a_b" - genuine collision, since
	// sanitizeObjectName's regexp `[^A-Za-z0-9_-]` treats '-' as SAFE (it
	// passes through unchanged), so a hyphen variant like "a-b" would
	// NOT collide with either of these; only characters the regexp
	// actually replaces (here: '.' and ' ', both -> '_') do.
	// sqlite_master (and therefore listDatabaseObjects) returns tables
	// ordered by name using SQLite's default byte-wise collation, and
	// ' ' (0x20) sorts before '.' (0x2E), so "a b" is the one that wins
	// the "a_b" filename.
	_, resp := postSQL(t, a, `{"db":"notes","statements":[
		{"sql":"CREATE TABLE \"a.b\"(x INTEGER)"},
		{"sql":"CREATE TABLE \"a b\"(x INTEGER)"},
		{"sql":"CREATE TABLE clean(x INTEGER)"}
	]}`)
	if resp.Status != "success" {
		t.Fatalf("setup: %+v", resp)
	}
	db, _ := a.openUserDBLocked("notes")

	changed, err := a.exportDatabase(db, "notes", "", true)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(strings.Join(changed, ","), "notes_clean_sch.json") {
		t.Error("unrelated non-colliding table was not exported")
	}

	// Exactly one "a_b" schema file exists, and it belongs to the winner.
	schPath := filepath.Join(a.StorageDir, "html", "db_json", "notes_a_b_sch.json")
	raw, err := os.ReadFile(schPath)
	if err != nil {
		t.Fatalf("expected the first colliding object to export normally: %v", err)
	}
	var sf schemaFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		t.Fatalf("parse %s: %v", schPath, err)
	}
	if sf.Object != "a b" {
		t.Errorf("winning object = %q, want %q (sort order)", sf.Object, "a b")
	}

	// The loser must not have overwritten it under a different path either.
	entries, _ := os.ReadDir(filepath.Join(a.StorageDir, "html", "db_json"))
	count := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), "a_b") {
			count++
		}
	}
	if count != 2 { // notes_a_b_sch.json + notes_a_b_data.json, from ONE winning table
		t.Errorf("expected exactly 2 files (sch+data) for the colliding pair's single winner, found %d: %v", count, entries)
	}
}

func TestHandleDBExportAndRestoreEndpoints(t *testing.T) {
	a := newTestApp(t)
	setupBackupDB(t, a, "notes")

	rec := postAdmin(t, a, "/api/db/export?db=notes", a.handleDBExport)
	if rec.Code != http.StatusOK {
		t.Fatalf("export endpoint: status %d body %s", rec.Code, rec.Body.String())
	}

	rec = postAdmin(t, a, "/api/db/restore?db=notes", a.handleDBRestore)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore endpoint: status %d body %s", rec.Code, rec.Body.String())
	}

	// Invalid db name is rejected before touching anything.
	rec = postAdmin(t, a, "/api/db/export?db=..%2Fevil", a.handleDBExport)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("path-traversal db name: status %d, want 400", rec.Code)
	}
}
