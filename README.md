<div align="center">

# dotctl

**One command to set up any machine — your packages, shell, and dotfiles, exactly how you like them.**

[![Install](https://img.shields.io/badge/install-curl%20%7C%20sh-2ea44f?logo=gnubash&logoColor=white)](#quick-start)
[![Release](https://img.shields.io/github/v/release/ved0el/dotctl?sort=semver)](https://github.com/ved0el/dotctl/releases/latest)
[![CI](https://github.com/ved0el/dotctl/actions/workflows/ci.yml/badge.svg)](https://github.com/ved0el/dotctl/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ved0el/dotctl)](https://goreportcard.com/report/github.com/ved0el/dotctl)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

### [Install ⟶](#quick-start) · [Docs](docs/architecture.md) · [Changelog](CHANGELOG.md)

</div>

> **Early release (`v0.3.0`).** Core install/link/package flow is stable and
> CI-verified on macOS + Linux. Expect additive changes before `v1.0.0`.

`dotctl` is a profile-based dotfiles and environment manager. You describe your desired
environment once — packages, shell configuration, and dotfiles — in a single git
repository. On a fresh machine, **one line** installs everything. On an existing
machine, the same command re-converges it to the declared state, idempotently and
without ever clobbering your files.

It is a small, typed Go binary fronted by a tiny shell bootstrap, so it works on a
bare server where only a POSIX shell and `curl` are guaranteed to exist.

---

## Features

- **One-line bootstrap** — `curl -fsSL … | sh` takes a bare machine to fully configured.
- **Idempotent** — re-runnable any time; converges to the desired state, never clobbers (conflicts are backed up first).
- **Composable profiles** — a machine is `base` + composable profiles (`tools`, `develop`). No config duplication.
- **Global vs local config** — shared config syncs via git; machine-specific tweaks stay local and unsynced.
- **Per-machine package tuning** — opt a single box into or out of packages without forking a profile.
- **Cross-platform** — macOS (Apple Silicon & Intel) and Linux (amd64 & arm64) first-class; Windows (PowerShell + Scoop) best-effort.
- **Convention over configuration** — a Stow-style directory layout means there's no link map to maintain.

## Quick start

**macOS / Linux** — one line sets up the whole machine:

```sh
curl -fsSL https://tinyurl.com/get-dotctl | sh
```

Pick which profiles this machine gets (first run only — it's remembered after):

```sh
curl -fsSL https://tinyurl.com/get-dotctl | DOTCTL_PROFILES=base,tools,develop sh
```

<details>
<summary>Other ways to install</summary>

```sh
# Pin a version, or use the canonical URL instead of the short link:
curl -fsSL https://raw.githubusercontent.com/ved0el/dotctl/main/install.sh | DOTCTL_VERSION=v0.3.0 sh

# Windows (best-effort, Tier 2):
irm https://raw.githubusercontent.com/ved0el/dotctl/main/install.ps1 | iex
```

</details>

The bootstrap detects your OS/arch, ensures `git` and a package manager are present,
clones the repo to `~/.dotfiles`, downloads the matching `dotctl` binary, and runs
`dotctl init`. Re-running the one-liner later simply updates and re-converges.

## Usage

| Command | What it does |
|---|---|
| `dotctl init` | Full setup: resolve profiles → install packages → link dotfiles → run hooks. Idempotent. |
| `dotctl apply` | Re-converge this machine to its declared config (no prompts). |
| `dotctl status` (`st`) | Show drift: links/packages present, missing, or wrong. Bare `dotctl` runs this; exits non-zero on drift. |
| `dotctl add <path>…` | Adopt existing dotfiles into a profile (move into the repo + symlink back). |
| `dotctl sync` | `git pull` the repo, then re-converge. |
| `dotctl save -m "…"` | Commit and push your dotfiles changes. |
| `dotctl doctor` | Diagnose environment problems (PATH, package manager, broken links). |
| `dotctl profile ls\|add\|rm` | Manage this machine's profiles. |
| `dotctl pkg install\|add\|rm` | Manage packages (add/rm mutate a profile's manifest). |
| `dotctl link` / `dotctl unlink` | Manage symlinks only. |
| `dotctl new` | Scaffold a fresh dotfiles repo. |

Every mutating command supports `--dry-run` / `-n` and `--verbose` / `-v`. Shell
completion: `dotctl completion zsh\|bash\|fish`.

## Configuration

### Profiles (global, synced)

Your repo holds composable profiles. `base` always applies; you add others per machine:

```
profiles/
├── base/          # always: git, tmux (brew/apt) + sheldon, mise (curl) + zshrc/tmux.conf/p10k/sheldon
├── tools/         # CLI tools via mise: config/mise/conf.d/tools.toml
└── develop/       # language runtimes via mise: config/mise/conf.d/develop.toml
```

Each profile is a Stow-style tree. Top-level files are linked as a unit; files under
`config/` are linked **leaf-by-leaf** (so multiple profiles can share a `~/.config`
subdir). Files are stored **without leading dots**:

| Repo path | Linked to |
|---|---|
| `profiles/base/zshrc` | `~/.zshrc` |
| `profiles/base/config/sheldon/plugins.toml` | `~/.config/sheldon/plugins.toml` |
| `profiles/tools/config/mise/conf.d/tools.toml` | `~/.config/mise/conf.d/tools.toml` |

Packages install via the system manager (brew/apt) or a cross-platform `install:`
script; **CLI tools & languages are managed by mise** (one source, no per-OS gaps):

```yaml
# profiles/base/packages.yaml
packages:
  - git                                       # system (brew/apt)
  - name: mise                                # installs itself, then drives the rest
    install: "curl --proto =https --tlsv1.2 -fsSL https://mise.run | sh"
    post_install: "mise install --yes"
```
```toml
# profiles/tools/config/mise/conf.d/tools.toml — picked up by `mise install`
[tools]
ripgrep = "latest"
eza = "latest"
# fd, bat, fzf, zoxide, jq, yq, delta, gh
```

(btop and tree aren't in mise's registry, so they install via brew/apt in
`profiles/tools/packages.yaml`. The develop profile manages node, python, go, bun.)

### Machine config (local, NOT synced)

`~/.config/dotctl/machine.yaml` selects profiles and tunes packages for this box only:

```yaml
repo: ~/.dotfiles
profiles: [base, tools, develop]
packages:
  add: [neovim]    # this machine also wants neovim
  exclude: [bun]   # ...but not bun
```

Machine-specific dotfiles can live in `~/.config/dotctl/local/` (applied last, wins on
conflict), or be appended via the `*.local` convention (`~/.zshrc` sources `~/.zshrc.local`).

## Supported platforms

| Tier | Platforms | Package manager |
|---|---|---|
| **Tier 1** | macOS arm64/amd64, Linux amd64/arm64 | Homebrew, apt, dnf |
| **Tier 2** | Windows amd64 (best-effort) | Scoop |

## How it works

```
curl … | sh        →   dotctl (Go binary)   →   the repo (source of truth)
   bootstrap              all real logic            declarative config
```

A thin, stable shell script bootstraps a versioned binary; the binary reconciles your
machine to the repo. See [docs/architecture.md](docs/architecture.md) for the full design.

## Development

Contributions welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for how to build, test,
and submit changes, and [docs/architecture.md](docs/architecture.md) to get oriented.

## License

[MIT](LICENSE) © 2026 ved0el
