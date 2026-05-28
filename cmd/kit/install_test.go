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
)

// execGit runs a git command in the test, fataling on failure.
func execGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// makeSourceRepo creates a local bare-ish git repo with one commit and
// returns its path. Tests can clone from "file://<path>".
func makeSourceRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	execGit(t, "-C", dir, "init")
	execGit(t, "-C", dir, "config", "user.email", "test@example.com")
	execGit(t, "-C", dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execGit(t, "-C", dir, "add", ".")
	execGit(t, "-C", dir, "commit", "-m", "initial")
	return dir
}

// headSHA returns the full commit SHA of HEAD in repoDir.
func headSHA(t *testing.T, repoDir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD in %s: %v", repoDir, err)
	}
	return strings.TrimSpace(string(out))
}

// TestRunInstall_EmptyLockfile verifies that an empty lockfile prints the
// "no kits in lockfile" message and exits 0.
func TestRunInstall_EmptyLockfile(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")
	makeStore(t, lockPath, nil)

	var stdout, stderr bytes.Buffer
	if err := runInstall(&stdout, &stderr, lockPath); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "no kits in lockfile") {
		t.Errorf("expected 'no kits in lockfile'; got: %q", got)
	}
}

// TestRunInstall_MalformedEntry verifies that an entry with an empty sourceUrl
// returns an actionable error.
func TestRunInstall_MalformedEntry(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:  "", // malformed: empty
			Commit:     "abc1234",
			InstallDir: filepath.Join(home, ".boxd", "kits", "sbx-kits"),
		},
	})

	var stdout, stderr bytes.Buffer
	err := runInstall(&stdout, &stderr, lockPath)
	if err == nil {
		t.Fatal("expected error for malformed entry")
	}
	if !strings.Contains(err.Error(), "github.com/example/sbx-kits") {
		t.Errorf("error should name the key; got: %v", err)
	}
	if !strings.Contains(err.Error(), "missing sourceUrl") {
		t.Errorf("error should mention 'missing sourceUrl'; got: %v", err)
	}
	if !strings.Contains(err.Error(), "boxd kit remove") {
		t.Errorf("error should mention 'boxd kit remove'; got: %v", err)
	}
}

// TestRunInstall_MissingDir verifies that when installDir doesn't exist, the
// repo is cloned and reset to the locked commit.
func TestRunInstall_MissingDir(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	srcRepo := makeSourceRepo(t)
	sha := headSHA(t, srcRepo)
	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:  "file://" + srcRepo,
			Commit:     sha,
			InstallDir: installDir,
		},
	})

	var stdout, stderr bytes.Buffer
	if err := runInstall(&stdout, &stderr, lockPath); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	// installDir must now exist.
	if _, err := os.Stat(installDir); err != nil {
		t.Fatalf("installDir not created: %v", err)
	}

	// HEAD must be the locked commit.
	got := headSHA(t, installDir)
	if got != sha {
		t.Errorf("expected HEAD %s, got %s", sha, got)
	}

	// Output must contain "cloned" and the short SHA.
	out := stdout.String()
	if !strings.Contains(out, "cloned") {
		t.Errorf("expected 'cloned' in output; got: %q", out)
	}
	if !strings.Contains(out, sha[:7]) {
		t.Errorf("expected short SHA %s in output; got: %q", sha[:7], out)
	}
}

// TestRunInstall_MatchingSHA verifies that when the installDir exists and HEAD
// already matches the locked commit, no git operations are performed and the
// output says "ok at <sha>".
func TestRunInstall_MatchingSHA(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	srcRepo := makeSourceRepo(t)
	sha := headSHA(t, srcRepo)

	// Clone the repo manually to simulate an already-installed state.
	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	execGit(t, "clone", "file://"+srcRepo, installDir)

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:  "file://" + srcRepo,
			Commit:     sha,
			InstallDir: installDir,
		},
	})

	var stdout, stderr bytes.Buffer
	if err := runInstall(&stdout, &stderr, lockPath); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "ok at") {
		t.Errorf("expected 'ok at' in output; got: %q", out)
	}
	if !strings.Contains(out, sha[:7]) {
		t.Errorf("expected short SHA %s in output; got: %q", sha[:7], out)
	}
}

