# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.1] — 2026-06-02

### Fixed

- **Release signing now publishes the cosign certificate** (`checksums.txt.pem`).
  The goreleaser `signs` block passed `--output-certificate=${certificate}` but
  never declared the `certificate:` filename, so the certificate was never uploaded
  and the keyless signature could not be verified. With the cert published,
  `install.sh`'s fail-closed cosign check succeeds wherever cosign is installed
  (v0.3.0 shipped without it).

## [0.3.0] — 2026-06-02

The daily-driver release: everything needed to live in dotctl day to day —
checking drift, adopting files, syncing, and managing profiles/packages — on a
hardened, signed install path. Supersedes the unreleased `0.1.2`/`0.2.0`
development versions, which ship together here.

### Added

- **`dotctl status`** (`st`) — read-only drift report; bare `dotctl` runs it and
  exits non-zero on drift (shell-prompt friendly).
- **`dotctl add <path>…`** — adopt existing dotfiles into a profile (reverse-link:
  move into the repo, then symlink back). Directories adopt **leaf-by-leaf**,
  matching the link convention, so the directory itself stays real.
- **`dotctl doctor`** — health checks (PATH, `~/.local/bin`, package manager,
  broken links, repo state).
- **`dotctl sync`** (git pull + reconcile) and **`dotctl save`** (commit + push,
  clean-tree aware) — the write-back loop.
- **`dotctl profile ls/add/rm`** — manage profile selection in machine.yaml
  (validated, persisted atomically; refuses to remove the last profile).
- **`dotctl pkg add/rm`** — mutate a profile's `packages.yaml` (add also installs).
- **`dotctl new`** — scaffold a fresh dotfiles repo.
- **Machine-local overlay**: `~/.config/dotctl/local/` is linked last and wins on
  conflict, through an **ungated** linker, for files unique to one machine.
- **dnf backend** and a `dnf:` per-manager name override.

### Changed

- The package manager is now **detected by probe** (brew → apt → dnf) instead of
  mapping GOOS → manager, so a Linux box uses whatever it actually has.
- The engine **collects package-install failures** (instead of warning then
  reporting success) and **skips a hook when its owning package isn't installed**,
  returning a non-zero error so callers exit non-zero. `pkg install` shares the
  same custom/managed split, so a custom package is never misrouted to brew/apt/dnf.
- `link` reports partial-apply progress and the backup directory on failure, so a
  half-converged `$HOME` is recoverable.
- **machine.yaml** and profile **packages.yaml** are written atomically (temp +
  rename); machine.yaml is validated (`KnownFields`, profiles must exist).
- The reconcile pipeline honors context cancellation (clean Ctrl-C).
- CI pins goreleaser to its `~> v2` line for reproducible releases.

### Security

- `install.sh` hardened: `--proto '=https' --tlsv1.2` fetches, trap-based temp
  cleanup, redirect-based version resolution that hard-fails rather than proceeding
  unverified, a checksum gate that **fails closed** on a missing entry, and a cosign
  check pinned to the exact release workflow + tag that **fails closed** when cosign
  is present but the signature can't be fetched.
- Releases are **cosign-signed** (keyless) over `checksums.txt`; CI/release
  workflows **pin third-party actions to commit SHAs**; Dependabot watches GitHub
  Actions and Go modules and bumps the pins.
- `add` / `pkg` / `profile` validate the `--profile` name so a crafted value
  can't write outside the `profiles/` tree.

## [0.1.1] — 2026-06-02

### Fixed

- `install.sh` now installs the `dotctl` binary to `~/.local/bin` (on `PATH` via
  the base profile's zshrc) instead of `~/.dotfiles/.bin`, fixing
  `dotctl: command not found` after a fresh install.
- `install.sh` reloads the shell (`exec $SHELL -l`) once setup finishes, so the
  newly linked config — and `dotctl` itself — are available immediately, with a
  graceful hint when there's no interactive terminal.

## [0.1.0] — 2026-06-02

First release. A profile-based dotfiles & environment manager: a typed Go CLI
fronted by a POSIX-sh installer that converges a machine to a declarative repo.

### Added

- **`dotctl` CLI** (cobra): `init`, `apply`, `link`/`unlink`, `pkg install`,
  `version`. Every mutating command supports `--dry-run`/`-n` and `--verbose`/`-v`.
- **Composable profiles** — `base`, `tools`, `develop`, `macos` — selected per
  machine via `~/.config/dotctl/machine.yaml` (`profiles` + package `add`/`exclude`).
- **Symlink engine** — top-level files link as a unit (`zshrc` → `~/.zshrc`);
  directories leaf-link (`claude/` → `~/.claude/`, `config/` → `~/.config/`) so
  apps keep writing state into real dirs and multiple profiles share a subdir.
  Idempotent; never clobbers (timestamped, path-preserving backup).
- **Conditional linking** — a tool's config links only when the tool is active
  (declared by a selected profile, or its command is on `PATH`), so macOS-only
  configs like `yabai`/`skhd` are skipped on Linux.
- **Package install** via brew/apt, a cross-platform custom `install:` script
  (sheldon, mise), or **mise** for CLI tools and language runtimes (one source,
  no per-OS gaps). `post_install` hooks run after linking.
- **One-line install** — `install.sh` (POSIX sh) detects platform, fetches the
  released binary (checksum-verified) or builds from source, and runs `init`.
- **Managed configs** — zsh (sheldon + powerlevel10k, performance-tuned with
  byte-compiled caches), tmux, mise, nano, bat, ripgrep, fd, yabai, skhd, claude;
  per-tool shell integration loaded from `~/.config/zsh/conf.d/`.
- **CI/CD** — multi-OS CI (build, race unit tests, golangci-lint, shellcheck,
  smoke) and dual-OS E2E (real bootstrap + integration tests); goreleaser
  publishes `darwin`/`linux` × `arm64`/`amd64` binaries with checksums.

[Unreleased]: https://github.com/ved0el/dotctl/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/ved0el/dotctl/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/ved0el/dotctl/compare/v0.1.1...v0.3.0
[0.1.1]: https://github.com/ved0el/dotctl/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ved0el/dotctl/releases/tag/v0.1.0
