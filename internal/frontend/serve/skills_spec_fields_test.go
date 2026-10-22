package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/ext/skill"
	"reasonix/internal/session/control"
)

// The specification's descriptive fields reach a management surface as the
// skill wrote them, so it can show where a skill came from and what it needs.
func TestSkillsListCarriesTheSpecDescriptiveFields(t *testing.T) {
	ctrl := control.New(control.Options{
		Skills: []skill.Skill{{
			Name: "pdf-processing", Description: "pdfs", Scope: skill.ScopeProject,
			SpecFields: skill.SpecFields{License: "Apache-2.0", Compatibility: "Requires git", Metadata: map[string]string{"author": "example-org"}},
		}},
	})
	defer ctrl.Close()
	srv := httptest.NewServer(operatorHandler(New(ctrl, NewBroadcaster(), config.ServeConfig{})))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/skills")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Skills []skillEntry `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || len(got.Skills) != 1 {
		t.Fatalf("decode: %v, %+v", err, got)
	}
	e := got.Skills[0]
	if e.License != "Apache-2.0" || e.Compatibility != "Requires git" || e.Metadata["author"] != "example-org" {
		t.Fatalf("entry lost the descriptive fields: %+v", e)
	}
}
