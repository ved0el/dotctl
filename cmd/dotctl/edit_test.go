package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// TestRunEditDryRunResolves: a dry-run edit returns nil only when the name
// resolved to a managed file, so this asserts resolution via the link convention.
func TestRunEditDryRunResolves(t *testing.T) {
	_, repo := withSandbox(t)
	if err := os.WriteFile(filepath.Join(repo, "profiles", "base", "zshrc"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runEdit(&cobra.Command{}, &globals{dryRun: true}, "zshrc"); err != nil {
		t.Errorf("expected zshrc to resolve and plan, got %v", err)
	}
}

func TestRunEditResolvesByLeafBasename(t *testing.T) {
	_, repo := withSandbox(t)
	// Declaring the tool makes the config/<tool> child "active" regardless of PATH,
	// so the gated leaf is linked (and resolvable) deterministically.
	seedPackages(t, repo, "base", "packages:\n  - footool\n")
	leaf := filepath.Join(repo, "profiles", "base", "config", "footool", "settings.toml")
	if err := os.MkdirAll(filepath.Dir(leaf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leaf, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runEdit(&cobra.Command{}, &globals{dryRun: true}, "config/footool/settings.toml"); err != nil {
		t.Errorf("full logical path should resolve, got %v", err)
	}
	if err := runEdit(&cobra.Command{}, &globals{dryRun: true}, "settings.toml"); err != nil {
		t.Errorf("basename should resolve, got %v", err)
	}
}

// TestRunEditAmbiguousBasenameErrors: a basename shared by two managed files must
// error (not silently open the wrong one); the full path disambiguates.
func TestRunEditAmbiguousBasenameErrors(t *testing.T) {
	_, repo := withSandbox(t)
	seedPackages(t, repo, "base", "packages:\n  - foo\n  - bar\n")
	for _, tool := range []string{"foo", "bar"} {
		p := filepath.Join(repo, "profiles", "base", "config", tool, "settings.toml")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := runEdit(&cobra.Command{}, &globals{dryRun: true}, "settings.toml"); err == nil {
		t.Error("expected an ambiguity error for a basename matching two files")
	}
	if err := runEdit(&cobra.Command{}, &globals{dryRun: true}, "config/foo/settings.toml"); err != nil {
		t.Errorf("the full logical path should resolve unambiguously, got %v", err)
	}
}

// TestRunEditWhitespaceEditorNoPanic guards the $EDITOR="   " panic.
func TestRunEditWhitespaceEditorNoPanic(t *testing.T) {
	_, repo := withSandbox(t)
	if err := os.WriteFile(filepath.Join(repo, "profiles", "base", "zshrc"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", "   ")
	if err := runEdit(&cobra.Command{}, &globals{dryRun: true}, "zshrc"); err != nil {
		t.Errorf("a whitespace-only $EDITOR should fall back to vi without panicking, got %v", err)
	}
}

func TestRunEditUnknownNameErrors(t *testing.T) {
	withSandbox(t)
	if err := runEdit(&cobra.Command{}, &globals{dryRun: true}, "does-not-exist"); err == nil {
		t.Error("expected an error for an unknown managed file")
	}
}
