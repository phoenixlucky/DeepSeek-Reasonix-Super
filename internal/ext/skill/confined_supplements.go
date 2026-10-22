package skill

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"reasonix/internal/base/fileutil"
	fileencoding "reasonix/internal/base/fileutil/encoding"
	"reasonix/internal/base/secrets"
)

const maxSupplementBytes = 128 << 20
const maxSupplementFiles = 20000

func completeConfinedSkill(sk Skill) (string, error) {
	path, err := fileutil.ResolveExistingPath(sk.Path)
	if err != nil {
		return "", fmt.Errorf("%w: skill %s source is unavailable", ErrInvocationUnavailable, sk.SlashName())
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("%w: skill %s source is unavailable", ErrInvocationUnavailable, sk.SlashName())
	}
	defer root.Close()
	body, err := confinedSupplementBody(root, sk)
	if err != nil {
		return "", fmt.Errorf("%w: skill %s: %w", ErrInvocationUnavailable, sk.SlashName(), err)
	}
	return body, nil
}

func confinedSupplementBody(root *os.Root, sk Skill) (string, error) {
	var body strings.Builder
	body.WriteString(sk.Body)
	for _, folder := range []string{"references", "scripts"} {
		directory, err := confinedSupplementPath(root, folder)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("cannot open %s within the skill directory", folder)
		}
		dir, err := root.Open(directory)
		if err != nil {
			return "", fmt.Errorf("cannot open %s within the skill directory", folder)
		}
		entries, err := dir.ReadDir(maxSupplementFiles + 1)
		_ = dir.Close()
		if err != nil && !errors.Is(err, io.EOF) || len(entries) > maxSupplementFiles {
			return "", fmt.Errorf("cannot list %s within the skill limit", folder)
		}
		names := []string{}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if folder == "references" && !strings.EqualFold(filepath.Ext(entry.Name()), ".md") || folder == "scripts" && !isScriptExt(filepath.Ext(entry.Name())) {
				continue
			}
			names = append(names, entry.Name())
		}
		slices.Sort(names)
		for i, name := range names {
			rel := filepath.Join(folder, name)
			resolved, err := confinedSupplementPath(root, rel)
			if err != nil {
				return "", fmt.Errorf("resource %s leaves the allowed skill source", rel)
			}
			info, err := root.Stat(resolved)
			if err != nil || !info.Mode().IsRegular() || secrets.SensitiveFileName(name) {
				return "", fmt.Errorf("resource %s is not an allowed skill file", rel)
			}
			if folder == "references" {
				data, err := readConfinedSupplement(root, resolved, maxSupplementBytes-body.Len())
				if err != nil {
					return "", err
				}
				if text := strings.TrimSpace(string(data)); text != "" {
					body.WriteString("\n\n## Reference: " + strings.TrimSuffix(name, filepath.Ext(name)) + "\n\n" + text)
				}
			} else {
				if i == 0 {
					body.WriteString("\n\n## Scripts\n\nRun a listed script with bash using the exact path shown below; quote the path if it contains spaces.\n\n")
				}
				body.WriteString("- `" + filepath.Join(filepath.Dir(sk.Path), rel) + "`\n")
			}
			if body.Len() > maxSupplementBytes {
				return "", fmt.Errorf("skill supplement limit exceeded")
			}
		}
	}
	return body.String(), nil
}

func confinedSupplementPath(root *os.Root, path string) (string, error) {
	resolved, err := fileutil.ResolveExistingPath(filepath.Join(root.Name(), path))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root.Name(), resolved)
	if err != nil || !filepath.IsLocal(rel) {
		return "", ErrInvocationUnavailable
	}
	for component := range strings.SplitSeq(filepath.ToSlash(rel), "/") {
		if secrets.SensitiveFileName(component) || component == ".git" || component == ".hg" || component == ".svn" {
			return "", ErrInvocationUnavailable
		}
	}
	return rel, nil
}

func readConfinedSupplement(root *os.Root, path string, limit int) ([]byte, error) {
	expected, err := root.Lstat(path)
	if err != nil || !expected.Mode().IsRegular() {
		return nil, fmt.Errorf("resource %s changed before reading", path)
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read resource %s within the skill directory", path)
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil || !os.SameFile(expected, actual) {
		return nil, fmt.Errorf("resource %s changed while opening", path)
	}
	if limit < 0 {
		return nil, fmt.Errorf("skill supplement limit exceeded")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, fmt.Errorf("cannot read resource %s within the skill limit", path)
	}
	data = fileencoding.DecodeToUTF8(data)
	if len(data) > limit {
		return nil, fmt.Errorf("decoded skill supplement limit exceeded")
	}
	return data, nil
}
