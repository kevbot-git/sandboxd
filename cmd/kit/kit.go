// Package kit implements the `boxd kit` subcommand group.
//
// Call Register(root) once from cmd/ to attach the kit subcommand tree to
// the root Cobra command. Subcommands (add, list, …) self-register via
// their init() functions.
package kit

import (
	"github.com/spf13/cobra"
)

// kitCmd is the "boxd kit" parent command. It has no Run of its own —
// Cobra prints help when it is invoked without a subcommand.
var kitCmd = &cobra.Command{
	Use:   "kit",
	Short: "Manage Kit Repos",
	Long:  "Manage Kit Repos: add, update, remove, list installed repos.",
}

// Register attaches the kit subcommand tree to root (the boxd root command).
func Register(root *cobra.Command) {
	root.AddCommand(kitCmd)
}
