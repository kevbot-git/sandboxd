package kits

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// seedKit creates a kit directory (with a minimal spec.yaml) at root/rel.
func seedKit(t *testing.T, root, rel string) string {
	t.Helper()
	return seedKitWithSpec(t, root, rel, "kind: mixin\n")
}

// seedKitWithSpec creates a kit directory at root/rel with the given
// spec.yaml content.
func seedKitWithSpec(t *testing.T, root, rel, spec string) string {
	t.Helper()
	dir := filepath.Join(root, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDiscover_MissingRoot(t *testing.T) {
	got, err := Discover(filepath.Join(t.TempDir(), "no-such-dir"))
	if err != nil {
		t.Fatalf("Discover on missing root returned error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil result on missing root, got %v", got)
	}
}

func TestDiscover_EmptyRoot(t *testing.T) {
	got, err := Discover(t.TempDir())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil result on empty root, got %v", got)
	}
}

func TestDiscover_OneKitAtDepth1(t *testing.T) {
	root := t.TempDir()
	kit := seedKit(t, root, "bun")

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []KitInfo{{Path: kit, Name: "bun"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v; want %#v", got, want)
	}
}

func TestDiscover_TwoLevelRepoLayout(t *testing.T) {
	root := t.TempDir()
	bun := seedKit(t, root, "sandbox-kits/bun")
	van := seedKit(t, root, "sandbox-kits/vanilla")
	legacy := seedKit(t, root, "work-kits/legacy")

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []KitInfo{
		{Path: bun, Name: filepath.Join("sandbox-kits", "bun")},
		{Path: van, Name: filepath.Join("sandbox-kits", "vanilla")},
		{Path: legacy, Name: filepath.Join("work-kits", "legacy")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v;\nwant %#v", got, want)
	}
}

func TestDiscover_DoesNotRecurseIntoKit(t *testing.T) {
	root := t.TempDir()
	outer := seedKit(t, root, "outer")
	// A spec.yaml inside the outer kit (a fixture, a vendored sub-thing,
	// whatever) must not be picked up as a separate kit.
	seedKit(t, root, filepath.Join("outer", "fixtures"))

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []KitInfo{{Path: outer, Name: "outer"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v; want %#v", got, want)
	}
}

func TestDiscover_SkipsDirsWithoutSpec(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "not-a-kit", "still-not"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil result, got %v", got)
	}
}

func TestDiscover_RejectsRootThatIsAFile(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "file")
	if err := os.WriteFile(root, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Discover(root)
	if err == nil {
		t.Fatal("expected error when root is a file")
	}
}

// Symlinked kit at depth 1: the user runs
// `ln -s ~/dev/some-kit ~/.boxd/kits/some-kit` and expects Discover
// to surface it. filepath.WalkDir does not follow symlinks; the
// walker in this package must.
func TestDiscover_FollowsPerKitSymlinkAtDepth1(t *testing.T) {
	realDir := t.TempDir()
	realKit := seedKit(t, realDir, "vanilla-sandbox")

	root := t.TempDir()
	linkPath := filepath.Join(root, "vanilla-sandbox")
	if err := os.Symlink(realKit, linkPath); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []KitInfo{{Path: linkPath, Name: "vanilla-sandbox"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v; want %#v", got, want)
	}
}

// Symlinked repo at depth 1 whose contents are kits at depth 2: the
// user runs `ln -s ~/dev/sandbox-kits ~/.boxd/kits/sandbox-kits` and
// expects each kit inside to be discovered as sandbox-kits/<kit>.
func TestDiscover_FollowsRepoSymlinkAndWalksChildren(t *testing.T) {
	realRepo := t.TempDir()
	seedKit(t, realRepo, "bun")
	seedKit(t, realRepo, "vanilla-sandbox")

	root := t.TempDir()
	linkPath := filepath.Join(root, "sandbox-kits")
	if err := os.Symlink(realRepo, linkPath); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	wantNames := []string{
		filepath.Join("sandbox-kits", "bun"),
		filepath.Join("sandbox-kits", "vanilla-sandbox"),
	}
	if len(got) != len(wantNames) {
		t.Fatalf("got %d kits, want %d: %#v", len(got), len(wantNames), got)
	}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Errorf("kit %d: name = %q, want %q", i, got[i].Name, want)
		}
		// Path must be the through-symlink form, not the resolved target.
		wantPath := filepath.Join(linkPath, filepath.Base(want))
		if got[i].Path != wantPath {
			t.Errorf("kit %d: path = %q, want %q (through-symlink form)", i, got[i].Path, wantPath)
		}
	}
}

func TestDiscover_SkipsHiddenDirectories(t *testing.T) {
	root := t.TempDir()
	// .git often lives next to kits; don't recurse into it.
	seedKit(t, root, filepath.Join(".git", "hidden-kit"))
	seedKit(t, root, "visible-kit")

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].Name != "visible-kit" {
		t.Errorf("expected only the visible kit, got %#v", got)
	}
}

func TestDiscover_PopulatesDisplayNameAndDescriptionFromSpec(t *testing.T) {
	root := t.TempDir()
	spec := `schemaVersion: "1"
kind: mixin
name: bun
displayName: Bun
description: Install the Bun runtime for the agent user
`
	kit := seedKitWithSpec(t, root, "bun", spec)

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []KitInfo{{
		Path:        kit,
		Name:        "bun",
		DisplayName: "Bun",
		Description: "Install the Bun runtime for the agent user",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v;\nwant %#v", got, want)
	}
}

func TestDiscover_MissingSpecFieldsLeaveZeroValues(t *testing.T) {
	root := t.TempDir()
	// Spec exists but lacks displayName/description.
	kit := seedKitWithSpec(t, root, "minimal", "kind: mixin\nname: minimal\n")

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []KitInfo{{Path: kit, Name: "minimal"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v;\nwant %#v", got, want)
	}
}

func TestDiscover_MalformedSpecDegradesGracefully(t *testing.T) {
	root := t.TempDir()
	// Garbage spec.yaml — the kit should still be discovered, just
	// with empty DisplayName/Description.
	kit := seedKitWithSpec(t, root, "broken", "{this: is: not: valid: yaml")

	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover should not error on malformed yaml: %v", err)
	}
	want := []KitInfo{{Path: kit, Name: "broken"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v;\nwant %#v", got, want)
	}
}

func TestValidate(t *testing.T) {
	tmp := t.TempDir()
	kit := seedKit(t, tmp, "kit-a")
	noSpec := filepath.Join(tmp, "no-spec")
	if err := os.MkdirAll(noSpec, 0o755); err != nil {
		t.Fatal(err)
	}
	regularFile := filepath.Join(tmp, "f.txt")
	if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		path      string
		wantErr   bool
		errSubstr string
	}{
		{name: "valid kit", path: kit},
		{name: "relative path rejected", path: "kit-a", wantErr: true, errSubstr: "absolute"},
		{name: "missing path", path: filepath.Join(tmp, "nope"), wantErr: true},
		{name: "regular file rejected", path: regularFile, wantErr: true, errSubstr: "not a directory"},
		{name: "dir without spec.yaml rejected", path: noSpec, wantErr: true, errSubstr: "spec.yaml"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.path)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if c.errSubstr != "" && !strings.Contains(err.Error(), c.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), c.errSubstr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
