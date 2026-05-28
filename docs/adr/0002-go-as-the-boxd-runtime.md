# Go as the boxd runtime

`boxd` is implemented in Go and distributed as a static binary, despite the maintainer being more fluent in TypeScript and despite the original framing comparing Bun to Go.

## Why

The decisive factor is the colleague-distribution story: `boxd` needs to be installable in one step by people who may not have any specific runtime pre-installed. Go produces a ~10MB single static binary; a Bun-compiled binary is roughly 60MB, and "install Bun first" reintroduces the friction the rewrite was meant to remove. Go also sits of-a-piece alongside `sbx`, `gh`, and `docker` — same install model (brew or curl-pipe), same kind of artifact.

## Consequences

- The maintainer is learning Go on this project, leaning on coding agents and code review rather than prior fluency.
- TUI work uses `charmbracelet/huh` (multi-select with preserved selection is a one-liner).
- Distribution leans on GitHub releases + brew tap rather than npm.
