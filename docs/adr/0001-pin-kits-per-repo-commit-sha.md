# Pin kits per-repo by commit SHA

`boxd`'s lockfile pins each installed kit repo to a single git commit SHA, not per-kit folder hashes (as the Matt Pocock skills CLI does) or per-ref tracking (e.g., "track main" or "track v1.2"). All kits within a repo move together when updated.

## Why

Kits within a repo share one git history — pinning per-repo matches that physical reality, and `git checkout <sha>` is enough to reproduce a locked state with no special tooling. The per-kit-granularity that folder-hash pinning would offer is largely theoretical for small config-shaped kits, and the "team release" affordance of ref tracking can be added later as a non-breaking `--ref` flag.

## Consequences

- A change to one kit forces re-testing siblings in the same repo (they move as a unit).
- The lockfile structure differs from the skills CLI's per-skill-entry shape, which may surprise users coming from that tool.
- Lockfile is keyed by repo (`github.com/org/repo`), with kit names listed as an array under each entry.
