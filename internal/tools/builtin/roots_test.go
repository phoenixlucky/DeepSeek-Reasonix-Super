package builtin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/tool"
)

func rootsOfTool(t *testing.T, tools []tool.Tool, name string, args map[string]any) []tool.Root {
	t.Helper()
	raw, _ := json.Marshal(args)
	for _, candidate := range tools {
		if candidate.Name() == name {
			return tool.RootsOf(candidate, raw)
		}
	}
	t.Fatalf("no built-in named %s", name)
	return nil
}

func rootFixture(t *testing.T) (ws, ext, token string, tools []tool.Tool) {
	t.Helper()
	base := testenv.TempDir(t)
	ws = filepath.Join(base, "project")
	ext = filepath.Join(base, "meta")
	for _, d := range []string{ws, ext} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	token = "__reasonix_external_folder/abcdef123456/meta"
	resolver := NewPathResolver()
	resolver.RegisterReadRoot(token, ext)
	return ws, ext, token, Workspace{Dir: ws, ReadPaths: resolver}.Tools()
}

func TestReadersNameTheWorkspaceUnlessAGrantedTokenNamesAnExternalFolder(t *testing.T) {
	ws, ext, token, tools := rootFixture(t)
	for _, tc := range []struct {
		tool string
		args map[string]any
		want tool.Root
	}{
		{"ls", map[string]any{}, tool.Root{Kind: tool.RootWorkspace, Path: ws, Display: ws}},
		{"ls", map[string]any{"path": token}, tool.Root{Kind: tool.RootExternal, Path: ext, Display: token}},
		{"read_file", map[string]any{"path": "a.txt"}, tool.Root{Kind: tool.RootWorkspace, Path: ws, Display: ws}},
		{"read_file", map[string]any{"path": token + "/a.txt"}, tool.Root{Kind: tool.RootExternal, Path: ext, Display: token}},
		{"glob", map[string]any{"pattern": "*.go"}, tool.Root{Kind: tool.RootWorkspace, Path: ws, Display: ws}},
		{"glob", map[string]any{"pattern": token + "/*.txt"}, tool.Root{Kind: tool.RootExternal, Path: ext, Display: token}},
		{"grep", map[string]any{"pattern": "x"}, tool.Root{Kind: tool.RootWorkspace, Path: ws, Display: ws}},
		{"grep", map[string]any{"pattern": "x", "path": token}, tool.Root{Kind: tool.RootExternal, Path: ext, Display: token}},
		{"write_file", map[string]any{"path": "n.txt"}, tool.Root{Kind: tool.RootWorkspace, Path: ws, Display: ws}},
		{"edit_file", map[string]any{"path": "n.txt"}, tool.Root{Kind: tool.RootWorkspace, Path: ws, Display: ws}},
		{"bash", map[string]any{"command": "ls"}, tool.Root{Kind: tool.RootWorkspace, Path: ws, Display: ws}},
	} {
		got := rootsOfTool(t, tools, tc.tool, tc.args)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%s %v: roots = %+v, want [%+v]", tc.tool, tc.args, got, tc.want)
		}
	}
}

func TestShellNamesEachGrantedFolderItsCommandMentions(t *testing.T) {
	ws, ext, token, tools := rootFixture(t)
	got := rootsOfTool(t, tools, "bash", map[string]any{"command": "ls " + token})
	want := []tool.Root{{Kind: tool.RootWorkspace, Path: ws, Display: ws}, {Kind: tool.RootExternal, Path: ext, Display: token}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("roots = %+v, want %+v", got, want)
	}
}

func TestAnAbsolutePathOutsideTheWorkspaceDoesNotNeedTheWorkspace(t *testing.T) {
	_, ext, _, tools := rootFixture(t)
	for _, name := range []string{"read_file", "ls", "write_file"} {
		if got := rootsOfTool(t, tools, name, map[string]any{"path": filepath.Join(ext, "a.txt")}); len(got) != 0 {
			t.Errorf("%s of an absolute path elsewhere named %+v; a missing project folder must not block it", name, got)
		}
	}
}

func TestGlobAndGrepOnAGoneRootAreRefusedNotAnsweredWithNoMatches(t *testing.T) {
	ws, ext, token, tools := rootFixture(t)
	if err := os.Rename(ext, ext+"-moved"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"glob", "grep"} {
		args := map[string]any{"pattern": token + "/*.txt"}
		if name == "grep" {
			args = map[string]any{"pattern": "hello", "path": token}
		}
		err := tool.MissingRoot(rootsOfTool(t, tools, name, args), os.Stat)
		if !errors.Is(err, tool.ErrRootMissing) {
			t.Errorf("%s on a folder that is gone: %v, want a root-missing refusal", name, err)
		}
	}
	if err := os.Rename(ws, ws+"-moved"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"glob", "grep", "ls", "read_file", "bash"} {
		args := map[string]any{"pattern": "*.go", "path": "."}
		if name == "bash" {
			args = map[string]any{"command": "ls"}
		}
		if err := tool.MissingRoot(rootsOfTool(t, tools, name, args), os.Stat); !errors.Is(err, tool.ErrRootMissing) {
			t.Errorf("%s with the project folder gone: %v, want a root-missing refusal", name, err)
		}
	}
}
