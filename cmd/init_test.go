package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kevbot-git/sandboxd/internal/selections"
)

// makeKitDir creates a directory that passes validateKitPath: a real
// directory containing a spec.yaml file.
func makeKitDir(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte("kind: mixin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNormaliseAndValidate_AcceptsValidKit(t *testing.T) {
	tmp := t.TempDir()
	kit := makeKitDir(t, tmp, "bun")

	got, err := normaliseAndValidate([]string{kit})
	if err != nil {
		t.Fatalf("normaliseAndValidate: %v", err)
	}
	if !reflect.DeepEqual(got, []string{kit}) {
		t.Errorf("got %v; want %v", got, []string{kit})
	}
}

func TestNormaliseAndValidate_RelativePathBecomesAbsolute(t *testing.T) {
	tmp := t.TempDir()
	kit := makeKitDir(t, tmp, "bun")

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prev)
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	got, err := normaliseAndValidate([]string{"bun"})
	if err != nil {
		t.Fatalf("normaliseAndValidate: %v", err)
	}
	if len(got) != 1 || !filepath.IsAbs(got[0]) {
		t.Fatalf("expected absolute path; got %v", got)
	}
	// Compare via EvalSymlinks because t.TempDir on macOS resolves through /private.
	wantResolved, _ := filepath.EvalSymlinks(kit)
	gotResolved, _ := filepath.EvalSymlinks(got[0])
	if wantResolved != gotResolved {
		t.Errorf("got %s; want %s", got[0], kit)
	}
}

func TestNormaliseAndValidate_RejectsMissingPath(t *testing.T) {
	tmp := t.TempDir()
	_, err := normaliseAndValidate([]string{filepath.Join(tmp, "no-such-kit")})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestNormaliseAndValidate_RejectsDirWithoutSpec(t *testing.T) {
	tmp := t.TempDir()
	noSpec := filepath.Join(tmp, "no-spec")
	if err := os.MkdirAll(noSpec, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := normaliseAndValidate([]string{noSpec})
	if err == nil {
		t.Fatal("expected error for dir without spec.yaml")
	}
	if !strings.Contains(err.Error(), "spec.yaml") {
		t.Errorf("error %q should mention spec.yaml", err.Error())
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in, want string
	}{
		{"~", home},
		{"~/", filepath.Join(home)},
		{"~/kits/bun", filepath.Join(home, "kits/bun")},
		{"/abs/path", "/abs/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := expandTilde(c.in)
			if got != c.want {
				t.Errorf("expandTilde(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

// resolveKits should pick --kit flags over saved selections.
func TestResolveKits_FlagOverridesSavedSelection(t *testing.T) {
	tmp := t.TempDir()
	flagKit := makeKitDir(t, tmp, "flag-kit")
	savedKit := makeKitDir(t, tmp, "saved-kit")

	store, err := selections.Load(filepath.Join(tmp, "selections.json"))
	if err != nil {
		t.Fatal(err)
	}
	cwd := "/Users/dev/proj"
	store.Set(cwd, []string{savedKit})

	got, persist, err := resolveKits(store, cwd, []string{flagKit})
	if err != nil {
		t.Fatalf("resolveKits: %v", err)
	}
	if !reflect.DeepEqual(got, []string{flagKit}) {
		t.Errorf("got %v; want flag value %v", got, []string{flagKit})
	}
	if !persist {
		t.Error("expected persist=true when --kit flags were supplied")
	}
}

// resolveKits should return the saved selection when no flags are given.
func TestResolveKits_FallsBackToSavedSelection(t *testing.T) {
	tmp := t.TempDir()
	savedKit := makeKitDir(t, tmp, "saved-kit")

	store, err := selections.Load(filepath.Join(tmp, "selections.json"))
	if err != nil {
		t.Fatal(err)
	}
	cwd := "/Users/dev/proj"
	store.Set(cwd, []string{savedKit})

	got, persist, err := resolveKits(store, cwd, nil)
	if err != nil {
		t.Fatalf("resolveKits: %v", err)
	}
	if !reflect.DeepEqual(got, []string{savedKit}) {
		t.Errorf("got %v; want saved %v", got, []string{savedKit})
	}
	if persist {
		t.Error("expected persist=false when using saved selection (no need to re-write)")
	}
}

func TestResolveKits_InvalidFlagPathErrors(t *testing.T) {
	tmp := t.TempDir()
	store, _ := selections.Load(filepath.Join(tmp, "selections.json"))
	_, _, err := resolveKits(store, "/cwd", []string{filepath.Join(tmp, "nonexistent")})
	if err == nil {
		t.Fatal("expected error for nonexistent kit path")
	}
}

// When the user has no --kit flag and no saved selection, and there are
// no kits discovered under ~/.boxd/kits/, resolveKits returns empty +
// persist=false so the caller proceeds to create a sandbox without kits.
func TestResolveKits_EmptyDiscoveryReturnsNoKitsNoPersist(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	store, _ := selections.Load(filepath.Join(fakeHome, "selections.json"))

	got, persist, err := resolveKits(store, "/Users/dev/some-fresh-cwd", nil)
	if err != nil {
		t.Fatalf("resolveKits: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil kit list, got %v", got)
	}
	if persist {
		t.Error("expected persist=false when discovery is empty (no need to write an empty selection)")
	}
}
