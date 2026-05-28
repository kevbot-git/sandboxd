// Package debug centralises the DEBUG-gated tracing boxd uses to log
// every subprocess invocation. When the DEBUG environment variable is
// set to any non-empty value, each call to Trace prints a line to
// stderr of the form:
//
//	> sbx ls
//	> sbx run claude . --name myproj --kit /path/to/bun
//
// The leading "> " is chosen so debug output is visually distinct from
// boxd's normal stderr ("boxd: ...") and from the wrapped subprocess's
// own output.
package debug

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Out is the stream debug traces are written to. Defaults to os.Stderr;
// tests reassign it to capture output.
var Out io.Writer = os.Stderr

// enabled reports whether tracing is currently on. Re-read on every
// call so a test or caller can flip DEBUG mid-process.
func enabled() bool {
	return os.Getenv("DEBUG") != ""
}

// Trace logs the given command invocation if DEBUG is set. name is the
// binary (e.g. "sbx"); args is the argv slice that will be passed to it.
func Trace(name string, args []string) {
	if !enabled() {
		return
	}
	if len(args) == 0 {
		fmt.Fprintf(Out, "> %s\n", name)
		return
	}
	fmt.Fprintf(Out, "> %s %s\n", name, strings.Join(args, " "))
}
