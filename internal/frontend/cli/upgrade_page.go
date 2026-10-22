package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// An archive that carries the built page puts it under this directory.
const pageArchiveRoot = "frontend-next/dist/"

// The page is verified against SHA256SUMS before it is read, so these bounds
// are not what stops a forged archive; they keep a malformed release from
// filling the disk.
const (
	maxPageEntries   = 4096
	maxPageFileBytes = 64 << 20
	maxPageBytes     = 128 << 20
)

type pageFile struct {
	rel  string
	data []byte
}

// pageTree is the page read out of a release archive, held in memory until the
// whole of it has been validated: nothing is written to the install before then.
type pageTree struct{ files []pageFile }

// upgradeRename is the one rename the page swap uses, so a test can fail either step.
var upgradeRename = os.Rename

// applyBinary replaces the running executable; a variable so a test can fail it
// after the page is already in place.
var applyBinary = replaceBinary

// extractPage reads the page tree out of a verified release archive. An archive
// without one yields (nil, nil): older releases carry none. Anything under
// frontend-next/ that is not a plain file or directory, and any name that could
// land outside it, refuses the whole page.
func extractPage(data []byte, archiveName string) (*pageTree, error) {
	var entries []rawPageEntry
	var err error
	if strings.HasSuffix(archiveName, ".zip") {
		entries, err = zipPageEntries(data)
	} else {
		entries, err = tarPageEntries(data)
	}
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	tree := &pageTree{}
	seen := map[string]bool{}
	names := newPageNames()
	var total int64
	for _, e := range entries {
		rel, err := pageRel(e.name)
		if err != nil {
			return nil, err
		}
		if seen[rel] {
			return nil, fmt.Errorf("page entry %q appears twice", rel)
		}
		seen[rel] = true
		if err := names.add(rel); err != nil {
			return nil, err
		}
		if len(tree.files) >= maxPageEntries {
			return nil, errors.New("page has too many files")
		}
		if e.size > maxPageFileBytes {
			return nil, fmt.Errorf("page file %q is too large", rel)
		}
		total += e.size
		if total > maxPageBytes {
			return nil, errors.New("page is too large")
		}
		body, err := e.read()
		if err != nil {
			return nil, err
		}
		if int64(len(body)) != e.size {
			return nil, fmt.Errorf("page file %q does not match its declared size", rel)
		}
		tree.files = append(tree.files, pageFile{rel: rel, data: body})
	}
	if !seen["index.html"] {
		return nil, errors.New("page has no index.html")
	}
	return tree, nil
}

type rawPageEntry struct {
	name string
	size int64
	read func() ([]byte, error)
}

// pageRel turns an archive name under frontend-next/dist/ into a path relative
// to the page, or says why it cannot be one. It reads the name as written:
// cleaning it first would turn a traversal into an innocent-looking path.
func pageRel(name string) (string, error) {
	if strings.ContainsAny(name, "\\\x00") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("page entry %q has a forbidden form", name)
	}
	parts := strings.Split(strings.TrimPrefix(name, "./"), "/")
	if len(parts) < 3 || parts[0] != "frontend-next" || parts[1] != "dist" {
		return "", fmt.Errorf("page entry %q is outside %s", name, pageArchiveRoot)
	}
	rel := parts[2:]
	for _, part := range rel {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			return "", fmt.Errorf("page entry %q is not a plain path", name)
		}
	}
	joined := strings.Join(rel, "/")
	if !filepath.IsLocal(filepath.FromSlash(joined)) {
		return "", fmt.Errorf("page entry %q is not local", name)
	}
	return joined, nil
}

// pageNames refuses entries that would land on one another once written: names
// that differ only by case (one file on a case-insensitive volume), a name that
// is both a file and the directory of another entry, and a trailing dot or space
// that Windows drops. Refusing them before anything is written keeps a bad
// archive from failing halfway through the swap.
type pageNames struct {
	folded map[string]string
	files  map[string]bool
	dirs   map[string]bool
}

func newPageNames() *pageNames {
	return &pageNames{folded: map[string]string{}, files: map[string]bool{}, dirs: map[string]bool{}}
}

func (n *pageNames) add(rel string) error {
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return fmt.Errorf("page entry %q ends in a dot or space", rel)
		}
	}
	key := strings.ToLower(rel)
	if other, clash := n.folded[key]; clash {
		return fmt.Errorf("page entries %q and %q differ only by case", other, rel)
	}
	n.folded[key] = rel
	if n.dirs[key] {
		return fmt.Errorf("page entry %q is a file and also a directory", rel)
	}
	n.files[key] = true
	for i := 1; i < len(parts); i++ {
		dir := strings.ToLower(strings.Join(parts[:i], "/"))
		if n.files[dir] {
			return fmt.Errorf("page entry %q is a file and also a directory", dir)
		}
		n.dirs[dir] = true
	}
	return nil
}

// underFrontend picks the entries that belong to the page or claim to, by the
// name as written, so a path that only reaches frontend-next after cleaning is
// still examined and refused rather than skipped.
func underFrontend(name string) bool {
	return strings.HasPrefix(strings.TrimLeft(strings.TrimPrefix(name, "./"), "/"), "frontend-next")
}

