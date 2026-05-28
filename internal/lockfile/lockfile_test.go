package lockfile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevbot-git/boxd/internal/debug"
)

// knownTime is used to produce deterministic JSON in schema lock-in tests.
var knownTime = time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

func TestRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "boxd.lock.json")

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load (missing file): %v", err)
	}

	if _, ok := s.Get("github.com/example/sbx-kits"); ok {
		t.Fatal("expected empty store; got entry")
	}

	entry := Entry{
		SourceURL:   "https://github.com/example/sbx-kits.git",
		Commit:      "abc1234567890abcdef1234567890abcdef123456",
		InstallDir:  "/home/user/.boxd/kits/sbx-kits",
		InstalledAt: knownTime,
		UpdatedAt:   knownTime,
		Kits:        []string{"bun", "node"},
	}
	s.Set("github.com/example/sbx-kits", entry)

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatalf("Load (after save): %v", err)
	}
	got, ok := s2.Get("github.com/example/sbx-kits")
	if !ok {
		t.Fatal("entry missing after reload")
	}
	if got.Commit != entry.Commit {
		t.Errorf("commit: got %q, want %q", got.Commit, entry.Commit)
	}
	if got.SourceURL != entry.SourceURL {
		t.Errorf("sourceUrl: got %q, want %q", got.SourceURL, entry.SourceURL)
	}
	if got.InstallDir != entry.InstallDir {
		t.Errorf("installDir: got %q, want %q", got.InstallDir, entry.InstallDir)
	}
	if len(got.Kits) != 2 || got.Kits[0] != "bun" || got.Kits[1] != "node" {
		t.Errorf("kits: got %v, want [bun node]", got.Kits)
	}
}

// TestSavedFormatIsStable locks in the exact on-disk JSON layout so a
// future refactor can't silently change field names or nesting.
func TestSavedFormatIsStable(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "boxd.lock.json")

	s, _ := Load(path)
	s.Set("github.com/example/sbx-kits", Entry{
		SourceURL:   "https://github.com/example/sbx-kits.git",
		Commit:      "abc1234",
		InstallDir:  "/home/user/.boxd/kits/sbx-kits",
		InstalledAt: knownTime,
		UpdatedAt:   knownTime,
		Kits:        []string{"bun"},
	})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	want := `{
  "version": 1,
  "kits": {
    "github.com/example/sbx-kits": {
      "sourceUrl": "https://github.com/example/sbx-kits.git",
      "commit": "abc1234",
      "installDir": "/home/user/.boxd/kits/sbx-kits",
      "installedAt": "2025-01-15T12:00:00Z",
      "updatedAt": "2025-01-15T12:00:00Z",
      "kits": [
        "bun"
      ]
    }
  }
}
`
	if string(raw) != want {
		t.Errorf("on-disk format changed.\ngot:\n%s\nwant:\n%s", raw, want)
	}
}

func TestMissingFileYieldsEmptyStore(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "does-not-exist.json")

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := s.Get("github.com/example/sbx-kits"); ok {
		t.Fatal("expected empty store from missing file")
	}
}

func TestCorruptedFileWarnsAndTreatsAsEmpty(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "boxd.lock.json")

	if err := os.WriteFile(path, []byte("not valid json {{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture stderr to verify the warning is printed.
	var buf bytes.Buffer
	orig := debug.Out
	// Redirect os.Stderr temporarily by swapping debug.Out is not enough —
	// the warning goes through fmt.Fprintf(os.Stderr, ...). We verify the
	// store is empty and that Load does not error instead.
	debug.Out = &buf
	defer func() { debug.Out = orig }()

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load on corrupted file should not error; got: %v", err)
	}
	if _, ok := s.Get("any/key"); ok {
		t.Fatal("expected empty store from corrupted file")
	}
}

