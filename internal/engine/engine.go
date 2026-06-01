// Package engine runs the ordered pipeline that converges a machine to its
// declared configuration: resolve packages → install → link dotfiles → run
// post_install hooks. It is the single home of orchestration; commands stay thin.
package engine

import (
	"context"
	"errors"
	"fmt"
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
}

// Options control a single reconcile run. The repo path here is authoritative
// (it wins over MachineConfig.Repo).
type Options struct {
	Repo     string   // repo root; profiles live under <Repo>/profiles
	Profiles []string // profiles to apply, in precedence order
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

	var failed []error
	customOK := map[string]bool{} // custom package name → installed/present

	// Custom-install packages (e.g. sheldon, mise) run their own cross-platform
	// command, guarded so an already-present binary is skipped. The rest go
	// through the platform package manager (brew/apt) as a batch.
	var managed []manifest.Package
	for _, p := range pkgs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !p.Custom() {
			managed = append(managed, p)
			continue
		}
		if pkg.CommandExists(p.Name) {
			d.Log.Debug("skip %s (already installed)", p.Name)
			customOK[p.Name] = true
			continue
		}
		d.Log.Step("installing %s (custom)", p.Name)
		if err := d.Runner.Run(ctx, "sh", "-c", localBinPath+p.Install); err != nil {
			d.Log.Warn("install %q failed: %v", p.Name, err)
			failed = append(failed, fmt.Errorf("install %s: %w", p.Name, err))
		} else {
			customOK[p.Name] = true
		}
	}

	managedFailed := false
	if len(managed) > 0 {
		if !d.Manager.Available() {
			d.Log.Warn("%s not found on PATH; package installs may fail", d.Manager.Name())
		}
		d.Log.Step("installing %d package(s) via %s", len(managed), d.Manager.Name())
		if err := d.Manager.Install(ctx, managed); err != nil {
			d.Log.Warn("%s install reported errors: %v", d.Manager.Name(), err)
			failed = append(failed, fmt.Errorf("%s install: %w", d.Manager.Name(), err))
			managedFailed = true
		}
	}

	// present reports whether p's tool is installed, so its hook can run. Only
	// probes the manager when the batch failed (avoids noise on the happy path
	// and in dry-run, where Install is a no-op that "succeeds").
	present := func(p manifest.Package) bool {
		if p.Custom() {
			return customOK[p.Name]
		}
		if !managedFailed {
			return true
		}
		ok, _ := d.Manager.IsInstalled(ctx, p)
		return ok
	}

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

	for _, p := range pkgs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if p.PostInstall == "" || p.Skipped(d.Manager.Name()) {
			continue
		}
		if !present(p) {
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

// localBinPath puts ~/.local/bin first so freshly self-installed tools (sheldon,
// mise — both land there) are found by the hooks, regardless of parent PATH.
const localBinPath = `export PATH="$HOME/.local/bin:$PATH"; `
