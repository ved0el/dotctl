package pkg

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ved0el/dotctl/internal/manifest"
)

var errFake = errors.New("fake runner error")

type fakeRunner struct {
	calls [][]string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.err
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return nil, f.err
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
