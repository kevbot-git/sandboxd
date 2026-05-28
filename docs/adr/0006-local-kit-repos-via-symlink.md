# Local Kit Repos are symlinked, not cloned

`boxd kit add --local <path>` installs a Local Kit Repo by creating a symlink in `~/.boxd/kits/<basename>` rather than cloning the repo. This keeps live working-tree changes immediately visible — the primary reason to use a local path at all — and avoids maintaining a second copy of the repo on disk.

The `--local` flag is required (rather than inferring local paths from heuristics like a leading `/` or `./`) because relative paths such as `dev/my-kits` are visually indistinguishable from the `org/repo` remote syntax.

## Considered Options

**Clone with `file://` URL** — would keep the install consistent with remote repos (boxd-owned directory, safe `git reset --hard`). Rejected because it forces a commit-then-update cycle to see local changes, which defeats the development workflow that local paths are meant to support.

**Heuristic path detection** — infer local vs remote from path shape (leading `/`, `./`, `~/`). Rejected because bare relative paths like `dev/my-kits` are ambiguous.

## Consequences

- `boxd kit update` on a Local Kit Repo re-reads HEAD and updates the lockfile only — no git write operations. The user owns the working tree; boxd does not manage it.
- `boxd kit list` shows branch name and a `*` suffix when the working tree is dirty (e.g. `[main*]`).
- Sparse-checkout and kit-level addressing (`boxd kit add --local ~/dev/my-kits/node`) are not supported for Local Kit Repos — the whole repo is always available via the symlink.
