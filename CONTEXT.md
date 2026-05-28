# boxd

`boxd` is the single CLI that both **installs** and **runs** sandbox kits. It supersedes the original `claude-contained` bash wrapper.

A distribution + orchestration layer for **Kit**s — `sbx`-compatible sandbox mixins — that lets a single user or team share sandbox configurations across machines and GitHub hosts.

## Design principles

- **Kits stay portable.** A **Kit** is a plain `sbx` mixin (`spec.yaml`, schema `kind: mixin`) — nothing in it depends on `boxd`. Anyone with `sbx` installed can use a kit directly without involving `boxd`; `boxd` is purely an installer + orchestrator on top.

## Language

**Kit**:
A self-contained sandbox mixin defined by a `spec.yaml` file (schema kind: `mixin`), consumed by `sbx kit` / `sbx run --kit`.
_Avoid_: Mixin (correct upstream, but `Kit` is the user-facing term), plugin, extension.

**Kit Repo**:
A git repository containing one or more **Kit**s, each in its own subdirectory with a `spec.yaml`. The unit of distribution.
_Avoid_: Bundle, package.

**Local Kit Repo**:
A **Kit Repo** installed from a local path (via `boxd kit add --local <path>`). Linked into `~/.boxd/kits/` via symlink rather than cloned; live changes in the working tree are immediately visible to `boxd`.
_Avoid_: Local kit, local repo (always qualify as "Local Kit Repo").

## Relationships

- A **Kit Repo** contains one or more **Kit**s
- Installing from a **Kit Repo** without naming kits opens an interactive picker; naming kits installs only those
- Installed **Kit Repo**s live under `~/.boxd/kits/<host>/<user>/<repo>/`, mirroring the canonical lockfile key — two repos with the same name from different orgs or hosts coexist without conflict
- **Local Kit Repo**s are installed as a symlink at `~/.boxd/kits/local/<basename>/`; on basename collision the user must supply `--as <alias>` to disambiguate
- Kit selection happens at `boxd init` time only (sandbox creation), via `--kits` flag or a multi-select TUI; selections are remembered per-cwd in `~/.boxd/selections.json`

## Example dialogue

> **Dev:** "Where does the `bun` **Kit** come from?"
> **Maintainer:** "It lives in the `kev/sbx-kits` **Kit Repo** alongside three others — same repo, different subdirectories."
