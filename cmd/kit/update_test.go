package kit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevbot-git/sandboxd/internal/lockfile"
)

// runGit is a helper that runs git with the given args and fails the test on error.
func runGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// makeKitRepo creates a temporary git repository seeded with one kit (a
// subdirectory containing spec.yaml) and returns its absolute path. The repo
// can be cloned via file://<path>.
func makeKitRepo(t *testing.T, kitName string) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, "-C", dir, "init")
	runGit(t, "-C", dir, "config", "user.email", "test@example.com")
	runGit(t, "-C", dir, "config", "user.name", "Test")

	kitDir := filepath.Join(dir, kitName)
	if err := os.MkdirAll(kitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kitDir, "spec.yaml"), []byte("displayName: "+kitName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, "-C", dir, "add", ".")
	runGit(t, "-C", dir, "commit", "-m", "initial commit")
	return dir
}

// cloneRepo clones src (file:// URL) into dest and returns dest.
func cloneRepo(t *testing.T, src, dest string) {
	t.Helper()
	cmd := exec.Command("git", "clone", "file://"+src, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
}

// addCommit adds a new file to the src repo and commits it.
func addCommit(t *testing.T, src, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(src, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, "-C", src, "add", ".")
	runGit(t, "-C", src, "commit", "-m", "add "+filename)
}

// ── Step 1: runUpdateOne errors for non-installed key ────────────────────────

func TestRunUpdateOne_NotInstalled(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")
	makeStore(t, lockPath, nil)

	var buf bytes.Buffer
	err := runUpdateOne(&buf, lockPath, "example/sbx-kits")
	if err == nil {
		t.Fatal("expected error for non-installed key")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error should mention 'not installed'; got: %v", err)
	}
}

// ── Step 2: runUpdateAll on empty lockfile prints nothing, exits 0 ───────────

func TestRunUpdateAll_EmptyLockfile(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")
	makeStore(t, lockPath, nil)

	var buf bytes.Buffer
	if err := runUpdateAll(&buf, lockPath); err != nil {
		t.Fatalf("runUpdateAll on empty lockfile: %v", err)
	}
	// No output expected for empty lockfile.
	if buf.Len() != 0 {
		t.Errorf("expected no output; got: %q", buf.String())
	}
}

// ── Step 3: updateEntry with no upstream change → "already at <sha>" ─────────

func TestUpdateEntry_NoChange(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Create source repo and clone it.
	src := makeKitRepo(t, "mykits")
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo(t, src, cloneDir)

	// Get the current SHA.
	shaCmd := exec.Command("git", "-C", cloneDir, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(shaOut))

	now := time.Now().UTC()
	entry := lockfile.Entry{
		SourceURL:   "file://" + src,
		Commit:      sha,
		InstallDir:  cloneDir,
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        []string{"mykits"},
	}
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/test/repo": entry,
	})

	store, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var buf bytes.Buffer
	if err := updateEntry(&buf, os.Stderr, store, "github.com/test/repo", entry); err != nil {
		t.Fatalf("updateEntry: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "already at") {
		t.Errorf("expected 'already at' in output; got: %q", got)
	}
	shortSHA := sha[:7]
	if !strings.Contains(got, shortSHA) {
		t.Errorf("expected short SHA %q in output; got: %q", shortSHA, got)
	}

	// Lockfile entry must be bit-for-bit unchanged.
	store2, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	e2, ok := store2.Get("github.com/test/repo")
	if !ok {
		t.Fatal("entry missing after updateEntry")
	}
	// updateEntry on a no-change entry should NOT call store.Save (entry unchanged).
	// Verify installedAt and updatedAt are unchanged.
	if !e2.InstalledAt.Equal(entry.InstalledAt) {
		t.Errorf("InstalledAt changed: got %v, want %v", e2.InstalledAt, entry.InstalledAt)
	}
	if !e2.UpdatedAt.Equal(entry.UpdatedAt) {
		t.Errorf("UpdatedAt changed: got %v, want %v", e2.UpdatedAt, entry.UpdatedAt)
	}
}

// ── Step 4: updateEntry when there IS a new upstream commit ──────────────────

