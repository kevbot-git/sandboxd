# Sandbox'd

A light wrapper over Docker's `sbx`, with quality-of-life improvements and a kits manager.

`boxd` (short for *Sandbox'd*, and easier to type) is a single CLI that installs sandbox **Kits** from git repos and runs `sbx` sandboxes with them. It's an installer + orchestrator on top of `sbx` — Kits themselves are plain `sbx` mixins and stay usable without `boxd`.

The project is called `sandboxd` (the repo, the Homebrew tap, the Go module); the binary it installs is `boxd` — same way `brew install aws-cli` gives you the `aws` command.

## Install

Requires [`sbx`](https://github.com/docker/sbx-releases) on your `PATH`.

```bash
go install github.com/kevbot-git/sandboxd/cmd/boxd@latest
```

A Homebrew tap (`kevbot-git/homebrew-sandboxd`) is planned but not yet wired up. Once it ships, `brew install kevbot-git/sandboxd/sandboxd` will install the `boxd` binary.

## Quick start

```bash
# Add a Kit Repo (one git repo can contain many Kits)
boxd kit add github.com/kevbot-git/sandbox-kits

# In your project directory: pick kits and create a sandbox for this cwd
boxd init

# Re-enter the sandbox later
boxd            # resumes the sandbox for $PWD, or falls through to `init`
boxd shell      # open a bash shell inside it
```

## Commands

- `boxd init` — create a sandbox for the current directory, selecting Kits via flag, saved selection, or TUI.
- `boxd shell` — open a bash shell in the current directory's sandbox.
- `boxd kit add|list|update|remove|install` — manage installed Kit Repos.

Run `boxd <command> --help` for full flags. Anything after `--` on `boxd init` is forwarded verbatim to `sbx run`.

## Concepts

- **Kit** — a self-contained `sbx` mixin (`spec.yaml`, `kind: mixin`).
- **Kit Repo** — a git repo containing one or more Kits, each in its own subdirectory. The unit of distribution.
- **Local Kit Repo** — a Kit Repo installed from a local path via `boxd kit add --local <path>`; symlinked rather than cloned so working-tree edits are live.

See [`CONTEXT.md`](./CONTEXT.md) for the full glossary and [`docs/adr/`](./docs/adr/) for architectural decisions.

## Disclaimer

`boxd` / *Sandbox'd* is an independent project and is not affiliated with, endorsed by, or sponsored by Docker, Inc. "Docker" and "sbx" are trademarks of their respective owners.

## Licence

Licensed under the [Apache License, Version 2.0](./LICENSE).
