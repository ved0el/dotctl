# Architecture

This is the living architecture reference for `dotctl`.

## Overview

`dotctl` reconciles a machine to a declarative environment described in a git
repository. It follows a three-tier model: a thin shell bootstrap installs a
typed Go binary, and the binary reconciles the machine to the repo.

```
curl -fsSL .../install.sh | sh          ← one-liner (install.ps1 for Windows, Tier 2)
        │  detect OS/arch · ensure git+pkg-mgr · clone repo · fetch binary · exec
        ▼
   dotctl  (Go binary — all logic)       ← downloaded from GitHub Releases
        │  resolve profiles · install packages · link dotfiles · run hooks
        ▼
   the repo (declarative source of truth)
```

This mirrors the model used by Terraform, Ansible, and chezmoi: the repo *describes*
desired state; the tool *reconciles* to it. A stable bootstrap plus a versioned core
means old machines never break when the logic evolves.

## Components

Each `internal/` package has a single responsibility and is independently testable.

| Package | Responsibility | Depends on |
|---|---|---|
| `console` | Structured logging + dry-run rendering; `Logger` passed in, no global state | — |
| `platform` | Static OS/arch + `HomeDir` detection (capability probes live with their consumer) | — |
| `manifest` | Parse `packages.yaml` (yaml.v3, `KnownFields`); walk profile trees | — |
| `machine` | Read/write `machine.yaml`; resolve `base + [profiles]`; package add/exclude math | manifest |
| `link` | Stow-convention symlink engine; consumes an injected filesystem + clock; one backup dir per run | platform |
| `pkg` | `Manager` interface + `brew`/`apt`/`dnf` backends (scoop later), selected by PATH probe; shells out via the `Runner` seam | platform, manifest |
| `engine` | The ordered pipeline `resolve → install → link → hooks` — the home of orchestration; `InstallSet` is the shared install split reused by `pkg install` | machine, link, pkg |

(`engine` is consumed by `init`/`apply`/`sync`; `sync` runs a `git pull --ff-only` before reconciling, and the machine-local overlay links last through an ungated linker.)

### Internal seams & key decisions

- **`engine` owns orchestration.** `init` = write `machine.yaml` + `engine`;
  `apply` = `engine`; `sync` (deferred) = `git pull` + `engine`. Commands stay thin; the
  pipeline lives in one tested place. Order is **install → link → run hooks**, so a
  plugin manager (sheldon, tmux/TPM, mise) sees its config already linked when its
  `post_install` hook fires.
- **Injected seams for testability:**
  - `link` consumes a filesystem interface (real `os` impl; in-memory in tests) and a
    clock (`func() time.Time`), so backup paths are deterministic and tests run in parallel.
  - `pkg` shells out through a `Runner` interface (`ExecRunner` real,
    `FakeRunner` test, `DryRunner` logs-only) — making `--dry-run` trivial and command
    construction unit-testable.
- **`pkg.Manager` contract:** `Name()`, `Available() bool`,
  `Install(ctx, []Package) error` (batch, idempotent), `IsInstalled(ctx, Package)`.
  `post_install` hooks run in `engine`, never inside a backend.
- **Config parsing:** `gopkg.in/yaml.v3` with `KnownFields(true)` so typos in user YAML fail loudly.
- **Cancellation:** a root `context.Context` (from `signal.NotifyContext`) threads through
  installs/links for clean Ctrl-C.
- **Version:** `var version` in package `main`, stamped via `-ldflags -X main.version`.
  Targeting the `main` package (not a module-qualified path) keeps the build and
  release config free of the module path, so renaming the repo needs no change there.
- **Profile conflict rule:** when two selected profiles provide the same target, the **later
  profile in `machine.yaml` order wins** (deterministic, documented).

## CLI surface

All commands below are implemented (through v0.3). Mutating commands support
`--dry-run` / `-n` and `--verbose` / `-v`. They reuse the same seams — `link`
forward (apply) and reversed (`add`), `link.Status` for `status`/`doctor`, the
`Runner` seam for `sync`/`save` — so the surface confirms the design.

