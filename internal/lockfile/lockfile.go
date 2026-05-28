// Package lockfile persists the installed Kit Repo registry that boxd
// maintains across commands. The file lives at ~/.boxd/boxd.lock.json by
// default; tests pass in an explicit path.
//
// Schema:
//
//	{
//	  "version": 1,
//	  "kits": {
//	    "<host>/<user>/<repo>": {
//	      "sourceUrl":   "https://...",
//	      "commit":      "<full-sha>",
//	      "installDir":  "/absolute/path",
//	      "installedAt": "<RFC3339>",
//	      "updatedAt":   "<RFC3339>",
//	      "kits":        ["name-a", "name-b"]
//	    }
//	  }
//	}
//
// Callers provide canonical keys in the form <host>/<user>/<repo> — this
// package stores them verbatim and does not normalise them.
package lockfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Entry holds the metadata boxd stores for one installed Kit Repo.
type Entry struct {
	SourceURL   string    `json:"sourceUrl"`
	Commit      string    `json:"commit"`
	InstallDir  string    `json:"installDir"`
	InstalledAt time.Time `json:"installedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Kits        []string  `json:"kits"`
	// InstallMode records how the Kit Repo was installed.
	// "full" means the whole repo is checked out (default for entries with
	// no installMode or empty string). "sparse" means only specific kits
	// are checked out via git sparse-checkout.
	InstallMode string `json:"installMode,omitempty"`
}

// Store is one boxd.lock.json file loaded into memory.
type Store struct {
	path string
	data storeData
}

type storeData struct {
	Version int              `json:"version"`
	Kits    map[string]Entry `json:"kits"`
}

// DefaultPath returns ~/.boxd/boxd.lock.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".boxd", "boxd.lock.json"), nil
}

// Load reads the store at path. A missing file yields an empty store
// (not an error). A corrupted file logs a warning to stderr and also
// yields an empty store — callers should never crash on bad state.
func Load(path string) (*Store, error) {
	s := &Store{
		path: path,
		data: storeData{Version: 1, Kits: map[string]Entry{}},
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
	if d.Kits == nil {
		d.Kits = map[string]Entry{}
	}
	if d.Version == 0 {
		d.Version = 1
	}
	s.data = d
	return s, nil
}

// Get returns the entry for the given canonical key, or (zero, false) if
// not present.
func (s *Store) Get(key string) (Entry, bool) {
	e, ok := s.data.Kits[key]
	return e, ok
}

// All returns a copy of every entry in the store, keyed by canonical key.
// The returned map is safe to mutate; changes do not affect the store.
func (s *Store) All() map[string]Entry {
	result := make(map[string]Entry, len(s.data.Kits))
	for k, v := range s.data.Kits {
		result[k] = v
	}
	return result
}

// Set stores an entry for the given canonical key, overwriting any prior
// entry.
func (s *Store) Set(key string, entry Entry) {
	if s.data.Kits == nil {
		s.data.Kits = map[string]Entry{}
	}
	s.data.Kits[key] = entry
}

// Delete removes the entry for the given canonical key. It is a no-op if the
// key is not present.
func (s *Store) Delete(key string) {
	delete(s.data.Kits, key)
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