func TestCorruptedFileStderrWarning(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "boxd.lock.json")
	if err := os.WriteFile(path, []byte("{bad json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Swap os.Stderr via a pipe to capture the warning.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	Load(path) //nolint:errcheck

	w.Close()
	os.Stderr = origStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if !strings.Contains(buf.String(), "corrupted") {
		t.Errorf("expected 'corrupted' warning on stderr; got: %q", buf.String())
	}
}

func TestDelete_RemovesKey(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "boxd.lock.json")

	s, _ := Load(path)
	s.Set("github.com/example/sbx-kits", Entry{Commit: "abc1234"})
	s.Set("github.com/other/repo", Entry{Commit: "def5678"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, _ := Load(path)
	s2.Delete("github.com/example/sbx-kits")
	if err := s2.Save(); err != nil {
		t.Fatalf("Save after Delete: %v", err)
	}

	s3, _ := Load(path)
	if _, ok := s3.Get("github.com/example/sbx-kits"); ok {
		t.Error("expected key to be deleted, but it's still present")
	}
	if _, ok := s3.Get("github.com/other/repo"); !ok {
		t.Error("other key should not have been deleted")
	}
}

func TestDelete_MissingKeyIsNoOp(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "boxd.lock.json")

	s, _ := Load(path)
	s.Set("github.com/example/sbx-kits", Entry{Commit: "abc1234"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, _ := Load(path)
	// Delete a key that doesn't exist — must not panic or error.
	s2.Delete("github.com/nonexistent/repo")
	if err := s2.Save(); err != nil {
		t.Fatalf("Save after no-op Delete: %v", err)
	}

	// Original key must still be present.
	s3, _ := Load(path)
	if _, ok := s3.Get("github.com/example/sbx-kits"); !ok {
		t.Error("original key should still be present after no-op Delete")
	}
}

// TestInstallModeRoundTrip verifies that InstallMode persists through
// save/load correctly, and that the omitempty behaviour holds for empty values.
func TestInstallModeRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "boxd.lock.json")

	s, _ := Load(path)
	s.Set("github.com/example/sbx-kits", Entry{
		Commit:      "abc1234",
		InstallMode: "sparse",
		Kits:        []string{"bun"},
	})
	s.Set("github.com/other/full-repo", Entry{
		Commit:      "def5678",
		InstallMode: "full",
		Kits:        []string{"a", "b"},
	})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, _ := Load(path)

	sparse, ok := s2.Get("github.com/example/sbx-kits")
	if !ok {
		t.Fatal("sparse entry missing")
	}
	if sparse.InstallMode != "sparse" {
		t.Errorf("InstallMode: got %q, want %q", sparse.InstallMode, "sparse")
	}

	full, ok := s2.Get("github.com/other/full-repo")
	if !ok {
		t.Fatal("full entry missing")
	}
	if full.InstallMode != "full" {
		t.Errorf("InstallMode: got %q, want %q", full.InstallMode, "full")
	}
}

// TestInstallModeOmitempty verifies that an Entry with no InstallMode does not
// emit the "installMode" key in the JSON (omitempty).
func TestInstallModeOmitempty(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "boxd.lock.json")

	s, _ := Load(path)
	s.Set("github.com/example/sbx-kits", Entry{Commit: "abc1234"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "installMode") {
		t.Errorf("expected no 'installMode' key for empty InstallMode; got:\n%s", raw)
	}
}

func TestAtomicWrite(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sub", "boxd.lock.json") // parent dir doesn't exist yet

	s, _ := Load(filepath.Join(tmp, "doesnt-matter.json"))
	s.path = path // redirect to the deep path
	s.Set("github.com/k/r", Entry{Commit: "cafebabe"})

	if err := s.Save(); err != nil {
		t.Fatalf("Save into new parent dir: %v", err)
	}
	// .tmp file must be cleaned up
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error(".tmp file was not removed after Save")
	}
	// The actual file must exist
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file not found: %v", err)
	}
}
