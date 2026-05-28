package runner

import (
	"reflect"
	"testing"
)

func TestBuildInit(t *testing.T) {
	cases := []struct {
		name   string
		nameIn string
		kits   []string
		branch string
		extra  []string
		want   []string
	}{
		{
			name:   "no kits, no branch, no extras",
			nameIn: "myproj",
			want:   []string{"run", "claude", ".", "--name", "myproj"},
		},
		{
			name:   "single kit",
			nameIn: "myproj",
			kits:   []string{"/path/to/bun"},
			want:   []string{"run", "claude", ".", "--name", "myproj", "--kit", "/path/to/bun"},
		},
		{
			name:   "multiple kits preserve order",
			nameIn: "myproj",
			kits:   []string{"/a", "/b", "/c"},
			want:   []string{"run", "claude", ".", "--name", "myproj", "--kit", "/a", "--kit", "/b", "--kit", "/c"},
		},
		{
			name:   "branch is appended after kits",
			nameIn: "myproj",
			kits:   []string{"/a"},
			branch: "feature-x",
			want:   []string{"run", "claude", ".", "--name", "myproj", "--kit", "/a", "--branch", "feature-x"},
		},
		{
			name:   "extras are appended last verbatim",
			nameIn: "myproj",
			kits:   []string{"/a"},
			extra:  []string{"~/.agents/skills/:readonly", "/another/workspace"},
			want: []string{
				"run", "claude", ".", "--name", "myproj",
				"--kit", "/a",
				"~/.agents/skills/:readonly", "/another/workspace",
			},
		},
		{
			name:   "empty branch flag is omitted",
			nameIn: "myproj",
			branch: "",
			want:   []string{"run", "claude", ".", "--name", "myproj"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildInit(tc.nameIn, tc.kits, tc.branch, tc.extra)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("BuildInit()\n got = %v\nwant = %v", got, tc.want)
			}
		})
	}
}

func TestBuildResume(t *testing.T) {
	cases := []struct {
		name  string
		extra []string
		want  []string
	}{
		{name: "no extras", want: []string{"run", "claude"}},
		{
			name:  "with extras",
			extra: []string{"--branch", "foo"},
			want:  []string{"run", "claude", "--branch", "foo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildResume(tc.extra)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("BuildResume()\n got = %v\nwant = %v", got, tc.want)
			}
		})
	}
}

func TestBuildShell(t *testing.T) {
	cases := []struct {
		name    string
		sandbox string
		workdir string
		want    []string
	}{
		{
			name:    "with workdir",
			sandbox: "my-sandbox",
			workdir: "/home/user/project",
			want:    []string{"exec", "-it", "-w", "/home/user/project", "my-sandbox", "bash"},
		},
		{
			name:    "empty workdir omits -w flag",
			sandbox: "my-sandbox",
			workdir: "",
			want:    []string{"exec", "-it", "my-sandbox", "bash"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildShell(tc.sandbox, tc.workdir)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("BuildShell()\n got = %v\nwant = %v", got, tc.want)
			}
		})
	}
}
