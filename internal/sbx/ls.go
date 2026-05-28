// Package sbx wraps the `sbx` CLI we shell out to. It does not embed sbx;
// it builds argv lists and parses sbx stdout.
package sbx

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/kevbot-git/boxd/internal/debug"
)

// Sandbox is one row from `sbx ls`. Only the fields boxd cares about are
// captured: the sandbox name (column 1) and its primary workspace path.
type Sandbox struct {
	Name      string
	Workspace string // first workspace listed, with any trailing :tag stripped
}

// List runs `sbx ls` and parses the result. When DEBUG is set, the
// invocation is logged to stderr first.
func List() ([]Sandbox, error) {
	debug.Trace("sbx", []string{"ls"})
	out, err := exec.Command("sbx", "ls").Output()
	if err != nil {
		return nil, fmt.Errorf("sbx ls: %w", err)
	}
	return parseLS(string(out))
}

// FindByWorkspace returns the name of the sandbox whose primary workspace
// equals workspace, or "" if none match.
func FindByWorkspace(workspace string) (string, error) {
	boxes, err := List()
	if err != nil {
		return "", err
	}
	for _, b := range boxes {
		if b.Workspace == workspace {
			return b.Name, nil
		}
	}
	return "", nil
}

// parseLS parses column-aligned `sbx ls` stdout. The header row must contain
// "WORKSPACE" as a column label; from that column onwards, each data row holds
// a comma-separated list of workspace entries. boxd only needs the first one,
// with any trailing :tag suffix (e.g. ":readonly") stripped — matching the
// shape extracted by the awk routine in claude-contained:15-27.
//
// JSON support (via a hypothesised `sbx ls --json`) is intentionally not
// implemented yet: sbx may or may not gain that flag, and the text parse is
// the only path that's known to work today.
func parseLS(input string) ([]Sandbox, error) {
	lines := strings.Split(input, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, nil
	}
	header := lines[0]
	wsCol := strings.Index(header, "WORKSPACE")
	if wsCol < 0 {
		return nil, fmt.Errorf("sbx ls: header missing WORKSPACE column: %q", header)
	}
	var out []Sandbox
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		nameEnd := strings.IndexAny(line, " \t")
		if nameEnd < 0 {
			continue
		}
		name := line[:nameEnd]
		if len(line) <= wsCol {
			continue
		}
		wsField := line[wsCol:]
		first := wsField
		if idx := strings.Index(wsField, ","); idx >= 0 {
			first = wsField[:idx]
		}
		first = strings.TrimRight(first, " \t")
		first = stripTagSuffix(first)
		out = append(out, Sandbox{Name: name, Workspace: first})
	}
	return out, nil
}

// stripTagSuffix removes a trailing ":word" suffix where word is one or more
// lowercase ASCII letters — matching the awk regex /:[a-z]+$/.
func stripTagSuffix(s string) string {
	i := strings.LastIndex(s, ":")
	if i < 0 || i == len(s)-1 {
		return s
	}
	for _, r := range s[i+1:] {
		if r < 'a' || r > 'z' {
			return s
		}
	}
	return s[:i]
}
