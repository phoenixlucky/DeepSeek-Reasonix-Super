package skill

import (
	"os"
	"path/filepath"
	fileencoding "reasonix/internal/base/fileutil/encoding"
	"slices"
	"strings"
)

// Complete returns the skill with its references and scripts folded into Body.
// Discovery leaves them out because a listing needs only the metadata, and
// reading every skill's sibling files per catalog build is what made opening a
// pane scale with the size of the skill library.
func (s Skill) Complete() Skill {
	if !s.supplementsDeferred {
		return s
	}
	if s.confinedSupplements {
		s.Body, s.completionError = completeConfinedSkill(s)
		s.supplementsDeferred = false
		return s
	}
	s.Body = loadBodyWithScripts(s.Path, loadBodyWithReferences(s.Path, s.Body))
	s.supplementsDeferred = false
	return s
}

// loadBodyWithReferences appends a directory-layout skill's sibling
// references/*.md files to its body (Anthropic Skills compatibility), so depth
// material is available without on-demand resolution. Flat skills have no
// references dir and are returned unchanged.
func loadBodyWithReferences(skillPath, body string) string {
	if filepath.Base(skillPath) != SkillFile {
		return body
	}
	refsDir := filepath.Join(filepath.Dir(skillPath), "references")
	entries, err := os.ReadDir(refsDir)
	if err != nil {
		return body
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return body
	}
	slices.Sort(names)
	var b strings.Builder
	b.WriteString(body)
	for _, n := range names {
		content, err := fileencoding.ReadFileUTF8(filepath.Join(refsDir, n))
		if err != nil {
			continue
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			continue
		}
		slug := strings.TrimSuffix(n, filepath.Ext(n))
		b.WriteString("\n\n## Reference: " + slug + "\n\n" + trimmed)
	}
	return b.String()
}

// loadBodyWithScripts appends a directory-layout skill's sibling scripts/
// directory listing to the body, so the model knows what scripts are
// available and can run them via bash (inheriting sandbox, gate, hooks).
func loadBodyWithScripts(skillPath, body string) string {
	if filepath.Base(skillPath) != SkillFile {
		return body
	}
	scriptsDir := filepath.Join(filepath.Dir(skillPath), "scripts")
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return body
	}
	var names []string
	for _, e := range entries {
		// Filter hidden files — bash should not see config dotfiles in scripts/.
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !isScriptExt(filepath.Ext(e.Name())) {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		return body
	}
	slices.Sort(names)
	var b strings.Builder
	b.WriteString(body)
	b.WriteString("\n\n## Scripts\n\nRun a listed script with bash using the exact path shown below; quote the path if it contains spaces.\n\n")
	for _, n := range names {
		b.WriteString("- `" + filepath.Join(scriptsDir, n) + "`\n")
	}
	return b.String()
}

func isScriptExt(ext string) bool {
	switch strings.ToLower(ext) {
	case "", ".sh", ".py", ".js", ".ts", ".rb", ".pl", ".php", ".ps1":
		return true
	default:
		return false
	}
}