// TestRunInstall_MismatchedSHA verifies that when the installDir exists but
// HEAD doesn't match the locked commit, a reset is performed.
func TestRunInstall_MismatchedSHA(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	srcRepo := makeSourceRepo(t)
	lockedSHA := headSHA(t, srcRepo)

	// Add a second commit to the source repo so we can advance past the first.
	if err := os.WriteFile(filepath.Join(srcRepo, "extra.md"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execGit(t, "-C", srcRepo, "add", ".")
	execGit(t, "-C", srcRepo, "commit", "-m", "second commit")

	// Clone at the latest state (second commit).
	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	execGit(t, "clone", "file://"+srcRepo, installDir)

	// Lockfile points to the first commit (lockedSHA).
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:  "file://" + srcRepo,
			Commit:     lockedSHA,
			InstallDir: installDir,
		},
	})

	var stdout, stderr bytes.Buffer
	if err := runInstall(&stdout, &stderr, lockPath); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	// HEAD must now be the locked (first) commit.
	got := headSHA(t, installDir)
	if got != lockedSHA {
		t.Errorf("expected HEAD reset to %s, got %s", lockedSHA, got)
	}

	out := stdout.String()
	if !strings.Contains(out, "reset") {
		t.Errorf("expected 'reset' in output; got: %q", out)
	}
	if !strings.Contains(out, lockedSHA[:7]) {
		t.Errorf("expected short SHA %s in output; got: %q", lockedSHA[:7], out)
	}
}

// TestRunInstall_DirtyWorkingTree verifies that when the installDir has
// uncommitted changes, a warning is emitted to stderr before the reset.
func TestRunInstall_DirtyWorkingTree(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	srcRepo := makeSourceRepo(t)
	lockedSHA := headSHA(t, srcRepo)

	// Add a second commit so we have somewhere to reset from.
	if err := os.WriteFile(filepath.Join(srcRepo, "extra.md"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execGit(t, "-C", srcRepo, "add", ".")
	execGit(t, "-C", srcRepo, "commit", "-m", "second commit")

	// Clone at the latest state.
	installDir := filepath.Join(home, ".boxd", "kits", "sbx-kits")
	execGit(t, "clone", "file://"+srcRepo, installDir)

	// Introduce an uncommitted change in the working tree.
	if err := os.WriteFile(filepath.Join(installDir, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execGit(t, "-C", installDir, "add", "dirty.txt")

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/example/sbx-kits": {
			SourceURL:  "file://" + srcRepo,
			Commit:     lockedSHA,
			InstallDir: installDir,
		},
	})

	var stdout, stderr bytes.Buffer
	if err := runInstall(&stdout, &stderr, lockPath); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "warning") {
		t.Errorf("expected warning on stderr; got: %q", stderrStr)
	}
	if !strings.Contains(stderrStr, "discarding local changes") {
		t.Errorf("expected 'discarding local changes' in stderr; got: %q", stderrStr)
	}
	if !strings.Contains(stderrStr, installDir) {
		t.Errorf("expected installDir in stderr warning; got: %q", stderrStr)
	}
}

// TestRunInstall_LocalRepo_MissingSymlink_Recreated verifies that when the
// lockfile has a local entry and the symlink is missing but the source dir
// exists, runInstall recreates the symlink.
func TestRunInstall_LocalRepo_MissingSymlink_Recreated(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	// Source repo: just a directory (doesn't need git history for this test).
	srcDir := t.TempDir()
	symlinkPath := filepath.Join(home, ".boxd", "kits", "my-local-kit")

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"local/my-local-kit": {
			SourceURL:   srcDir,
			Commit:      "abc1234abc1234abc1234abc1234abc1234abc1234",
			InstallDir:  symlinkPath,
			InstallMode: "local",
		},
	})

	var stdout, stderr bytes.Buffer
	if err := runInstall(&stdout, &stderr, lockPath); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	// Symlink must now exist at InstallDir pointing to SourceURL.
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", symlinkPath, err)
	}
	if target != srcDir {
		t.Errorf("symlink target: got %q, want %q", target, srcDir)
	}

	// Output must mention the key.
	out := stdout.String()
	if !strings.Contains(out, "local/my-local-kit") {
		t.Errorf("expected key in output; got: %q", out)
	}
}

// TestRunInstall_LocalRepo_MissingSourcePath_WarnSkip verifies that when both
// the symlink and the source directory are absent, runInstall warns on stderr
// and returns nil (exit 0).
func TestRunInstall_LocalRepo_MissingSourcePath_WarnSkip(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	nonexistentSrc := filepath.Join(home, "does-not-exist", "src")
	nonexistentSymlink := filepath.Join(home, ".boxd", "kits", "ghost-kit")

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"local/ghost-kit": {
			SourceURL:   nonexistentSrc,
			Commit:      "abc1234abc1234abc1234abc1234abc1234abc1234",
			InstallDir:  nonexistentSymlink,
			InstallMode: "local",
		},
	})

	var stdout, stderr bytes.Buffer
	if err := runInstall(&stdout, &stderr, lockPath); err != nil {
		t.Fatalf("runInstall: expected nil, got %v", err)
	}

	stderrStr := stderr.String()
	if !strings.Contains(strings.ToLower(stderrStr), "warning") {
		t.Errorf("expected 'warning' in stderr; got: %q", stderrStr)
	}
	if !strings.Contains(stderrStr, nonexistentSrc) {
		t.Errorf("expected missing source path in stderr; got: %q", stderrStr)
	}
}

