# `~/.boxd/` over `~/.agents/`

Installed kits, the lockfile, and the per-cwd selections file all live under `~/.boxd/`, not under `~/.agents/` (the namespace used by Matt Pocock's skills CLI).

## Why

`boxd` is a sandbox-kit manager, not an agent-skills tool — putting its state under `~/.agents/` would mis-label what it owns and bet on a namespace owned by an unrelated CLI. If `~/.agents/` semantics shift upstream (new manifest files, claimed subdirectories), `boxd` would have a coordination problem it has no upstream authority to resolve. Owning `~/.boxd/` outright means one directory to back up, one directory to nuke for a clean uninstall, and zero coupling to anyone else's release cadence.

## Consequences

- Users with both skills CLI and `boxd` installed have two top-level dotdirs (`~/.agents/`, `~/.boxd/`) for related-but-distinct concerns. The slight loss of "everything agent-related in one place" tidiness is the cost of decoupling.
- The `kits/` and `selections.json` paths under `~/.boxd/` form part of `boxd`'s public contract — referenced in lockfile entries, error messages, and docs — and shouldn't be moved casually after release.