| Command | Does |
|---|---|
| `dotctl init` | Full setup: resolve → install → link → hooks. Idempotent. Called by `install.sh`. |
| `dotctl apply` | Re-converge this machine (no prompts). |
| `dotctl status` (`st`) | Drift report (links + packages); bare `dotctl` runs it; non-zero exit on drift. |
| `dotctl add <path>…` | Adopt a file/dir into a profile (the link engine, reversed). |
| `dotctl edit <name>` | Resolve a logical name to its repo source (via `link.Targets`) and open it in `$EDITOR`. |
| `dotctl sync` | `git pull --ff-only` then reconcile. |
| `dotctl save -m "…"` | `git add -A && commit && push` (clean-tree aware). |
| `dotctl upgrade` | Upgrade installed packages (managed + custom), then re-link + hooks; no `git pull`. |
| `dotctl doctor` | Health checks: PATH, `~/.local/bin`, package manager, broken links, repo. |
| `dotctl profile ls\|add\|rm` | Manage profile selection in machine.yaml. |
| `dotctl pkg install\|add\|rm` | Install configured packages; add/rm mutate a profile manifest. |
| `dotctl link` / `unlink` | Symlinks only. |
| `dotctl new` | Scaffold a fresh dotfiles repo. |
| `dotctl completion <shell>` | Shell completion (cobra). |
| `dotctl version` | Print build version. |

### Future (v1.0 and beyond)

Templated file content (per-machine/OS values) · secret management (age/gpg) ·
`status` content diff · machine classes/tags · `dotctl uninstall`/teardown ·
frozen/versioned config schema · self-update · Windows Tier 2 (install.ps1, scoop).

## Symlink convention (modified Stow)

Dotfiles are stored **without leading dots** for clean browsing. Resolution:

| Repo path | Linked to | Notes |
|---|---|---|
| `<profile>/zshrc` | `~/.zshrc` | top-level entry → dot-prefixed, linked as a unit |
| `<profile>/gitconfig` | `~/.gitconfig` | file |
| `<profile>/config/sheldon/plugins.toml` | `~/.config/sheldon/plugins.toml` | under `config/`, linked **leaf-by-leaf** |
| `<profile>/config/mise/conf.d/tools.toml` | `~/.config/mise/conf.d/tools.toml` | intermediate dirs created real, not symlinked |

- **Top-level** entries link as a whole unit. **Under `config/`** the tree is linked
  leaf-by-leaf, with intermediate directories created as real dirs — so multiple
  profiles can contribute files into the same `~/.config/<x>/` directory (e.g.
  `tools` and `develop` each drop a file into `~/.config/mise/conf.d/`) without clobbering.
- **Idempotent:** a correct existing symlink is skipped; a real file in the way is
  moved to `~/.dotfiles-backup/<timestamp>/<relpath>` (path-preserving) before linking — never clobbered.
- Each profile has its own tree; a machine's selected profiles stack into `$HOME`.

## Configuration model

### Global (synced) — `profiles/`

Composable profile trees, shared across machines via git. A profile contributes
**packages** (`packages.yaml`) and/or **dotfiles** (its file tree).

**Two install backends:**

- **System package manager** (`brew`/`apt`) — for ubiquitous, OS-native tools. A
  package may set a per-manager name override (`apt: fd-find`).
- **Custom install command** (`install:`) — a cross-platform script, bypassing
  brew/apt. Used for tools that ship their own installer (sheldon, mise). Run with
  a `command -v` idempotency guard; `~/.local/bin` is prepended to PATH.

```yaml
# profiles/base/packages.yaml
packages:
  - git                                    # system (brew/apt)
  - name: sheldon
    install: "curl … crate.sh | bash -s -- --repo rossmacarthur/sheldon --to ~/.local/bin"
    post_install: "sheldon lock"
  - name: mise
    install: "curl https://mise.run | sh"
    post_install: "mise install --yes"      # materializes the conf.d/*.toml below
```

**CLI tools and language runtimes are managed by mise**, not brew/apt — one
cross-platform source, no per-OS gaps. Each profile drops a `conf.d` file that
mise merges:

