// Package engine runs the ordered pipeline that converges a machine to its
// declared configuration: resolve packages → install → run post_install hooks →
// link dotfiles. It is the single home of orchestration; commands stay thin.
package engine

import (
	"context"
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

// Run executes the reconcile pipeline. A package-install error is reported but
// does not abort linking (the machine should still get its dotfiles); a hook or
// link failure stops the run.
func Run(ctx context.Context, opts Options, cfg machine.Config, d Deps) error {
	profileRoot := filepath.Join(opts.Repo, machine.ProfilesSubdir)

	pkgs, err := machine.ResolvePackages(profileRoot, opts.Profiles, cfg)
	if err != nil {
		return fmt.Errorf("resolve packages: %w", err)
	}

	// Custom-install packages (e.g. sheldon, mise) run their own cross-platform
	// command, guarded so an already-present binary is skipped. Everything else
	// goes through the platform package manager (brew/apt).
	var managed []manifest.Package
	for _, p := range pkgs {
		if p.Custom() {
			if err := runCustomInstall(ctx, p, d); err != nil {
				return err
			}
			continue
		}
		managed = append(managed, p)
	}

	if len(managed) > 0 {
		if !d.Manager.Available() {
			d.Log.Warn("%s not found on PATH; package installs may fail", d.Manager.Name())
		}
		d.Log.Step("installing %d package(s) via %s", len(managed), d.Manager.Name())
		if err := d.Manager.Install(ctx, managed); err != nil {
			d.Log.Warn("package install reported errors: %v", err)
		}
	}

	// Link dotfiles before running hooks so plugin managers (sheldon, tmux/TPM,
	// mise) see their config in place when their post_install hook fires.
	for _, profile := range opts.Profiles {
		d.Log.Step("linking profile %q", profile)
		if err := d.Linker.Apply(filepath.Join(profileRoot, profile)); err != nil {
			return fmt.Errorf("link profile %q: %w", profile, err)
		}
	}

	if err := runHooks(ctx, pkgs, d); err != nil {
		return err
	}
	d.Log.OK("reconcile complete")
	return nil
}

// localBinPath puts ~/.local/bin first so freshly self-installed tools (sheldon,
// mise — both land there) are found by the idempotency guard and the hooks,
// regardless of the parent process PATH.
const localBinPath = `export PATH="$HOME/.local/bin:$PATH"; `

// runCustomInstall runs a package's custom install command, skipping it when the
// binary is already present (idempotent re-runs). The presence check is done in
// Go — p.Name never enters the shell string — so a package name can't inject.
func runCustomInstall(ctx context.Context, p manifest.Package, d Deps) error {
	if pkg.CommandExists(p.Name) {
		d.Log.Debug("skip %s (already installed)", p.Name)
		return nil
	}
	d.Log.Step("installing %s (custom)", p.Name)
	if err := d.Runner.Run(ctx, "sh", "-c", localBinPath+p.Install); err != nil {
		return fmt.Errorf("install %q: %w", p.Name, err)
	}
	return nil
}

func runHooks(ctx context.Context, pkgs []manifest.Package, d Deps) error {
	for _, p := range pkgs {
		// Skip hooks for packages not managed on this platform (e.g. a mise
		// hook on apt, where mise was never installed).
		if p.PostInstall == "" || p.Skipped(d.Manager.Name()) {
			continue
		}
		d.Log.Step("hook: %s", p.PostInstall)
		if err := d.Runner.Run(ctx, "sh", "-c", localBinPath+p.PostInstall); err != nil {
			return fmt.Errorf("post_install for %q: %w", p.Name, err)
		}
	}
	return nil
}
