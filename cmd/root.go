package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/kevbot-git/sandboxd/internal/runner"
	"github.com/kevbot-git/sandboxd/internal/sbx"
	"github.com/spf13/cobra"
)

var version = "dev"

var rootBranch string

var rootCmd = &cobra.Command{
	Use:     "boxd",
	Short:   "Install and run sandbox kits",
	Long:    "boxd installs sandbox kits from git repos and runs sbx sandboxes with them.\n\nWith no subcommand, boxd resumes the sandbox for the current working directory if one\nexists, otherwise it falls through to the `boxd init` flow.",
	Version: version,
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
			var extra []string
			if rootBranch != "" {
				extra = append(extra, "--branch", rootBranch)
			}
			extra = append(extra, args...)
			return runner.Exec(runner.BuildResume(extra))
		}
		fmt.Fprintf(os.Stderr, "boxd: no sandbox for %s — running init...\n", cwd)
		return doInit(cwd, nil, rootBranch, args)
	},
}

func init() {
	rootCmd.Flags().StringVar(&rootBranch, "branch", "", "git branch to check out in the sandbox workspace")
}

// Execute runs the root command and translates exec.ExitError into a
// matching boxd exit code, so `boxd shell` -> `bash; exit 5` makes boxd
// exit 5 too. Other errors are printed and exit 1.
func Execute() {
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	if err := rootCmd.Execute(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "boxd: %v\n", err)
		os.Exit(1)
	}
}
