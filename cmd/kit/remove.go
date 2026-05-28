package kit

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kevbot-git/sandboxd/internal/git"
	"github.com/kevbot-git/sandboxd/internal/lockfile"
	"github.com/kevbot-git/sandboxd/internal/repo"
	"github.com/kevbot-git/sandboxd/internal/selections"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:     "remove <addr>",
	Aliases: []string{"rm"},
	Short:   "Uninstall a Kit Repo",
	Long: `Uninstall a Kit Repo, removing its install directory and lockfile entry.

<addr> can be one of:
  user/repo[/kit]            — remove the whole repo, or a specific kit from a sparse install
  host.tld/user/repo[/kit]   — explicit host form

If <addr> includes a /kit suffix the repo must be sparsely installed;
individual kits cannot be removed from a fully-installed repo.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lockPath, err := lockfile.DefaultPath()
		if err != nil {
			return fmt.Errorf("lockfile path: %w", err)
		}
		selPath, err := selections.DefaultPath()
		if err != nil {
			return fmt.Errorf("selections path: %w", err)
		}
		return runRemove(cmd.ErrOrStderr(), lockPath, selPath, args[0])
	},
}

func init() {
	kitCmd.AddCommand(removeCmd)
}

// runRemove is the implementation of `boxd kit remove <addr>`.
// stderr receives warning lines about orphaned selections.
// lockPath and selPath are passed explicitly for testability.
func runRemove(stderr io.Writer, lockPath, selPath, addrStr string) error {
	// Try a direct lockfile-key lookup first. Local entries are keyed
	// `local/<ident>` and remote entries `<host>/<user>/<repo>` — both can
	// be passed verbatim, sidestepping repo.ParseAddress.
	if store, err := lockfile.Load(lockPath); err == nil {
		if entry, ok := store.Get(addrStr); ok {
			return runRemoveWhole(stderr, store, lockPath, selPath,
				repo.Address{Key: addrStr}, entry)
		}
	}

	// 1. Parse address.
	addr, err := repo.ParseAddress(addrStr)
	if err != nil {
		return err
	}

	// 2. Load the lockfile; error if the key is not present.
	store, err := lockfile.Load(lockPath)
	if err != nil {
		return fmt.Errorf("load lockfile: %w", err)
	}
	entry, ok := store.Get(addr.Key)
	if !ok {
		return fmt.Errorf("%s is not installed", addr.Key)
	}

	// 3. Route based on kit suffix.
	if addr.Kit != "" {
		return runRemoveKit(stderr, store, lockPath, selPath, addr, entry)
	}

	// Case C: no kit suffix — remove the whole repo (existing behavior).
	return runRemoveWhole(stderr, store, lockPath, selPath, addr, entry)
}

// runRemoveKit handles the `boxd kit remove <repo>/<kit>` case.
func runRemoveKit(
	stderr io.Writer,
	store *lockfile.Store,
	lockPath, selPath string,
	addr repo.Address,
	entry lockfile.Entry,
) error {
	installMode := entry.InstallMode
	if installMode == "" {
		installMode = "full"
	}

	if installMode == "full" {
		// Case B: cannot remove individual kits from a full install.
		return fmt.Errorf(
			"cannot remove individual kits from a fully-installed repo; use 'boxd kit remove %s' to remove the whole repo",
			addr.User+"/"+addr.Repo,
		)
	}

	// Case A: sparse — remove one kit from the sparse set.
	newKits := make([]string, 0, len(entry.Kits))
	for _, k := range entry.Kits {
		if k != addr.Kit {
			newKits = append(newKits, k)
		}
	}
	sort.Strings(newKits)

	if len(newKits) == 0 {
		// Last kit removed — remove the whole repo.
		if err := os.RemoveAll(entry.InstallDir); err != nil {
			return fmt.Errorf("remove %s: %w", entry.InstallDir, err)
		}
		store.Delete(addr.Key)
		if err := store.Save(); err != nil {
			return fmt.Errorf("save lockfile: %w", err)
		}
		fmt.Printf("removed %s (last kit removed)\n", addr.Key)
		return nil
	}

	// Update sparse checkout to the new set.
	if err := git.SparseCheckoutSet(entry.InstallDir, newKits); err != nil {
		return fmt.Errorf("sparse-checkout set: %w", err)
	}

	entry.Kits = newKits
	store.Set(addr.Key, entry)
	if err := store.Save(); err != nil {
		return fmt.Errorf("save lockfile: %w", err)
	}

	fmt.Printf("removed %s from %s (remaining: %s)\n", addr.Kit, addr.Key, strings.Join(newKits, ", "))
	return nil
}

// runRemoveLocalWhole handles removal of a locally-sourced Kit Repo.
// It removes only the symlink at entry.InstallDir; the source directory is untouched.
func runRemoveLocalWhole(
	stderr io.Writer,
	store *lockfile.Store,
	lockPath, selPath string,
	addr repo.Address,
	entry lockfile.Entry,
) error {
	installDir := entry.InstallDir

	// Warn about orphaned selections (read-only; do not modify).
	selStore, err := selections.Load(selPath)
	if err != nil {
		return fmt.Errorf("load selections: %w", err)
	}
	for cwd, kitPaths := range selStore.All() {
		for _, kp := range kitPaths {
			if kp == installDir || strings.HasPrefix(kp, installDir+"/") {
				fmt.Fprintf(stderr,
					"boxd: warning: %s has a saved kit selection inside %s; sbx will error when that sandbox is recreated until it's re-picked\n",
					cwd, installDir,
				)
				break
			}
		}
	}

	// Remove the symlink; if it's already missing, warn and continue.
	if _, err := os.Lstat(installDir); err != nil {
		fmt.Fprintf(stderr,
			"boxd: warning: %s: symlink at %s already missing, cleaning up lockfile\n",
			addr.Key, installDir,
		)
	} else {
		// os.Remove removes the symlink itself, not the target directory.
		if err := os.Remove(installDir); err != nil {
			return fmt.Errorf("remove symlink %s: %w", installDir, err)
		}
	}

	// Remove the lockfile entry and save.
	store.Delete(addr.Key)
	if err := store.Save(); err != nil {
		return fmt.Errorf("save lockfile: %w", err)
	}

	fmt.Printf("removed %s\n", addr.Key)
	return nil
}

// runRemoveWhole removes the whole repo regardless of install mode.
func runRemoveWhole(
	stderr io.Writer,
	store *lockfile.Store,
	lockPath, selPath string,
	addr repo.Address,
	entry lockfile.Entry,
) error {
	// For local repos: remove only the symlink; leave the source directory intact.
	if entry.InstallMode == "local" {
		return runRemoveLocalWhole(stderr, store, lockPath, selPath, addr, entry)
	}

	installDir := entry.InstallDir

	// Warn about orphaned selections (read-only; do not modify).
	selStore, err := selections.Load(selPath)
	if err != nil {
		return fmt.Errorf("load selections: %w", err)
	}
	for cwd, kitPaths := range selStore.All() {
		for _, kp := range kitPaths {
			if kp == installDir || strings.HasPrefix(kp, installDir+"/") {
				fmt.Fprintf(stderr,
					"boxd: warning: %s has a saved kit selection inside %s; sbx will error when that sandbox is recreated until it's re-picked\n",
					cwd, installDir,
				)
				break
			}
		}
	}

	// Remove the install directory (tolerate already-missing).
	if err := os.RemoveAll(installDir); err != nil {
		return fmt.Errorf("remove %s: %w", installDir, err)
	}

	// Remove the lockfile entry and save.
	store.Delete(addr.Key)
	if err := store.Save(); err != nil {
		return fmt.Errorf("save lockfile: %w", err)
	}

	// Report success.
	fmt.Printf("removed %s\n", addr.Key)
	return nil
}
