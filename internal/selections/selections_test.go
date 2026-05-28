package selections

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingReturnsEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selections.json")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error for missing file: %v", err)
	}
	if got, ok := s.Get("/anything"); ok {
		t.Errorf("expected empty store, got %v for /anything", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boxd", "selections.json")

	s1, err := Load(path)
	if err != nil {
		t.Fatalf("Load (first): %v", err)
	}
	s1.Set("/Users/dev/foo", []string{"/kits/bun", "/kits/vanilla"})
	s1.Set("/Users/dev/bar", []string{"/kits/legacy"})
	if err := s1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatalf("Load (second): %v", err)
	}

	cases := []struct {
		cwd  string
		want []string
	}{
		{"/Users/dev/foo", []string{"/kits/bun", "/kits/vanilla"}},
		{"/Users/dev/bar", []string{"/kits/legacy"}},
	}
	for _, c := range cases {
		got, ok := s2.Get(c.cwd)
		if !ok {
			t.Errorf("Get(%q) returned ok=false", c.cwd)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Get(%q) = %v; want %v", c.cwd, got, c.want)
		}
	}
}

func TestSetOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selections.json")
	s, _ := Load(path)
	s.Set("/foo", []string{"a"})
	s.Set("/foo", []string{"b", "c"})
	got, _ := s.Get("/foo")
	want := []string{"b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after overwrite: got %v; want %v", got, want)
	}
}

func TestLoadCorruptedReturnsEmptyAndDoesNotError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selections.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("seed corrupted file: %v", err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load on corrupted file returned error: %v", err)
	}
	if _, ok := s.Get("/anything"); ok {
		t.Error("expected empty store after corrupted load")
	}
	// Saving the empty store should overwrite the corrupted file cleanly.
	if err := s.Save(); err != nil {
		t.Fatalf("Save on recovered store: %v", err)
	}
	s2, _ := Load(path)
	if _, ok := s2.Get("/anything"); ok {
		t.Error("expected empty store after re-load")
	}
}

func TestSaveIsAtomicAcrossInterruptedWrites(t *testing.T) {
	// We can't truly simulate an interrupted Save, but we can assert
	// that a partial .tmp file left over from a prior crash doesn't
	// affect Load — Load only reads the canonical path.
	dir := t.TempDir()
	path := filepath.Join(dir, "selections.json")
	if err := os.WriteFile(path+".tmp", []byte("garbage"), 0o644); err != nil {
		t.Fatalf("seed leftover tmp: %v", err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := s.Get("/anything"); ok {
		t.Error("Load read from .tmp leftover; should only read canonical path")
	}
}

func TestSaveCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "selections.json")
	s, _ := Load(path)
	s.Set("/x", []string{"/y"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s after Save: %v", path, err)
	}
}

func TestSavedFormatIsStable(t *testing.T) {
	// Locks in the on-disk schema so step 2 doesn't accidentally diverge.
	path := filepath.Join(t.TempDir(), "selections.json")
	s, _ := Load(path)
	s.Set("/Users/dev/foo", []string{"/kits/bun"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := `{
  "version": 1,
  "selections": {
    "/Users/dev/foo": [
      "/kits/bun"
    ]
  }
}
`
	if string(got) != want {
		t.Errorf("on-disk format drift:\n got: %s\nwant: %s", got, want)
	}
}
