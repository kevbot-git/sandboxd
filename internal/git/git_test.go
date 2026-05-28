package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kevbot-git/sandboxd/internal/debug"
)

// ── Pure argv tests ───────────────────────────────────────────────────────────

func TestBuildCloneArgs(t *testing.T) {
	args := buildCloneArgs("https://github.com/example/sbx-kits.git", "/home/user/.boxd/kits/sbx-kits")
	want := []string{"clone", "https://github.com/example/sbx-kits.git", "/home/user/.boxd/kits/sbx-kits"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d, want %d; args: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

func TestBuildRevParseArgs(t *testing.T) {
	args := buildRevParseArgs("/home/user/.boxd/kits/sbx-kits")
	want := []string{"-C", "/home/user/.boxd/kits/sbx-kits", "rev-parse", "HEAD"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d, want %d; args: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

func TestBuildFetchArgs(t *testing.T) {
	args := buildFetchArgs("/home/user/.boxd/kits/sbx-kits")
	want := []string{"-C", "/home/user/.boxd/kits/sbx-kits", "fetch", "origin"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d, want %d; args: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

func TestBuildResetHardArgs(t *testing.T) {
	args := buildResetHardArgs("/home/user/.boxd/kits/sbx-kits", "origin/HEAD")
	want := []string{"-C", "/home/user/.boxd/kits/sbx-kits", "reset", "--hard", "origin/HEAD"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d, want %d; args: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

// TestDebugTrace verifies that Clone and RevParseHEAD route through
// debug.Trace("git", ...) when DEBUG is set.
func TestDebugTrace(t *testing.T) {
	// Create a minimal local git repo to clone from.
	src := makeLocalRepo(t)

	var buf strings.Builder
	orig := debug.Out
	debug.Out = &buf
	defer func() { debug.Out = orig }()

	t.Setenv("DEBUG", "1")

	destDir := filepath.Join(t.TempDir(), "clone")
	if err := Clone("file://"+src, destDir); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if _, err := RevParseHEAD(destDir); err != nil {
		t.Fatalf("RevParseHEAD: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "> git clone") {
		t.Errorf("expected '> git clone' in trace output; got:\n%s", out)
	}
	if !strings.Contains(out, "> git -C") {
		t.Errorf("expected '> git -C' in trace output; got:\n%s", out)
	}
}

// ── file:// integration tests ─────────────────────────────────────────────────

// TestCloneAndRevParseHEAD exercises Clone + RevParseHEAD against a real
// local repo seeded in t.TempDir(). No network required.
func TestCloneAndRevParseHEAD(t *testing.T) {
	src := makeLocalRepo(t)
	destDir := filepath.Join(t.TempDir(), "cloned")

	if err := Clone("file://"+src, destDir); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Confirm the destination was created.
	if _, err := os.Stat(destDir); err != nil {
		t.Fatalf("clone dest not found: %v", err)
	}

	sha, err := RevParseHEAD(destDir)
	if err != nil {
		t.Fatalf("RevParseHEAD: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("SHA should be 40 hex chars; got %q (len %d)", sha, len(sha))
	}

	// SHA from the source must match the clone.
	srcSHA := gitOutput(t, "-C", src, "rev-parse", "HEAD")
	if sha != srcSHA {
		t.Errorf("clone SHA %q != source SHA %q", sha, srcSHA)
	}
}

func TestFetch(t *testing.T) {
	src := makeLocalRepo(t)
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	run(t, "git", "clone", "file://"+src, cloneDir)

	// Make a new commit on the source after the clone.
	newFile := filepath.Join(src, "extra.md")
	if err := os.WriteFile(newFile, []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, "git", "-C", src, "add", ".")
	run(t, "git", "-C", src, "commit", "-m", "second commit")

	if err := Fetch(cloneDir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// FETCH_HEAD should exist after a successful fetch.
	fetchHead := filepath.Join(cloneDir, ".git", "FETCH_HEAD")
	if _, err := os.Stat(fetchHead); err != nil {
		t.Errorf("FETCH_HEAD not found after Fetch: %v", err)
	}

	// The new commit should be visible on origin/<branch> in the clone.
	srcSHA := gitOutput(t, "-C", src, "rev-parse", "HEAD")
	branch := gitOutput(t, "-C", src, "branch", "--show-current")
	originSHA := gitOutput(t, "-C", cloneDir, "rev-parse", "origin/"+branch)
	if originSHA != srcSHA {
		t.Errorf("origin/%s after fetch: got %q, want %q", branch, originSHA, srcSHA)
	}
}

func TestResetHard(t *testing.T) {
	src := makeLocalRepo(t)
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	run(t, "git", "clone", "file://"+src, cloneDir)

	// Make a new commit on source, then fetch it into the clone.
	newFile := filepath.Join(src, "extra.md")
	if err := os.WriteFile(newFile, []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, "git", "-C", src, "add", ".")
	run(t, "git", "-C", src, "commit", "-m", "second commit")

	if err := Fetch(cloneDir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	srcSHA := gitOutput(t, "-C", src, "rev-parse", "HEAD")

	branch := gitOutput(t, "-C", src, "branch", "--show-current")
	if err := ResetHard(cloneDir, "origin/"+branch); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}

	cloneSHA, err := RevParseHEAD(cloneDir)
	if err != nil {
		t.Fatalf("RevParseHEAD after ResetHard: %v", err)
	}
	if cloneSHA != srcSHA {
		t.Errorf("HEAD after reset --hard: got %q, want %q", cloneSHA, srcSHA)
	}
}

func TestFetchAndResetHard_DebugTrace(t *testing.T) {
	src := makeLocalRepo(t)
	cloneDir := filepath.Join(t.TempDir(), "cloned")
	run(t, "git", "clone", "file://"+src, cloneDir)

	var buf strings.Builder
	orig := debug.Out
	debug.Out = &buf
	defer func() { debug.Out = orig }()

	t.Setenv("DEBUG", "1")

	if err := Fetch(cloneDir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if err := ResetHard(cloneDir, "HEAD"); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "> git -C") {
		t.Errorf("expected '> git -C' in trace output; got:\n%s", out)
	}
	if !strings.Contains(out, "fetch") {
		t.Errorf("expected 'fetch' in trace output; got:\n%s", out)
	}
	if !strings.Contains(out, "reset") {
		t.Errorf("expected 'reset' in trace output; got:\n%s", out)
	}
}

func TestRevParseHEAD_NonRepoErrors(t *testing.T) {
	tmp := t.TempDir()
	_, err := RevParseHEAD(tmp)
	if err == nil {
		t.Fatal("expected error for non-repo directory")
	}
}

// ── Sparse-checkout arg tests ─────────────────────────────────────────────────

func TestBuildCloneNoCheckoutArgs(t *testing.T) {
	args := buildCloneNoCheckoutArgs("https://github.com/example/sbx-kits.git", "/tmp/sbx-kits")
	want := []string{"clone", "--no-checkout", "https://github.com/example/sbx-kits.git", "/tmp/sbx-kits"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d, want %d; args: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

func TestBuildSparseCheckoutInitArgs(t *testing.T) {
	args := buildSparseCheckoutInitArgs("/tmp/repo")
	want := []string{"-C", "/tmp/repo", "sparse-checkout", "init", "--cone"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d, want %d; args: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

func TestBuildSparseCheckoutSetArgs(t *testing.T) {
	args := buildSparseCheckoutSetArgs("/tmp/repo", []string{"bun", "node"})
	want := []string{"-C", "/tmp/repo", "sparse-checkout", "set", "bun", "node"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d, want %d; args: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

func TestBuildSparseCheckoutDisableArgs(t *testing.T) {
	args := buildSparseCheckoutDisableArgs("/tmp/repo")
	want := []string{"-C", "/tmp/repo", "sparse-checkout", "disable"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d, want %d; args: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

func TestBuildCheckoutHEADArgs(t *testing.T) {
	args := buildCheckoutHEADArgs("/tmp/repo")
	want := []string{"-C", "/tmp/repo", "checkout"}
	if len(args) != len(want) {
		t.Fatalf("len: got %d, want %d; args: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], w)
		}
	}
}

// TestSparseCheckout is an integration test that exercises the full sparse
// checkout flow: CloneNoCheckout → SparseCheckoutInit → SparseCheckoutSet →
// CheckoutHEAD. It verifies that only the requested directory is materialised.
func TestSparseCheckout(t *testing.T) {
	src := makeLocalRepoWithDirs(t)
	destDir := filepath.Join(t.TempDir(), "sparse-clone")

	if err := CloneNoCheckout("file://"+src, destDir); err != nil {
		t.Fatalf("CloneNoCheckout: %v", err)
	}

	if err := SparseCheckoutInit(destDir); err != nil {
		t.Fatalf("SparseCheckoutInit: %v", err)
	}

	if err := SparseCheckoutSet(destDir, []string{"dir1"}); err != nil {
		t.Fatalf("SparseCheckoutSet: %v", err)
	}

	if err := CheckoutHEAD(destDir); err != nil {
		t.Fatalf("CheckoutHEAD: %v", err)
	}

	// dir1 must be present.
	if _, err := os.Stat(filepath.Join(destDir, "dir1")); err != nil {
		t.Errorf("expected dir1 to be present after sparse checkout; got: %v", err)
	}

	// dir2 must NOT be present.
	if _, err := os.Stat(filepath.Join(destDir, "dir2")); err == nil {
		t.Error("expected dir2 to be absent from sparse checkout, but it exists")
	}
}

// TestSparseCheckoutDisable verifies that SparseCheckoutDisable converts a
// sparse checkout to a full checkout, making all dirs available.
func TestSparseCheckoutDisable(t *testing.T) {
	src := makeLocalRepoWithDirs(t)
	destDir := filepath.Join(t.TempDir(), "sparse-clone")

	run(t, "git", "clone", "--no-checkout", "file://"+src, destDir)
	if err := SparseCheckoutInit(destDir); err != nil {
		t.Fatalf("SparseCheckoutInit: %v", err)
	}
	if err := SparseCheckoutSet(destDir, []string{"dir1"}); err != nil {
		t.Fatalf("SparseCheckoutSet: %v", err)
	}
	if err := CheckoutHEAD(destDir); err != nil {
		t.Fatalf("CheckoutHEAD: %v", err)
	}

	// dir2 absent at this point.
	if _, err := os.Stat(filepath.Join(destDir, "dir2")); err == nil {
		t.Fatal("dir2 should not exist before SparseCheckoutDisable")
	}

	if err := SparseCheckoutDisable(destDir); err != nil {
		t.Fatalf("SparseCheckoutDisable: %v", err)
	}

	// After disable, both dirs should exist.
	if _, err := os.Stat(filepath.Join(destDir, "dir1")); err != nil {
		t.Errorf("dir1 missing after SparseCheckoutDisable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "dir2")); err != nil {
		t.Errorf("dir2 missing after SparseCheckoutDisable: %v", err)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// makeLocalRepo creates a temporary git repository with a single commit
// and returns its absolute path. The repo can be cloned via file://<path>.
func makeLocalRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run(t, "git", "-C", dir, "init")
	run(t, "git", "-C", dir, "config", "user.email", "test@example.com")
	run(t, "git", "-C", dir, "config", "user.name", "Test")

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# test kit repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, "git", "-C", dir, "add", ".")
	run(t, "git", "-C", dir, "commit", "-m", "initial commit")

	return dir
}

// run executes a command and fails the test on error.
func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("command %q %v failed: %v\n%s", name, args, err, out)
	}
}

// gitOutput runs git with args and returns trimmed stdout.
func gitOutput(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// makeLocalRepoWithDirs creates a temporary git repo that contains two
// subdirectories (dir1/ and dir2/), each with a spec.yaml. Used by sparse
// checkout integration tests.
func makeLocalRepoWithDirs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run(t, "git", "-C", dir, "init")
	run(t, "git", "-C", dir, "config", "user.email", "test@example.com")
	run(t, "git", "-C", dir, "config", "user.name", "Test")

	for _, sub := range []string{"dir1", "dir2"} {
		subDir := filepath.Join(dir, sub)
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatal(err)
		}
		spec := filepath.Join(subDir, "spec.yaml")
		if err := os.WriteFile(spec, []byte("kind: mixin\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(t, "git", "-C", dir, "add", ".")
	run(t, "git", "-C", dir, "commit", "-m", "initial commit")

	return dir
}

// ── WorktreePath tests ─────────────────────────────────────────────────────

func TestBuildWorktreeListArgs(t *testing.T) {
	args := buildWorktreeListArgs("/tmp/repo")
	want := []string{"-C", "/tmp/repo", "worktree", "list", "--porcelain"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("buildWorktreeListArgs() = %v; want %v", args, want)
	}
}

func TestParseWorktreePath_Match(t *testing.T) {
	output := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n\nworktree /repo/.sbx/feature\nHEAD def456\nbranch refs/heads/feature\n\n"
	path, ok := parseWorktreePath(output, "feature")
	if !ok {
		t.Fatal("expected match for branch 'feature'")
	}
	if path != "/repo/.sbx/feature" {
		t.Errorf("got path %q; want %q", path, "/repo/.sbx/feature")
	}
}

func TestParseWorktreePath_NoMatch(t *testing.T) {
	output := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n\n"
	_, ok := parseWorktreePath(output, "nonexistent")
	if ok {
		t.Error("expected no match for branch 'nonexistent'")
	}
}

func TestParseWorktreePath_DetachedHEAD(t *testing.T) {
	// A detached HEAD entry has no branch line — must not match any branch.
	output := "worktree /repo/main\nHEAD abc123\nbranch refs/heads/main\n\nworktree /repo/detached\nHEAD def456\ndetached\n\n"
	_, ok := parseWorktreePath(output, "detached")
	if ok {
		t.Error("detached worktree must not match a branch lookup")
	}
}

func TestWorktreePath_Match(t *testing.T) {
	repo := makeLocalRepo(t)
	defaultBranch := gitOutput(t, "-C", repo, "branch", "--show-current")
	// Create a worktree for branch "feature" under a temp dir.
	wtDir := filepath.Join(t.TempDir(), "feature-wt")
	run(t, "git", "-C", repo, "checkout", "-b", "feature")
	run(t, "git", "-C", repo, "checkout", defaultBranch) // keep default branch checked out in main worktree
	run(t, "git", "-C", repo, "worktree", "add", wtDir, "feature")

	got, err := WorktreePath(repo, "feature")
	if err != nil {
		t.Fatalf("WorktreePath: %v", err)
	}
	// Resolve symlinks so comparisons survive macOS /private prefix.
	gotR, _ := filepath.EvalSymlinks(got)
	wtR, _ := filepath.EvalSymlinks(wtDir)
	if gotR != wtR {
		t.Errorf("got %q; want %q", got, wtDir)
	}
}

func TestWorktreePath_NoWorktreeForBranch(t *testing.T) {
	repo := makeLocalRepo(t)
	defaultBranch := gitOutput(t, "-C", repo, "branch", "--show-current")
	// Branch exists but has no worktree.
	run(t, "git", "-C", repo, "checkout", "-b", "orphan")
	run(t, "git", "-C", repo, "checkout", defaultBranch)

	_, err := WorktreePath(repo, "orphan")
	if err == nil {
		t.Fatal("expected error for branch with no worktree")
	}
	if !strings.Contains(err.Error(), "not found in any worktree") {
		t.Errorf("error %q should contain 'not found in any worktree'", err.Error())
	}
}

func TestWorktreePath_NotARepo(t *testing.T) {
	tmp := t.TempDir()
	_, err := WorktreePath(tmp, "any")
	if err == nil {
		t.Fatal("expected error for non-repo directory")
	}
	if !strings.Contains(err.Error(), "requires a git repository") {
		t.Errorf("error %q should contain 'requires a git repository'", err.Error())
	}
}
