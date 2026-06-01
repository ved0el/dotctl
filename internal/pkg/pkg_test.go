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

func TestSelect(t *testing.T) {
	tests := []struct {
		goos    string
		want    string
		wantErr bool
	}{
		{goos: "darwin", want: "brew"},
		{goos: "linux", want: "apt"},
		{goos: "plan9", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			m, err := Select(tt.goos, &fakeRunner{})
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
