package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ved0el/dotctl/internal/console"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/manifest"
)

type fakeManager struct {
	installed   []manifest.Package
	upgraded    []manifest.Package
	installErr  error // returned by Install (simulates a batch failure)
	installedOK bool  // value IsInstalled reports
}

func (f *fakeManager) Name() string    { return "fake" }
func (f *fakeManager) Available() bool { return true }
func (f *fakeManager) Install(_ context.Context, pkgs []manifest.Package) error {
	f.installed = append(f.installed, pkgs...)
	return f.installErr
}
func (f *fakeManager) Upgrade(_ context.Context, pkgs []manifest.Package) error {
	f.upgraded = append(f.upgraded, pkgs...)
	return f.installErr
}
func (f *fakeManager) IsInstalled(_ context.Context, _ manifest.Package) (bool, error) {
	return f.installedOK, nil
}

type fakeRunner struct{ calls [][]string }

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	return nil
}
func (f *fakeRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

type fakeLinker struct{ applied []string }

func (f *fakeLinker) Apply(profileDir string) error {
	f.applied = append(f.applied, profileDir)
	return nil
}

func TestRun(t *testing.T) {
	repo := t.TempDir()
	base := filepath.Join(repo, "profiles", "base")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "packages.yaml"),
		[]byte("packages:\n  - ripgrep\n  - name: mise\n    post_install: \"echo hi\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fm := &fakeManager{}
	fr := &fakeRunner{}
	fl := &fakeLinker{}
	log := console.New(&bytes.Buffer{}, false)

	err := Run(context.Background(),
		Options{Repo: repo, Profiles: []string{"base"}},
		machine.Config{},
		Deps{Linker: fl, Manager: fm, Runner: fr, Log: log})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(fm.installed) != 2 {
		t.Errorf("expected 2 packages installed, got %d (%+v)", len(fm.installed), fm.installed)
	}
	if len(fr.calls) != 1 || fr.calls[0][0] != "sh" {
		t.Errorf("expected one sh hook call, got %v", fr.calls)
	}
	if len(fl.applied) != 1 || fl.applied[0] != base {
		t.Errorf("expected base profile linked, got %v", fl.applied)
	}
}

func TestRunCustomInstall(t *testing.T) {
	repo := t.TempDir()
	base := filepath.Join(repo, "profiles", "base")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	// A custom-install package with a name that is definitely not on PATH, so
	// the Go-level presence check doesn't skip it; git: managed (brew/apt).
	const fake = "dotctl-absent-tool-xyz"
	if err := os.WriteFile(filepath.Join(base, "packages.yaml"),
		[]byte("packages:\n  - git\n  - name: "+fake+"\n    install: \"curl example | sh\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fm := &fakeManager{}
	fr := &fakeRunner{}
	if err := Run(context.Background(),
		Options{Repo: repo, Profiles: []string{"base"}},
		machine.Config{},
		Deps{Linker: &fakeLinker{}, Manager: fm, Runner: fr, Log: console.New(&bytes.Buffer{}, false)}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// git goes to the manager; the custom package does NOT.
	if len(fm.installed) != 1 || fm.installed[0].Name != "git" {
		t.Errorf("expected only git via manager, got %+v", fm.installed)
	}
	// The custom install ran via the runner (no shell guard — presence checked in Go).
	var sawCustom bool
	for _, c := range fr.calls {
		if strings.Contains(strings.Join(c, " "), "curl example") {
			sawCustom = true
		}
	}
	if !sawCustom {
		t.Errorf("expected custom install to run, got %v", fr.calls)
	}
}

func TestRunCollectsInstallFailureAndSkipsDependentHook(t *testing.T) {
	repo := t.TempDir()
	base := filepath.Join(repo, "profiles", "base")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	// A managed package with a hook; the manager fails to install it.
	if err := os.WriteFile(filepath.Join(base, "packages.yaml"),
		[]byte("packages:\n  - name: tmux\n    post_install: \"echo hi\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fm := &fakeManager{installErr: errors.New("boom"), installedOK: false}
	fr := &fakeRunner{}
	err := Run(context.Background(),
		Options{Repo: repo, Profiles: []string{"base"}},
		machine.Config{},
		Deps{Linker: &fakeLinker{}, Manager: fm, Runner: fr, Log: console.New(&bytes.Buffer{}, false)})

	if err == nil {
		t.Error("expected non-nil error when install fails")
	}
	// The hook must be skipped because tmux didn't install (IsInstalled → false).
	for _, c := range fr.calls {
		if strings.Contains(strings.Join(c, " "), "echo hi") {
			t.Errorf("hook should be skipped for a package that failed to install, got %v", fr.calls)
		}
	}
}

func TestRunLinksOverlayThroughOverlayLinker(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "profiles", "base"), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := t.TempDir()
	prof := &fakeLinker{}
	ovl := &fakeLinker{}
	if err := Run(context.Background(),
		Options{Repo: repo, Profiles: []string{"base"}, Overlay: overlay},
		machine.Config{},
		Deps{Linker: prof, Overlay: ovl, Manager: &fakeManager{}, Runner: &fakeRunner{}, Log: console.New(&bytes.Buffer{}, false)}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The overlay must go through the ungated overlay linker, never the gated one.
	if len(ovl.applied) != 1 || ovl.applied[0] != overlay {
		t.Errorf("overlay linker applied = %v, want [%s]", ovl.applied, overlay)
	}
	for _, a := range prof.applied {
		if a == overlay {
			t.Error("overlay must not be applied through the gated profile linker")
		}
	}
}

func TestRunOverlayNilFallsBackToLinker(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "profiles", "base"), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := t.TempDir()
	fl := &fakeLinker{}
	if err := Run(context.Background(),
		Options{Repo: repo, Profiles: []string{"base"}, Overlay: overlay},
		machine.Config{},
		Deps{Linker: fl, Manager: &fakeManager{}, Runner: &fakeRunner{}, Log: console.New(&bytes.Buffer{}, false)}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sawOverlay bool
	for _, a := range fl.applied {
		if a == overlay {
			sawOverlay = true
		}
	}
	if !sawOverlay {
		t.Errorf("a nil Overlay must fall back to Linker; applied=%v", fl.applied)
	}
}

func TestRunCustomPackageHookIgnoresSkip(t *testing.T) {
	repo := t.TempDir()
	base := filepath.Join(repo, "profiles", "base")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	// A custom-install package (absent → install runs) with skip:[fake] and a hook.
	// skip gates the managed channel, so it must NOT suppress a custom package's hook.
	if err := os.WriteFile(filepath.Join(base, "packages.yaml"),
		[]byte("packages:\n  - name: dotctl-absent-tool-xyz\n    install: \"echo i\"\n    post_install: \"echo hook\"\n    skip: [fake]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{}
	if err := Run(context.Background(),
		Options{Repo: repo, Profiles: []string{"base"}},
		machine.Config{},
		Deps{Linker: &fakeLinker{}, Manager: &fakeManager{}, Runner: fr, Log: console.New(&bytes.Buffer{}, false)}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sawHook bool
	for _, c := range fr.calls {
		if strings.Contains(strings.Join(c, " "), "echo hook") {
			sawHook = true
		}
	}
	if !sawHook {
		t.Error("a custom package's post_install hook must run even with skip set")
	}
}

func TestInstallSetCancelledStopsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pkgs := []manifest.Package{{Name: "a", Install: "echo a"}, {Name: "b", Install: "echo b"}}
	fr := &fakeRunner{}
	_, failed := InstallSet(ctx, pkgs, &fakeManager{}, fr, console.New(&bytes.Buffer{}, false))
	if len(failed) == 0 {
		t.Error("expected a cancellation error from InstallSet")
	}
	if len(fr.calls) != 0 {
		t.Errorf("a cancelled ctx must run no installs, got %v", fr.calls)
	}
}

func TestUpgrade(t *testing.T) {
	repo := t.TempDir()
	base := filepath.Join(repo, "profiles", "base")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "packages.yaml"),
		[]byte("packages:\n  - git\n  - name: dotctl-absent-xyz\n    install: \"echo i\"\n    post_install: \"echo hook\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fm := &fakeManager{installedOK: true}
	fr := &fakeRunner{}
	if err := Upgrade(context.Background(),
		Options{Repo: repo, Profiles: []string{"base"}},
		machine.Config{},
		Deps{Linker: &fakeLinker{}, Manager: fm, Runner: fr, Log: console.New(&bytes.Buffer{}, false)}); err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	// git is managed + installed → upgraded via the manager.
	if len(fm.upgraded) != 1 || fm.upgraded[0].Name != "git" {
		t.Errorf("expected git upgraded, got %+v", fm.upgraded)
	}
	// the custom tool refreshes by re-running its installer, and its hook runs.
	var sawInstall, sawHook bool
	for _, c := range fr.calls {
		j := strings.Join(c, " ")
		if strings.Contains(j, "echo i") {
			sawInstall = true
		}
		if strings.Contains(j, "echo hook") {
			sawHook = true
		}
	}
	if !sawInstall {
		t.Error("custom upgrade should re-run the installer")
	}
	if !sawHook {
		t.Error("post_install hook should run after upgrade")
	}
}

func TestUpgradeSkipsNotInstalledManaged(t *testing.T) {
	repo := t.TempDir()
	base := filepath.Join(repo, "profiles", "base")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "packages.yaml"), []byte("packages:\n  - git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fm := &fakeManager{installedOK: false} // git reports not installed
	if err := Upgrade(context.Background(),
		Options{Repo: repo, Profiles: []string{"base"}},
		machine.Config{},
		Deps{Linker: &fakeLinker{}, Manager: fm, Runner: &fakeRunner{}, Log: console.New(&bytes.Buffer{}, false)}); err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if len(fm.upgraded) != 0 {
		t.Errorf("a not-installed package must not be upgraded, got %+v", fm.upgraded)
	}
}

// TestUpgradeRunsHookEvenIfUpgradeFails pins the intentional inverse of InstallSet:
// a package present before the upgrade keeps its hook even if the batch fails (it's
// still installed, just not upgraded).
func TestUpgradeRunsHookEvenIfUpgradeFails(t *testing.T) {
	repo := t.TempDir()
	base := filepath.Join(repo, "profiles", "base")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "packages.yaml"),
		[]byte("packages:\n  - name: tmux\n    post_install: \"echo hi\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// tmux is installed, but the upgrade batch fails.
	fm := &fakeManager{installErr: errors.New("boom"), installedOK: true}
	fr := &fakeRunner{}
	err := Upgrade(context.Background(),
		Options{Repo: repo, Profiles: []string{"base"}},
		machine.Config{},
		Deps{Linker: &fakeLinker{}, Manager: fm, Runner: fr, Log: console.New(&bytes.Buffer{}, false)})
	if err == nil {
		t.Error("expected a non-nil error when the upgrade batch fails")
	}
	var sawHook bool
	for _, c := range fr.calls {
		if strings.Contains(strings.Join(c, " "), "echo hi") {
			sawHook = true
		}
	}
	if !sawHook {
		t.Error("hook must still run for a present package even when its upgrade fails")
	}
}

func TestRunCancelled(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "profiles", "base"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, Options{Repo: repo, Profiles: []string{"base"}}, machine.Config{},
		Deps{Linker: &fakeLinker{}, Manager: &fakeManager{}, Runner: &fakeRunner{}, Log: console.New(&bytes.Buffer{}, false)})
	if err == nil {
		t.Error("expected context cancellation error")
	}
}
