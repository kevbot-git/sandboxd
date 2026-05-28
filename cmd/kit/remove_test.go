package kit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevbot-git/boxd/internal/lockfile"
	"github.com/kevbot-git/boxd/internal/selections"
)

// makeSelectionsStore creates a selections.json at selPath with the given data.
func makeSelectionsStore(t *testing.T, selPath string, data map[string][]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(selPath), 0o755); err != nil {
		t.Fatalf("makeSelectionsStore MkdirAll: %v", err)
	}
	s, err := selections.Load(selPath)
	if err != nil {
		t.Fatalf("makeSelectionsStore Load: %v", err)
	}
	for cwd, kits := range data {
		s.Set(cwd, kits)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("makeSelectionsStore Save: %v", err)
	}
}

// paths returns the standard lockfile and selections paths under fakeHome.
func defaultPaths(home string) (lockPath, selPath string) {
	lockPath = filepath.Join(home, ".boxd", "boxd.lock.json")
	selPath = filepath.Join(home, ".boxd", "selections.json")
	return
}

// TestRunRemove_KitSuffix_SparseRemoveNotLast verifies Case A: removing one kit
// from a sparse repo (not the last kit) shrinks the sparse set.
func TestRunRemove_KitSuffix_SparseRemoveNotLast(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	srcURL := makeMultiKitRepo(t)

	// Create a real sparse install with both kit1 and kit2.
	installDir := filepath.Join(home, ".boxd", "kits", "test-repo")
	if err := os.MkdirAll(filepath.Dir(installDir), 0o755); err != nil {
		t.Fatal(err)
	}
	gitExec(t, "clone", "--no-checkout", srcURL, installDir)
	gitExec(t, "-C", installDir, "sparse-checkout", "init", "--cone")
	gitExec(t, "-C", installDir, "sparse-checkout", "set", "kit1", "kit2")
	gitExec(t, "-C", installDir, "checkout")

	commitCmd := exec.Command("git", "-C", installDir, "rev-parse", "HEAD")
	commitOut, err := commitCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	commit := strings.TrimSpace(string(commitOut))

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/test/test-repo": {
			SourceURL:   srcURL,
			Commit:      commit,
			InstallDir:  installDir,
			InstalledAt: time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
			Kits:        []string{"kit1", "kit2"},
			InstallMode: "sparse",
		},
	})

	var stderr bytes.Buffer
	if err := runRemove(&stderr, lockPath, selPath, "github.com/test/test-repo/kit2"); err != nil {
		t.Fatalf("runRemove Case A: %v", err)
	}

	// kit2 should be gone from disk (sparse checkout updated).
	if _, err := os.Stat(filepath.Join(installDir, "kit2")); err == nil {
		t.Error("kit2 should be absent after sparse removal")
	}

	// kit1 should still be there.
	if _, err := os.Stat(filepath.Join(installDir, "kit1")); err != nil {
		t.Errorf("kit1 should still exist after partial sparse removal: %v", err)
	}

	// Lockfile should have kit1 only, still sparse.
	s, _ := lockfile.Load(lockPath)
	entry, ok := s.Get("github.com/test/test-repo")
	if !ok {
		t.Fatal("entry missing after partial removal")
	}
	if entry.InstallMode != "sparse" {
		t.Errorf("InstallMode: got %q, want %q", entry.InstallMode, "sparse")
	}
	if len(entry.Kits) != 1 || entry.Kits[0] != "kit1" {
		t.Errorf("Kits: got %v, want [kit1]", entry.Kits)
	}
}

