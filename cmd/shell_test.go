package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// makeGitRepo creates a temp git repo with one commit and returns its path.
func makeGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("-C", dir, "init")
	run("-C", dir, "config", "user.email", "test@example.com")
	run("-C", dir, "config", "user.name", "Test")
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("-C", dir, "add", ".")
	run("-C", dir, "commit", "-m", "init")
	return dir
}

// makeWorktree adds a git worktree for branch to the repo and returns the path.
func makeWorktree(t *testing.T, repo, branch, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "worktree", "add", dir, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	return dir
}

func TestResolveShellWorkdir_ExplicitWorkdirWins(t *testing.T) {
	repo := makeGitRepo(t)
	wtDir := filepath.Join(t.TempDir(), "feat-wt")

	// Set up a feature worktree.
	// Create feature branch without checking it out, then add worktree.
	exec.Command("git", "-C", repo, "branch", "feature").Run()
	makeWorktree(t, repo, "feature", wtDir)

	explicit := "/explicit/path"
	got, err := resolveShellWorkdir(repo, explicit, "feature")
	if err != nil {
		t.Fatalf("resolveShellWorkdir: %v", err)
	}
	if got != explicit {
		t.Errorf("got %q; want %q (explicit --workdir must win)", got, explicit)
	}
}

func TestResolveShellWorkdir_BranchUsesWorktree(t *testing.T) {
	repo := makeGitRepo(t)
	wtDir := filepath.Join(t.TempDir(), "feat-wt")

	// Create feature branch without checking it out, then add worktree.
	exec.Command("git", "-C", repo, "branch", "feature").Run()
	makeWorktree(t, repo, "feature", wtDir)

	got, err := resolveShellWorkdir(repo, "", "feature")
	if err != nil {
		t.Fatalf("resolveShellWorkdir: %v", err)
	}
	gotR, _ := filepath.EvalSymlinks(got)
	wtR, _ := filepath.EvalSymlinks(wtDir)
	if gotR != wtR {
		t.Errorf("got %q; want %q (branch should resolve to worktree)", got, wtDir)
	}
}

func TestResolveShellWorkdir_NoBranchUsesCwd(t *testing.T) {
	cwd := "/some/cwd"
	got, err := resolveShellWorkdir(cwd, "", "")
	if err != nil {
		t.Fatalf("resolveShellWorkdir: %v", err)
	}
	if got != cwd {
		t.Errorf("got %q; want %q (no branch should return cwd)", got, cwd)
	}
}

func TestResolveShellWorkdir_UnknownBranchErrors(t *testing.T) {
	repo := makeGitRepo(t)
	_, err := resolveShellWorkdir(repo, "", "nosuchbranch")
	if err == nil {
		t.Fatal("expected error for unknown branch")
	}
	if !strings.Contains(err.Error(), "not found in any worktree") {
		t.Errorf("error %q should mention 'not found in any worktree'", err.Error())
	}
}

func TestResolveShellWorkdir_NotARepoErrors(t *testing.T) {
	tmp := t.TempDir()
	_, err := resolveShellWorkdir(tmp, "", "anybranch")
	if err == nil {
		t.Fatal("expected error for non-repo directory")
	}
	if !strings.Contains(err.Error(), "requires a git repository") {
		t.Errorf("error %q should mention 'requires a git repository'", err.Error())
	}
}
