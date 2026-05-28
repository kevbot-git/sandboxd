package kit

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kevbot-git/sandboxd/internal/git"
	"github.com/kevbot-git/sandboxd/internal/lockfile"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List installed Kit Repos",
	Long:    "List all Kit Repos registered in the lockfile with their commit, last-updated date, and kit names.",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		lockPath, err := lockfile.DefaultPath()
		if err != nil {
			return fmt.Errorf("lockfile path: %w", err)
		}
		return runList(cmd.OutOrStdout(), lockPath)
	},
}

func init() {
	kitCmd.AddCommand(listCmd)
}

// runList writes the installed Kit Repo table to w.
// It reads from the lockfile at lockPath. A missing or corrupted lockfile
// is treated as empty (matching lockfile.Load semantics).
func runList(w io.Writer, lockPath string) error {
	store, err := lockfile.Load(lockPath)
	if err != nil {
		return fmt.Errorf("load lockfile: %w", err)
	}

	all := store.All()
	if len(all) == 0 {
		fmt.Fprintln(w, "no kits installed; try 'boxd kit add <user>/<repo>'")
		return nil
	}

	// Sort keys alphabetically.
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Use a tabwriter for aligned columns.
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, key := range keys {
		e := all[key]

		date := e.UpdatedAt.Format("2006-01-02")

		kitList := strings.Join(e.Kits, ", ")
		if kitList == "" {
			kitList = "(none)"
		}

		if e.InstallMode == "local" {
			// Local Kit Repo: show worktree status instead of commit SHA.
			if _, err := os.Stat(e.InstallDir); err != nil {
				// Symlink target is gone.
				fmt.Fprintf(tw, "%s\t%s\t%s\tkits: %s\n", key, "[missing]", date, kitList)
			} else {
				branch, _ := git.CurrentBranch(e.InstallDir)
				if branch == "" {
					branch = "HEAD"
				}
				if git.IsDirty(e.InstallDir) {
					branch = branch + "*"
				}
				fmt.Fprintf(tw, "%s\t[%s]\t%s\tkits: %s\n", key, branch, date, kitList)
			}
		} else {
			// Remote Kit Repo: show commit SHA and dir-missing indicator if needed.
			shortSHA := e.Commit
			if len(shortSHA) > 7 {
				shortSHA = shortSHA[:7]
			}

			dirStatus := ""
			if _, err := os.Stat(e.InstallDir); err != nil {
				dirStatus = " [dir missing]"
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\tkits: %s%s\n", key, shortSHA, date, kitList, dirStatus)
		}
	}
	tw.Flush()

	return nil
}
