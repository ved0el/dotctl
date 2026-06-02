// Package engine runs the ordered pipeline that converges a machine to its
// declared configuration: resolve packages → install → link dotfiles → run
// post_install hooks. It is the single home of orchestration; commands stay thin.
package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ved0el/dotctl/internal/console"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/manifest"
	"github.com/ved0el/dotctl/internal/pkg"
)

// Linker applies a profile's symlinks into the home directory. *link.Linker
// satisfies it; tests can substitute a fake with no filesystem side effects.
type Linker interface {
	Apply(profileDir string) error
}

// Deps are the collaborators reconcile orchestrates. Manager and Runner must be
// built with the same dry-ness (both real or both dry) so a dry run never mutates.
type Deps struct {
	Linker  Linker
	Manager pkg.Manager
	Runner  pkg.Runner
	Log     *console.Logger
	// Overlay links the machine-local overlay dir. It is intentionally NOT
	// tool-gated (the overlay is user-authored, machine-specific config, so every
	// file should land even for tools absent from synced profiles). If nil, the
	// overlay falls back to Linker.
	Overlay Linker
}

// Options control a single reconcile run. The repo path here is authoritative
// (it wins over MachineConfig.Repo).
type Options struct {
	Repo     string   // repo root; profiles live under <Repo>/profiles
	Profiles []string // profiles to apply, in precedence order
	Overlay  string   // optional machine-local overlay dir, linked last (wins on conflict)
}

// Run executes the reconcile pipeline. Package-install failures are collected
// (not fatal — the machine should still get its dotfiles), and a hook is skipped
// when its owning package isn't actually installed. A link failure IS fatal (it
// would leave $HOME half-converged). Run returns a non-nil error — joined across
// all collected failures — when anything went wrong, so callers exit non-zero.
func Run(ctx context.Context, opts Options, cfg machine.Config, d Deps) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	profileRoot := filepath.Join(opts.Repo, machine.ProfilesSubdir)

	pkgs, err := machine.ResolvePackages(profileRoot, opts.Profiles, cfg)
	if err != nil {
		return fmt.Errorf("resolve packages: %w", err)
	}

	// Install packages (custom-install scripts + the managed batch). present maps
	// each package name to whether its tool ended up installed, gating hooks below.
	present, failed := InstallSet(ctx, pkgs, d.Manager, d.Runner, d.Log)
	return finish(ctx, opts, profileRoot, d, pkgs, present, failed)
}

// Upgrade brings installed packages to their latest versions, then links and runs
// hooks. It is Run with a different package phase (UpgradeSet instead of
// InstallSet) and the same link+hooks tail — so a `dotctl upgrade` still converges
// dotfiles and re-runs plugin hooks (e.g. `mise install`, `sheldon lock`).
func Upgrade(ctx context.Context, opts Options, cfg machine.Config, d Deps) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	profileRoot := filepath.Join(opts.Repo, machine.ProfilesSubdir)
	pkgs, err := machine.ResolvePackages(profileRoot, opts.Profiles, cfg)
	if err != nil {
		return fmt.Errorf("resolve packages: %w", err)
	}
	present, failed := UpgradeSet(ctx, pkgs, d.Manager, d.Runner, d.Log)
	return finish(ctx, opts, profileRoot, d, pkgs, present, failed)
}

// finish is the shared tail of Run and Upgrade: link each profile, then the
// machine-local overlay (ungated, wins on conflict), then run each present
// package's post_install hook. Collected failures are returned joined.
func finish(ctx context.Context, opts Options, profileRoot string, d Deps, pkgs []manifest.Package, present map[string]bool, failed []error) error {
	// Link dotfiles before hooks so plugin managers (sheldon, tmux/TPM, mise)
	// see their config in place when their post_install hook fires.
	for _, profile := range opts.Profiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		d.Log.Step("linking profile %q", profile)
		if err := d.Linker.Apply(filepath.Join(profileRoot, profile)); err != nil {
			return fmt.Errorf("link profile %q: %w", profile, err)
		}
	}

	// Machine-local overlay links last, so it wins on conflict over synced
	// profiles. It uses the ungated overlay linker (see Deps.Overlay) so a tool's
	// config still lands even when that tool isn't in any synced profile.
	if opts.Overlay != "" {
		if _, err := os.Stat(opts.Overlay); err == nil {
			d.Log.Step("linking local overlay")
			overlay := d.Overlay
			if overlay == nil {
				overlay = d.Linker
			}
			if err := overlay.Apply(opts.Overlay); err != nil {
				return fmt.Errorf("link overlay: %w", err)
			}
		}
	}

	for _, p := range pkgs {
		if err := ctx.Err(); err != nil {
			return err
		}
		// skip: gates the managed channel, not custom hooks. present[p.Name]
		// already reflects whether the package is actually there.
		if p.PostInstall == "" || (!p.Custom() && p.Skipped(d.Manager.Name())) {
			continue
		}
		if !present[p.Name] {
			d.Log.Warn("skipping hook for %q (not installed)", p.Name)
			continue
		}
		d.Log.Step("hook: %s", p.PostInstall)
		if err := d.Runner.Run(ctx, "sh", "-c", localBinPath+p.PostInstall); err != nil {
			d.Log.Warn("hook for %q failed: %v", p.Name, err)
			failed = append(failed, fmt.Errorf("post_install %s: %w", p.Name, err))
		}
	}

	if len(failed) > 0 {
		d.Log.Warn("reconcile finished with %d error(s)", len(failed))
		return errors.Join(failed...)
	}
	d.Log.OK("reconcile complete")
	return nil
}