func TestUpdateEntry_WithChange(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Create source repo and clone it.
	src := makeKitRepo(t, "mykits")
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo(t, src, cloneDir)

	// Get the old SHA.
	shaCmd := exec.Command("git", "-C", cloneDir, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	oldSHA := strings.TrimSpace(string(shaOut))

	now := time.Now().UTC()
	entry := lockfile.Entry{
		SourceURL:   "file://" + src,
		Commit:      oldSHA,
		InstallDir:  cloneDir,
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        []string{"mykits"},
	}
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/test/repo": entry,
	})

	// Add a new commit to the source repo (add a new kit).
	newKitDir := filepath.Join(src, "newkit")
	if err := os.MkdirAll(newKitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newKitDir, "spec.yaml"), []byte("displayName: newkit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	addCommit(t, src, "newkit/spec.yaml", "already written")

	// Get the new SHA from the source.
	newSHACmd := exec.Command("git", "-C", src, "rev-parse", "HEAD")
	newSHAOut, err := newSHACmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD new: %v", err)
	}
	newSHA := strings.TrimSpace(string(newSHAOut))

	store, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var buf bytes.Buffer
	if err := updateEntry(&buf, os.Stderr, store, "github.com/test/repo", entry); err != nil {
		t.Fatalf("updateEntry: %v", err)
	}

	got := buf.String()
	oldShort := oldSHA[:7]
	newShort := newSHA[:7]

	if !strings.Contains(got, oldShort) {
		t.Errorf("expected old short SHA %q in output; got: %q", oldShort, got)
	}
	if !strings.Contains(got, newShort) {
		t.Errorf("expected new short SHA %q in output; got: %q", newShort, got)
	}
	if !strings.Contains(got, "→") {
		t.Errorf("expected '→' in output; got: %q", got)
	}

	// Reload lockfile and verify it was updated.
	store2, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	e2, ok := store2.Get("github.com/test/repo")
	if !ok {
		t.Fatal("entry missing after update")
	}
	if e2.Commit != newSHA {
		t.Errorf("lockfile commit: got %q, want %q", e2.Commit, newSHA)
	}
	if !e2.InstalledAt.Equal(entry.InstalledAt) {
		t.Errorf("InstalledAt should be preserved; got %v, want %v", e2.InstalledAt, entry.InstalledAt)
	}
	if e2.UpdatedAt.Equal(entry.UpdatedAt) {
		t.Errorf("UpdatedAt should have changed but is still %v", e2.UpdatedAt)
	}
	// New kit should be in kits list.
	foundNewKit := false
	for _, k := range e2.Kits {
		if k == "newkit" {
			foundNewKit = true
		}
	}
	if !foundNewKit {
		t.Errorf("expected 'newkit' in updated kits list; got: %v", e2.Kits)
	}
}

// ── Step 5: runUpdateAll with two entries → both processed ───────────────────

func TestRunUpdateAll_TwoEntries(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Create two source repos and clone them.
	src1 := makeKitRepo(t, "kit1")
	src2 := makeKitRepo(t, "kit2")
	clone1 := filepath.Join(t.TempDir(), "clone1")
	clone2 := filepath.Join(t.TempDir(), "clone2")
	cloneRepo(t, src1, clone1)
	cloneRepo(t, src2, clone2)

	getSHA := func(dir string) string {
		t.Helper()
		cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("rev-parse: %v", err)
		}
		return strings.TrimSpace(string(out))
	}

	sha1 := getSHA(clone1)
	sha2 := getSHA(clone2)
	now := time.Now().UTC()

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/test/repo1": {
			SourceURL:   "file://" + src1,
			Commit:      sha1,
			InstallDir:  clone1,
			InstalledAt: now,
			UpdatedAt:   now,
			Kits:        []string{"kit1"},
		},
		"github.com/test/repo2": {
			SourceURL:   "file://" + src2,
			Commit:      sha2,
			InstallDir:  clone2,
			InstalledAt: now,
			UpdatedAt:   now,
			Kits:        []string{"kit2"},
		},
	})

	var buf bytes.Buffer
	if err := runUpdateAll(&buf, lockPath); err != nil {
		t.Fatalf("runUpdateAll: %v", err)
	}

	got := buf.String()
	// Both repos should appear in output (no-change path).
	if !strings.Contains(got, "github.com/test/repo1") {
		t.Errorf("missing repo1 in output; got: %q", got)
	}
	if !strings.Contains(got, "github.com/test/repo2") {
		t.Errorf("missing repo2 in output; got: %q", got)
	}
	if !strings.Contains(got, "already at") {
		t.Errorf("expected 'already at' for unchanged repos; got: %q", got)
	}
}

// ── Step 5b: updateEntry when installDir is missing → re-clones then updates ──

