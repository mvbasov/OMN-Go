package backend

import (
	"errors"
	"fmt"
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
