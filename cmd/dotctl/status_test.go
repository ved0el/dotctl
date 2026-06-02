package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// TestRunStatusReportsDrift: a declared top-level dotfile that isn't linked is
// drift, so runStatus must return a non-nil error (bare `dotctl` exits non-zero).
func TestRunStatusReportsDrift(t *testing.T) {
	_, repo := withSandbox(t)
	if err := os.WriteFile(filepath.Join(repo, "profiles", "base", "zshrc"), []byte("# zsh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A custom-install package that's definitely absent exercises packageInstalled's
	// custom branch (PATH lookup only, no real package-manager call) and counts as
	// a missing package on top of the missing link.
	if err := os.WriteFile(filepath.Join(repo, "profiles", "base", "packages.yaml"),
		[]byte("packages:\n  - name: dotctl-absent-tool-xyz\n    install: \"true\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runStatus(&cobra.Command{}, &globals{}); err == nil {
		t.Error("expected non-nil error when a declared link is missing")
	}
}

// TestRunStatusInSync: when every declared link exists and no packages are
// declared, runStatus reports in-sync (nil error).
func TestRunStatusInSync(t *testing.T) {
	home, repo := withSandbox(t)
	src := filepath.Join(repo, "profiles", "base", "zshrc")
	if err := os.WriteFile(src, []byte("# zsh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(err)
	}
	if err := runStatus(&cobra.Command{}, &globals{}); err != nil {
		t.Errorf("expected in-sync (nil), got %v", err)
	}
}