// TestRunRemove_KitSuffix_SparseRemoveLast verifies Case A (last kit): removing
// the last kit from a sparse repo removes the whole repo.
func TestRunRemove_KitSuffix_SparseRemoveLast(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	srcURL := makeMultiKitRepo(t)

	// Create a real sparse install with only kit1.
	installDir := filepath.Join(home, ".boxd", "kits", "test-repo")
	if err := os.MkdirAll(filepath.Dir(installDir), 0o755); err != nil {
		t.Fatal(err)
	}
	gitExec(t, "clone", "--no-checkout", srcURL, installDir)
	gitExec(t, "-C", installDir, "sparse-checkout", "init", "--cone")
	gitExec(t, "-C", installDir, "sparse-checkout", "set", "kit1")
	gitExec(t, "-C", installDir, "checkout")

	commitCmd := exec.Command("git", "-C", installDir, "rev-parse", "HEAD")
	commitOut, err := commitCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	commit := strings.TrimSpace(string(commitOut))

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/test/test-repo": {
			SourceURL:   srcURL,
			Commit:      commit,
			InstallDir:  installDir,
			InstalledAt: time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
			Kits:        []string{"kit1"},
			InstallMode: "sparse",
		},
	})

	var stderr bytes.Buffer
	if err := runRemove(&stderr, lockPath, selPath, "github.com/test/test-repo/kit1"); err != nil {
		t.Fatalf("runRemove Case A last kit: %v", err)
	}

	// Install dir should be gone entirely.
	if _, err := os.Stat(installDir); err == nil {
		t.Error("installDir should be removed after last kit removal")
	}

	// Lockfile entry should be gone.
	s, _ := lockfile.Load(lockPath)
	if _, ok := s.Get("github.com/test/test-repo"); ok {
		t.Error("lockfile entry should be deleted after last kit removal")
	}
}

// TestRunRemove_KitSuffix_FullRepoError verifies Case B: removing a kit suffix
// from a fully-installed repo returns a clear error.
func TestRunRemove_KitSuffix_FullRepoError(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:   "https://github.com/example/sbx-kits.git",
			Commit:      "abc1234",
			InstallDir:  installDir,
			InstalledAt: time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
			Kits:        []string{"bun", "node"},
			InstallMode: "full",
		},
	})

	var stderr bytes.Buffer
	err := runRemove(&stderr, lockPath, selPath, "example/sbx-kits/bun")
	if err == nil {
		t.Fatal("expected error when removing kit from fully-installed repo")
	}
	if !strings.Contains(err.Error(), "cannot remove individual kits") {
		t.Errorf("error should mention 'cannot remove individual kits'; got: %v", err)
	}
	if !strings.Contains(err.Error(), "boxd kit remove") {
		t.Errorf("error should mention 'boxd kit remove'; got: %v", err)
	}
}

// TestRunRemove_KitSuffix_FullRepoError_EmptyInstallMode verifies Case B also
// triggers for entries with no InstallMode (treated as "full").
func TestRunRemove_KitSuffix_FullRepoError_EmptyInstallMode(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:  "https://github.com/example/sbx-kits.git",
			Commit:     "abc1234",
			InstallDir: installDir,
			Kits:       []string{"bun"},
			// InstallMode intentionally empty — treated as "full".
		},
	})

	var stderr bytes.Buffer
	err := runRemove(&stderr, lockPath, selPath, "example/sbx-kits/bun")
	if err == nil {
		t.Fatal("expected error for empty InstallMode treated as full")
	}
	if !strings.Contains(err.Error(), "cannot remove individual kits") {
		t.Errorf("error should mention 'cannot remove individual kits'; got: %v", err)
	}
}

// TestRunRemove_NotInstalled verifies a clear error when the key is absent.
func TestRunRemove_NotInstalled(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)
	makeStore(t, lockPath, nil)

	var stderr bytes.Buffer
	err := runRemove(&stderr, lockPath, selPath, "example/sbx-kits")
	if err == nil {
		t.Fatal("expected error for non-installed key")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error should say 'not installed'; got: %v", err)
	}
}

// TestRunRemove_Success verifies that the install dir is removed and the
// lockfile entry is deleted.
func TestRunRemove_Success(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	// Create a fake install dir.
	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Put a sentinel file inside.
	if err := os.WriteFile(filepath.Join(installDir, "sentinel"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			InstallDir: installDir,
			Commit:     "abc1234",
			UpdatedAt:  now,
		},
	})

	var stderr bytes.Buffer
	err := runRemove(&stderr, lockPath, selPath, "example/sbx-kits")
	if err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	// Install dir must be gone.
	if _, statErr := os.Stat(installDir); !os.IsNotExist(statErr) {
		t.Error("expected install dir to be removed, but it still exists")
	}

	// Lockfile entry must be gone.
	s, _ := lockfile.Load(lockPath)
	if _, ok := s.Get("github.com/example/sbx-kits"); ok {
		t.Error("expected lockfile entry to be deleted")
	}
}

