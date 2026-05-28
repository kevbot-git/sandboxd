package kit

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kevbot-git/sandboxd/internal/git"
	"github.com/kevbot-git/sandboxd/internal/lockfile"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Rehydrate ~/.boxd/kits/ from the lockfile",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		lockPath, err := lockfile.DefaultPath()
		if err != nil {
			return fmt.Errorf("lockfile path: %w", err)
		}
		return runInstall(cmd.OutOrStdout(), cmd.ErrOrStderr(), lockPath)
	},
}

func init() {
	kitCmd.AddCommand(installCmd)
}

// runInstall rehydrates ~/.boxd/kits/ from the lockfile at lockPath.
// w receives human-readable progress; stderr receives warnings.
func runInstall(w io.Writer, stderr io.Writer, lockPath string) error {
	store, err := lockfile.Load(lockPath)
	if err != nil {
		return fmt.Errorf("load lockfile: %w", err)
	}

	all := store.All()
	if len(all) == 0 {
		fmt.Fprintln(w, "no kits in lockfile")
		return nil
	}

	// Process entries in sorted key order for deterministic output.
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := all[key]

		if entry.SourceURL == "" {
			return fmt.Errorf("%s: malformed lockfile entry (missing sourceUrl) — try 'boxd kit remove %s' and re-add it", key, key)
		}

		if entry.InstallMode == "local" {
			if err := rehydrateLocalEntry(w, stderr, key, entry); err != nil {
				return err
			}
			continue
		}

		shortSHA := entry.Commit
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}

		info, statErr := os.Stat(entry.InstallDir)
		if statErr != nil || !info.IsDir() {
			// installDir doesn't exist: clone then reset.
			if err := git.Clone(entry.SourceURL, entry.InstallDir); err != nil {
				return fmt.Errorf("%s: clone failed: %w", key, err)
			}
			if err := git.ResetHard(entry.InstallDir, entry.Commit); err != nil {
				return fmt.Errorf("%s: reset failed: %w", key, err)
			}
			fmt.Fprintf(w, "cloned %s @ %s\n", key, shortSHA)
			continue
		}

		// installDir exists: check current SHA.
		currentSHA, err := git.RevParseHEAD(entry.InstallDir)
		if err != nil {
			return fmt.Errorf("%s: rev-parse HEAD: %w", key, err)
		}

		if currentSHA == entry.Commit {
			fmt.Fprintf(w, "%s: ok at %s\n", key, shortSHA)
			continue
		}

		// SHA mismatch: warn if dirty, then reset.
		if isDirty(entry.InstallDir) {
			fmt.Fprintf(stderr, "boxd: warning: %s: discarding local changes in %s\n", key, entry.InstallDir)
		}
		if err := git.ResetHard(entry.InstallDir, entry.Commit); err != nil {
			return fmt.Errorf("%s: reset failed: %w", key, err)
		}
		fmt.Fprintf(w, "reset %s to %s\n", key, shortSHA)
	}

	return nil
}

// rehydrateLocalEntry handles a lockfile entry with InstallMode == "local".
// It manages the symlink at entry.InstallDir pointing to entry.SourceURL.
func rehydrateLocalEntry(w io.Writer, stderr io.Writer, key string, entry lockfile.Entry) error {
	// 1. Check if source path exists.
	if _, err := os.Stat(entry.SourceURL); err != nil {
		fmt.Fprintf(stderr, "boxd: warning: %s: source path missing, skipping: %s\n", key, entry.SourceURL)
		return nil
	}

	// 2. Check current symlink state.
	_, lstatErr := os.Lstat(entry.InstallDir)
	if lstatErr != nil {
		// Nothing at InstallDir: create the symlink.
		if err := os.MkdirAll(filepath.Dir(entry.InstallDir), 0o755); err != nil {
			return fmt.Errorf("%s: create parent dir: %w", key, err)
		}
		if err := os.Symlink(entry.SourceURL, entry.InstallDir); err != nil {
			return fmt.Errorf("%s: create symlink: %w", key, err)
		}
		fmt.Fprintf(w, "linked %s → %s\n", key, entry.SourceURL)
		return nil
	}

	// Something exists at InstallDir: check it's a symlink pointing to the right place.
	target, err := os.Readlink(entry.InstallDir)
	if err != nil {
		return fmt.Errorf("%s: %s exists but is not a symlink (readlink: %w)", key, entry.InstallDir, err)
	}
	if target != entry.SourceURL {
		return fmt.Errorf("%s: symlink at %s points to %q, expected %q — refusing to overwrite", key, entry.InstallDir, target, entry.SourceURL)
	}

	// Correct symlink already present.
	fmt.Fprintf(w, "%s: ok\n", key)
	return nil
}

// isDirty returns true if the working tree has any changes (tracked or staged).
func isDirty(repoDir string) bool {
	cmd := exec.Command("git", "-C", repoDir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false // if we can't check, don't block the reset
	}
	return len(strings.TrimSpace(string(out))) > 0
}
