package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevbot-git/boxd/internal/kits"
	"github.com/kevbot-git/boxd/internal/runner"
	"github.com/kevbot-git/boxd/internal/sbx"
	"github.com/kevbot-git/boxd/internal/selections"
	"github.com/kevbot-git/boxd/internal/tui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [-- sbx-positional-args...]",
	Short: "Create a sandbox for the current directory",
	Long: "Create a new sbx sandbox named after the current directory's basename, mounting\n" +
		"the selected kits. Kit selection precedence: --kit flags > saved selection in\n" +
		"~/.boxd/selections.json > multi-select TUI over kits discovered under ~/.boxd/kits/.\n\n" +
		"If no kits are found and none are specified, the sandbox is created with no kits.\n\n" +
		"Positional arguments after `--` are forwarded verbatim to `sbx run`, so extra\n" +
		"workspace mounts (e.g. `~/.agents/skills/:readonly`) can be added per-invocation.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := absCwd()
		if err != nil {
			return err
		}
		existing, err := sbx.FindByWorkspace(cwd)
		if err != nil {
			return err
		}
		if existing != "" {
			return fmt.Errorf("a sandbox %q already exists for %s — run `boxd shell` to enter it, or `boxd` to resume", existing, cwd)
		}
		flagKits, _ := cmd.Flags().GetStringSlice("kit")
		branch, _ := cmd.Flags().GetString("branch")
		return doInit(cwd, flagKits, branch, args)
	},
}

func init() {
	initCmd.Flags().StringSlice("kit", nil, "absolute path to a kit directory (repeatable)")
	initCmd.Flags().String("branch", "", "git branch to check out in the sandbox workspace")
	rootCmd.AddCommand(initCmd)
}

// doInit is the create-a-sandbox flow shared by `boxd init` and the
// no-args `boxd` fall-through path. It assumes the cwd has no existing
// sandbox; the caller is responsible for that check.
func doInit(cwd string, flagKits []string, branch string, positional []string) error {
	store, err := loadSelections()
	if err != nil {
		return err
	}

	resolved, persist, err := resolveKits(store, cwd, flagKits)
	if err != nil {
		return err
	}
	if persist {
		store.Set(cwd, resolved)
		if err := store.Save(); err != nil {
			return fmt.Errorf("save selections: %w", err)
		}
	}

	name := filepath.Base(cwd)
	return runner.Exec(runner.BuildInit(name, resolved, branch, positional))
}

// resolveKits applies the kit-selection precedence:
//  1. --kit flags → normalise to absolute paths, validate, mark for persist.
//  2. Saved selection for cwd → use as-is, no persist.
//  3. Discover kits under ~/.boxd/kits/.
//     - If none found → warn, return empty list, no persist.
//     - Else → open the multi-select TUI, mark for persist.
func resolveKits(store *selections.Store, cwd string, flagKits []string) ([]string, bool, error) {
	if len(flagKits) > 0 {
		abs, err := normaliseAndValidate(flagKits)
		if err != nil {
			return nil, false, err
		}
		return abs, true, nil
	}

	if saved, ok := store.Get(cwd); ok && len(saved) > 0 {
		fmt.Fprintf(os.Stderr, "boxd: using saved kits for %s:\n", cwd)
		for _, k := range saved {
			fmt.Fprintf(os.Stderr, "  - %s\n", k)
		}
		return saved, false, nil
	}

	root, err := kits.DefaultRoot()
	if err != nil {
		return nil, false, err
	}
	available, err := kits.Discover(root)
	if err != nil {
		return nil, false, err
	}
	if len(available) == 0 {
		fmt.Fprintf(os.Stderr, "boxd: no kits found in %s — creating sandbox without kits\n", root)
		return nil, false, nil
	}

	picked, err := tui.PromptKitSelection(available)
	if err != nil {
		return nil, false, err
	}
	return picked, true, nil
}

func normaliseAndValidate(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = expandTilde(p)
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("kit path %q: %w", p, err)
		}
		if err := kits.Validate(abs); err != nil {
			return nil, err
		}
		out = append(out, abs)
	}
	return out, nil
}

func expandTilde(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func loadSelections() (*selections.Store, error) {
	path, err := selections.DefaultPath()
	if err != nil {
		return nil, err
	}
	return selections.Load(path)
}
