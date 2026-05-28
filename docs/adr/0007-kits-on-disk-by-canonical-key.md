# Kit Repos live on disk under their canonical key

Installed Kit Repos are placed at `~/.boxd/kits/<host>/<user>/<repo>/`
(matching the lockfile key `<host>/<user>/<repo>`), and Local Kit Repos
at `~/.boxd/kits/local/<basename>/` (key `local/<basename>`). Two repos
with the same name from different orgs or hosts coexist without conflict.

## Why

Previously the install dir was `~/.boxd/kits/<repo>/` — a basename derived
from a key that was already canonical at the lockfile level. The
asymmetry produced a real collision: installing `alice/sbx-kits` after
`kev/sbx-kits` failed with a refuse-to-overwrite error, and the only
workarounds were manual removal or renaming.

Mirroring the lockfile key on disk is the only option that has no first-
install-wins behaviour: the path is fully derivable from the address, in
either order, on either machine.

## Considered Options

**Omit the host segment (`~/.boxd/kits/<user>/<repo>/`).** Shorter paths;
matches the GitHub-URL shape users already think in. Rejected because
the same-user-same-repo case across two hosts (rare but possible) would
need a lazy disambiguation rule, and "one and only one layout" is
easier to reason about than "usually short, sometimes qualified."

**Flatten with a separator (`<host>__<user>__<repo>/`).** Keeps a single
level under `~/.boxd/kits/`, preserving discovery depth. Rejected
because the separator is non-standard, kits become awkward to type in
`--kit` paths, and the value of the flat structure is small —
`maxDiscoveryDepth` is a configurable constant.

## Consequences

- `internal/kits/kits.go` `maxDiscoveryDepth` is raised from 4 to 6 to
  leave headroom for in-repo nesting (`sbx-kits/web/bun/spec.yaml`).
- Kit `Name` (path relative to `~/.boxd/kits/`) becomes
  `github.com/kev/sbx-kits/bun` — longer; the TUI continues to prefer
  `displayName` from `spec.yaml`, so this surfaces only as a fallback.
- `boxd kit add --local` gains an `--as <alias>` flag, used only when a
  basename would collide within `~/.boxd/kits/local/`. This refines
  ADR 0006's "basename collisions are an error" rule rather than
  superseding it — symlink-vs-clone is unchanged.
- No migration is implemented. Existing installs are re-added by hand;
  pre-release single-user state makes this acceptable.
