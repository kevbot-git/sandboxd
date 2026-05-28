// Package runner builds and executes `sbx` argv lists. Each command type
// has its own Build* function so cmd/ can construct the call without
// embedding string concatenation, and tests can assert the argv shape
// without exec'ing anything.
package runner

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/kevbot-git/sandboxd/internal/debug"
)

// BuildInit returns the argv (excluding the leading "sbx") for creating a
// fresh sandbox with `sbx run claude . --name <name> [--kit P]... [--branch B] [extra...]`.
//
// extra is appended verbatim and is intended for any additional positional
// arguments the user passes after `--` on `boxd init`.
func BuildInit(name string, kits []string, branch string, extra []string) []string {
	args := []string{"run", "claude", ".", "--name", name}
	for _, k := range kits {
		args = append(args, "--kit", k)
	}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, extra...)
	return args
}

// BuildResume returns the argv for resuming an existing sandbox. It mirrors
// claude-contained's cmd_run exactly: `sbx run claude [extra...]`, with no
// --name flag — sbx is expected to attach by workspace match against cwd.
func BuildResume(extra []string) []string {
	return append([]string{"run", "claude"}, extra...)
}

// BuildShell returns the argv for `sbx exec -it [-w <workdir>] <name> bash`.
// When workdir is non-empty, -w <workdir> is inserted before the sandbox name.
func BuildShell(name, workdir string) []string {
	args := []string{"exec", "-it"}
	if workdir != "" {
		args = append(args, "-w", workdir)
	}
	return append(args, name, "bash")
}

// Exec runs `sbx <args...>` with the parent's stdio attached. It blocks
// until the subprocess exits and returns its error (nil on exit 0).
// When DEBUG is set, the invocation is logged to stderr first.
func Exec(args []string) error {
	debug.Trace("sbx", args)
	cmd := exec.Command("sbx", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sbx %v: %w", args, err)
	}
	return nil
}
