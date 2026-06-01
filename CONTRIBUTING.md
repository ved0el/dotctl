# Contributing to dotctl

Thanks for your interest in improving `dotctl`. This guide gets you from a fresh
clone to a reviewed pull request as quickly as possible.

By participating, you agree to keep interactions respectful and constructive.

## TL;DR

```sh
git clone https://github.com/ved0el/dotctl
cd dotctl
make                # list all available targets (same as `make help`)
make build          # or: go build ./cmd/dotctl
make test           # go test ./...
make lint           # golangci-lint + shellcheck
```

Open a branch, make focused changes with tests, ensure `make test lint` is green,
and open a PR with a [Conventional Commit](#commit-messages) title.

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.26+ | build & test the core binary |
| [golangci-lint](https://golangci-lint.run/) | latest | Go linting |
| [shellcheck](https://www.shellcheck.net/) | latest | lint `install.sh` |
| [goreleaser](https://goreleaser.com/) | latest | release builds (maintainers only) |
| Docker | optional | run the bootstrap smoke test locally |

## Project layout

```
cmd/dotctl/      CLI entry point (cobra)
internal/
  console/       leveled output & dry-run rendering
  platform/      OS/arch + home detection
  manifest/      parse packages.yaml, walk profile trees
  machine/       machine.yaml + profile/package resolution
  link/          Stow-convention symlink engine
  pkg/           package-manager backends (brew, apt)
  engine/        orchestration pipeline (install → link → hooks)
profiles/          dotfiles profiles (base/tools/develop)
test/            data-driven integration tests (build tag: integration)
install.sh       POSIX sh bootstrap
docs/            architecture & guides
```

See [docs/architecture.md](docs/architecture.md) for how the pieces fit together.

## Building & running

```sh
go build -o dotctl ./cmd/dotctl
./dotctl --help

# try a change without touching your real machine:
./dotctl init --dry-run --verbose
```

## Testing

We aim for **80%+ coverage** and test behavior, not implementation.

```sh
go test ./...                          # unit tests
go test -cover ./...                   # with coverage
go test -tags=integration ./test/...       # real machine checks (post-bootstrap)
shellcheck install.sh                  # bootstrap linting
```

Guidelines:

- **Table-driven tests** for resolution/parsing logic.
- The `link` engine is tested against a **temporary `$HOME`** — never your real home.
- `pkg` backends are exercised through the `Manager` interface with mocks; real
  installs sit behind the `integration` build tag.
- Add or update tests in the same PR as the behavior change.

## Continuous integration

GitHub Actions runs three workflows:

| Workflow | Trigger | What it does |
|---|---|---|
| `ci.yml` — test | push / PR (macOS + Ubuntu) | `go vet`, `go test -cover ./...` |
| `ci.yml` — lint | push / PR | `golangci-lint`, `shellcheck install.sh` |
| `ci.yml` — smoke | push / PR | build, `--dry-run` the full plan, then real `link`/`unlink` into a throwaway `HOME` (no installs) |
| `e2e.yml` | push / manual / weekly | **real** `bootstrap` on **macOS + Linux** (all profiles) + `go test -tags=integration` — installs packages, runs plugin hooks, verifies the machine matches the repo |

`ci.yml` gives fast multi-OS feedback (build, unit, lint, smoke — no network).
`e2e.yml` is the comprehensive run on both macOS (Homebrew) and Linux (apt): a single
green check means everything builds, installs, links, and tests. Tools that only ship
via brew are marked `skip: [apt]` in the manifests, so the Linux run installs the
apt-available subset and stays green. The integration suite is data-driven, so adding
a package or dotfile is covered without editing tests — scope a local run with
`DOTCTL_PROFILES=base,tools`.

## Code style

- Format with `gofmt` / `goimports` (CI enforces it).
- Keep files small and focused (≈200–400 lines; 800 is a hard ceiling) — extract a
  package when a file does too much.
- Handle errors explicitly; wrap with context (`fmt.Errorf("…: %w", err)`).
- Prefer pure functions and value returns; avoid hidden mutation of shared state.
- **Never clobber** user files — back up before any destructive filesystem action.
- Every mutating command must honor `--dry-run`.

Run the linters before pushing:

```sh
golangci-lint run
```

## Commit messages

This project follows **[Conventional Commits](https://www.conventionalcommits.org/)**
and enforces them with commitlint. Format:

```
<type>(<optional scope>): <subject>

<optional body>

<optional footer>
```

Allowed `type` values: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`, `build`, `style`, `revert`.

Examples:

```
feat(link): back up conflicting files before symlinking
fix(pkg): map fd to fd-find on apt
docs: document the local overlay precedence
```

Rules of thumb: imperative subject, no trailing period, keep the subject ≤ 72 chars.

## Pull requests

1. Branch from `main` (`feat/…`, `fix/…`, `docs/…`).
2. Keep PRs focused; one logical change per PR.
3. Ensure `make test lint` (or the equivalent `go test ./...` + linters) passes.
4. Fill in the PR description: what changed, why, and how you verified it.
5. Link any related issue.

A maintainer will review and may request changes. Once approved and CI is green,
it will be merged.

## Reporting bugs & requesting features

Open an issue with:

- what you expected vs. what happened,
- your OS/arch and `dotctl version`,
- minimal steps to reproduce (a `--dry-run --verbose` log helps a lot).

## Renaming / forking the module

The module path appears in Go imports (required by the language) but **nowhere in
the build or release config**, by design:

- Version is stamped via `-ldflags -X main.version` (package name, not module path),
  so the `Makefile` and `.goreleaser.yml` never reference the repo.
- goreleaser auto-detects the GitHub owner/repo from the git remote (no `release:` block).
- `install.sh` derives everything from a single `REPO_SLUG` (overridable via env).

To rename the module, only two mechanical steps are needed:

```sh
NEW=github.com/you/newname
go mod edit -module "$NEW"
grep -rl github.com/ved0el/dotctl --include='*.go' | xargs sed -i '' "s#github.com/ved0el/dotctl#$NEW#g"
# update REPO_SLUG default in install.sh and the URLs in README/CONTRIBUTING
go build ./... && go test ./...
```

## Security

Please don't open public issues for security problems. Report them privately via
GitHub's [**Report a vulnerability**](https://github.com/ved0el/dotctl/security/advisories/new)
button (the repository's *Security* tab), so they can be handled before disclosure.

---

Happy hacking — and thank you for contributing.
