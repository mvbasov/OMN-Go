package render

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"net.basov.omngo/backend/internal/logx"
)

// An error answer carries the status code, the JSON content type and the
// shape of section 1.4 of doc/API.md. The writeJSONError of each feature
// package sends this value through WriteJSON.
func TestWriteJSONErrorShape(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusBadRequest, JSONStatus{Status: "error", Message: `bad "name"`},
		newTestApp(t).log(logx.Server))

	if w.Code != http.StatusBadRequest {
		t.Errorf("the code is %d, want 400", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("the content type is %q, want application/json", got)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, w.Body.String())
	}
	if body["status"] != "error" || body["message"] != `bad "name"` || len(body) != 2 {
		t.Errorf("the body is %v", body)
	}
}
