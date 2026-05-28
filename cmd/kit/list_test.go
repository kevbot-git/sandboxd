package kit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevbot-git/sandboxd/internal/lockfile"
)

// makeStore creates and saves a lockfile at lockPath with the given entries,
// then returns the path. If entries is empty, it just creates an empty store.
func makeStore(t *testing.T, lockPath string, entries map[string]lockfile.Entry) {
	t.Helper()
	store, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("makeStore Load: %v", err)
	}
	for key, entry := range entries {
		store.Set(key, entry)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("makeStore Save: %v", err)
	}
}

func TestRunList_EmptyLockfile(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")
	makeStore(t, lockPath, nil)

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "no kits installed") {
		t.Errorf("expected 'no kits installed'; got: %q", got)
	}
	if !strings.Contains(got, "boxd kit add") {
		t.Errorf("expected 'boxd kit add' hint; got: %q", got)
	}
}

func TestRunList_MissingLockfile(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")
	// Don't create the file — it should be treated as empty.

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList on missing file: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "no kits installed") {
		t.Errorf("expected 'no kits installed' for missing file; got: %q", got)
	}
}

func TestRunList_SingleEntry(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	updatedAt := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:   "https://github.com/example/sbx-kits.git",
			Commit:      "abc1234567890",
			InstallDir:  "/home/user/.boxd/kits/sbx-kits",
			InstalledAt: updatedAt,
			UpdatedAt:   updatedAt,
			Kits:        []string{"bun", "node"},
		},
	})

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "github.com/example/sbx-kits") {
		t.Errorf("missing key; got: %q", got)
	}
	if !strings.Contains(got, "abc1234") {
		t.Errorf("missing short SHA; got: %q", got)
	}
	if !strings.Contains(got, "2025-03-10") {
		t.Errorf("missing date; got: %q", got)
	}
	if !strings.Contains(got, "bun") || !strings.Contains(got, "node") {
		t.Errorf("missing kit names; got: %q", got)
	}
}

func TestRunList_MultipleEntriesAlphabetical(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/zz/repo": {Commit: "zzz0000", UpdatedAt: now, Kits: []string{"z"}},
		"github.com/aa/repo": {Commit: "aaa1111", UpdatedAt: now, Kits: []string{"a"}},
		"github.com/mm/repo": {Commit: "mmm2222", UpdatedAt: now, Kits: []string{"m"}},
	})

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := buf.String()
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines; got %d:\n%s", len(lines), got)
	}

	// Lines should be in alphabetical order by key.
	if !strings.HasPrefix(lines[0], "github.com/aa/repo") {
		t.Errorf("line 0 should start with github.com/aa/repo; got: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "github.com/mm/repo") {
		t.Errorf("line 1 should start with github.com/mm/repo; got: %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "github.com/zz/repo") {
		t.Errorf("line 2 should start with github.com/zz/repo; got: %q", lines[2])
	}
}

func TestRunList_DirMissing(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	updatedAt := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	// InstallDir is set to a path that doesn't exist on disk.
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:   "https://github.com/example/sbx-kits.git",
			Commit:      "abc1234567890",
			InstallDir:  filepath.Join(home, ".boxd", "kits", "sbx-kits-nonexistent"),
			InstalledAt: updatedAt,
			UpdatedAt:   updatedAt,
			Kits:        []string{"bun", "node"},
		},
	})

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "[dir missing]") {
		t.Errorf("expected '[dir missing]' indicator for absent installDir; got: %q", got)
	}
}

func TestRunList_DirPresent_NoMissingIndicator(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Create the install dir so it actually exists.
	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}

	updatedAt := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:   "https://github.com/example/sbx-kits.git",
			Commit:      "abc1234567890",
			InstallDir:  installDir,
			InstalledAt: updatedAt,
			UpdatedAt:   updatedAt,
			Kits:        []string{"bun", "node"},
		},
	})

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := buf.String()
	if strings.Contains(got, "[dir missing]") {
		t.Errorf("expected no '[dir missing]' for existing installDir; got: %q", got)
	}
}

