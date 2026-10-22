package delivery

import "testing"

func TestOnlyConfirmedPresenterResultsBecomeDeliveries(t *testing.T) {
	body := `{"version":1,"files":[{"path":"docs/report.md","description":"Report"}]}`
	if len(Decode("present_files", "tool:present_files", false, body)) != 1 {
		t.Fatal("successful builtin delivery not decoded")
	}
	for _, tc := range []struct {
		name, identity, output string
		failed                 bool
	}{
		{"bash", "tool:bash", body, false},
		{"present_files", "mcp-tool:server/present_files", body, false},
		{"present_files", "", body, false},
		{"present_files", "tool:present_files", body, true},
		{"present_files", "tool:present_files", `{"version":2,"files":[{"path":"x"}]}`, false},
		{"present_files", "tool:present_files", `{"version":1,"files":[{"path":"../secret"}]}`, false},
	} {
		if got := Decode(tc.name, tc.identity, tc.failed, tc.output); len(got) != 0 {
			t.Fatalf("accepted %+v: %+v", tc, got)
		}
	}
}
