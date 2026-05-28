package kit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kevbot-git/boxd/internal/git"
	"github.com/kevbot-git/boxd/internal/kits"
	"github.com/kevbot-git/boxd/internal/lockfile"
	"github.com/kevbot-git/boxd/internal/repo"
	"github.com/spf13/cobra"
)

var addLocalFlag bool

var addCmd = &cobra.Command{
	Use:   "add <addr>",
	Short: "Install a Kit Repo",
	Long: `Install a Kit Repo by cloning it into ~/.boxd/kits/<repo>/.

<addr> can be one of:
  user/repo                  — short form (defaults to github.com)
  host.tld/user/repo         — explicit host
  https://host/user/repo     — full URL (with or without .git)

All forms accept an optional /kit suffix which identifies a single kit
within the repo; this performs a sparse checkout of only that kit.

Use --local <path> to install a Local Kit Repo by symlinking it into
~/.boxd/kits/<basename> rather than cloning it.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if addLocalFlag {
			return runAddLocal(args[0])
		}
		return runAdd(args[0])
	},
}

func init() {
	kitCmd.AddCommand(addCmd)
	addCmd.Flags().BoolVar(&addLocalFlag, "local", false, "Install a Local Kit Repo by symlinking (no clone)")
}

// resolvePath expands a leading ~ and makes the path absolute relative to
// the current working directory. It never accesses the filesystem.
func resolvePath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("abs path: %w", err)
	}
	return abs, nil
}

// runAddLocal installs a Local Kit Repo by symlinking path into
// ~/.boxd/kits/<basename>. No git write operations are performed on path.
func runAddLocal(path string) error {
	// 1. Resolve to absolute path.
	absPath, err := resolvePath(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// 2. Verify it is a git repository by reading HEAD.
	commit, err := git.RevParseHEAD(absPath)
	if err != nil {
		return fmt.Errorf("%s: not a git repository", absPath)
	}

	// 3. Compute the basename and the symlink destination.
	basename := filepath.Base(absPath)
	kitsRoot, err := kits.DefaultRoot()
	if err != nil {
		return fmt.Errorf("kits root: %w", err)
	}
	symlinkPath := filepath.Join(kitsRoot, basename)

	// 4. Load the lockfile and check for a duplicate key.
	lockPath, err := lockfile.DefaultPath()
	if err != nil {
		return fmt.Errorf("lockfile path: %w", err)
	}
	store, err := lockfile.Load(lockPath)
	if err != nil {
		return fmt.Errorf("load lockfile: %w", err)
	}
	if _, exists := store.Get(basename); exists {
		return fmt.Errorf("%s: already installed; use 'boxd kit update' to refresh", basename)
	}

	// 5. Check that the symlink destination doesn't already exist.
	if _, err := os.Lstat(symlinkPath); err == nil {
		return fmt.Errorf("%s: already exists at %s", basename, symlinkPath)
	}

	// 6. Create the kits root directory (if needed) and symlink.
	if err := os.MkdirAll(kitsRoot, 0o755); err != nil {
		return fmt.Errorf("create kits root: %w", err)
	}
	if err := os.Symlink(absPath, symlinkPath); err != nil {
		return fmt.Errorf("create symlink: %w", err)
	}

	// 7. Discover kits inside the repo.
	discovered, err := kits.Discover(absPath)
	if err != nil {
		return fmt.Errorf("discover kits in %s: %w", absPath, err)
	}
	kitNames := make([]string, 0, len(discovered))
	for _, k := range discovered {
		kitNames = append(kitNames, k.Name)
	}
	sort.Strings(kitNames)

	// 8. Write the lockfile entry.
	now := time.Now().UTC()
	store.Set(basename, lockfile.Entry{
		SourceURL:   absPath,
		Commit:      commit,
		InstallDir:  symlinkPath,
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        kitNames,
		InstallMode: "local",
	})
	if err := store.Save(); err != nil {
		return fmt.Errorf("save lockfile: %w", err)
	}

	shortSHA := commit
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	kitList := strings.Join(kitNames, ", ")
	if kitList == "" {
		kitList = "(none)"
	}
	fmt.Printf("installed %s @ %s [local] (%d kits: %s)\n", basename, shortSHA, len(kitNames), kitList)
	return nil
}

// runAdd is the implementation of `boxd kit add <addr>`.
func runAdd(addrStr string) error {
	// 1. Parse address.
	addr, err := repo.ParseAddress(addrStr)
	if err != nil {
		return err
	}

	// 2. Load the lockfile.
	lockPath, err := lockfile.DefaultPath()
	if err != nil {
		return fmt.Errorf("lockfile path: %w", err)
	}
	store, err := lockfile.Load(lockPath)
	if err != nil {
		return fmt.Errorf("load lockfile: %w", err)
	}

	existing, alreadyInstalled := store.Get(addr.Key)

	// 3. Route based on kit suffix and existing install state.
	if addr.Kit != "" {
		return runAddWithKit(addr, store, lockPath, existing, alreadyInstalled)
	}

	// No kit suffix.
	if alreadyInstalled {
		if existing.InstallMode == "sparse" {
			// Case C: convert sparse → full.
			return runConvertSparseToFull(addr, store, lockPath, existing)
		}
		// Already fully installed.
		return fmt.Errorf("%s: already installed; use 'boxd kit update' to refresh", addr.Key)
	}

	// Case E: fresh full install (existing behavior).
	return runFullInstall(addr, store, lockPath)
}

// runAddWithKit handles the `boxd kit add <repo>/<kit>` case.
func runAddWithKit(
	addr repo.Address,
	store *lockfile.Store,
	lockPath string,
	existing lockfile.Entry,
	alreadyInstalled bool,
) error {
	if !alreadyInstalled {
		// Case A: fresh sparse install.
		return runSparseInstall(addr, store, lockPath)
	}

	// Key is already installed.
	installMode := existing.InstallMode
	if installMode == "" {
		installMode = "full"
	}

	if installMode == "full" {
		// Case D: kit suffix on a fully-installed repo → no-op, print warning.
		fmt.Printf("%s is already available (repo is fully installed)\n", addr.Kit)
		return nil
	}

	// Case B: add kit to existing sparse repo.
	return runAddKitToSparse(addr, store, lockPath, existing)
}

// runSparseInstall handles Case A: fresh sparse install.
func runSparseInstall(addr repo.Address, store *lockfile.Store, lockPath string) error {
	kitsRoot, err := kits.DefaultRoot()
	if err != nil {
		return fmt.Errorf("kits root: %w", err)
	}
	installDir := filepath.Join(kitsRoot, addr.Repo)

	if _, err := os.Stat(installDir); err == nil {
		return fmt.Errorf("%s: directory exists; refusing to overwrite — remove it manually or pick a different name", addr.Repo)
	}

	if err := git.CloneNoCheckout(addr.SourceURL, installDir); err != nil {
		return fmt.Errorf("clone --no-checkout %s: %w", addr.SourceURL, err)
	}

	if err := git.SparseCheckoutInit(installDir); err != nil {
		return fmt.Errorf("sparse-checkout init: %w", err)
	}

	if err := git.SparseCheckoutSet(installDir, []string{addr.Kit}); err != nil {
		return fmt.Errorf("sparse-checkout set: %w", err)
	}

	if err := git.CheckoutHEAD(installDir); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}

	commit, err := git.RevParseHEAD(installDir)
	if err != nil {
		return fmt.Errorf("get commit SHA: %w", err)
	}

	now := time.Now().UTC()
	store.Set(addr.Key, lockfile.Entry{
		SourceURL:   addr.SourceURL,
		Commit:      commit,
		InstallDir:  installDir,
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        []string{addr.Kit},
		InstallMode: "sparse",
	})
	if err := store.Save(); err != nil {
		return fmt.Errorf("save lockfile: %w", err)
	}

	shortSHA := commit
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	fmt.Printf("installed %s @ %s (sparse: %s)\n", addr.Key, shortSHA, addr.Kit)
	return nil
}

// runAddKitToSparse handles Case B: adding a kit to an existing sparse install.
func runAddKitToSparse(
	addr repo.Address,
	store *lockfile.Store,
	lockPath string,
	existing lockfile.Entry,
) error {
	// Build the new union set.
	kitSet := make(map[string]struct{}, len(existing.Kits)+1)
	for _, k := range existing.Kits {
		kitSet[k] = struct{}{}
	}
	kitSet[addr.Kit] = struct{}{}

	newKits := make([]string, 0, len(kitSet))
	for k := range kitSet {
		newKits = append(newKits, k)
	}
	sort.Strings(newKits)

	if err := git.SparseCheckoutSet(existing.InstallDir, newKits); err != nil {
		return fmt.Errorf("sparse-checkout set: %w", err)
	}

	if err := git.CheckoutHEAD(existing.InstallDir); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}

	now := time.Now().UTC()
	existing.Kits = newKits
	existing.UpdatedAt = now
	store.Set(addr.Key, existing)
	if err := store.Save(); err != nil {
		return fmt.Errorf("save lockfile: %w", err)
	}

	fmt.Printf("added %s to %s (sparse: %s)\n", addr.Kit, addr.Key, strings.Join(newKits, ", "))
	return nil
}

// runConvertSparseToFull handles Case C: no kit suffix on an existing sparse install.
func runConvertSparseToFull(
	addr repo.Address,
	store *lockfile.Store,
	lockPath string,
	existing lockfile.Entry,
) error {
	if err := git.SparseCheckoutDisable(existing.InstallDir); err != nil {
		return fmt.Errorf("sparse-checkout disable: %w", err)
	}

	discovered, err := kits.Discover(existing.InstallDir)
	if err != nil {
		return fmt.Errorf("discover kits in %s: %w", existing.InstallDir, err)
	}
	kitNames := make([]string, 0, len(discovered))
	for _, k := range discovered {
		kitNames = append(kitNames, k.Name)
	}
	sort.Strings(kitNames)

	now := time.Now().UTC()
	existing.Kits = kitNames
	existing.InstallMode = "full"
	existing.UpdatedAt = now
	store.Set(addr.Key, existing)
	if err := store.Save(); err != nil {
		return fmt.Errorf("save lockfile: %w", err)
	}

	kitList := strings.Join(kitNames, ", ")
	if kitList == "" {
		kitList = "(none)"
	}
	fmt.Printf("converted %s to full install (%d kits: %s)\n", addr.Key, len(kitNames), kitList)
	return nil
}

// runFullInstall handles Case E: fresh full install (the original behavior).
func runFullInstall(addr repo.Address, store *lockfile.Store, lockPath string) error {
	// Compute install directory: ~/.boxd/kits/<repo>
	kitsRoot, err := kits.DefaultRoot()
	if err != nil {
		return fmt.Errorf("kits root: %w", err)
	}
	installDir := filepath.Join(kitsRoot, addr.Repo)

	// Error if the install directory already exists.
	if _, err := os.Stat(installDir); err == nil {
		return fmt.Errorf("%s: directory exists; refusing to overwrite — remove it manually or pick a different name", addr.Repo)
	}

	// Clone the repo.
	if err := git.Clone(addr.SourceURL, installDir); err != nil {
		return fmt.Errorf("clone %s: %w", addr.SourceURL, err)
	}

	// Capture the commit SHA.
	commit, err := git.RevParseHEAD(installDir)
	if err != nil {
		return fmt.Errorf("get commit SHA: %w", err)
	}

	// Discover kits inside the cloned repo.
	discovered, err := kits.Discover(installDir)
	if err != nil {
		return fmt.Errorf("discover kits in %s: %w", installDir, err)
	}
	kitNames := make([]string, 0, len(discovered))
	for _, k := range discovered {
		kitNames = append(kitNames, k.Name)
	}
	sort.Strings(kitNames)

	// Update the lockfile atomically.
	now := time.Now().UTC()
	store.Set(addr.Key, lockfile.Entry{
		SourceURL:   addr.SourceURL,
		Commit:      commit,
		InstallDir:  installDir,
		InstalledAt: now,
		UpdatedAt:   now,
		Kits:        kitNames,
		InstallMode: "full",
	})
	if err := store.Save(); err != nil {
		return fmt.Errorf("save lockfile: %w", err)
	}

	// Print success.
	shortSHA := commit
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	kitList := strings.Join(kitNames, ", ")
	if kitList == "" {
		kitList = "(none)"
	}
	fmt.Printf("installed %s @ %s (%d kits: %s)\n", addr.Key, shortSHA, len(kitNames), kitList)
	return nil
}
