package worktree

import (
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/base/workspaceid"
)

// ProjectRoot maps a managed checkout to its main checkout's corresponding
// folder. Unknown or missing Git identities keep their own directory.
func ProjectRoot(root, managedRoot string) string {
	if !IsManagedPath(root, managedRoot) {
		return root
	}
	info := workspaceid.Describe(root)
	if info.RepoDir == "" || info.RepoDir == info.GitDir || filepath.Base(info.RepoDir) != ".git" {
		return root
	}
	managed, err := filepath.EvalSymlinks(managedRoot)
	if err != nil || !IsManagedPath(info.CheckoutRoot, managed) || !filepath.IsLocal(info.Prefix) || !registeredCheckout(info) {
		return root
	}
	return filepath.Join(filepath.Dir(info.RepoDir), info.Prefix)
}

func registeredCheckout(info workspaceid.Info) bool {
	rel, err := filepath.Rel(info.RepoDir, info.GitDir)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	repo, err := os.OpenRoot(info.RepoDir)
	if err != nil {
		return false
	}
	defer repo.Close()
	git, err := repo.OpenRoot(rel)
	if err != nil {
		return false
	}
	defer git.Close()
	body, err := git.ReadFile("gitdir")
	if err != nil || strings.TrimSpace(string(body)) == "" {
		return false
	}
	backlink := strings.TrimSpace(string(body))
	if !filepath.IsAbs(backlink) {
		backlink = filepath.Join(info.GitDir, backlink)
	}
	marker, err := filepath.EvalSymlinks(backlink)
	if err != nil || filepath.Base(marker) != ".git" {
		return false
	}
	rel, err = filepath.Rel(info.CheckoutRoot, filepath.Dir(marker))
	return err == nil && rel == "."
}
