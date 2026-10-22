package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// skipUnlessPermissionsDeny skips fixtures whose premise is that chmod 000
// denies a read: Windows has no such mode and root ignores it.
func skipUnlessPermissionsDeny(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 does not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 000 does not deny reads")
	}
}

func unreadableFactFixture(name string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: legacy %s\nmetadata:\n  type: project\n  scope: project\n---\n\nbody of %s\n", name, name, name)
}

func migratedFactFixture(name string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: migrated %s\nid: legacy-%s\nrevision: 1\nmetadata:\n  type: project\n  scope: project\n---\n\nbody of %s\n", name, name, name, name)
}

func factHasIdentity(t *testing.T, path string) bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	frontmatter, _ := splitFrontmatter(string(raw))
	return strings.TrimSpace(frontmatter["id"]) != "" && frontmatter["revision"] != ""
}

// One unreadable fact used to end the pass, leaving later facts in the same
// directory and every later directory unmigrated. It is now named in the
// report and skipped, and the rest of the store still migrates.
func TestMigrationContinuesPastAnUnreadableFact(t *testing.T) {
	skipUnlessPermissionsDeny(t)
	root := testenv.TempDir(t)
	global := filepath.Join(root, "global")
	project := filepath.Join(root, "project")
	for _, dir := range []string{global, project} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := Store{Dir: project, GlobalDir: global}

	write := func(dir, name string) string {
		path := filepath.Join(dir, name+".md")
		if err := os.WriteFile(path, []byte(unreadableFactFixture(name)), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	alphaPath := write(global, "alpha")
	betaPath := write(global, "beta")
	zetaPath := write(global, "zeta")
	omegaPath := write(project, "omega")

	betaInfo, err := os.Stat(betaPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(betaPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(betaPath, 0o644) })

	report, err := s.MigrateV2WithVersion("test")
	if err != nil {
		t.Fatalf("an unreadable fact is reported, not returned as a failed pass: %v", err)
	}
	if want := []string{"beta.md"}; !slices.Equal(report.Unreadable, want) {
		t.Fatalf("report.Unreadable = %v, want %v", report.Unreadable, want)
	}
	if report.Migrated != 3 {
		t.Fatalf("report.Migrated = %d, want alpha, zeta and omega migrated", report.Migrated)
	}
	for name, path := range map[string]string{"alpha": alphaPath, "zeta": zetaPath, "omega": omegaPath} {
		if !factHasIdentity(t, path) {
			t.Errorf("%s.md was not migrated", name)
		}
	}
	if info, statErr := os.Stat(betaPath); statErr != nil || info.Size() != betaInfo.Size() || info.Mode().Perm() != 0o000 {
		t.Fatalf("the unreadable fact was touched by the pass: info=%v err=%v", info, statErr)
	}
}

// The report is ordered so an operator sees the same list twice: entry order is
// lexical inside one directory, so only sorting across the two directories
// puts the project's alpha before the global zeta.
func TestMigrationReportsUnreadableFactsSorted(t *testing.T) {
	skipUnlessPermissionsDeny(t)
	root := testenv.TempDir(t)
	global := filepath.Join(root, "global")
	project := filepath.Join(root, "project")
	for _, dir := range []string{global, project} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	block := func(dir, name string) {
		path := filepath.Join(dir, name+".md")
		if err := os.WriteFile(path, []byte(unreadableFactFixture(name)), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	}
	block(global, "zeta")
	block(project, "alpha")

	report, err := (Store{Dir: project, GlobalDir: global}).MigrateV2WithVersion("test")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alpha.md", "zeta.md"}; !slices.Equal(report.Unreadable, want) {
		t.Fatalf("report.Unreadable = %v, want the sorted names %v", report.Unreadable, want)
	}
}

// A fully readable store reports nothing unreadable: the common path is
// unchanged.
func TestMigrationOfAReadableStoreReportsNoUnreadableFacts(t *testing.T) {
	dir := testenv.TempDir(t)
	path := filepath.Join(dir, "alpha.md")
	if err := os.WriteFile(path, []byte(unreadableFactFixture("alpha")), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := (Store{Dir: dir}).MigrateV2()
	if err != nil {
		t.Fatal(err)
	}
	if report.Migrated != 1 || len(report.Unreadable) != 0 {
		t.Fatalf("report = %+v, want one migrated fact and no unreadable names", report)
	}
}

// A migrated fact behind an unreadable one still reaches the repair path: its
// index line is republished even though the pass could not read its neighbour.
func TestMigrationReachesTheRepairPathPastAnUnreadableFact(t *testing.T) {
	skipUnlessPermissionsDeny(t)
	root := testenv.TempDir(t)
	global := filepath.Join(root, "global")
	project := filepath.Join(root, "project")
	for _, dir := range []string{global, project} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	betaPath := filepath.Join(global, "beta.md")
	if err := os.WriteFile(betaPath, []byte(unreadableFactFixture("beta")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(global, "delta.md"), []byte(migratedFactFixture("delta")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(betaPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(betaPath, 0o644) })

	report, err := (Store{Dir: project, GlobalDir: global}).MigrateV2WithVersion("test")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"beta.md"}; !slices.Equal(report.Unreadable, want) {
		t.Fatalf("report.Unreadable = %v, want %v", report.Unreadable, want)
	}
	index, err := os.ReadFile(filepath.Join(global, indexFile))
	if err != nil {
		t.Fatalf("the pass never reached the repair path: %v", err)
	}
	if !strings.Contains(string(index), "delta.md") {
		t.Fatalf("the repair path did not republish the already-migrated fact:\n%s", index)
	}
}
