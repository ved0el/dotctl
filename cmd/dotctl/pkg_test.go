package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/manifest"
)

// seedPackages writes a packages.yaml into <repo>/profiles/<profile>.
func seedPackages(t *testing.T, repo, profile, body string) {
	t.Helper()
	dir := filepath.Join(repo, "profiles", profile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "packages.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func profilePkgNames(t *testing.T, repo, profile string) []string {
	t.Helper()
	pkgs, err := manifest.WalkProfile(filepath.Join(repo, "profiles", profile))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = p.Name
	}
	return names
}

func TestPkgMutateRemoveWritesManifest(t *testing.T) {
	_, repo := withSandbox(t)
	seedPackages(t, repo, "base", "packages:\n  - git\n  - ripgrep\n")

	if err := pkgMutate(&cobra.Command{}, &globals{}, "base", []string{"ripgrep"}, false); err != nil {
		t.Fatalf("pkgMutate rm: %v", err)
	}
	if got := profilePkgNames(t, repo, "base"); len(got) != 1 || got[0] != "git" {
		t.Errorf("after rm, got %v, want [git]", got)
	}
}

func TestPkgMutateAddDryRunWritesNothing(t *testing.T) {
	_, repo := withSandbox(t)
	seedPackages(t, repo, "base", "packages:\n  - git\n")

	if err := pkgMutate(&cobra.Command{}, &globals{dryRun: true}, "base", []string{"ripgrep"}, true); err != nil {
		t.Fatalf("pkgMutate add --dry-run: %v", err)
	}
	if got := profilePkgNames(t, repo, "base"); len(got) != 1 || got[0] != "git" {
		t.Errorf("dry-run must not write; got %v, want [git]", got)
	}
}

// TestPkgMutateAddAlreadyDeclaredIsNoop covers the fix where `pkg add` of an
// already-declared package returns early — no rewrite, no install. We assert the
// file is byte-identical: a stray WriteProfile would re-serialize via yaml.Marshal
// and change the formatting, so equality proves the write path was never reached.
func TestPkgMutateAddAlreadyDeclaredIsNoop(t *testing.T) {
	_, repo := withSandbox(t)
	const seed = "packages:\n  - git\n"
	seedPackages(t, repo, "base", seed)
	path := filepath.Join(repo, "profiles", "base", "packages.yaml")

	if err := pkgMutate(&cobra.Command{}, &globals{}, "base", []string{"git"}, true); err != nil {
		t.Fatalf("pkgMutate add already-declared: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != seed {
		t.Errorf("manifest must be untouched (no rewrite); got:\n%s", got)
	}
}

// TestPkgMutateAddDryRunAllDeclared: dry-run of an all-already-declared add must
// not write and must not emit a misleading "would add" plan (it returns early).
func TestPkgMutateAddDryRunAllDeclared(t *testing.T) {
	_, repo := withSandbox(t)
	const seed = "packages:\n  - git\n"
	seedPackages(t, repo, "base", seed)
	path := filepath.Join(repo, "profiles", "base", "packages.yaml")

	if err := pkgMutate(&cobra.Command{}, &globals{dryRun: true}, "base", []string{"git"}, true); err != nil {
		t.Fatalf("dry-run add already-declared: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != seed {
		t.Errorf("dry-run must not write; got:\n%s", got)
	}
}

func TestPkgMutateRejectsTraversalProfile(t *testing.T) {
	_, repo := withSandbox(t)
	seedPackages(t, repo, "base", "packages:\n  - git\n")

	if err := pkgMutate(&cobra.Command{}, &globals{}, filepath.Join("..", "..", "evil"), []string{"x"}, true); err == nil {
		t.Error("expected error for a traversal profile name")
	}
}
