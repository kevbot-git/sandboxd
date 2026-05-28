package sbx

import (
	"reflect"
	"testing"
)

func TestParseLS(t *testing.T) {
	// Fixtures mirror the column-aligned shape that the awk routine in
	// claude-contained:15-27 was written against. Real `sbx ls` output will
	// be substituted in when available; the parser only cares about the
	// WORKSPACE column position and comma-separated workspace entries.
	cases := []struct {
		name  string
		input string
		want  []Sandbox
	}{
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "header only",
			input: "NAME       STATUS   WORKSPACE\n",
			want:  nil,
		},
		{
			name: "single sandbox, single workspace, no tag",
			input: "" +
				"NAME       STATUS   WORKSPACE\n" +
				"my-box     running  /Users/dev/foo\n",
			want: []Sandbox{{Name: "my-box", Workspace: "/Users/dev/foo"}},
		},
		{
			name: "strips :readonly tag from sole workspace",
			input: "" +
				"NAME       STATUS   WORKSPACE\n" +
				"my-box     running  /Users/dev/foo:readonly\n",
			want: []Sandbox{{Name: "my-box", Workspace: "/Users/dev/foo"}},
		},
		{
			name: "takes first of comma-separated workspaces, strips tag from it only",
			input: "" +
				"NAME       STATUS   WORKSPACE\n" +
				"my-box     running  /Users/dev/foo, /Users/dev/.agents/skills/:readonly\n",
			want: []Sandbox{{Name: "my-box", Workspace: "/Users/dev/foo"}},
		},
		{
			name: "comma-then-tag combinations",
			input: "" +
				"NAME       STATUS   WORKSPACE\n" +
				"a          running  /a:readonly, /b\n" +
				"b          running  /b, /c:readonly\n",
			want: []Sandbox{
				{Name: "a", Workspace: "/a"},
				{Name: "b", Workspace: "/b"},
			},
		},
		{
			name: "multiple sandboxes",
			input: "" +
				"NAME       STATUS   WORKSPACE\n" +
				"alpha      running  /Users/dev/foo\n" +
				"beta       stopped  /Users/dev/bar\n" +
				"gamma      running  /Users/dev/baz:readonly\n",
			want: []Sandbox{
				{Name: "alpha", Workspace: "/Users/dev/foo"},
				{Name: "beta", Workspace: "/Users/dev/bar"},
				{Name: "gamma", Workspace: "/Users/dev/baz"},
			},
		},
		{
			name: "trailing whitespace on workspace value is stripped",
			input: "" +
				"NAME       STATUS   WORKSPACE\n" +
				"my-box     running  /Users/dev/foo   \n",
			want: []Sandbox{{Name: "my-box", Workspace: "/Users/dev/foo"}},
		},
		{
			name: "blank data lines are skipped",
			input: "" +
				"NAME       STATUS   WORKSPACE\n" +
				"alpha      running  /Users/dev/foo\n" +
				"\n" +
				"beta       running  /Users/dev/bar\n",
			want: []Sandbox{
				{Name: "alpha", Workspace: "/Users/dev/foo"},
				{Name: "beta", Workspace: "/Users/dev/bar"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLS(tc.input)
			if err != nil {
				t.Fatalf("parseLS returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseLS()\n got = %#v\nwant = %#v", got, tc.want)
			}
		})
	}
}

func TestParseLS_HeaderWithoutWorkspace(t *testing.T) {
	_, err := parseLS("NAME       STATUS   PATH\nalpha      running  /Users/dev/foo\n")
	if err == nil {
		t.Fatal("expected error when header is missing WORKSPACE column")
	}
}

func TestStripTagSuffix(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/foo", "/foo"},
		{"/foo:readonly", "/foo"},
		{"/foo:bar:readonly", "/foo:bar"},
		{"/foo:", "/foo:"},          // empty tag — awk regex requires one or more chars
		{"/foo:Readonly", "/foo:Readonly"}, // uppercase letter — awk used [a-z]+
		{"/foo:ro123", "/foo:ro123"},       // digits not allowed by [a-z]+
		{"c:/Users/dev", "c:/Users/dev"},   // colon followed by non-letter
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := stripTagSuffix(tc.in)
			if got != tc.want {
				t.Errorf("stripTagSuffix(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}