// TestRunRemove_IdempotentDirAlreadyGone verifies that if the dir is already
// absent, the lockfile entry is still removed cleanly.
func TestRunRemove_IdempotentDirAlreadyGone(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	// Deliberately do NOT create the install dir.

	now := time.Now().UTC()
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			InstallDir: installDir,
			Commit:     "abc1234",
			UpdatedAt:  now,
		},
	})

	var stderr bytes.Buffer
	err := runRemove(&stderr, lockPath, selPath, "example/sbx-kits")
	if err != nil {
		t.Fatalf("runRemove with missing dir: %v", err)
	}

	// Lockfile entry must still be gone.
	s, _ := lockfile.Load(lockPath)
	if _, ok := s.Get("github.com/example/sbx-kits"); ok {
		t.Error("expected lockfile entry to be deleted even when dir was already missing")
	}
}

// TestRunRemove_SelectionsWarning verifies that a warning is printed for each
// cwd whose kit paths overlap the install dir.
func TestRunRemove_SelectionsWarning(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			InstallDir: installDir,
			Commit:     "abc1234",
			UpdatedAt:  now,
		},
	})

	// Two cwds reference paths inside the install dir; one does not.
	makeSelectionsStore(t, selPath, map[string][]string{
		"/home/user/project-a": {filepath.Join(installDir, "bun"), "/other/kit"},
		"/home/user/project-b": {filepath.Join(installDir, "node")},
		"/home/user/unrelated": {"/totally/different/path"},
	})

	var stderr bytes.Buffer
	err := runRemove(&stderr, lockPath, selPath, "example/sbx-kits")
	if err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	stderrStr := stderr.String()

	// Two warnings expected (for project-a and project-b), not for unrelated.
	if !strings.Contains(stderrStr, "/home/user/project-a") {
		t.Errorf("expected warning for project-a; stderr: %q", stderrStr)
	}
	if !strings.Contains(stderrStr, "/home/user/project-b") {
		t.Errorf("expected warning for project-b; stderr: %q", stderrStr)
	}
	if strings.Contains(stderrStr, "/home/user/unrelated") {
		t.Errorf("unexpected warning for unrelated cwd; stderr: %q", stderrStr)
	}
}

