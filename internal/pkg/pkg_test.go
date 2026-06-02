package pkg

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ved0el/dotctl/internal/console"
	"github.com/ved0el/dotctl/internal/manifest"
)

var errFake = errors.New("fake runner error")

type fakeRunner struct {
	calls [][]string
	out   []byte // payload returned by Output (lets tests exercise status parsing)
	err   error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.err
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.out, f.err
}

func TestSelectWith(t *testing.T) {
	// present maps the probe commands that "exist"; Select returns the first
	// candidate in detection order (brew → apt → dnf).
	tests := []struct {
		name    string
		present map[string]bool
		want    string
		wantErr bool
	}{
		{name: "brew wins", present: map[string]bool{"brew": true, "apt-get": true}, want: "brew"},
		{name: "apt when no brew", present: map[string]bool{"apt-get": true, "dnf": true}, want: "apt"},
		{name: "dnf when only dnf", present: map[string]bool{"dnf": true}, want: "dnf"},
		{name: "none", present: map[string]bool{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := selectWith(&fakeRunner{}, func(c string) bool { return tt.present[c] })
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if m.Name() != tt.want {
				t.Errorf("got %s, want %s", m.Name(), tt.want)
			}
		})
	}
}

func TestDnfInstallUsesOverride(t *testing.T) {
	f := &fakeRunner{}
	pkgs := []manifest.Package{{Name: "fd", Dnf: "fd-find"}, {Name: "ripgrep"}}
	if err := (dnfManager{r: f}).Install(context.Background(), pkgs); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.calls[0], " ")
	if !strings.Contains(joined, "fd-find") || !strings.Contains(joined, "ripgrep") {
		t.Errorf("dnf call missing expected names: %q", joined)
	}
}

func TestBrewInstallArgs(t *testing.T) {
	f := &fakeRunner{}
	pkgs := []manifest.Package{{Name: "ripgrep"}, {Name: "fd", Apt: "fd-find"}}
	if err := (brewManager{r: f}).Install(context.Background(), pkgs); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected 1 call, got %v", f.calls)
	}
	want := []string{"brew", "install", "ripgrep", "fd"} // brew ignores apt override
	got := f.calls[0]
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAptInstallUsesOverride(t *testing.T) {
	f := &fakeRunner{}
	pkgs := []manifest.Package{{Name: "ripgrep"}, {Name: "fd", Apt: "fd-find"}}
	if err := (aptManager{r: f}).Install(context.Background(), pkgs); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected 1 call, got %v", f.calls)
	}
	joined := strings.Join(f.calls[0], " ")
	if !strings.Contains(joined, "ripgrep") || !strings.Contains(joined, "fd-find") {
		t.Errorf("apt call missing expected names: %q", joined)
	}
	if strings.Contains(joined, " fd ") || strings.HasSuffix(joined, " fd") {
		t.Errorf("apt should use fd-find override, not fd: %q", joined)
	}
}

func TestBrewUpgradeArgs(t *testing.T) {
	f := &fakeRunner{}
	if err := (brewManager{r: f}).Upgrade(context.Background(), []manifest.Package{{Name: "ripgrep"}, {Name: "fd", Brew: "fd"}}); err != nil {
		t.Fatal(err)
	}
	want := []string{"brew", "upgrade", "ripgrep", "fd"}
	if len(f.calls) != 1 || strings.Join(f.calls[0], " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", f.calls, want)
	}
}

func TestAptUpgradeUsesOnlyUpgrade(t *testing.T) {
	f := &fakeRunner{}
	if err := (aptManager{r: f}).Upgrade(context.Background(), []manifest.Package{{Name: "fd", Apt: "fd-find"}}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.calls[0], " ")
	if !strings.Contains(joined, "--only-upgrade") || !strings.Contains(joined, "fd-find") {
		t.Errorf("apt upgrade should use --only-upgrade with the override name: %q", joined)
	}
}

func TestDnfUpgradeUsesOverride(t *testing.T) {
	f := &fakeRunner{}
	if err := (dnfManager{r: f}).Upgrade(context.Background(), []manifest.Package{{Name: "fd", Dnf: "fd-find"}}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.calls[0], " ")
	if !strings.Contains(joined, "upgrade") || !strings.Contains(joined, "fd-find") {
		t.Errorf("dnf upgrade should upgrade the override name: %q", joined)
	}
}

func TestUpgradeEmptyNoCall(t *testing.T) {
	f := &fakeRunner{}
	if err := (brewManager{r: f}).Upgrade(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("empty upgrade should not call the runner, got %v", f.calls)
	}
}

