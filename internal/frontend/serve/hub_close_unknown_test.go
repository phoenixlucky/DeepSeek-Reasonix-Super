package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClosingARuntimeThatIsGoneAnswersWithTheNotFoundCode(t *testing.T) {
	h := NewHub(HubOptions{})
	req := httptest.NewRequest(http.MethodPost, "/runtimes/r1/close", nil)
	req.SetPathValue("id", "r1")
	rec := httptest.NewRecorder()
	h.closeRuntime(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body struct {
		Code   string         `json:"code"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the refusal is not an envelope: %v: %s", err, rec.Body.String())
	}
	if body.Code != codeNotFound || body.Params["kind"] != "runtime" || body.Params["name"] != "r1" {
		t.Fatalf("refusal = %+v, want %s naming runtime r1", body, codeNotFound)
	}
}