func TestUpdateEntry_MissingDirReclones(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Create a source repo (acts as the "remote").
	src := makeKitRepo(t, "mykits")

	// Set up a clone destination path that does NOT exist yet.
	cloneDir := filepath.Join(t.TempDir(), "cloned-missing")
	// Deliberately do NOT create cloneDir — simulate a deleted install dir.

	// Get the SHA from the source repo.
	shaCmd := exec.Command("git", "-C", src, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(shaOut))

	now := time.Now().UTC()
	entry := lockfile.Entry{
		SourceURL:   "file://" + src,
		Commit:      sha,
		InstallDir:  cloneDir,
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        []string{"mykits"},
	}
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/test/repo": entry,
	})

	store, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var buf bytes.Buffer
	if err := updateEntry(&buf, os.Stderr, store, "github.com/test/repo", entry); err != nil {
		t.Fatalf("updateEntry with missing dir: %v", err)
	}

	// cloneDir should now exist (was re-cloned).
	if _, statErr := os.Stat(cloneDir); os.IsNotExist(statErr) {
		t.Error("expected cloneDir to exist after re-clone, but it does not")
	}
}

func TestUpdateEntry_MissingDir_NoSourceURL_Errors(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Install dir that doesn't exist, and no SourceURL.
	cloneDir := filepath.Join(t.TempDir(), "nonexistent")

	now := time.Now().UTC()
	entry := lockfile.Entry{
		SourceURL:   "", // empty — malformed
		Commit:      "abc1234",
		InstallDir:  cloneDir,
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        []string{},
	}
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/test/repo": entry,
	})

	store, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var buf bytes.Buffer
	err = updateEntry(&buf, os.Stderr, store, "github.com/test/repo", entry)
	if err == nil {
		t.Fatal("expected error when installDir missing and SourceURL empty")
	}
	if !strings.Contains(err.Error(), "malformed lockfile") {
		t.Errorf("error should mention 'malformed lockfile'; got: %v", err)
	}
}

// ── Local Kit Repo tests ──────────────────────────────────────────────────────

// TestUpdateEntry_LocalRepo_SHAAdvances: local repo's HEAD has moved forward.
// updateEntry should read the new SHA and save the lockfile. No git fetch or
// reset should be attempted (the repo has no origin remote, so those would
// fail — confirming they are never called).
func TestUpdateEntry_LocalRepo_SHAAdvances(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Create a plain local git repo (not a clone — no origin remote).
	srcRepo := makeKitRepo(t, "my-kits")

	// Get the initial SHA.
	shaCmd := exec.Command("git", "-C", srcRepo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	oldSHA := strings.TrimSpace(string(shaOut))

	// Create a symlink pointing at the local repo.
	symlinkPath := filepath.Join(home, ".boxd", "kits", "my-kits")
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(srcRepo, symlinkPath); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	entry := lockfile.Entry{
		SourceURL:   srcRepo,
		Commit:      oldSHA,
		InstallDir:  symlinkPath,
		InstallMode: "local",
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        []string{"my-kits"},
	}
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"my-kits": entry,
	})

	// Advance the repo's HEAD with a new commit.
	addCommit(t, srcRepo, "extra.txt", "extra")

	// Get the new SHA directly from the source repo.
	newSHACmd := exec.Command("git", "-C", srcRepo, "rev-parse", "HEAD")
	newSHAOut, err := newSHACmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD new: %v", err)
	}
	newSHA := strings.TrimSpace(string(newSHAOut))

	store, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := updateEntry(&stdout, &stderr, store, "my-kits", entry); err != nil {
		t.Fatalf("updateEntry: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "→") {
		t.Errorf("expected '→' in output; got: %q", got)
	}
	if !strings.Contains(got, oldSHA[:7]) {
		t.Errorf("expected old short SHA in output; got: %q", got)
	}
	if !strings.Contains(got, newSHA[:7]) {
		t.Errorf("expected new short SHA in output; got: %q", got)
	}

	// Lockfile should have the new SHA.
	store2, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	e2, ok := store2.Get("my-kits")
	if !ok {
		t.Fatal("entry missing after updateEntry")
	}
	if e2.Commit != newSHA {
		t.Errorf("lockfile commit: got %q, want %q", e2.Commit, newSHA)
	}
	if !e2.InstalledAt.Equal(entry.InstalledAt) {
		t.Errorf("InstalledAt should be preserved")
	}
	if e2.UpdatedAt.Equal(entry.UpdatedAt) {
		t.Errorf("UpdatedAt should have changed")
	}
	// No stderr output expected (no warnings).
	if stderr.Len() != 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
}