func TestBrewIsInstalled(t *testing.T) {
	ok, err := (brewManager{r: &fakeRunner{}}).IsInstalled(context.Background(), manifest.Package{Name: "ripgrep"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected installed when `brew list` succeeds")
	}
}

func TestBrewIsInstalledFalseOnError(t *testing.T) {
	ok, err := (brewManager{r: &fakeRunner{err: errFake}}).IsInstalled(context.Background(), manifest.Package{Name: "nope"})
	if err != nil {
		t.Fatalf("IsInstalled should not surface a check failure as error: %v", err)
	}
	if ok {
		t.Error("expected not installed when `brew list` fails")
	}
}

func TestBrewIsInstalledQueriesByBrewName(t *testing.T) {
	f := &fakeRunner{}
	if _, err := (brewManager{r: f}).IsInstalled(context.Background(), manifest.Package{Name: "fd", Brew: "fd-bin"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"brew", "list", "--versions", "fd-bin"} // uses the Brew override, not Name
	if len(f.calls) != 1 || strings.Join(f.calls[0], " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", f.calls, want)
	}
}

func TestAptIsInstalled(t *testing.T) {
	tests := []struct {
		name string
		out  string
		err  error
		want bool
	}{
		{"installed", "install ok installed", nil, true},
		{"config-files only", "deinstall ok config-files", nil, false},
		{"query error means absent", "", errFake, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeRunner{out: []byte(tt.out), err: tt.err}
			ok, err := (aptManager{r: f}).IsInstalled(context.Background(), manifest.Package{Name: "ripgrep"})
			if err != nil {
				t.Fatalf("IsInstalled: %v", err)
			}
			if ok != tt.want {
				t.Errorf("got %v, want %v", ok, tt.want)
			}
		})
	}
}

func TestDnfIsInstalled(t *testing.T) {
	// rpm -q signals via exit code: success → installed, non-zero → absent.
	if ok, _ := (dnfManager{r: &fakeRunner{}}).IsInstalled(context.Background(), manifest.Package{Name: "ripgrep"}); !ok {
		t.Error("expected installed when rpm -q succeeds")
	}
	if ok, _ := (dnfManager{r: &fakeRunner{err: errFake}}).IsInstalled(context.Background(), manifest.Package{Name: "nope"}); ok {
		t.Error("expected absent when rpm -q exits non-zero")
	}
}

func TestInstallEmptyNoCall(t *testing.T) {
	f := &fakeRunner{}
	if err := (brewManager{r: f}).Install(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("empty install should not call runner, got %v", f.calls)
	}
}

func TestInstallSkipsUnsupportedManager(t *testing.T) {
	f := &fakeRunner{}
	pkgs := []manifest.Package{{Name: "ripgrep"}, {Name: "mise", Skip: []string{"apt"}}}
	if err := (aptManager{r: f}).Install(context.Background(), pkgs); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.calls[0], " ")
	if strings.Contains(joined, "mise") {
		t.Errorf("mise (skip: apt) should not be installed via apt: %q", joined)
	}
	if !strings.Contains(joined, "ripgrep") {
		t.Errorf("ripgrep should still be installed: %q", joined)
	}
}

func TestDryRunnerRunLogsAndSucceeds(t *testing.T) {
	var buf bytes.Buffer
	d := DryRunner{Log: console.New(&buf, false)}
	if err := d.Run(context.Background(), "echo", "hi", "there"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "echo hi there") {
		t.Errorf("expected the planned command in output, got %q", got)
	}
}

func TestDryRunnerOutputLogsAndReturnsNil(t *testing.T) {
	var buf bytes.Buffer
	out, err := DryRunner{Log: console.New(&buf, false)}.Output(context.Background(), "git", "status")
	if err != nil || out != nil {
		t.Fatalf("expected (nil, nil), got (%q, %v)", out, err)
	}
	if !strings.Contains(buf.String(), "git status") {
		t.Errorf("expected the planned command in output, got %q", buf.String())
	}
}

func TestDryRunnerHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	d := DryRunner{Log: console.New(&buf, false)}
	if err := d.Run(ctx, "echo", "hi"); err == nil {
		t.Error("Run: expected a cancellation error")
	}
	if _, err := d.Output(ctx, "echo", "hi"); err == nil {
		t.Error("Output: expected a cancellation error")
	}
	if buf.Len() != 0 {
		t.Errorf("a cancelled dry-run must log nothing, got %q", buf.String())
	}
}

func TestCommandExistsOnPath(t *testing.T) {
	dir := t.TempDir()
	const tool = "dotctl-onpath-xyz"
	if err := os.WriteFile(filepath.Join(dir, tool), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if !CommandExists(tool) {
		t.Error("expected CommandExists to find a tool on PATH")
	}
	if CommandExists("dotctl-not-a-real-command-xyz") {
		t.Error("expected CommandExists of a bogus name = false")
	}
}

func TestCommandExistsFallbackLocalBin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir()) // empty-ish PATH so only the ~/.local/bin fallback can match
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	const tool = "dotctl-localbin-only-xyz"
	if err := os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !CommandExists(tool) {
		t.Error("expected CommandExists to find a tool in ~/.local/bin")
	}
}

func TestIsInstalledSkippedReturnsTrue(t *testing.T) {
	// Even with a failing runner, a skipped package reports satisfied.
	ok, err := (aptManager{r: &fakeRunner{err: errFake}}).IsInstalled(
		context.Background(), manifest.Package{Name: "mise", Skip: []string{"apt"}})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("skipped package should report installed = true")
	}
}