```toml
# profiles/tools/config/mise/conf.d/tools.toml   → ~/.config/mise/conf.d/tools.toml
[tools]
ripgrep = "latest"
eza = "latest"
# … fd, bat, fzf, zoxide, jq, yq, delta, gh, btop, tree

# profiles/develop/config/mise/conf.d/develop.toml → ~/.config/mise/conf.d/develop.toml
[tools]
node = "lts"
# … python, go, bun
```

Picking a profile links its `conf.d` file; the base profile's `mise install` hook
installs whatever is present. (A `skip: [<manager>]` field also exists for the
rare brew/apt-only package, but the mise approach avoids needing it.)

### Local (machine-only, NOT synced) — `~/.config/dotctl/`

**`machine.yaml`** — profile selection + per-machine package tuning:

```yaml
repo: ~/.dotfiles
profiles: [base, tools, develop]
packages:
  add: [neovim]    # this box also wants neovim
  exclude: [bun]   # ...but not bun
```

Effective package set = `(profile packages ∪ add) − exclude`.

**Local overlay** — `~/.config/dotctl/local/` (same Stow convention), applied last
and winning on conflict, for files unique to this machine.

**`.local` sourcing** — synced rc files source a machine-local sibling if present:
`[ -f ~/.zshrc.local ] && source ~/.zshrc.local`.

### Apply precedence (lowest → highest)

`base` → selected profiles → local overlay → `.local` runtime sourcing

## Bootstrap data flow

1. `install.sh` (POSIX `sh`, `set -eu`): detect OS/arch; ensure `git` + a package
   manager (Homebrew on macOS).
2. Clone repo → `~/.dotfiles` (idempotent: pull if already present).
3. Download the matching `dotctl` binary from GitHub Releases; verify checksum.
   Fallback: `go build` if Go is present.
4. `exec dotctl init` with profiles from `DOTCTL_PROFILES` env / flag / interactive
   prompt / default `base` when piped non-interactively.
5. `dotctl`: write `machine.yaml` → install packages per backend (continue-on-error,
   summarized) → link dotfiles (back up conflicts) → run `post_install` hooks → print report.

## Error handling

- **Bootstrap:** fail-fast with clear messages; fully re-runnable.
- **Core:** explicit errors per phase; never clobber real files (timestamped backup);
  package failures are collected and reported rather than aborting the run;
  `--dry-run` on all mutating commands; `doctor` surfaces problems proactively.

## Platform tiers

| Tier | Platforms | Package manager | Bootstrap |
|---|---|---|---|
| **Tier 1** | macOS arm64/amd64, Linux amd64/arm64 | Homebrew, apt, dnf | `curl \| sh` |
| **Tier 2** | Windows amd64 (best-effort) | Scoop | `irm \| iex` (`install.ps1`) |

The Go binary cross-compiles to Windows for free; Tier 2 cost is isolated to the
bootstrap shim, symlink semantics, and the Scoop backend — never the Unix path.

## Build, test & release

- **Test:** Go table-driven unit tests (80%+); `link` against a temp `$HOME`;
  `pkg` backends mocked via the `Manager` interface, real installs behind an
  `integration` build tag; `shellcheck` + a containerized end-to-end smoke test for
  `install.sh`. CI matrix: `macos-latest` + `ubuntu-latest`.
- **Release:** `goreleaser` on git tag → GitHub Releases. **v0.1:**
  `darwin/linux × arm64/amd64` + `checksums.txt`; Windows builds are added with Tier 2.
  `install.sh` pins a version (overridable to `latest`) and verifies the checksum before exec.

## Deferred / future

Shipped since v0.1: the full daily-driver command set (`status`, `add`, `sync`,
`save`, `doctor`, `profile`, `pkg add/rm`, `new`, `completion`), the `dnf` backend,
probe-based manager selection, and the machine-local overlay. Still ahead:

- **Backends / platform:** `scoop` + Windows Tier 2 (`install.ps1`, Windows symlink
  semantics — Developer Mode / junction fallback).
- See *Future (v1.0 and beyond)* above for the remaining command set (templating,
  secrets, `status` content-diff, `uninstall`, self-update).
- Optional private-repo sync for local config + secrets (`local_repo` in `machine.yaml`).
- Auto-detect profiles from OS/hostname/env as a bootstrap suggestion.
