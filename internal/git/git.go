// Package git wraps the git subprocess. Every invocation routes through
// debug.Trace("git", args) so `DEBUG=1 boxd kit add ...` logs each git
// call to stderr in the same ">" format used by the sbx runner.
package git

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/kevbot-git/sandboxd/internal/debug"
)

// buildCloneArgs returns the argv slice for `git clone sourceURL destDir`.
// Exported for tests that want to assert argv shape without running git.
func buildCloneArgs(sourceURL, destDir string) []string {
	return []string{"clone", sourceURL, destDir}
}

// buildRevParseArgs returns the argv slice for
// `git -C repoDir rev-parse HEAD`.
func buildRevParseArgs(repoDir string) []string {
	return []string{"-C", repoDir, "rev-parse", "HEAD"}
}

// buildFetchArgs returns the argv slice for `git -C repoDir fetch origin`.
func buildFetchArgs(repoDir string) []string {
	return []string{"-C", repoDir, "fetch", "origin"}
}

// buildResetHardArgs returns the argv slice for
// `git -C repoDir reset --hard <ref>`.
func buildResetHardArgs(repoDir, ref string) []string {
	return []string{"-C", repoDir, "reset", "--hard", ref}
}

// buildCloneNoCheckoutArgs returns the argv slice for
// `git clone --no-checkout sourceURL destDir`.
func buildCloneNoCheckoutArgs(sourceURL, destDir string) []string {
	return []string{"clone", "--no-checkout", sourceURL, destDir}
}

// buildSparseCheckoutInitArgs returns the argv slice for
// `git -C repoDir sparse-checkout init --cone`.
func buildSparseCheckoutInitArgs(repoDir string) []string {
	return []string{"-C", repoDir, "sparse-checkout", "init", "--cone"}
}

// buildSparseCheckoutSetArgs returns the argv slice for
// `git -C repoDir sparse-checkout set <paths...>`.
func buildSparseCheckoutSetArgs(repoDir string, paths []string) []string {
	args := []string{"-C", repoDir, "sparse-checkout", "set"}
	return append(args, paths...)
}

// buildSparseCheckoutDisableArgs returns the argv slice for
// `git -C repoDir sparse-checkout disable`.
func buildSparseCheckoutDisableArgs(repoDir string) []string {
	return []string{"-C", repoDir, "sparse-checkout", "disable"}
}

// buildCheckoutHEADArgs returns the argv slice for `git -C repoDir checkout`.
func buildCheckoutHEADArgs(repoDir string) []string {
	return []string{"-C", repoDir, "checkout"}
}

// buildWorktreeListArgs returns the argv for `git -C dir worktree list --porcelain`.
func buildWorktreeListArgs(dir string) []string {
	return []string{"-C", dir, "worktree", "list", "--porcelain"}
}

// parseWorktreePath scans the output of `git worktree list --porcelain` and
// returns the worktree path whose branch line matches refs/heads/<branch>.
// Returns ("", false) when no entry matches.
func parseWorktreePath(output, branch string) (string, bool) {
	target := "branch refs/heads/" + branch
	var currentPath string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		} else if line == target {
			return currentPath, true
		}
	}
	return "", false
}

// WorktreePath returns the absolute path of the git worktree that has
// branch checked out, searching from dir. dir must be inside a git
// repository; if it is not, the error contains the message
// "boxd shell --branch requires a git repository".
// If no worktree for branch is found the error contains
// "not found in any worktree under <dir>".
func WorktreePath(dir, branch string) (string, error) {
	// Verify we're inside a git repo.
	checkArgs := []string{"-C", dir, "rev-parse", "--show-toplevel"}
	debug.Trace("git", checkArgs)
	checkCmd := exec.Command("git", checkArgs...)
	if _, err := checkCmd.Output(); err != nil {
		return "", fmt.Errorf("boxd shell --branch requires a git repository")
	}

	args := buildWorktreeListArgs(dir)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git worktree list in %s: %w", dir, err)
	}

	path, ok := parseWorktreePath(string(out), branch)
	if !ok {
		return "", fmt.Errorf("branch %q: not found in any worktree under %s", branch, dir)
	}
	return path, nil
}

// CloneNoCheckout clones sourceURL into destDir without checking out the
// working tree, using `git clone --no-checkout`. This is the first step of
// a sparse checkout workflow.
func CloneNoCheckout(sourceURL, destDir string) error {
	args := buildCloneNoCheckoutArgs(sourceURL, destDir)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone --no-checkout %s: %w\n%s", sourceURL, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SparseCheckoutInit runs `git -C repoDir sparse-checkout init --cone`.
func SparseCheckoutInit(repoDir string) error {
	args := buildSparseCheckoutInitArgs(repoDir)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git sparse-checkout init in %s: %w\n%s", repoDir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SparseCheckoutSet runs `git -C repoDir sparse-checkout set <paths...>`.
func SparseCheckoutSet(repoDir string, paths []string) error {
	args := buildSparseCheckoutSetArgs(repoDir, paths)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git sparse-checkout set in %s: %w\n%s", repoDir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SparseCheckoutDisable runs `git -C repoDir sparse-checkout disable`,
// converting a sparse checkout to a full checkout.
func SparseCheckoutDisable(repoDir string) error {
	args := buildSparseCheckoutDisableArgs(repoDir)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git sparse-checkout disable in %s: %w\n%s", repoDir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CheckoutHEAD runs `git -C repoDir checkout` to materialise the working tree
// according to the current sparse-checkout configuration.
func CheckoutHEAD(repoDir string) error {
	args := buildCheckoutHEADArgs(repoDir)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout in %s: %w\n%s", repoDir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Fetch runs `git -C repoDir fetch origin`. Routes through debug.Trace.
func Fetch(repoDir string) error {
	args := buildFetchArgs(repoDir)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch in %s: %w\n%s", repoDir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ResetHard runs `git -C repoDir reset --hard <ref>`. Routes through
// debug.Trace.
func ResetHard(repoDir, ref string) error {
	args := buildResetHardArgs(repoDir, ref)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset --hard %s in %s: %w\n%s", ref, repoDir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Clone clones sourceURL into destDir using `git clone`. The directory
// destDir must not already exist (git creates it). Stderr from git is
// included in the error message on failure.
func Clone(sourceURL, destDir string) error {
	args := buildCloneArgs(sourceURL, destDir)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone %s: %w\n%s", sourceURL, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RevParseHEAD returns the full commit SHA of HEAD in repoDir.
func RevParseHEAD(repoDir string) (string, error) {
	args := buildRevParseArgs(repoDir)
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %w", repoDir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentBranch returns the name of the currently checked-out branch in
// repoDir. Returns an empty string (without error) when the repo is in
// detached-HEAD state.
func CurrentBranch(repoDir string) (string, error) {
	args := []string{"-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD"}
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --abbrev-ref HEAD in %s: %w", repoDir, err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		// Detached HEAD state — return empty string.
		return "", nil
	}
	return branch, nil
}

// IsDirty returns true when the working tree in repoDir has any uncommitted
// changes (staged or unstaged). Returns false if the status cannot be
// determined (e.g. the directory is not a git repo).
func IsDirty(repoDir string) bool {
	args := []string{"-C", repoDir, "status", "--porcelain"}
	debug.Trace("git", args)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}
