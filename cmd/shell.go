package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kevbot-git/boxd/internal/git"
	"github.com/kevbot-git/boxd/internal/runner"
	"github.com/kevbot-git/boxd/internal/sbx"
	"github.com/spf13/cobra"
)

var shellWorkdir string
var shellBranch string

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open a bash shell in the current directory's sandbox",
	Long: `Open a bash shell in the current directory's sandbox.

By default the shell opens at the current working directory inside the
sandbox (-w <cwd>). Use --workdir to open at a different path instead.
Use --branch to open at the worktree path for a given git branch.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := absCwd()
		if err != nil {
			return err
		}
		name, err := sbx.FindByWorkspace(cwd)
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("no sandbox for %s — run `boxd init` to create one", cwd)
		}
		workdir, err := resolveShellWorkdir(cwd, shellWorkdir, shellBranch)
		if err != nil {
			return err
		}
		return runner.Exec(runner.BuildShell(name, workdir))
	},
}

// resolveShellWorkdir returns the working directory to use for the shell
// command, applying the precedence: explicit workdir > branch worktree > cwd.
func resolveShellWorkdir(cwd, explicit, branch string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if branch != "" {
		return git.WorktreePath(cwd, branch)
	}
	return cwd, nil
}

// absCwd returns the current working directory in absolute form. Shared
// by every cmd that needs to key into sbx by cwd or store selections.
func absCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	return filepath.Abs(cwd)
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellWorkdir, "workdir", "w", "", "Working directory to open inside the sandbox (default: current directory)")
	shellCmd.Flags().StringVar(&shellBranch, "branch", "", "git branch whose worktree to use as the working directory")
}
