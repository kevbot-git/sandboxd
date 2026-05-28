package debug

import (
	"bytes"
	"testing"
)

func TestTrace_SilentWhenDebugUnset(t *testing.T) {
	t.Setenv("DEBUG", "")
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	Trace("sbx", []string{"ls"})
	if buf.Len() != 0 {
		t.Errorf("expected no output, got %q", buf.String())
	}
}

func TestTrace_PrintsWhenDebugSet(t *testing.T) {
	t.Setenv("DEBUG", "1")
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	Trace("sbx", []string{"ls"})
	want := "> sbx ls\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestTrace_HandlesEmptyArgs(t *testing.T) {
	t.Setenv("DEBUG", "1")
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	Trace("sbx", nil)
	want := "> sbx\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestTrace_JoinsArgsWithSingleSpace(t *testing.T) {
	t.Setenv("DEBUG", "1")
	var buf bytes.Buffer
	Out = &buf
	t.Cleanup(func() { Out = nil })

	Trace("sbx", []string{"run", "claude", ".", "--name", "myproj", "--kit", "/p"})
	want := "> sbx run claude . --name myproj --kit /p\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

// Any non-empty value enables tracing, matching the user's "if DEBUG is
// set" wording — DEBUG=0 still counts as set.
func TestTrace_NonOneValueStillEnables(t *testing.T) {
	for _, v := range []string{"0", "yes", "true", "anything"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("DEBUG", v)
			var buf bytes.Buffer
			Out = &buf
			t.Cleanup(func() { Out = nil })

			Trace("sbx", []string{"ls"})
			if buf.Len() == 0 {
				t.Errorf("DEBUG=%q produced no output; expected trace line", v)
			}
		})
	}
}