// TestRunRemove_SelectionsNotAutoClean verifies that selections.json is NOT
// modified after a successful remove.
func TestRunRemove_SelectionsNotAutoClean(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			InstallDir: installDir,
			Commit:     "abc1234",
			UpdatedAt:  now,
		},
	})

	originalKits := []string{filepath.Join(installDir, "bun")}
	makeSelectionsStore(t, selPath, map[string][]string{
		"/home/user/project-a": originalKits,
	})

	// Record the raw bytes before removal.
	before, err := os.ReadFile(selPath)
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	var stderr bytes.Buffer
	if err := runRemove(&stderr, lockPath, selPath, "example/sbx-kits"); err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	// selections.json must be byte-for-byte identical.
	after, err := os.ReadFile(selPath)
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("selections.json was modified after remove.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestRunRemove_InvalidAddress verifies a parse error is returned.
func TestRunRemove_InvalidAddress(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	var stderr bytes.Buffer
	err := runRemove(&stderr, lockPath, selPath, "notanaddress")
	if err == nil {
		t.Fatal("expected error for invalid address")
	}
}

// TestRunRemove_LocalRepo_HappyPath verifies that removing a local repo entry
// deletes the symlink at ~/.boxd/kits/<basename> but leaves the source dir intact.
func TestRunRemove_LocalRepo_HappyPath(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	// Create a real source directory with a file inside.
	srcDir := filepath.Join(home, "src", "my-kits")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sentinel.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create the symlink at ~/.boxd/kits/my-kits pointing to srcDir.
	kitsDir := filepath.Join(home, ".boxd", "kits")
	if err := os.MkdirAll(kitsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(kitsDir, "my-kits")
	if err := os.Symlink(srcDir, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// Write lockfile entry with InstallMode: "local", key = basename.
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"my-kits": {
			SourceURL:   srcDir,
			InstallDir:  symlinkPath,
			InstallMode: "local",
		},
	})

	var stderr bytes.Buffer
	if err := runRemove(&stderr, lockPath, selPath, "my-kits"); err != nil {
		t.Fatalf("runRemove local happy path: %v", err)
	}

	// Symlink must be gone.
	if _, err := os.Lstat(symlinkPath); err == nil {
		t.Error("symlink should be gone after local remove")
	}

	// Source directory must still exist with its file intact.
	if _, err := os.Stat(srcDir); err != nil {
		t.Errorf("source directory should still exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srcDir, "sentinel.txt")); err != nil {
		t.Errorf("sentinel file in source dir should still exist: %v", err)
	}

	// Lockfile entry must be gone.
	s, _ := lockfile.Load(lockPath)
	if _, ok := s.Get("my-kits"); ok {
		t.Error("lockfile entry should be deleted after local remove")
	}
}

// TestRunRemove_LocalRepo_AlreadyMissingSymlink verifies that when the symlink
// is already missing, we warn on stderr, clean up the lockfile, and return nil.
func TestRunRemove_LocalRepo_AlreadyMissingSymlink(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	srcDir := filepath.Join(home, "src", "my-kits")
	symlinkPath := filepath.Join(home, ".boxd", "kits", "my-kits")

	// Do NOT create the symlink — it's already missing.
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"my-kits": {
			SourceURL:   srcDir,
			InstallDir:  symlinkPath,
			InstallMode: "local",
		},
	})

	var stderr bytes.Buffer
	err := runRemove(&stderr, lockPath, selPath, "my-kits")
	if err != nil {
		t.Fatalf("runRemove with missing symlink should return nil, got: %v", err)
	}

	// stderr must contain "warning".
	stderrStr := stderr.String()
	if !strings.Contains(strings.ToLower(stderrStr), "warning") {
		t.Errorf("expected warning on stderr; got: %q", stderrStr)
	}

	// Lockfile entry must be gone.
	s, _ := lockfile.Load(lockPath)
	if _, ok := s.Get("my-kits"); ok {
		t.Error("lockfile entry should be deleted even when symlink was already missing")
	}
}

// TestRunRemove_LocalRepo_SourceUntouched verifies that multiple files in the
// source directory are all untouched after removing the symlink.
func TestRunRemove_LocalRepo_SourceUntouched(t *testing.T) {
	home := redirectHome(t)
	lockPath, selPath := defaultPaths(home)

	srcDir := filepath.Join(home, "src", "my-kits")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{"alpha.sh", "beta.sh", "gamma.sh"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(srcDir, f), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	kitsDir := filepath.Join(home, ".boxd", "kits")
	if err := os.MkdirAll(kitsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(kitsDir, "my-kits")
	if err := os.Symlink(srcDir, symlinkPath); err != nil {
		t.Fatal(err)
	}

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"my-kits": {
			SourceURL:   srcDir,
			InstallDir:  symlinkPath,
			InstallMode: "local",
		},
	})

	var stderr bytes.Buffer
	if err := runRemove(&stderr, lockPath, selPath, "my-kits"); err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	// All source files must still exist.
	for _, f := range files {
		path := filepath.Join(srcDir, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("source file %s should still exist after remove: %v", f, err)
		}
	}
}

// TestRemoveCmd_RmAliasRegistered verifies that "rm" is registered as an alias
// for the remove command.
func TestRemoveCmd_RmAliasRegistered(t *testing.T) {
	for _, alias := range removeCmd.Aliases {
		if alias == "rm" {
			return
		}
	}
	t.Error("removeCmd.Aliases does not contain 'rm'")
}
