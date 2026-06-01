package machine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ved0el/dotctl/internal/manifest"
)

func writeProfile(t *testing.T, root, profile, body string) {
	t.Helper()
	dir := filepath.Join(root, profile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, "packages.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func names(pkgs []manifest.Package) []string {
	out := make([]string, len(pkgs))
	for i, p := range pkgs {
		out[i] = p.Name
	}
	return out
}

func TestResolvePackages(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "base", "packages:\n  - ripgrep\n  - name: fd\n    apt: fd-find\n")
	writeProfile(t, root, "develop", "packages:\n  - mise\n  - fd\n") // fd reappears: later wins
	writeProfile(t, root, "empty", "")                                // no packages.yaml

	tests := []struct {
		name     string
		profiles []string
		cfg      func(*Config)
		want     []string
	}{
		{name: "single profile", profiles: []string{"base"}, want: []string{"ripgrep", "fd"}},
		{name: "union preserves order", profiles: []string{"base", "develop"}, want: []string{"ripgrep", "fd", "mise"}},
		{name: "missing profile file is ok", profiles: []string{"base", "empty"}, want: []string{"ripgrep", "fd"}},
		{name: "add appends", profiles: []string{"base"}, cfg: func(c *Config) { c.Packages.Add = []string{"tmux"} }, want: []string{"ripgrep", "fd", "tmux"}},
		{name: "exclude removes", profiles: []string{"base"}, cfg: func(c *Config) { c.Packages.Exclude = []string{"fd"} }, want: []string{"ripgrep"}},
		{name: "exclude beats add", profiles: []string{"base"}, cfg: func(c *Config) {
			c.Packages.Add = []string{"tmux"}
			c.Packages.Exclude = []string{"tmux"}
		}, want: []string{"ripgrep", "fd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			if tt.cfg != nil {
				tt.cfg(&cfg)
			}
			got, err := ResolvePackages(root, tt.profiles, cfg)
			if err != nil {
				t.Fatalf("ResolvePackages: %v", err)
			}
			if !equalStrings(names(got), tt.want) {
				t.Errorf("got %v, want %v", names(got), tt.want)
			}
		})
	}

	// Verify the later profile's override fields actually win.
	t.Run("later profile overrides fields", func(t *testing.T) {
		got, err := ResolvePackages(root, []string{"base", "develop"}, Config{})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range got {
			if p.Name == "fd" && p.Apt != "" {
				t.Errorf("expected develop's plain fd to override base's fd-find, got Apt=%q", p.Apt)
			}
		}
	})
}

func TestLoadMissingReturnsZero(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Repo != "" || len(cfg.Profiles) != 0 {
		t.Errorf("expected zero config, got %+v", cfg)
	}
}

func TestLoadEmptyProfilesIsDistinct(t *testing.T) {
	// A present file with no profiles must load cleanly with zero profiles — apply
	// relies on len(Profiles)==0 to return ErrNotBootstrapped.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "machine.yaml"), []byte("repo: ~/.dotfiles\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("expected zero profiles, got %v", cfg.Profiles)
	}
	if cfg.Repo != "~/.dotfiles" {
		t.Errorf("expected repo parsed, got %q", cfg.Repo)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Config{Repo: "~/.dotfiles", Profiles: []string{"base", "develop"}}
	want.Packages.Add = []string{"tmux"}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Repo != want.Repo || !equalStrings(got.Profiles, want.Profiles) || !equalStrings(got.Packages.Add, want.Packages.Add) {
		t.Errorf("round trip mismatch: got %+v want %+v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMiseToolKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.toml")
	body := "# comment\n[tools]\nnode = \"lts\"\nripgrep = \"latest\"  # inline comment\n\n[settings]\nidiomatic_version_file = true\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := miseToolKeys(path)
	want := []string{"node", "ripgrep"}
	if !equalStrings(got, want) {
		t.Errorf("miseToolKeys = %v, want %v", got, want)
	}
	// Missing file → nil, no panic.
	if k := miseToolKeys(filepath.Join(dir, "nope.toml")); k != nil {
		t.Errorf("expected nil for missing file, got %v", k)
	}
}

func TestActiveToolsIncludesPackagesAndMiseKeys(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "base", "packages:\n  - git\n")
	confd := filepath.Join(root, "tools", "config", "mise", "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confd, "tools.toml"), []byte("[tools]\nbat = \"latest\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	active := ActiveTools(root, []string{"base", "tools"}, Config{})
	if !active["git"] || !active["bat"] {
		t.Errorf("expected git (package) and bat (mise key) active, got %v", active)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	// typo: 'profile' instead of 'profiles' must error, not be silently ignored
	if err := os.WriteFile(filepath.Join(dir, "machine.yaml"), []byte("profile: [base]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("expected error for unknown field 'profile'")
	}
}

func TestValidate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, []string{"base"}); err != nil {
		t.Errorf("base should validate: %v", err)
	}
	if err := Validate(root, []string{"base", "nope"}); err == nil {
		t.Error("expected error for missing profile dir")
	}
}

func TestSaveAtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	var cfg Config
	cfg.Repo = "~/.dotfiles"
	cfg.Profiles = []string{"base", "tools"}
	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !equalStrings(got.Profiles, cfg.Profiles) || got.Repo != cfg.Repo {
		t.Errorf("round trip mismatch: %+v", got)
	}
	// no leftover temp files
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
