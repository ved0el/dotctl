package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseFile(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantErr  bool
		wantPkgs []Package
	}{
		{
			name:     "bare scalars",
			file:     "basic.yaml",
			wantPkgs: []Package{{Name: "ripgrep"}, {Name: "bat"}},
		},
		{
			name: "mapping overrides and hook",
			file: "overrides.yaml",
			wantPkgs: []Package{
				{Name: "ripgrep"},
				{Name: "fd", Apt: "fd-find"},
				{Name: "mise", PostInstall: "mise --version"},
			},
		},
		{
			name:     "per-manager skip",
			file:     "skip.yaml",
			wantPkgs: []Package{{Name: "mise", Skip: []string{"apt"}}},
		},
		{
			name:     "custom install command",
			file:     "install.yaml",
			wantPkgs: []Package{{Name: "sheldon", Install: "curl example.com | sh", PostInstall: "sheldon lock"}},
		},
		{
			name:    "unknown key rejected",
			file:    "unknown.yaml",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFile(filepath.Join("testdata", tt.file))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if !reflect.DeepEqual(got, tt.wantPkgs) {
				t.Errorf("got %+v, want %+v", got, tt.wantPkgs)
			}
		})
	}
}

func TestSkipped(t *testing.T) {
	p := Package{Name: "mise", Skip: []string{"apt"}}
	if !p.Skipped("apt") {
		t.Error("expected Skipped(apt) = true")
	}
	if p.Skipped("brew") {
		t.Error("expected Skipped(brew) = false")
	}
}

// TestWriteProfileRoundTrip locks the MarshalYAML/WriteProfile → ParseFile write
// path used by `dotctl pkg add/rm`: every package shape must survive a round trip
// unchanged, or a future field added to one of the three hand-maintained
// structures (MarshalYAML, allowedFields, the UnmarshalYAML raw struct) would
// silently corrupt a user's packages.yaml.
func TestWriteProfileRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		pkgs []Package
	}{
		{"plain names", []Package{{Name: "ripgrep"}, {Name: "bat"}}},
		{"per-manager overrides", []Package{{Name: "fd", Apt: "fd-find", Dnf: "fd-find"}}},
		{"custom install + hook + skip", []Package{{Name: "mise", Install: "curl x | sh", PostInstall: "mise install", Skip: []string{"apt"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := WriteProfile(dir, tt.pkgs); err != nil {
				t.Fatalf("WriteProfile: %v", err)
			}
			got, err := ParseFile(filepath.Join(dir, "packages.yaml"))
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if !reflect.DeepEqual(got, tt.pkgs) {
				t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, tt.pkgs)
			}
		})
	}
}

// TestWriteProfilePlainPackageIsBareScalar asserts a plain package serializes as
// a bare scalar (`- ripgrep`), not a `- name: ripgrep` mapping.
func TestWriteProfilePlainPackageIsBareScalar(t *testing.T) {
	dir := t.TempDir()
	if err := WriteProfile(dir, []Package{{Name: "ripgrep"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "packages.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "- ripgrep") || strings.Contains(string(data), "name:") {
		t.Errorf("plain package should be a bare scalar, got:\n%s", data)
	}
}

func TestWalkProfileMissingFileIsNotError(t *testing.T) {
	pkgs, err := WalkProfile(t.TempDir()) // dir has no packages.yaml
	if err != nil {
		t.Fatalf("WalkProfile: %v", err)
	}
	if pkgs != nil {
		t.Errorf("expected nil packages for missing file, got %+v", pkgs)
	}
}
