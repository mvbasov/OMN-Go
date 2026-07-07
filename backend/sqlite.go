package backend

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "modernc.org/sqlite" // pure-Go driver, works with CGO_ENABLED=0 on all targets
)

// ----------------------------------------------------------------------
// Server-backed SQLite for note scripts
// ----------------------------------------------------------------------
//
// Replaces the deprecated WebSQL API (window.openDatabase): browsers have
// removed it, and it was per-browser anyway - a note's data silently
// differed between devices. Databases now live server-side as ordinary
// SQLite files under <StorageDir>/db/<name>.sqlite, so every device
// viewing the note sees the same data.
//
// Wire protocol - POST /api/sql, JSON both ways:
//
//	request:  { "db": "mydata",
//	            "statements": [ {"sql": "INSERT ... VALUES(?,?)", "args": [1, "x"]},
//	                            {"sql": "SELECT * FROM t", "args": []} ] }
//	response: { "status": "success",
//	            "results": [ {"rows_affected":1, "last_insert_id":7},
//	                         {"columns":["a","b"], "rows":[[1,"x"]],
//	                          "rows_affected":0, "last_insert_id":0} ] }
//
// ALL statements of one request run inside ONE transaction: any failure
// rolls the whole batch back and reports which statement failed. This is
// what gives the JS shim's transaction() its atomicity.
//
// Deliberate limits:
//   - admin-only endpoint (registered with requireAdmin=true): SQL is
//     arbitrary code over shared state; guests on the LAN get no access.
//     Local connections (the app's own UI) bypass auth as everywhere else.
//   - db names are [A-Za-z0-9_-], max 64 chars - the name is used as a
//     filename, so this is the path-traversal guard.
//   - request body capped at 1 MB, max 500 statements per batch.

// dbNameRe is the whitelist for user database names (used as filenames).
var dbNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

const (
	sqlMaxBodyBytes  = 1 << 20 // 1 MB
	sqlMaxStatements = 500
)

// openUserDB returns the cached handle for a named user database, opening
// (and creating) it on first use. Handles live for the process lifetime -
// SQLite files need no explicit close, and the OS reclaims everything on
// exit, matching how the rest of this app treats resources.
func (a *App) openUserDB(name string) (*sql.DB, error) {
	if !dbNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid database name %q (allowed: letters, digits, '_', '-', max 64 chars)", name)
	}

	a.sqlMu.Lock()
	defer a.sqlMu.Unlock()

	if a.sqlDBs == nil {
		a.sqlDBs = make(map[string]*sql.DB)
	}
	if db, ok := a.sqlDBs[name]; ok {
		return db, nil
	}

	dir := filepath.Join(a.StorageDir, "db")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// busy_timeout: don't fail instantly if a second request races this
	// one. journal_mode TRUNCATE instead of WAL on purpose: WAL needs a
	// shared-memory-mapped -shm file, which is unreliable on Android's
	// FUSE-backed scoped storage where these files live; TRUNCATE keeps
	// everything as plain file I/O, which that storage handles fine.
	dsn := "file:" + filepath.Join(dir, name+".sqlite") +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(TRUNCATE)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// One connection per database: serializes all access so concurrent
	// requests queue instead of tripping over SQLITE_BUSY, and keeps the
	// transaction-per-request model trivially correct.
	db.SetMaxOpenConns(1)

	a.sqlDBs[name] = db
	return db, nil
}

type sqlStatement struct {
	SQL  string        `json:"sql"`
	Args []interface{} `json:"args"`
}

type sqlRequest struct {
	DB         string         `json:"db"`
	Statements []sqlStatement `json:"statements"`
}

type sqlResult struct {
	Columns      []string        `json:"columns,omitempty"`
	Rows         [][]interface{} `json:"rows,omitempty"`
	RowsAffected int64           `json:"rows_affected"`
	LastInsertID int64           `json:"last_insert_id"`
}

type sqlResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	// Index of the statement that failed (only with status "error" when a
	// specific statement, rather than the request itself, was at fault).
	FailedStatement *int        `json:"failed_statement,omitempty"`
	Results         []sqlResult `json:"results,omitempty"`
}

func writeSQLResponse(w http.ResponseWriter, httpStatus int, resp sqlResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("handleSQL: failed to encode response: %v", err)
	}
}

// returnsRows decides Query vs Exec per statement. First-keyword sniffing
// is imperfect (a "WITH ... INSERT" is treated as a query and loses its
// rows_affected), but it errs on the safe side: worst case a result set
// is empty or a counter is zero - never data corruption.
func returnsRows(query string) bool {
	q := strings.ToUpper(strings.TrimSpace(query))
	for _, kw := range []string{"SELECT", "WITH", "PRAGMA", "EXPLAIN", "VALUES"} {
		if strings.HasPrefix(q, kw) {
			return true
		}
	}
	return false
}

// handleSQL executes one atomic batch of statements against one named
// user database. See the file-top comment for the wire protocol.
func (a *App) handleSQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeSQLResponse(w, http.StatusMethodNotAllowed, sqlResponse{Status: "error", Message: "POST only"})
		return
	}

	var req sqlRequest
	r.Body = http.MaxBytesReader(w, r.Body, sqlMaxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSQLResponse(w, http.StatusBadRequest, sqlResponse{Status: "error", Message: "bad request: " + err.Error()})
		return
	}
	if len(req.Statements) == 0 {
		writeSQLResponse(w, http.StatusBadRequest, sqlResponse{Status: "error", Message: "no statements"})
		return
	}
	if len(req.Statements) > sqlMaxStatements {
		writeSQLResponse(w, http.StatusBadRequest, sqlResponse{Status: "error",
			Message: fmt.Sprintf("too many statements (%d > %d)", len(req.Statements), sqlMaxStatements)})
		return
	}

	db, err := a.openUserDB(req.DB)
	if err != nil {
		writeSQLResponse(w, http.StatusBadRequest, sqlResponse{Status: "error", Message: err.Error()})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		writeSQLResponse(w, http.StatusInternalServerError, sqlResponse{Status: "error", Message: "begin: " + err.Error()})
		return
	}

	results := make([]sqlResult, 0, len(req.Statements))
	for i, stmt := range req.Statements {
		res, stmtErr := runStatement(tx, stmt)
		if stmtErr != nil {
			tx.Rollback()
			idx := i
			writeSQLResponse(w, http.StatusBadRequest, sqlResponse{
				Status:          "error",
				Message:         stmtErr.Error(),
				FailedStatement: &idx,
			})
			return
		}
		results = append(results, res)
	}

	if err := tx.Commit(); err != nil {
		writeSQLResponse(w, http.StatusInternalServerError, sqlResponse{Status: "error", Message: "commit: " + err.Error()})
		return
	}
	writeSQLResponse(w, http.StatusOK, sqlResponse{Status: "success", Results: results})
}

func runStatement(tx *sql.Tx, stmt sqlStatement) (sqlResult, error) {
	if !returnsRows(stmt.SQL) {
		res, err := tx.Exec(stmt.SQL, stmt.Args...)
		if err != nil {
			return sqlResult{}, err
		}
		affected, _ := res.RowsAffected()
		lastID, _ := res.LastInsertId()
		return sqlResult{RowsAffected: affected, LastInsertID: lastID}, nil
	}

	rows, err := tx.Query(stmt.SQL, stmt.Args...)
	if err != nil {
		return sqlResult{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return sqlResult{}, err
	}
	out := sqlResult{Columns: cols, Rows: [][]interface{}{}}

	for rows.Next() {
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return sqlResult{}, err
		}
		// []byte would JSON-encode as base64; hand text out as text.
		for i, v := range raw {
			if b, ok := v.([]byte); ok {
				raw[i] = string(b)
			}
		}
		out.Rows = append(out.Rows, raw)
	}
	return out, rows.Err()
}
