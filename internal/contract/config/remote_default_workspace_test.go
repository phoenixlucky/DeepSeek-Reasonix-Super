package config

import "testing"

func TestWSLDefaultWorkspacePreservesTheDeclaredPath(t *testing.T) {
	for _, tc := range []struct {
		name       string
		workspace  string
		workspaces []string
		want       string
	}{
		{"empty", "", nil, ""},
		{"windows", ` C:\work\project `, []string{`D:\other`}, `C:\work\project`},
		{"distribution", " /home/dev/project ", []string{"/other"}, "/home/dev/project"},
		{"fallback", " ", []string{"", ` C:\work\fallback `, "/other"}, `C:\work\fallback`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := RemoteHostEntry{Name: "dev", Kind: RemoteKindWSL, Distro: "Ubuntu", Workspace: tc.workspace, Workspaces: tc.workspaces}
			if got := entry.DefaultWorkspace(); got != tc.want {
				t.Fatalf("DefaultWorkspace() = %q, want %q", got, tc.want)
			}
		})
	}
}
