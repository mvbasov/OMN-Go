package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
)

// TestSyncErrorStatus pins the Phase 4 typed-sentinel mapping: each sync
// sentinel maps to its wire status, an unrelated error and nil map to
// ok=false (a plain "error"), and - the whole point of moving off string
// comparison - the match survives wrapping with %w.
func TestSyncErrorStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus string
		wantOK     bool
	}{
		{"conflict", ErrSyncConflict, "conflict", true},
		{"push conflict", ErrPushConflict, "push_conflict", true},
		{"needs message", ErrCommitMessageRequired, "needs_commit_message", true},
		{"wrapped conflict", fmt.Errorf("pull step: %w", ErrSyncConflict), "conflict", true},
		{"generic error", errors.New("disk exploded"), "", false},
		{"nil", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, ok := syncErrorStatus(tt.err)
			if ok != tt.wantOK || status != tt.wantStatus {
				t.Errorf("syncErrorStatus(%v) = (%q, %v), want (%q, %v)",
					tt.err, status, ok, tt.wantStatus, tt.wantOK)
			}
			// A mapped error must carry a non-empty user message; an
			// unmapped one must not.
			if ok && msg == "" {
				t.Errorf("mapped error %v produced an empty message", tt.err)
			}
			if !ok && msg != "" {
				t.Errorf("unmapped error %v produced message %q, want empty", tt.err, msg)
			}
		})
	}
}

// TestSyncConflictErrorWrapsSentinel pins that the file-carrying conflict
// error still reads as ErrSyncConflict everywhere the state machine looks
// (errors.Is / syncErrorStatus), while its file list is retrievable with
// errors.As - and that the list survives being wrapped with %w.
func TestSyncConflictErrorWrapsSentinel(t *testing.T) {
	ce := &syncConflictError{Files: []string{"md/A.md", "md/B.md"}}

	if !errors.Is(ce, ErrSyncConflict) {
		t.Error("syncConflictError does not unwrap to ErrSyncConflict")
	}
	if status, _, ok := syncErrorStatus(ce); !ok || status != "conflict" {
		t.Errorf("syncErrorStatus(syncConflictError) = (%q, %v), want (conflict, true)", status, ok)
	}

	wrapped := fmt.Errorf("pull: %w", error(ce))
	var got *syncConflictError
	if !errors.As(wrapped, &got) {
		t.Fatal("errors.As could not recover syncConflictError through a %w wrap")
	}
	if len(got.Files) != 2 || got.Files[0] != "md/A.md" || got.Files[1] != "md/B.md" {
		t.Errorf("recovered Files = %v, want [md/A.md md/B.md]", got.Files)
	}
}

// TestWriteSyncConflictJSON pins the wire shape the conflict modal consumes:
// status "conflict", the message, and a files array that is ALWAYS present -
// [] rather than null even when there are no per-file conflicts - so the
// frontend can iterate it without a nil guard.
func TestWriteSyncConflictJSON(t *testing.T) {
	// With files.
	rec := httptest.NewRecorder()
	writeSyncConflictJSON(rec, "Fast-forward not possible.", []string{"md/A.md"})
	var body struct {
		Status  string   `json:"status"`
		Message string   `json:"message"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Status != "conflict" || body.Message == "" || len(body.Files) != 1 || body.Files[0] != "md/A.md" {
		t.Errorf("unexpected body: %+v", body)
	}

	// Nil files must serialize as [] (never null) and never omit the key.
	rec2 := httptest.NewRecorder()
	writeSyncConflictJSON(rec2, "diverged", nil)
	raw := rec2.Body.String()
	if !contains(raw, `"files":[]`) {
		t.Errorf("nil files did not serialize as []: %s", raw)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestSyncSentinelsAreDistinct guards that the three sentinels are not
// accidentally the same value (which would collapse their wire statuses).
func TestSyncSentinelsAreDistinct(t *testing.T) {
	all := []error{ErrSyncConflict, ErrPushConflict, ErrCommitMessageRequired}
	for i := range all {
		for j := range all {
			if i != j && errors.Is(all[i], all[j]) {
				t.Errorf("sentinels %d and %d are not distinct", i, j)
			}
		}
	}
}
