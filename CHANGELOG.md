# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.2] — 2026-06-02

### Changed

- CI: pin goreleaser to its `~> v2` line in the release workflow instead of
  `version: latest`, silencing the goreleaser-action "will lock to '~> v2'"
  warning and keeping releases reproducible.

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

[Unreleased]: https://github.com/ved0el/dotctl/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/ved0el/dotctl/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/ved0el/dotctl/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ved0el/dotctl/releases/tag/v0.1.0