// InstallSet installs a resolved package set and reports, per package name,
// whether its tool ended up present (so callers can gate post_install hooks).
// Custom-install packages (e.g. sheldon, mise) run their own cross-platform
// command, skipped when the binary is already on PATH; the rest go through the
// platform package manager as one batch. Failures are collected, not fatal — the
// machine should still get its dotfiles. `pkg install` reuses this so a custom
// package is never misrouted to brew/apt/dnf.
//
// Managed presence is trusted on a clean batch and probed (IsInstalled) only
// after a failure — avoiding noise on the happy path and in dry-run, where
// Install is a logged no-op that "succeeds".
func InstallSet(ctx context.Context, pkgs []manifest.Package, mgr pkg.Manager, runner pkg.Runner, log *console.Logger) (map[string]bool, []error) {
	present := map[string]bool{}
	var managed []manifest.Package
	var failed []error

	for _, p := range pkgs {
		if err := ctx.Err(); err != nil {
			return present, append(failed, err)
		}
		if !p.Custom() {
			managed = append(managed, p)
			continue
		}
		if pkg.CommandExists(p.Name) {
			log.Debug("skip %s (already installed)", p.Name)
			present[p.Name] = true
			continue
		}
		log.Step("installing %s (custom)", p.Name)
		if err := runner.Run(ctx, "sh", "-c", localBinPath+p.Install); err != nil {
			log.Warn("install %q failed: %v", p.Name, err)
			failed = append(failed, fmt.Errorf("install %s: %w", p.Name, err))
		} else {
			present[p.Name] = true
		}
	}

	managedFailed := false
	if len(managed) > 0 {
		if !mgr.Available() {
			log.Warn("%s not found on PATH; package installs may fail", mgr.Name())
		}
		log.Step("installing %d package(s) via %s", len(managed), mgr.Name())
		if err := mgr.Install(ctx, managed); err != nil {
			log.Warn("%s install reported errors: %v", mgr.Name(), err)
			failed = append(failed, fmt.Errorf("%s install: %w", mgr.Name(), err))
			managedFailed = true
		}
	}
	for _, p := range managed {
		if !managedFailed {
			present[p.Name] = true
			continue
		}
		ok, _ := mgr.IsInstalled(ctx, p)
		present[p.Name] = ok
	}
	return present, failed
}

// UpgradeSet upgrades installed packages to their latest versions, mirroring
// InstallSet's shape. Managed packages are upgraded as one batch — but only the
// ones actually installed (probed via IsInstalled), so the manager never has to
// special-case an absent package. Custom-install tools (sheldon, mise) self-manage,
// so they're refreshed by re-running their installer. Returns the presence map
// (for hook gating) and collected failures.
func UpgradeSet(ctx context.Context, pkgs []manifest.Package, mgr pkg.Manager, runner pkg.Runner, log *console.Logger) (map[string]bool, []error) {
	present := map[string]bool{}
	var installed []manifest.Package
	var failed []error

	for _, p := range pkgs {
		if err := ctx.Err(); err != nil {
			return present, append(failed, err)
		}
		if p.Custom() {
			log.Step("upgrading %s (custom)", p.Name)
			if err := runner.Run(ctx, "sh", "-c", localBinPath+p.Install); err != nil {
				log.Warn("upgrade %q failed: %v", p.Name, err)
				failed = append(failed, fmt.Errorf("upgrade %s: %w", p.Name, err))
			} else {
				present[p.Name] = true
			}
			continue
		}
		ok, _ := mgr.IsInstalled(ctx, p)
		present[p.Name] = ok
		if ok {
			installed = append(installed, p)
		} else {
			log.Debug("skip %s (not installed; run 'dotctl pkg install' first)", p.Name)
		}
	}

	if len(installed) > 0 {
		if !mgr.Available() {
			log.Warn("%s not found on PATH; upgrades may fail", mgr.Name())
		}
		log.Step("upgrading %d package(s) via %s", len(installed), mgr.Name())
		if err := mgr.Upgrade(ctx, installed); err != nil {
			log.Warn("%s upgrade reported errors: %v", mgr.Name(), err)
			failed = append(failed, fmt.Errorf("%s upgrade: %w", mgr.Name(), err))
		}
	}
	return present, failed
}

// localBinPath puts ~/.local/bin first so freshly self-installed tools (sheldon,
// mise — both land there) are found by the hooks, regardless of parent PATH.
const localBinPath = `export PATH="$HOME/.local/bin:$PATH"; `