// TestUpdateEntry_LocalRepo_NoChange: local repo HEAD matches the lockfile SHA.
func TestUpdateEntry_LocalRepo_NoChange(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	srcRepo := makeKitRepo(t, "my-kits")

	shaCmd := exec.Command("git", "-C", srcRepo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(shaOut))

	symlinkPath := filepath.Join(home, ".boxd", "kits", "my-kits")
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(srcRepo, symlinkPath); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	entry := lockfile.Entry{
		SourceURL:   srcRepo,
		Commit:      sha,
		InstallDir:  symlinkPath,
		InstallMode: "local",
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        []string{"my-kits"},
	}
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"my-kits": entry,
	})

	store, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := updateEntry(&stdout, &stderr, store, "my-kits", entry); err != nil {
		t.Fatalf("updateEntry: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "already at") {
		t.Errorf("expected 'already at' in output; got: %q", got)
	}

	// Lockfile should be unchanged.
	store2, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	e2, ok := store2.Get("my-kits")
	if !ok {
		t.Fatal("entry missing after updateEntry")
	}
	if !e2.UpdatedAt.Equal(entry.UpdatedAt) {
		t.Errorf("UpdatedAt should not change for no-op; got %v, want %v", e2.UpdatedAt, entry.UpdatedAt)
	}
}

// TestUpdateEntry_LocalRepo_MissingTarget: symlink target has been deleted.
// updateEntry should warn to stderr and return nil (no error, no lockfile change).
func TestUpdateEntry_LocalRepo_MissingTarget(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Source path that does NOT exist.
	missingSource := filepath.Join(t.TempDir(), "gone-repo")

	// Create a symlink pointing at the missing source.
	symlinkPath := filepath.Join(home, ".boxd", "kits", "my-kits")
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(missingSource, symlinkPath); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	entry := lockfile.Entry{
		SourceURL:   missingSource,
		Commit:      "abc1234def5678",
		InstallDir:  symlinkPath,
		InstallMode: "local",
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        []string{"my-kits"},
	}
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"my-kits": entry,
	})

	store, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var stdout, stderrBuf bytes.Buffer
	err = updateEntry(&stdout, &stderrBuf, store, "my-kits", entry)
	if err != nil {
		t.Fatalf("expected nil error for missing target, got: %v", err)
	}

	stderrStr := stderrBuf.String()
	if !strings.Contains(strings.ToLower(stderrStr), "warning") {
		t.Errorf("expected 'warning' in stderr; got: %q", stderrStr)
	}
	if !strings.Contains(stderrStr, missingSource) {
		t.Errorf("expected missing source path in stderr; got: %q", stderrStr)
	}

	// Lockfile entry must be unchanged.
	store2, err := lockfile.Load(lockPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	e2, ok := store2.Get("my-kits")
	if !ok {
		t.Fatal("entry missing after warning-skip")
	}
	if e2.Commit != entry.Commit {
		t.Errorf("Commit should be unchanged; got %q, want %q", e2.Commit, entry.Commit)
	}
}

// TestRunUpdateOne_LocalRepoByBasename: runUpdateOne with a bare basename finds
// the local entry without ParseAddress.
func TestRunUpdateOne_LocalRepoByBasename(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	srcRepo := makeKitRepo(t, "my-kits")

	shaCmd := exec.Command("git", "-C", srcRepo, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(shaOut))

	symlinkPath := filepath.Join(home, ".boxd", "kits", "my-kits")
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(srcRepo, symlinkPath); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"my-kits": {
			SourceURL:   srcRepo,
			Commit:      sha,
			InstallDir:  symlinkPath,
			InstallMode: "local",
			InstalledAt: now,
			UpdatedAt:   now,
			Kits:        []string{"my-kits"},
		},
	})

	var buf bytes.Buffer
	if err := runUpdateOne(&buf, lockPath, "my-kits"); err != nil {
		t.Fatalf("runUpdateOne by bare basename: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "already at") {
		t.Errorf("expected 'already at'; got: %q", got)
	}
}

// ── runUpdateOne narrows to one entry ─────────────────────────────────────────

func TestRunUpdateOne_SingleEntry(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	src := makeKitRepo(t, "mykits")
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	cloneRepo(t, src, cloneDir)

	shaCmd := exec.Command("git", "-C", cloneDir, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	sha := strings.TrimSpace(string(shaOut))
	now := time.Now().UTC()

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/test/repo": {
			SourceURL:   "file://" + src,
			Commit:      sha,
			InstallDir:  cloneDir,
			InstalledAt: now,
			UpdatedAt:   now,
			Kits:        []string{"mykits"},
		},
	})

	var buf bytes.Buffer
	if err := runUpdateOne(&buf, lockPath, "test/repo"); err != nil {
		t.Fatalf("runUpdateOne: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "already at") {
		t.Errorf("expected 'already at'; got: %q", got)
	}
}