// TestRunInstall_LocalRepo_CorrectSymlink_NoOp verifies that when the symlink
// already points to the correct source, runInstall is a no-op.
func TestRunInstall_LocalRepo_CorrectSymlink_NoOp(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	srcDir := t.TempDir()
	symlinkPath := filepath.Join(home, ".boxd", "kits", "stable-kit")

	// Pre-create the correct symlink.
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(srcDir, symlinkPath); err != nil {
		t.Fatal(err)
	}

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"local/stable-kit": {
			SourceURL:   srcDir,
			Commit:      "abc1234abc1234abc1234abc1234abc1234abc1234",
			InstallDir:  symlinkPath,
			InstallMode: "local",
		},
	})

	var stdout, stderr bytes.Buffer
	if err := runInstall(&stdout, &stderr, lockPath); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	// Symlink must still point to srcDir.
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("symlink missing after no-op: %v", err)
	}
	if target != srcDir {
		t.Errorf("symlink target changed: got %q, want %q", target, srcDir)
	}

	// Output must indicate no-op (contains "ok").
	out := stdout.String()
	if !strings.Contains(out, "ok") {
		t.Errorf("expected 'ok' in output; got: %q", out)
	}
}

// TestRunInstall_LocalRepo_WrongTarget_Error verifies that when the symlink at
// InstallDir points to a different path than SourceURL, runInstall returns an
// error rather than silently overwriting.
func TestRunInstall_LocalRepo_WrongTarget_Error(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	srcDirA := t.TempDir() // the intended source (in lockfile)
	srcDirB := t.TempDir() // the actual current target (wrong)
	symlinkPath := filepath.Join(home, ".boxd", "kits", "conflicted-kit")

	// Create symlink pointing to B (the wrong place).
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(srcDirB, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// Lockfile records A as the expected source.
	makeStore(t, lockPath, map[string]lockfile.Entry{
		"local/conflicted-kit": {
			SourceURL:   srcDirA,
			Commit:      "abc1234abc1234abc1234abc1234abc1234abc1234",
			InstallDir:  symlinkPath,
			InstallMode: "local",
		},
	})

	var stdout, stderr bytes.Buffer
	err := runInstall(&stdout, &stderr, lockPath)
	if err == nil {
		t.Fatal("runInstall: expected error for wrong symlink target, got nil")
	}
	// Error message should clearly describe the conflict.
	if !strings.Contains(err.Error(), "local/conflicted-kit") {
		t.Errorf("error should name the key; got: %v", err)
	}
}

// TestRunInstall_MultipleEntriesSortedOrder verifies that multiple entries are
// processed in sorted key order and all are installed.
func TestRunInstall_MultipleEntriesSortedOrder(t *testing.T) {
	home := redirectHome(t)
	lockPath := filepath.Join(home, ".boxd", "boxd.lock.json")

	srcA := makeSourceRepo(t)
	shaA := headSHA(t, srcA)
	srcB := makeSourceRepo(t)
	shaB := headSHA(t, srcB)

	installDirA := filepath.Join(home, ".boxd", "kits", "aaa-repo")
	installDirB := filepath.Join(home, ".boxd", "kits", "bbb-repo")

	makeStore(t, lockPath, map[string]lockfile.Entry{
		"github.com/zzz/bbb-repo": {
			SourceURL:  "file://" + srcB,
			Commit:     shaB,
			InstallDir: installDirB,
			UpdatedAt:  time.Now(),
		},
		"github.com/aaa/aaa-repo": {
			SourceURL:  "file://" + srcA,
			Commit:     shaA,
			InstallDir: installDirA,
			UpdatedAt:  time.Now(),
		},
	})

	var stdout, stderr bytes.Buffer
	if err := runInstall(&stdout, &stderr, lockPath); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	// Both dirs must exist.
	if _, err := os.Stat(installDirA); err != nil {
		t.Errorf("installDirA not created: %v", err)
	}
	if _, err := os.Stat(installDirB); err != nil {
		t.Errorf("installDirB not created: %v", err)
	}

	// Output lines should be in alphabetical order.
	out := stdout.String()
	posA := strings.Index(out, "aaa/aaa-repo")
	posB := strings.Index(out, "zzz/bbb-repo")
	if posA == -1 || posB == -1 {
		t.Fatalf("expected both repo keys in output; got: %q", out)
	}
	if posA > posB {
		t.Errorf("expected aaa/aaa-repo before zzz/bbb-repo in output; got: %q", out)
	}
}
