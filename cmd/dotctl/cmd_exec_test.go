package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// execRoot drives args through the real root cobra tree, exercising the command
// constructors, deps wiring, and RunE closures end-to-end. Cobra's own output is
// discarded; only non-destructive / dry-run paths are used here.
func execRoot(t *testing.T, args ...string) error {
	t.Helper()
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root.ExecuteContext(context.Background())
}

// requirePkgManager skips a test when no supported package manager is on PATH.
// Commands that build engine.Deps call pkg.Select (a real PATH probe), so without
// this guard those tests would fail on a minimal host (e.g. bare container) rather
// than on any real defect.
func requirePkgManager(t *testing.T) {
	t.Helper()
	for _, m := range []string{"brew", "apt-get", "dnf"} {
		if _, err := exec.LookPath(m); err == nil {
			return
		}
	}
	t.Skip("no supported package manager (brew/apt-get/dnf) on PATH")
}

func TestExecVersion(t *testing.T) {
	withSandbox(t)
	if err := execRoot(t, "version"); err != nil {
		t.Errorf("version: %v", err)
	}
}

func TestExecNewScaffold(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(t.TempDir(), "fresh") // must NOT exist yet
	t.Setenv("HOME", home)
	t.Setenv("DOTCTL_REPO", repo)

	if err := execRoot(t, "new"); err != nil {
		t.Fatalf("new: %v", err)
	}
	for _, p := range []string{"base", "tools", "develop"} {
		if _, err := os.Stat(filepath.Join(repo, "profiles", p, "packages.yaml")); err != nil {
			t.Errorf("expected scaffolded %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, "README.md")); err != nil {
		t.Errorf("expected scaffolded README: %v", err)
	}
	// Re-running refuses (profiles/ already exists).
	if err := execRoot(t, "new"); err == nil {
		t.Error("expected 'new' to refuse when profiles/ already exists")
	}
}

func TestExecProfileLifecycle(t *testing.T) {
	home, repo := withSandbox(t)
	if err := os.MkdirAll(filepath.Join(repo, "profiles", "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := execRoot(t, "profile", "add", "tools"); err != nil {
		t.Fatalf("profile add: %v", err)
	}
	if err := execRoot(t, "profile", "ls"); err != nil {
		t.Errorf("profile ls: %v", err)
	}
	if err := execRoot(t, "profile", "add", "base"); err != nil {
		t.Fatalf("profile add base: %v", err)
	}
	if err := execRoot(t, "profile", "rm", "tools"); err != nil {
		t.Fatalf("profile rm: %v", err)
	}
	if got := loadMachine(t, home).Profiles; len(got) != 1 || got[0] != "base" {
		t.Errorf("after rm, got %v, want [base]", got)
	}
}

func TestExecPkgRmAndAddAlreadyDeclared(t *testing.T) {
	_, repo := withSandbox(t)
	seedPackages(t, repo, "base", "packages:\n  - git\n  - ripgrep\n")

	if err := execRoot(t, "pkg", "rm", "ripgrep"); err != nil {
		t.Fatalf("pkg rm: %v", err)
	}
	if got := profilePkgNames(t, repo, "base"); len(got) != 1 || got[0] != "git" {
		t.Errorf("after pkg rm, got %v, want [git]", got)
	}
	// Already-declared add is a no-op — never reaches a real install.
	if err := execRoot(t, "pkg", "add", "git"); err != nil {
		t.Fatalf("pkg add already-declared: %v", err)
	}
}

func TestExecSaveDryRun(t *testing.T) {
	withSandbox(t)
	if err := execRoot(t, "save", "-n", "-m", "test commit"); err != nil {
		t.Errorf("save --dry-run: %v", err)
	}
}

func TestExecLinkAndUnlinkDryRun(t *testing.T) {
	withSandbox(t) // empty base profile → link/unlink are no-ops
	if err := execRoot(t, "link", "-n"); err != nil {
		t.Errorf("link --dry-run: %v", err)
	}
	if err := execRoot(t, "unlink", "-n"); err != nil {
		t.Errorf("unlink --dry-run: %v", err)
	}
}

func TestExecDoctorReportsProblems(t *testing.T) {
	withSandbox(t) // no .git in the sandbox repo
	if err := execRoot(t, "doctor"); err == nil {
		t.Error("expected doctor to report problems")
	}
}

// TestExecInitDryRun drives the full deps wiring (manager, both linkers, runner)
// and the engine pipeline in dry-run, asserting it writes nothing. The empty base
// profile makes installs and links no-ops, so no real side effects occur.
func TestExecInitDryRun(t *testing.T) {
	requirePkgManager(t)
	home, _ := withSandbox(t)
	if err := execRoot(t, "init", "--profiles", "base", "-n"); err != nil {
		t.Fatalf("init --dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "dotctl", "machine.yaml")); err == nil {
		t.Error("dry-run init must not persist machine.yaml")
	}
}

func TestExecApplyDryRun(t *testing.T) {
	requirePkgManager(t)
	home, repo := withSandbox(t)
	saveMachine(t, home, repo, "base")
	if err := execRoot(t, "apply", "-n"); err != nil {
		t.Errorf("apply --dry-run: %v", err)
	}
}

func TestExecApplyRequiresBootstrap(t *testing.T) {
	withSandbox(t) // no machine.yaml written
	if err := execRoot(t, "apply"); err == nil {
		t.Error("expected apply to refuse on an unbootstrapped machine")
	}
}

func TestExecPkgInstallEmptyIsNoop(t *testing.T) {
	requirePkgManager(t)
	withSandbox(t) // empty base profile → nothing to install
	if err := execRoot(t, "pkg", "install"); err != nil {
		t.Errorf("pkg install (empty): %v", err)
	}
}
