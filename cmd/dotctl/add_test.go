package main

import (
	"os"
	"path/filepath"
	"testing"
)

// withSandbox points HOME and DOTCTL_REPO at temp dirs and returns (home, repo).
func withSandbox(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	repo := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DOTCTL_REPO", repo)
	if err := os.MkdirAll(filepath.Join(repo, "profiles", "base"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, repo
}

func TestRunAddAdoptsFile(t *testing.T) {
	home, repo := withSandbox(t)
	// an existing real dotfile under a config dir
	src := filepath.Join(home, ".config", "foo", "foo.conf")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runAdd(&globals{}, "base", []string{src}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	// moved into the repo at profiles/base/config/foo/foo.conf
	dest := filepath.Join(repo, "profiles", "base", "config", "foo", "foo.conf")
	if data, err := os.ReadFile(dest); err != nil || string(data) != "hi\n" {
		t.Fatalf("repo copy missing/wrong: data=%q err=%v", string(data), err)
	}
	// original is now a symlink back to the repo
	target, err := os.Readlink(src)
	if err != nil {
		t.Fatalf("original should be a symlink: %v", err)
	}
	if target != dest {
		t.Errorf("%s → %s, want %s", src, target, dest)
	}
}

func TestRunAddRejectsNonHomePath(t *testing.T) {
	withSandbox(t)
	if err := runAdd(&globals{}, "base", []string{"/etc/hosts"}); err == nil {
		t.Error("expected error adopting a path outside HOME")
	}
}

func TestRunAddSkipsSymlink(t *testing.T) {
	home, _ := withSandbox(t)
	link := filepath.Join(home, ".alreadylink")
	if err := os.Symlink(filepath.Join(home, "target"), link); err != nil {
		t.Fatal(err)
	}
	// already a symlink → skipped, no error
	if err := runAdd(&globals{}, "base", []string{link}); err != nil {
		t.Errorf("expected skip (nil error) for existing symlink, got %v", err)
	}
}
