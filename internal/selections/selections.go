// Package selections persists the per-cwd kit selections that boxd
// remembers between sandbox creations. The file lives at
// ~/.boxd/selections.json by default; tests pass in an explicit path.
//
// Schema (intentionally compatible with the end-state ADR'd format so
// step 2 needs no migration):
//
//	{
//	  "version": 1,
//	  "selections": {
//	    "/absolute/cwd/path": ["/absolute/kit/path", ...]
//	  }
//	}
//
// Callers are responsible for normalising cwd and kit paths to absolute
// form (via filepath.Abs) before calling Get/Set — this package does not
// touch path strings beyond using them as map keys/values.
package selections

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Store is one selections.json file loaded into memory.
type Store struct {
	path string
	data storeData
}

type storeData struct {
	Version    int                 `json:"version"`
	Selections map[string][]string `json:"selections"`
}

// DefaultPath returns ~/.boxd/selections.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".boxd", "selections.json"), nil
}

// Load reads the store at path. A missing file yields an empty store
// (not an error). A corrupted file logs a warning to stderr and also
// yields an empty store — callers should never crash on bad state.
func Load(path string) (*Store, error) {
	s := &Store{
		path: path,
		data: storeData{Version: 1, Selections: map[string][]string{}},
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	var d storeData
	if err := json.Unmarshal(raw, &d); err != nil {
		fmt.Fprintf(os.Stderr, "boxd: warning: %s is corrupted, treating as empty (%v)\n", path, err)
		return s, nil
	}
	if d.Selections == nil {
		d.Selections = map[string][]string{}
	}
	if d.Version == 0 {
		d.Version = 1
	}
	s.data = d
	return s, nil
}

// Get returns the saved kit list for cwd, or (nil, false) if none is saved.
func (s *Store) Get(cwd string) ([]string, bool) {
	v, ok := s.data.Selections[cwd]
	return v, ok
}

// All returns a copy of every entry in the store, keyed by cwd.
// The returned map is safe to mutate; changes do not affect the store.
func (s *Store) All() map[string][]string {
	result := make(map[string][]string, len(s.data.Selections))
	for k, v := range s.data.Selections {
		cp := make([]string, len(v))
		copy(cp, v)
		result[k] = cp
	}
	return result
}

// Set stores kits for cwd, overwriting any prior entry.
func (s *Store) Set(cwd string, kits []string) {
	if s.data.Selections == nil {
		s.data.Selections = map[string][]string{}
	}
	s.data.Selections[cwd] = kits
}

// Save writes the store to disk atomically (write to .tmp + rename).
// Creates the parent directory if it doesn't exist.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	s.data.Version = 1
	out, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
