package skill

import (
	"path/filepath"
	"reasonix/internal/base/testenv"
	"slices"
	"strings"
	"testing"
)

func TestSpecIssuesJudgeOnlyWhatIsWritten(t *testing.T) {
	dir := filepath.Join("skills", "pdf-processing", SkillFile)
	long := strings.Repeat("x", 65)
	for _, tc := range []struct {
		name string
		path string
		stem string
		fm   map[string]string
		want []string
	}{
		{"conforming", dir, "pdf-processing", map[string]string{"name": "pdf-processing", "description": "d"}, nil},
		{"no name falls back silently", dir, "pdf-processing", map[string]string{"description": "d"}, nil},
		{"uppercase", dir, "pdf-processing", map[string]string{"name": "PDF-processing"}, []string{"uppercase", "match its directory"}},
		{"hyphens", dir, "-a--b-", map[string]string{"name": "-a--b-"}, []string{"starts or ends", "consecutive"}},
		{"characters", dir, "a_b", map[string]string{"name": "a_b"}, []string{"characters other than"}},
		{"too long", dir, long, map[string]string{"name": long}, []string{"65 characters"}},
		{"file skill needs no directory match", filepath.Join("skills", "notes.md"), "notes", map[string]string{"name": "other"}, nil},
		{"long description", dir, "pdf-processing", map[string]string{"description": strings.Repeat("d", 1025)}, []string{"1025 characters"}},
		{"empty compatibility", dir, "pdf-processing", map[string]string{"compatibility": " "}, []string{"compatibility is 0"}},
		{"unicode lowercase name", dir, "数据分析", map[string]string{"name": "数据分析"}, nil},
	} {
		got := specIssues(tc.path, tc.stem, tc.fm)
		if len(got) != len(tc.want) {
			t.Errorf("%s: issues = %q, want %d matching %q", tc.name, got, len(tc.want), tc.want)
			continue
		}
		for _, w := range tc.want {
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, w) }) {
				t.Errorf("%s: issues %q do not say %q", tc.name, got, w)
			}
		}
	}
}

func TestASkillThatBreaksTheSpecStillLoadsAndIsReported(t *testing.T) {
	home := testenv.TempDir(t)
	writeSkill(t, home, ".reasonix/skills/Big-Skill/SKILL.md", "---\nname: Big-Skill\ndescription: does things\n---\nBODY")
	sk, ok := New(Options{HomeDir: home, DisableBuiltins: true}).Read("Big-Skill")
	if !ok || sk.Body != "BODY" {
		t.Fatalf("a skill that departs from the spec stopped loading: %+v ok=%v", sk, ok)
	}
	if !slices.ContainsFunc(sk.Invalid, func(s string) bool { return strings.Contains(s, "uppercase") }) {
		t.Fatalf("Invalid = %q, want the uppercase finding", sk.Invalid)
	}
}
