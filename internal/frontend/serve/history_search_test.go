package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHistorySearchRefusesAnEmptyQueryWithItsCode(t *testing.T) {

	mux := http.NewServeMux()
	mux.HandleFunc("GET /history/search", historySearch)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/history/search?q=%20", nil))
	var reason Reason
	_ = json.Unmarshal(rec.Body.Bytes(), &reason)
	if rec.Code != http.StatusBadRequest || reason.Code != "history.query_required" {
		t.Fatalf("status %d code %q, want 400 history.query_required", rec.Code, reason.Code)
	}
}

func TestHistorySearchAnswersAnArrayEvenWithNoIndexYet(t *testing.T) {

	mux := http.NewServeMux()
	mux.HandleFunc("GET /history/search", historySearch)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/history/search?q=zebra", nil))
	if rec.Code != http.StatusOK || rec.Body.String() == "null\n" {
		t.Fatalf("status %d body %q, want 200 and a JSON array", rec.Code, rec.Body.String())
	}
}