func TestRunList_LsAliasRegistered(t *testing.T) {
	// Verify the Cobra command has "ls" in its Aliases.
	for _, alias := range listCmd.Aliases {
		if alias == "ls" {
			return
		}
	}
	t.Error("listCmd.Aliases does not contain 'ls'")
}

func TestRunList_LocalEntry_CleanBranch(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	repoPath := makeLocalKitRepo(t)
	basename := filepath.Base(repoPath)

	// Create symlink at ~/.boxd/kits/<basename> → repoPath.
	symlinkPath := filepath.Join(home, ".boxd", "kits", basename)
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repoPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	updatedAt := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"local/" + basename: {
			SourceURL:   repoPath,
			Commit:      "abc1234567890",
			InstallDir:  symlinkPath,
			InstalledAt: updatedAt,
			UpdatedAt:   updatedAt,
			Kits:        []string{"mykit"},
			InstallMode: "local",
		},
	})

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := buf.String()
	// Should contain a branch name in brackets (main or master).
	if !strings.Contains(got, "[main]") && !strings.Contains(got, "[master]") {
		t.Errorf("expected [main] or [master] in output; got: %q", got)
	}
	// Should NOT contain a 7-char SHA for this local entry.
	if strings.Contains(got, "abc1234") {
		t.Errorf("local entry should not show commit SHA; got: %q", got)
	}
}

func TestRunList_LocalEntry_DirtyBranch(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	repoPath := makeLocalKitRepo(t)
	basename := filepath.Base(repoPath)

	// Stage an untracked file so the working tree is dirty.
	if err := os.WriteFile(filepath.Join(repoPath, "dirty.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitExec(t, "-C", repoPath, "add", "dirty.txt")

	symlinkPath := filepath.Join(home, ".boxd", "kits", basename)
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repoPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	updatedAt := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"local/" + basename: {
			SourceURL:   repoPath,
			Commit:      "abc1234567890",
			InstallDir:  symlinkPath,
			InstalledAt: updatedAt,
			UpdatedAt:   updatedAt,
			Kits:        []string{"mykit"},
			InstallMode: "local",
		},
	})

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "[main*]") && !strings.Contains(got, "[master*]") {
		t.Errorf("expected [main*] or [master*] in output; got: %q", got)
	}
}

func TestRunList_LocalEntry_MissingTarget(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Symlink points to a nonexistent directory.
	symlinkPath := filepath.Join(home, ".boxd", "kits", "gone-repo")
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nonexistent/path/that/does/not/exist", symlinkPath); err != nil {
		t.Fatal(err)
	}

	updatedAt := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"local/gone-repo": {
			SourceURL:   "/nonexistent/path/that/does/not/exist",
			Commit:      "abc1234567890",
			InstallDir:  symlinkPath,
			InstalledAt: updatedAt,
			UpdatedAt:   updatedAt,
			Kits:        []string{"mykit"},
			InstallMode: "local",
		},
	})

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "[missing]") {
		t.Errorf("expected [missing] in output; got: %q", got)
	}
}

func TestRunList_RemoteEntry_Unchanged(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	updatedAt := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:   "https://github.com/example/sbx-kits.git",
			Commit:      "abc1234567890",
			InstallDir:  filepath.Join(home, ".boxd", "kits", "sbx-kits"),
			InstalledAt: updatedAt,
			UpdatedAt:   updatedAt,
			Kits:        []string{"bun"},
			// No InstallMode — defaults to remote behavior.
		},
	})

	var buf bytes.Buffer
	if err := runList(&buf, lockPath); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := buf.String()
	// Remote entry must still show the commit SHA.
	if !strings.Contains(got, "abc1234") {
		t.Errorf("remote entry should show commit SHA; got: %q", got)
	}
	// Remote entry must NOT show branch notation.
	if strings.Contains(got, "[main]") || strings.Contains(got, "[master]") {
		t.Errorf("remote entry should not show branch notation; got: %q", got)
	}
}
