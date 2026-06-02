// Package pkg installs packages through a platform package manager (Homebrew or
// apt in v0.1), behind a small Manager interface and an injectable command Runner.
package pkg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ved0el/dotctl/internal/manifest"
)

// CommandExists reports whether a command is available: on PATH, or in
// ~/.local/bin (where self-installing tools like sheldon and mise land, which
// may not be on the parent process PATH).
func CommandExists(name string) bool {
	if _, err := exec.LookPath(name); err == nil {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, ".local", "bin", name)); err == nil {
			return true
		}
	}
	return false
}

// Manager installs packages for one platform package manager. Install is a
// batch, idempotent operation (the underlying tools no-op on already-installed
// packages). post_install hooks are NOT run here — that is reconcile's job.
type Manager interface {
	Name() string
	Available() bool
	Install(ctx context.Context, pkgs []manifest.Package) error
	// Upgrade brings the given (already-installed) packages to their latest
	// versions. Callers pass only installed packages so the manager need not
	// special-case absent ones.
	Upgrade(ctx context.Context, pkgs []manifest.Package) error
	// IsInstalled reports whether a package is present, resolving the same
	// per-manager name as Install. A non-zero manager exit is treated as
	// "not installed" rather than an error.
	IsInstalled(ctx context.Context, p manifest.Package) (bool, error)
}

// managerCandidates is the detection order: the first whose probe command is on
// PATH wins. Probing the real environment (not GOOS) is what lets one Linux box
// use apt and another use dnf without code changes.
var managerCandidates = []struct {
	probe string
	make  func(Runner) Manager
}{
	{"brew", func(r Runner) Manager { return brewManager{r: r} }},
	{"apt-get", func(r Runner) Manager { return aptManager{r: r} }},
	{"dnf", func(r Runner) Manager { return dnfManager{r: r} }},
}

// Select detects the platform package manager actually present and wires it to
// run commands via r.
func Select(r Runner) (Manager, error) {
	return selectWith(r, func(cmd string) bool { _, err := exec.LookPath(cmd); return err == nil })
}

// selectWith is Select with an injectable command-probe, for testing.
func selectWith(r Runner, has func(string) bool) (Manager, error) {
	for _, c := range managerCandidates {
		if has(c.probe) {
			return c.make(r), nil
		}
	}
	return nil, fmt.Errorf("no supported package manager found (need one of brew, apt, dnf)")
}

// supported drops packages marked to skip the named manager.
func supported(pkgs []manifest.Package, manager string) []manifest.Package {
	out := make([]manifest.Package, 0, len(pkgs))
	for _, p := range pkgs {
		if !p.Skipped(manager) {
			out = append(out, p)
		}
	}
	return out
}

// pkgNames returns the install names, preferring the manager-specific override
// (via pick) and falling back to the logical Name.
func pkgNames(pkgs []manifest.Package, pick func(manifest.Package) string) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		if name := pick(p); name != "" {
			out = append(out, name)
		} else {
			out = append(out, p.Name)
		}
	}
	return out
}
