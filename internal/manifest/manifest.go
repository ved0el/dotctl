// Package manifest parses per-profile packages.yaml files into Package values.
package manifest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Package is a single declared package. In packages.yaml an entry may be a bare
// string (the logical name) or a mapping with per-manager overrides and an
// optional post-install hook.
type Package struct {
	Name        string   `yaml:"name"`
	Brew        string   `yaml:"brew,omitempty"`
	Apt         string   `yaml:"apt,omitempty"`
	Dnf         string   `yaml:"dnf,omitempty"`
	Install     string   `yaml:"install,omitempty"` // custom install command (cross-platform; bypasses brew/apt)
	PostInstall string   `yaml:"post_install,omitempty"`
	Skip        []string `yaml:"skip,omitempty"` // package managers to skip (e.g. ["apt"])
}

// Custom reports whether this package installs via a custom command rather than
// the platform package manager.
func (p Package) Custom() bool { return p.Install != "" }

// MarshalYAML emits a bare string for a plain package (only Name set), matching
// the scalar form authors write, and the mapping form only when overrides exist.
func (p Package) MarshalYAML() (any, error) {
	if p.Brew == "" && p.Apt == "" && p.Dnf == "" && p.Install == "" && p.PostInstall == "" && len(p.Skip) == 0 {
		return p.Name, nil
	}
	type alias Package // no MarshalYAML → no recursion
	return alias(p), nil
}

// Skipped reports whether this package should be skipped on the named manager.
func (p Package) Skipped(manager string) bool {
	for _, m := range p.Skip {
		if m == manager {
			return true
		}
	}
	return false
}

// allowedFields is the set of valid keys in a mapping-form package entry.
var allowedFields = map[string]struct{}{
	"name": {}, "brew": {}, "apt": {}, "dnf": {}, "install": {}, "post_install": {}, "skip": {},
}

// UnmarshalYAML accepts either a scalar (logical name) or a mapping form.
func (p *Package) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		p.Name = node.Value
		return nil
	}
	// Reject unknown keys explicitly: a nested node.Decode does not inherit the
	// parent decoder's KnownFields setting, so typos would otherwise pass silently.
	for i := 0; i+1 < len(node.Content); i += 2 {
		if _, ok := allowedFields[node.Content[i].Value]; !ok {
			return fmt.Errorf("unknown field %q in package entry", node.Content[i].Value)
		}
	}
	// Decode into a local alias to avoid infinite recursion, then assign
	// field-by-field (a direct Package(raw) conversion is illegal once tags differ).
	var raw struct {
		Name        string   `yaml:"name"`
		Brew        string   `yaml:"brew"`
		Apt         string   `yaml:"apt"`
		Dnf         string   `yaml:"dnf"`
		Install     string   `yaml:"install"`
		PostInstall string   `yaml:"post_install"`
		Skip        []string `yaml:"skip"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	if raw.Name == "" {
		return errors.New("package entry missing 'name'")
	}
	p.Name, p.Brew, p.Apt, p.Dnf, p.Install, p.PostInstall, p.Skip = raw.Name, raw.Brew, raw.Apt, raw.Dnf, raw.Install, raw.PostInstall, raw.Skip
	return nil
}

type file struct {
	Packages []Package `yaml:"packages"`
}

// ParseFile reads and validates a packages.yaml file. Unknown keys are rejected
// so typos fail loudly. An empty file yields no packages.
func ParseFile(path string) ([]Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var doc file
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc.Packages, nil
}

// WalkProfile returns the packages declared in <profileDir>/packages.yaml. A missing
// file is not an error — a profile may declare only dotfiles.
func WalkProfile(profileDir string) ([]Package, error) {
	pkgs, err := ParseFile(filepath.Join(profileDir, "packages.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return pkgs, err
}

// WriteProfile writes the package list to <profileDir>/packages.yaml, replacing
// the file. Used by `dotctl pkg add/rm` to mutate a profile's manifest.
func WriteProfile(profileDir string, pkgs []Package) error {
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", profileDir, err)
	}
	data, err := yaml.Marshal(file{Packages: pkgs})
	if err != nil {
		return fmt.Errorf("marshal packages: %w", err)
	}
	path := filepath.Join(profileDir, "packages.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
