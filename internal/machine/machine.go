// Package machine manages per-machine config (machine.yaml) and resolves the
// effective package set from the selected profiles.
package machine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ved0el/dotctl/internal/manifest"
	"gopkg.in/yaml.v3"
)

const (
	configFile = "machine.yaml"

	// ProfilesSubdir is the repo subdirectory holding profile trees.
	ProfilesSubdir = "profiles"
	// DefaultProfile is applied when a machine declares no profiles.
	DefaultProfile = "base"
)

// ErrNotBootstrapped indicates no machine config exists yet; the caller requires
// one (e.g. `apply` before a first `bootstrap`).
var ErrNotBootstrapped = errors.New("no machine config found; run 'dotctl init --profiles ...' first")

// Config is the local, unsynced per-machine configuration.
type Config struct {
	Repo     string   `yaml:"repo"`
	Profiles []string `yaml:"profiles"`
	Packages struct {
		Add     []string `yaml:"add"`
		Exclude []string `yaml:"exclude"`
	} `yaml:"packages"`
}

// Load reads <configDir>/machine.yaml. A missing file yields a zero config and
// no error — the first install writes it.
func Load(configDir string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(filepath.Join(configDir, configFile))
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read machine config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse machine config: %w", err)
	}
	return cfg, nil
}

// Save writes machine.yaml, creating configDir if needed.
func Save(configDir string, cfg Config) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal machine config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, configFile), data, 0o644); err != nil {
		return fmt.Errorf("write machine config: %w", err)
	}
	return nil
}

// ResolvePackages computes the effective package set for the given profiles:
// packages are unioned across profiles in order (a later profile overrides an
// earlier one with the same name), names in Exclude are dropped, and bare Add
// names not already present are appended. Exclude takes precedence over Add.
func ResolvePackages(profileRoot string, profiles []string, cfg Config) ([]manifest.Package, error) {
	var order []string
	byName := map[string]manifest.Package{}
	for _, profile := range profiles {
		pkgs, err := manifest.WalkProfile(filepath.Join(profileRoot, profile))
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", profile, err)
		}
		for _, p := range pkgs {
			if _, seen := byName[p.Name]; !seen {
				order = append(order, p.Name)
			}
			byName[p.Name] = p // later profile wins
		}
	}
	for _, name := range cfg.Packages.Add {
		if _, seen := byName[name]; !seen {
			order = append(order, name)
			byName[name] = manifest.Package{Name: name}
		}
	}
	exclude := make(map[string]struct{}, len(cfg.Packages.Exclude))
	for _, name := range cfg.Packages.Exclude {
		exclude[name] = struct{}{}
	}
	out := make([]manifest.Package, 0, len(order))
	for _, name := range order {
		if _, skip := exclude[name]; skip {
			continue
		}
		out = append(out, byName[name])
	}
	return out, nil
}

// ActiveTools returns the set of tool/command names this machine will have after
// a reconcile: declared package names plus the mise `[tools]` keys from the
// selected profiles' conf.d files. Used to gate config linking — a config/<tool>
// links when its tool is active here (or the command is already installed).
func ActiveTools(profileRoot string, profiles []string, cfg Config) map[string]bool {
	set := map[string]bool{}
	if pkgs, err := ResolvePackages(profileRoot, profiles, cfg); err == nil {
		for _, p := range pkgs {
			set[p.Name] = true
		}
	}
	for _, prof := range profiles {
		dir := filepath.Join(profileRoot, prof, "config", "mise", "conf.d")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			for _, k := range miseToolKeys(filepath.Join(dir, e.Name())) {
				set[k] = true
			}
		}
	}
	return set
}

// miseToolKeys extracts the keys of the [tools] table from a mise TOML config —
// a minimal line scan (no TOML dependency for reading a flat key list).
func miseToolKeys(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var keys []string
	inTools := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inTools = line == "[tools]"
			continue
		}
		if inTools {
			if k, _, ok := strings.Cut(line, "="); ok {
				keys = append(keys, strings.Trim(strings.TrimSpace(k), `"`))
			}
		}
	}
	return keys
}
