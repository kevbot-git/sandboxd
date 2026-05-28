# Per-cwd selection memory over per-project manifest

`boxd` does **not** read a per-project manifest file (e.g., `boxd.toml`) to know which kits a sandbox should use. Instead, kit selection happens at `boxd init` time via a `--kits` flag or an interactive TUI, and the chosen set is remembered per-cwd in `~/.boxd/selections.json` (keyed by absolute project-directory path).

## Why

A per-project manifest was considered and deliberately rejected. The implicit per-cwd memory keeps the kit-selection concept out of the project's git history, which fits the current scale (one maintainer plus a small team of colleagues who can be onboarded verbally). The colleague-onboarding cost the manifest would have eliminated — "which kits should I pick?" — is judged smaller than the ongoing cost of every project carrying an extra config file. If the team grows or the kit set diversifies enough that verbal handover becomes painful, an *optional* manifest can be layered in later as a non-breaking addition: when present, it would skip the TUI and serve as the source of truth.

## Consequences

- Selections are machine-local — moving a project directory loses its remembered kits (degrades to an empty TUI, which is the same as first-run; acceptable).
- Reproducible kit setup across machines / colleagues requires verbal or README-level instruction, not a checked-in file.
- A future reader of `boxd`'s code coming from `npm`/`cargo` will reasonably expect a manifest; this ADR exists so they don't "fix" the absence by adding one without considering the trade.
