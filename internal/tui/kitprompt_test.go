package tui

import (
	"strings"
	"testing"

	"github.com/kevbot-git/sandboxd/internal/kits"
)

func TestPromptKitSelection_EmptyAvailableErrors(t *testing.T) {
	_, err := PromptKitSelection(nil)
	if err == nil {
		t.Fatal("expected error when no kits are available")
	}
}

// The non-empty path requires a real TTY to run huh's form loop, so
// labelFor (the pure helper that builds the multi-select line) is the
// only piece worth unit-testing directly. The form-driving code is
// short and read-reviewable.

func TestLabelFor_DisplayNameAndDescription(t *testing.T) {
	got := labelFor(kits.KitInfo{
		Path:        "/k/bun",
		Name:        "sandbox-kits/bun",
		DisplayName: "Bun",
		Description: "Install the Bun runtime for the agent user",
	})
	// Display name appears first, plainly.
	if !strings.HasPrefix(got, "Bun  ") {
		t.Errorf("expected label to start with %q; got %q", "Bun  ", got)
	}
	// Description text is present somewhere after the name (lipgloss
	// may wrap it in ANSI escapes, so we just check the substring).
	if !strings.Contains(got, "Install the Bun runtime for the agent user") {
		t.Errorf("description not present in label: %q", got)
	}
}

func TestLabelFor_NoSpecFallsBackToPathName(t *testing.T) {
	got := labelFor(kits.KitInfo{
		Path: "/k/bun",
		Name: "sandbox-kits/bun",
	})
	if got != "sandbox-kits/bun" {
		t.Errorf("got %q; want %q", got, "sandbox-kits/bun")
	}
}

func TestLabelFor_OnlyDisplayName(t *testing.T) {
	got := labelFor(kits.KitInfo{
		Path:        "/k/bun",
		Name:        "sandbox-kits/bun",
		DisplayName: "Bun",
	})
	if got != "Bun" {
		t.Errorf("got %q; want %q", got, "Bun")
	}
}

func TestLabelFor_OnlyDescription(t *testing.T) {
	got := labelFor(kits.KitInfo{
		Path:        "/k/bun",
		Name:        "sandbox-kits/bun",
		Description: "A description",
	})
	if !strings.HasPrefix(got, "sandbox-kits/bun  ") {
		t.Errorf("expected fallback name as prefix; got %q", got)
	}
	if !strings.Contains(got, "A description") {
		t.Errorf("description not present in label: %q", got)
	}
}
