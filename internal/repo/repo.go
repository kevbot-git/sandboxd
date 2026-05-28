// Package repo parses Kit Repo addresses into their canonical components.
//
// Three input forms are accepted:
//
//   - Short:        <user>/<repo>[/<kit>]
//     Host defaults to github.com when the first segment has no ".".
//
//   - Host-prefix:  <host>/<user>/<repo>[/<kit>]
//     A first segment that contains "." is treated as a host name.
//
//   - Full URL:     https://<host>/<user>/<repo>[.git][/<kit>]
//     The scheme must be "https://" (no bare "http://"). A trailing
//     ".git" is stripped before further parsing.
//
// In all forms the optional trailing /<kit> segment records which single
// kit the user targeted. The canonical key is always <host>/<user>/<repo>
// and the source URL is always https://<host>/<user>/<repo>.git.
package repo

import (
	"fmt"
	"strings"
)

// Address is the parsed, normalised form of a Kit Repo address.
type Address struct {
	// Host is the git hosting domain (e.g. "github.com").
	Host string
	// User is the organisation or user name on the host.
	User string
	// Repo is the repository name.
	Repo string
	// Kit is the optional single-kit suffix the user requested.
	// Empty if no suffix was supplied.
	Kit string
	// SourceURL is the canonical clone URL: https://<host>/<user>/<repo>.git
	SourceURL string
	// Key is the canonical lockfile key: <host>/<user>/<repo>
	Key string
}

// ParseAddress parses s into an Address. It returns an error for any
// input that does not match one of the three recognised forms, or that
// would produce an empty host, user, or repo component.
func ParseAddress(s string) (Address, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Address{}, fmt.Errorf("address is empty")
	}

	var host, user, repo, kit string

	if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
		// Full URL form.
		if strings.HasPrefix(s, "http://") {
			return Address{}, fmt.Errorf("only https:// URLs are supported (got %q)", s)
		}
		// Strip scheme.
		rest := strings.TrimPrefix(s, "https://")
		// Strip trailing .git before splitting — but only from the repo
		// segment, not from a /kit suffix if one is present.
		// We strip .git only if there are exactly 3 path segments (host/user/repo)
		// or 4 (host/user/repo/kit) and the third ends with .git.
		parts := strings.Split(rest, "/")
		if len(parts) < 3 {
			return Address{}, fmt.Errorf("URL %q must have at least host/user/repo path segments", s)
		}
		host = parts[0]
		user = parts[1]
		repo = strings.TrimSuffix(parts[2], ".git")
		if len(parts) == 4 {
			kit = parts[3]
		} else if len(parts) > 4 {
			return Address{}, fmt.Errorf("URL %q has too many path segments (expected host/user/repo[/kit])", s)
		}
	} else {
		// Non-URL form: split by "/" and decide based on whether the first
		// segment contains a "." (host-prefix) or not (short form).
		parts := strings.Split(s, "/")
		if strings.Contains(parts[0], ".") {
			// Host-prefix form: host/user/repo[/kit]
			if len(parts) < 3 {
				return Address{}, fmt.Errorf("host-prefix address %q must have at least host/user/repo", s)
			}
			host = parts[0]
			user = parts[1]
			repo = parts[2]
			if len(parts) == 4 {
				kit = parts[3]
			} else if len(parts) > 4 {
				return Address{}, fmt.Errorf("address %q has too many segments (expected host/user/repo[/kit])", s)
			}
		} else {
			// Short form: user/repo[/kit]
			if len(parts) < 2 {
				return Address{}, fmt.Errorf("short address %q must have at least user/repo", s)
			}
			host = "github.com"
			user = parts[0]
			repo = parts[1]
			if len(parts) == 3 {
				kit = parts[2]
			} else if len(parts) > 3 {
				return Address{}, fmt.Errorf("address %q has too many segments (expected user/repo[/kit])", s)
			}
		}
	}

	// Validate the required components are non-empty.
	if host == "" {
		return Address{}, fmt.Errorf("address %q: host is empty", s)
	}
	if user == "" {
		return Address{}, fmt.Errorf("address %q: user is empty", s)
	}
	if repo == "" {
		return Address{}, fmt.Errorf("address %q: repo is empty", s)
	}

	key := host + "/" + user + "/" + repo
	sourceURL := "https://" + host + "/" + user + "/" + repo + ".git"

	return Address{
		Host:      host,
		User:      user,
		Repo:      repo,
		Kit:       kit,
		SourceURL: sourceURL,
		Key:       key,
	}, nil
}
