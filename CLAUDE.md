# CLAUDE.md

Guidance for Claude Code (and other agents) working in this repository.

## What this project is

`dotctl` is a profile-based dotfiles & environment manager: a typed **Go** binary fronted
by a thin **shell** bootstrap. One command sets up a fresh machine (packages, shell,
dotfiles) and re-converges existing machines idempotently.

Full design: [`docs/architecture.md`](docs/architecture.md).

## Repository layout

```
cmd/dotctl/      CLI entry (cobra)
internal/        logic — one package per responsibility:
  console/       leveled output & dry-run rendering
  platform/      OS/arch + home detection
  manifest/      parse packages.yaml, walk profile trees
  machine/       machine.yaml + profile/package resolution
  link/          Stow-convention symlink engine
  pkg/           package-manager backends (brew, apt)
  engine/        orchestration pipeline (install → link → hooks)
profiles/          dotfiles profiles (base/tools/develop)
install.sh       unix bootstrap   ·   install.ps1   windows bootstrap
docs/            architecture & specs
```

## Common commands

```sh
go build -o dotctl ./cmd/dotctl     # build
go test ./...                       # unit tests
go test -cover ./...                # coverage (target 80%+)
go test -tags=integration ./...     # real package-manager calls
golangci-lint run                   # Go lint
shellcheck install.sh               # bootstrap lint
./dotctl init --dry-run -v     # exercise safely
```

## Architecture invariants — do not break these

- **Idempotent.** Every command can run repeatedly and converges to the same state.
- **Never clobber.** A real file in a link's path is moved to
  `~/.dotfiles-backup/<timestamp>/` before linking — never overwritten.
- **Dry-run everywhere.** Every mutating command must support `--dry-run`.
- **Bootstrap stays thin & portable.** `install.sh` targets POSIX `sh`, passes
  `shellcheck`, and only detects platform + fetches/execs the binary. Real logic
  lives in Go, not the shell.
- **Package managers are pluggable.** New managers implement the `pkg.Manager`
  interface; never special-case a manager outside its backend. `pkg.Select` picks
  the manager by probing PATH (brew/apt/dnf), not by GOOS.
- **Hooks are trusted code.** `install:` and `post_install:` run arbitrary shell
  from the synced repo. Treat `profiles/` as trusted; review changes pulled via
  `sync` as you would any code you execute.
- **Global vs local separation.** `profiles/` is synced via git; anything under
  `~/.config/dotctl/` is machine-local and must never be written into the repo.

## Conventions

- **Go:** `gofmt`/`goimports`; small focused files (≈200–400 lines, 800 max);
  explicit error handling with `%w` wrapping; prefer pure functions and value
  returns over mutation of shared state.
- **Config:** YAML throughout (`machine.yaml`, `packages.yaml`).
- **Symlink rule:** repo files have no leading dot. Top-level `name` → `~/.name`
  (linked as a unit); files under `config/` are linked **leaf-by-leaf** to
  `~/.config/...` (intermediate dirs created real) so multiple profiles can share a
  `~/.config` subdir (e.g. both `tools` and `develop` write `~/.config/mise/conf.d/`).
- **Tool sourcing:** git/tmux via brew/apt; sheldon/mise via cross-platform `install:`
  scripts; all other CLI tools + languages via **mise** (`conf.d/*.toml` per profile).

## Documentation — keep it in sync (required)

Docs are part of the change, not an afterthought. **Every change that alters
observable behavior must update the docs in the same commit.** Before marking work
complete, confirm:

- **`README.md`** — if commands, flags, install steps, profiles, or supported
  platforms changed.
- **`docs/architecture.md`** — if packages, interfaces/seams, the pipeline order,
  config formats, the symlink convention, or platform tiers changed. This is the
  living design reference; it must match the code.
- **`CLAUDE.md`** — if conventions, invariants, commands, or layout changed.
- **`CONTRIBUTING.md`** — if the build/test/lint workflow or commit rules changed.
- **`profiles/*` comments** — if a profile's purpose or package set changed.
- **`CHANGELOG.md`** — add an entry under `[Unreleased]` for every user-visible change.

A PR that changes behavior without a matching doc update is incomplete. When in
doubt, grep the docs for the symbol/command you changed and reconcile every hit.

## Commits & PRs

- Follow **Conventional Commits** (commitlint-enforced):
  `type(scope): subject`. Types: `feat fix refactor docs test chore perf ci build style revert`.
- Imperative subject, no trailing period, ≤72 chars.
- Keep changes focused; add/update tests in the same change.

## Testing expectations

- Table-driven unit tests; 80%+ coverage.
- `link` is tested against a **temporary `$HOME`**, never the real one.
- `pkg` backends use the `Manager` interface with mocks; real installs behind the
  `integration` build tag.

## Gotchas

- Windows is **Tier 2 / best-effort**: keep its concerns (symlink semantics, Scoop,
  `install.ps1`) isolated — they must not complicate the Unix path.
- On a bare machine only `sh` + `curl` are guaranteed; do not assume `bash`,
  `python`, or `go` exist during bootstrap.
