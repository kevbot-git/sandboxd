// Package tui contains the interactive prompts boxd uses when it can't
// resolve kit selections from flags or saved state.
package tui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/kevbot-git/boxd/internal/kits"
)

// ErrCancelled is returned when the user aborts the prompt (e.g. Ctrl+C).
var ErrCancelled = errors.New("kit selection cancelled")

// descStyle renders a kit's description in slightly dimmer text next
// to the display name. We use an explicit gray foreground rather than
// lipgloss.Faint because Faint is just an intensity attribute — the
// foreground color is still inherited from the surrounding style, so
// when huh wraps a selected option in green, a Faint description goes
// "faint green" instead of staying dim. Setting an explicit foreground
// makes lipgloss preserve the inner color escape so the description
// stays grey regardless of selection state.
var descStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "240",
	Dark:  "245",
})

// PromptKitSelection opens a multi-select TUI over the available kits.
// Returns the absolute paths (KitInfo.Path) of the chosen kits, in the
// order huh produced them. Selecting zero kits is allowed and returns
// an empty slice — the caller decides whether to create a sandbox with
// no --kit flags or to treat that as an error.
//
// Caller is responsible for handling the empty-available case (e.g.
// before calling this function, check len(available) > 0 and warn the
// user that nothing was discovered).
func PromptKitSelection(available []kits.KitInfo) ([]string, error) {
	if len(available) == 0 {
		return nil, errors.New("no kits available to select")
	}

	options := make([]huh.Option[string], 0, len(available))
	for _, k := range available {
		options = append(options, huh.NewOption(labelFor(k), k.Path))
	}

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select kits").
				Description("Space to toggle, Enter to confirm.").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrCancelled
		}
		return nil, err
	}
	return selected, nil
}

// labelFor builds the multi-select line for one kit: bold-ish display
// name followed by a faint description. Falls back through the layers
// so kits with no spec metadata still render usefully:
//
//	"Bun  Install the Bun runtime for the agent user"
//	"sandbox-kits/bun"            // no displayName, no description
//	"Bun"                         // displayName but no description
func labelFor(k kits.KitInfo) string {
	name := k.DisplayName
	if name == "" {
		name = k.Name
	}
	if k.Description == "" {
		return name
	}
	return fmt.Sprintf("%s  %s", name, descStyle.Render(k.Description))
}
