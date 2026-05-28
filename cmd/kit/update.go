package kit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kevbot-git/sandboxd/internal/git"
	"github.com/kevbot-git/sandboxd/internal/kits"
	"github.com/kevbot-git/sandboxd/internal/lockfile"
	"github.com/kevbot-git/sandboxd/internal/repo"
	"github.com/spf13/cobra"
)

// updateLocalEntry handles the update cycle for a local-mode Kit Repo entry.
// It reads the current HEAD SHA from the symlink target (no git write ops).
func updateLocalEntry(w io.Writer, stderr io.Writer, store *lockfile.Store, key string, entry lockfile.Entry) error {
	// 1. Check that the symlink target still exists (os.Stat follows symlinks).
	if _, err := os.Stat(entry.InstallDir); err != nil {
		fmt.Fprintf(stderr, "boxd: warning: %s: symlink target missing at %s, skipping\n", key, entry.SourceURL)
		return nil
	}

	// 2. Read current HEAD SHA from the target repo.
	newSHA, err := git.RevParseHEAD(entry.InstallDir)
	if err != nil {
		return fmt.Errorf("%s: rev-parse HEAD: %w", key, err)
	}

	// 3. No change.
	if newSHA == entry.Commit {
		fmt.Fprintf(w, "%s: already at %s\n", key, short(entry.Commit))
		return nil
	}

	// 4. SHA advanced — update lockfile.
	oldShort := short(entry.Commit)
	newShort := short(newSHA)
	fmt.Fprintf(w, "%s: %s → %s\n", key, oldShort, newShort)

	entry.Commit = newSHA
	entry.UpdatedAt = time.Now().UTC()
	store.Set(key, entry)
	if err := store.Save(); err != nil {
		return fmt.Errorf("%s: save lockfile: %w", key, err)
	}
	return nil
}

var updateCmd = &cobra.Command{
	Use:   "update [<addr>]",
	Short: "Refresh a Kit Repo's pinned commit",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lockPath, err := lockfile.DefaultPath()
		if err != nil {
			return fmt.Errorf("lockfile path: %w", err)
		}
		if len(args) == 0 {
			return runUpdateAll(cmd.OutOrStdout(), lockPath)
		}
		return runUpdateOne(cmd.OutOrStdout(), lockPath, args[0])
	},
}

func init() {
	kitCmd.AddCommand(updateCmd)
}

// runUpdateAll updates every entry in the lockfile.
func runUpdateAll(w io.Writer, lockPath string) error {
	store, err := lockfile.Load(lockPath)
	if err != nil {
		return fmt.Errorf("load lockfile: %w", err)
	}
	all := store.All()
	if len(all) == 0 {
		return nil
	}
	// Process entries in sorted order for deterministic output.
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := updateEntry(w, os.Stderr, store, key, all[key]); err != nil {
			return err
		}
	}
	return nil
}

// runUpdateOne updates the single lockfile entry identified by addrStr.
func runUpdateOne(w io.Writer, lockPath string, addrStr string) error {
	// Try a direct lockfile-key lookup first. Both local (`local/<ident>`)
	// and remote (`<host>/<user>/<repo>`) keys can be passed verbatim.
	if store, err := lockfile.Load(lockPath); err == nil {
		if entry, ok := store.Get(addrStr); ok {
			return updateEntry(w, os.Stderr, store, addrStr, entry)
		}
	}

	addr, err := repo.ParseAddress(addrStr)
	if err != nil {
		return err
	}
	store, err := lockfile.Load(lockPath)
	if err != nil {
		return fmt.Errorf("load lockfile: %w", err)
	}
	entry, ok := store.Get(addr.Key)
	if !ok {
		return fmt.Errorf("%s is not installed", addr.Key)
	}
	return updateEntry(w, os.Stderr, store, addr.Key, entry)
}

// updateEntry performs the fetch+reset cycle for one lockfile entry, writes
// one output line to w, and saves the store if anything changed.
// Local-mode entries are handled by updateLocalEntry (no git write ops).
func updateEntry(w io.Writer, stderr io.Writer, store *lockfile.Store, key string, entry lockfile.Entry) error {
	// Dispatch local-mode entries to a separate path.
	if entry.InstallMode == "local" {
		return updateLocalEntry(w, stderr, store, key, entry)
	}

	installDir := entry.InstallDir

	// 0. If the install directory is missing, re-clone it before fetching.
	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		if entry.SourceURL == "" {
			return fmt.Errorf("%s: malformed lockfile entry (missing sourceUrl) — try 'boxd kit remove %s' and re-add it", key, key)
		}
		if err := os.MkdirAll(filepath.Dir(installDir), 0o755); err != nil {
			return fmt.Errorf("%s: create parent dir: %w", key, err)
		}
		if err := git.Clone(entry.SourceURL, installDir); err != nil {
			return fmt.Errorf("%s: clone: %w", key, err)
		}
	}

	// 1. Fetch from origin.
	if err := git.Fetch(installDir); err != nil {
		return fmt.Errorf("%s: fetch: %w", key, err)
	}

	// 2. Reset to origin/HEAD.
	if err := git.ResetHard(installDir, "origin/HEAD"); err != nil {
		return fmt.Errorf("%s: reset --hard origin/HEAD: %w", key, err)
	}

	// 3. Get new SHA.
	newSHA, err := git.RevParseHEAD(installDir)
	if err != nil {
		return fmt.Errorf("%s: rev-parse HEAD: %w", key, err)
	}

	// 4. Discover new kits.
	discovered, err := kits.Discover(installDir)
	if err != nil {
		return fmt.Errorf("%s: discover kits: %w", key, err)
	}
	newKitNames := make([]string, 0, len(discovered))
	for _, k := range discovered {
		newKitNames = append(newKitNames, k.Name)
	}
	sort.Strings(newKitNames)

	// 5. Determine if anything changed.
	shaChanged := newSHA != entry.Commit
	kitsChanged := !stringSlicesEqual(newKitNames, entry.Kits)

	oldShort := short(entry.Commit)
	newShort := short(newSHA)

	if !shaChanged && !kitsChanged {
		fmt.Fprintf(w, "%s: already at %s\n", key, oldShort)
		return nil
	}

	// Compute kit delta.
	added, removed := kitDelta(entry.Kits, newKitNames)

	// Build output line.
	line := fmt.Sprintf("%s: %s → %s", key, oldShort, newShort)
	if added != 0 || removed != 0 {
		line += fmt.Sprintf(" (+%d kits / -%d kits)", added, removed)
	}
	fmt.Fprintln(w, line)

	// Save updated entry (preserve InstalledAt, bump UpdatedAt).
	entry.Commit = newSHA
	entry.Kits = newKitNames
	entry.UpdatedAt = time.Now().UTC()
	store.Set(key, entry)
	if err := store.Save(); err != nil {
		return fmt.Errorf("%s: save lockfile: %w", key, err)
	}
	return nil
}

// short returns the first 7 characters of a SHA, or the full string if shorter.
func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// stringSlicesEqual returns true when a and b have identical length and
// elements in the same order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// kitDelta computes how many kits were added and removed between old and new.
func kitDelta(oldKits, newKits []string) (added, removed int) {
	oldSet := make(map[string]struct{}, len(oldKits))
	for _, k := range oldKits {
		oldSet[k] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(newKits))
	for _, k := range newKits {
		newSet[k] = struct{}{}
	}
	for k := range newSet {
		if _, ok := oldSet[k]; !ok {
			added++
		}
	}
	for k := range oldSet {
		if _, ok := newSet[k]; !ok {
			removed++
		}
	}
	return added, removed
}

