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

func postSQL(t *testing.T, a *App, body string) (*httptest.ResponseRecorder, sqlResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handleSQL(rec, req)
	var resp sqlResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not JSON (%v): %s", err, rec.Body.String())
	}
	return rec, resp
}

func TestSQLRoundTrip(t *testing.T) {
	a := newTestApp(t)

	rec, resp := postSQL(t, a, `{"db":"notes","statements":[
		{"sql":"CREATE TABLE t(id INTEGER PRIMARY KEY, name TEXT, score REAL)"},
		{"sql":"INSERT INTO t(name, score) VALUES(?, ?)","args":["alice", 1.5]},
		{"sql":"INSERT INTO t(name, score) VALUES(?, ?)","args":["bob", 2.5]},
		{"sql":"SELECT id, name, score FROM t ORDER BY id","args":[]}
	]}`)
	if rec.Code != http.StatusOK || resp.Status != "success" {
		t.Fatalf("status=%d resp=%+v", rec.Code, resp)
	}
	if len(resp.Results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(resp.Results))
	}
	if resp.Results[1].LastInsertID != 1 || resp.Results[2].LastInsertID != 2 {
		t.Errorf("last_insert_id wrong: %+v %+v", resp.Results[1], resp.Results[2])
	}
	sel := resp.Results[3]
	if len(sel.Columns) != 3 || sel.Columns[1] != "name" {
		t.Errorf("columns = %v", sel.Columns)
	}
	if len(sel.Rows) != 2 {
		t.Fatalf("rows = %v", sel.Rows)
	}
	// TEXT must arrive as a JSON string (not base64'd bytes).
	if sel.Rows[0][1] != "alice" {
		t.Errorf("row[0].name = %v (%T)", sel.Rows[0][1], sel.Rows[0][1])
	}
	// The database file must exist under <storage>/db/.
	if _, err := os.Stat(filepath.Join(a.StorageDir, "db", "notes.sqlite")); err != nil {
		t.Errorf("database file not created: %v", err)
	}
}

// A failing statement must roll back the WHOLE batch - this is the
// atomicity the JS shim's transaction() relies on.
func TestSQLBatchRollsBackAtomically(t *testing.T) {
	a := newTestApp(t)

	_, resp := postSQL(t, a, `{"db":"notes","statements":[
		{"sql":"CREATE TABLE t(a INTEGER)"}]}`)
	if resp.Status != "success" {
		t.Fatalf("setup failed: %+v", resp)
	}

	rec, resp := postSQL(t, a, `{"db":"notes","statements":[
		{"sql":"INSERT INTO t VALUES(1)"},
		{"sql":"INSERT INTO nonexistent VALUES(2)"}
	]}`)
	if rec.Code == http.StatusOK || resp.Status != "error" {
		t.Fatalf("expected error, got status=%d resp=%+v", rec.Code, resp)
	}
	if resp.FailedStatement == nil || *resp.FailedStatement != 1 {
		t.Errorf("failed_statement = %v, want 1", resp.FailedStatement)
	}

	// The first (valid) INSERT must NOT have survived.
	_, resp = postSQL(t, a, `{"db":"notes","statements":[{"sql":"SELECT COUNT(*) AS n FROM t"}]}`)
	if resp.Status != "success" {
		t.Fatalf("count query failed: %+v", resp)
	}
	// JSON numbers decode as float64.
	if n, ok := resp.Results[0].Rows[0][0].(float64); !ok || n != 0 {
		t.Errorf("row count after rollback = %v, want 0", resp.Results[0].Rows[0][0])
	}
}

func TestSQLDatabaseNameValidation(t *testing.T) {
	a := newTestApp(t)
	for _, bad := range []string{"../evil", "a/b", "", "name.with.dots", strings.Repeat("x", 65)} {
		body, _ := json.Marshal(sqlRequest{DB: bad, Statements: []sqlStatement{{SQL: "SELECT 1"}}})
		rec, resp := postSQL(t, a, string(body))
		if rec.Code != http.StatusBadRequest || resp.Status != "error" {
			t.Errorf("db name %q: expected 400/error, got %d/%s", bad, rec.Code, resp.Status)
		}
	}
	// Nothing may have been created outside (or inside) db/ for these.
	entries, _ := os.ReadDir(filepath.Join(a.StorageDir, "db"))
	if len(entries) != 0 {
		t.Errorf("invalid names created files: %v", entries)
	}
}

func TestSQLRequestLimits(t *testing.T) {
	a := newTestApp(t)

	// Too many statements.
	stmts := make([]sqlStatement, sqlMaxStatements+1)
	for i := range stmts {
		stmts[i] = sqlStatement{SQL: "SELECT 1"}
	}
	body, _ := json.Marshal(sqlRequest{DB: "notes", Statements: stmts})
	rec, resp := postSQL(t, a, string(body))
	if rec.Code != http.StatusBadRequest || resp.Status != "error" {
		t.Errorf("oversized batch: expected 400/error, got %d/%s", rec.Code, resp.Status)
	}

	// Empty batch.
	rec, resp = postSQL(t, a, `{"db":"notes","statements":[]}`)
	if rec.Code != http.StatusBadRequest || resp.Status != "error" {
		t.Errorf("empty batch: expected 400/error, got %d/%s", rec.Code, resp.Status)
	}

	// GET refused.
	req := httptest.NewRequest(http.MethodGet, "/api/sql", nil)
	rec2 := httptest.NewRecorder()
	a.handleSQL(rec2, req)
	if rec2.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: status %d, want 405", rec2.Code)
	}
}
