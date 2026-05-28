// Package kits handles kit discovery and validation. A kit is any
// directory containing a spec.yaml file (see CONTEXT.md for the
// Kit/Kit-Repo glossary). In step 2, the installer subcommands will
// populate ~/.boxd/kits/ from git repos; in step 1, the user is
// expected to manually drop or symlink kits there.
package kits

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// KitInfo is one kit found by Discover.
type KitInfo struct {
	// Path is the absolute path to the kit's root directory (the dir
	// containing spec.yaml). Symlinks are not resolved — Path is the
	// route by which Discover reached the kit, so the user-facing
	// label stays meaningful. sbx is happy to consume either form.
	Path string
	// Name is the path relative to the discovery root
	// (e.g. "sandbox-kits/bun"). Used as a fallback label when
	// spec.yaml has no displayName.
	Name string
	// DisplayName and Description are pulled from the kit's spec.yaml
	// (top-level `displayName:` and `description:` fields). Either may
	// be empty if absent from spec.yaml or if parsing failed — the TUI
	// degrades gracefully to using Name.
	DisplayName string
	Description string
}

// specFile is the subset of spec.yaml boxd reads at discovery time —
// just the human-facing labels. The rest of the file (schemaVersion,
// kind, commands, network, …) is sbx's concern.
type specFile struct {
	DisplayName string `yaml:"displayName"`
	Description string `yaml:"description"`
}

// readSpec returns the parsed displayName/description from a spec.yaml
// file. Any error (missing file, unreadable, malformed yaml) yields a
// zero-value Spec; this is best-effort metadata, not load-bearing.
func readSpec(path string) specFile {
	raw, err := os.ReadFile(path)
	if err != nil {
		return specFile{}
	}
	var s specFile
	_ = yaml.Unmarshal(raw, &s)
	return s
}

// DefaultRoot returns ~/.boxd/kits/ — the directory boxd scans for
// installed kits.
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".boxd", "kits"), nil
}

const maxDiscoveryDepth = 6

// Discover walks root looking for directories that contain a spec.yaml.
// Each such directory is reported as one KitInfo, with Name set to the
// path relative to root. Discovery does not descend into a found kit
// (spec.yaml-bearing dirs are leaves).
//
// Unlike filepath.WalkDir, Discover follows symlinks — at every level —
// so a user can stage kits with either `ln -s real-kit ~/.boxd/kits/<name>`
// (per-kit symlinks at depth 1) or `ln -s real-repo-dir ~/.boxd/kits/<repo>`
// (parent symlink whose contents are walked at depth 2+). Hidden dirs
// (names starting with ".") and depth > 4 are skipped, which keeps walks
// bounded and avoids descending into things like .git/.
//
// Returns (nil, nil) if root doesn't exist yet — the expected first-run
// state before step 2's installer lands.
func Discover(root string) ([]KitInfo, error) {
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	var out []KitInfo
	walkForKits(root, root, 0, &out)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// walkForKits is the symlink-following walker that backs Discover. It
// recurses into directories (resolved through symlinks) up to
// maxDiscoveryDepth, recording any directory that contains spec.yaml
// and not descending into it.
func walkForKits(root, current string, depth int, out *[]KitInfo) {
	if depth > maxDiscoveryDepth {
		return
	}
	specPath := filepath.Join(current, "spec.yaml")
	if _, err := os.Stat(specPath); err == nil {
		rel, _ := filepath.Rel(root, current)
		if rel == "." {
			rel = filepath.Base(root)
		}
		spec := readSpec(specPath)
		*out = append(*out, KitInfo{
			Path:        current,
			Name:        rel,
			DisplayName: spec.DisplayName,
			Description: spec.Description,
		})
		return
	}
	entries, err := os.ReadDir(current)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := filepath.Join(current, e.Name())
		// os.Stat follows symlinks; if the target is a directory we
		// recurse into it. The depth limit protects against cycles.
		childInfo, err := os.Stat(child)
		if err != nil {
			continue
		}
		if !childInfo.IsDir() {
			continue
		}
		walkForKits(root, child, depth+1, out)
	}
}

// Validate checks that path is an absolute path to an existing directory
// containing a spec.yaml file. Used both for --kit flag values and for
// any kit path that's about to be passed to sbx. Symlinks are followed.
func Validate(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("kit path %s must be absolute", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("kit path %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("kit path %s: not a directory", path)
	}
	if _, err := os.Stat(filepath.Join(path, "spec.yaml")); err != nil {
		return fmt.Errorf("kit path %s: missing spec.yaml", path)
	}
	return nil
}
