package manifest

import (
	"path/filepath"
	"reflect"
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

func TestWalkProfileMissingFileIsNotError(t *testing.T) {
	pkgs, err := WalkProfile(t.TempDir()) // dir has no packages.yaml
	if err != nil {
		t.Fatalf("WalkProfile: %v", err)
	}
	if pkgs != nil {
		t.Errorf("expected nil packages for missing file, got %+v", pkgs)
	}
}
