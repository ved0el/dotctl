//go:build integration

// Package integration holds data-driven checks that run against a real,
// bootstrapped machine:
//
//	go test -tags=integration ./test/...
//
// Everything is derived from the repo's profiles — adding a package or a dotfile
// is covered automatically, with no edits to this file.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ved0el/dotctl/internal/console"
	"github.com/ved0el/dotctl/internal/link"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/pkg"
)

// selectProfiles honors DOTCTL_PROFILES (comma-separated) when set, so CI can scope
// the checks to the profiles it actually bootstrapped; otherwise all profiles.
func selectProfiles(t *testing.T, profileRoot string) []string {
	if v := os.Getenv("DOTCTL_PROFILES"); v != "" {
		return strings.Split(v, ",")
	}
	return discoverProfiles(t, profileRoot)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("DOTCTL_REPO"); v != "" {
		return v
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root (go.mod)")
		}
		dir = parent
	}
}

func discoverProfiles(t *testing.T, profileRoot string) []string {
	t.Helper()
	entries, err := os.ReadDir(profileRoot)
	if err != nil {
		t.Fatalf("read profiles: %v", err)
	}
	var profiles []string
	for _, e := range entries {
		if e.IsDir() {
			profiles = append(profiles, e.Name())
		}
	}
	if len(profiles) == 0 {
		t.Fatal("no profiles found")
	}
	return profiles
}

// TestDeclaredPackagesInstalled verifies every package declared across all
// profiles is installed on this machine. The list comes from the manifests, so a
// newly added package is checked automatically.
func TestDeclaredPackagesInstalled(t *testing.T) {
	repo := repoRoot(t)
	profileRoot := filepath.Join(repo, "profiles")
	profiles := selectProfiles(t, profileRoot)

	mgr, err := pkg.Select(runtime.GOOS, pkg.ExecRunner{})
	if err != nil {
		t.Fatalf("select manager: %v", err)
	}
	if !mgr.Available() {
		t.Skipf("%s not available on this host", mgr.Name())
	}

	pkgs, err := machine.ResolvePackages(profileRoot, profiles, machine.Config{})
	if err != nil {
		t.Fatalf("resolve packages: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages declared in any profile")
	}

	ctx := context.Background()
	for _, p := range pkgs {
		t.Run(p.Name, func(t *testing.T) {
			var (
				ok  bool
				err error
			)
			if p.Custom() {
				// Custom-install packages (sheldon, mise) aren't known to brew/apt;
				// verify the binary is reachable instead.
				ok = pkg.CommandExists(p.Name)
			} else {
				ok, err = mgr.IsInstalled(ctx, p)
			}
			if err != nil {
				t.Fatalf("check %q: %v", p.Name, err)
			}
			if !ok {
				t.Errorf("package %q is declared but not installed", p.Name)
			}
		})
	}
}

// TestDeclaredDotfilesLinked verifies every dotfile a profile declares is a
// symlink pointing back into the repo. Targets are derived via link.Targets, so
// a newly added dotfile is checked automatically.
func TestDeclaredDotfilesLinked(t *testing.T) {
	repo := repoRoot(t)
	profileRoot := filepath.Join(repo, "profiles")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	l := link.NewLinker(home, link.OsFS{}, false, console.New(os.Stdout, false))
	// Mirror init's gating so we don't expect configs for absent tools (e.g. yabai
	// on Linux). A config/<tool> is active if declared by a profile or on PATH.
	profilesSel := selectProfiles(t, profileRoot)
	active := machine.ActiveTools(profileRoot, profilesSel, machine.Config{})
	l.Gate = func(name string) bool {
		return active[name] || pkg.CommandExists(name)
	}
	for _, profile := range profilesSel {
		pairs, err := l.Targets(filepath.Join(profileRoot, profile))
		if err != nil {
			t.Fatalf("targets for %q: %v", profile, err)
		}
		for _, p := range pairs {
			t.Run(filepath.Base(p.Dst), func(t *testing.T) {
				target, err := os.Readlink(p.Dst)
				if err != nil {
					t.Fatalf("%s is not a symlink (run `dotctl link`): %v", p.Dst, err)
				}
				if target != p.Src {
					t.Errorf("%s → %s, want %s", p.Dst, target, p.Src)
				}
			})
		}
	}
}