func tarPageEntries(data []byte) ([]rawPageEntry, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var out []rawPageEntry
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if !underFrontend(h.Name) {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		default:
			return nil, fmt.Errorf("page entry %q is not a regular file", h.Name)
		}
		if h.Size < 0 || h.Size > maxPageFileBytes {
			return nil, fmt.Errorf("page file %q is too large", h.Name)
		}
		if total += h.Size; total > maxPageBytes {
			return nil, errors.New("page is too large")
		}
		body, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if err != nil {
			return nil, err
		}
		out = append(out, rawPageEntry{name: h.Name, size: h.Size, read: func() ([]byte, error) { return body, nil }})
		if len(out) > maxPageEntries+1 {
			return nil, errors.New("page has too many files")
		}
	}
}

func zipPageEntries(data []byte) ([]rawPageEntry, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var out []rawPageEntry
	for _, f := range zr.File {
		if !underFrontend(f.Name) {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if !f.Mode().IsRegular() {
			return nil, fmt.Errorf("page entry %q is not a regular file", f.Name)
		}
		if f.UncompressedSize64 > maxPageFileBytes {
			return nil, fmt.Errorf("page file %q is too large", f.Name)
		}
		file := f
		out = append(out, rawPageEntry{name: f.Name, size: int64(f.UncompressedSize64), read: func() ([]byte, error) {
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, maxPageFileBytes+1))
		}})
		if len(out) > maxPageEntries+1 {
			return nil, errors.New("page has too many files")
		}
	}
	return out, nil
}

// installedPageDir is the page beside the real executable, or "" when this
// install never had one, or has one that is a link: an upgrade does not add a
// page the user did not choose, and does not replace one the user pointed at a
// build of their own.
func installedPageDir() string {
	dist, linked := pageLocation()
	if linked {
		return ""
	}
	return dist
}

// pageDirIsLink reports a page directory (or the frontend-next folder holding
// it) that is a symbolic link.
func pageDirIsLink() bool {
	_, linked := pageLocation()
	return linked
}

func pageLocation() (dist string, linked bool) {
	exe, err := upgradeExecutable()
	if err != nil {
		return "", false
	}
	resolved, _ := resolveSymlinks(exe)
	folder := filepath.Join(filepath.Dir(resolved), "frontend-next")
	dist = filepath.Join(folder, "dist")
	for _, p := range []string{folder, dist} {
		st, err := os.Lstat(p)
		if err != nil {
			return "", false
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return "", true
		}
	}
	if st, err := os.Stat(dist); err != nil || !st.IsDir() {
		return "", false
	}
	return dist, false
}

// applyUpgrade puts the new page in place and then the new binary, and undoes
// the page when the binary does not land, so the old kernel is never left with a
// page that belongs to the new one. Only frontend-next/dist is replaced.
func applyUpgrade(newBin []byte, tree *pageTree) error {
	dist := ""
	if tree != nil {
		dist = installedPageDir()
	}
	if dist == "" {
		return applyBinary(newBin)
	}
	undo, done, err := swapPage(dist, tree)
	if err != nil {
		return fmt.Errorf("replace page: %w", err)
	}
	if err := applyBinary(newBin); err != nil {
		if uerr := undo(); uerr != nil {
			return fmt.Errorf("%w; restoring the old page also failed: %w", err, uerr)
		}
		return err
	}
	done()
	return nil
}

// swapPage stages the new page beside the live one on the same volume, moves the
// live one aside and the staged one in. undo restores the old page; done drops it.
func swapPage(dist string, tree *pageTree) (undo func() error, done func(), err error) {
	parent := filepath.Dir(dist)
	oldPath := filepath.Join(parent, ".dist.old")
	_ = os.RemoveAll(oldPath)
	stage, err := os.MkdirTemp(parent, ".dist.new-")
	if err != nil {
		return nil, nil, err
	}
	discard := func() { _ = os.RemoveAll(stage) }
	// MkdirTemp makes it 0700; the page it becomes has to be as reachable as the one it replaces.
	if err := os.Chmod(stage, 0o755); err != nil {
		discard()
		return nil, nil, err
	}
	for _, f := range tree.files {
		target := filepath.Join(stage, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			discard()
			return nil, nil, err
		}
		if err := os.WriteFile(target, f.data, 0o644); err != nil {
			discard()
			return nil, nil, err
		}
	}
	if err := upgradeRename(dist, oldPath); err != nil {
		discard()
		return nil, nil, err
	}
	if err := upgradeRename(stage, dist); err != nil {
		discard()
		if rerr := upgradeRename(oldPath, dist); rerr != nil {
			return nil, nil, fmt.Errorf("%w; putting the old page back also failed: %w", err, rerr)
		}
		return nil, nil, err
	}
	undo = func() error {
		failed := filepath.Join(parent, ".dist.failed")
		_ = os.RemoveAll(failed)
		if err := upgradeRename(dist, failed); err != nil {
			return err
		}
		if err := upgradeRename(oldPath, dist); err != nil {
			return err
		}
		_ = os.RemoveAll(failed)
		return nil
	}
	done = func() { _ = os.RemoveAll(oldPath) }
	return undo, done, nil
}
