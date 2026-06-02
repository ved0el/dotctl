package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ved0el/dotctl/internal/console"
	"github.com/ved0el/dotctl/internal/link"
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

// TestRunAddAdoptsDirectoryLeafByLeaf is the regression for the directory-adoption
// corruption bug: adopting a directory must leaf-link its files (matching the link
// engine), leaving the directory itself real — never a whole-directory symlink that
// a later `apply` would resolve through to back up the repo's own files.
func TestRunAddAdoptsDirectoryLeafByLeaf(t *testing.T) {
	home, repo := withSandbox(t)
	dir := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(filepath.Join(dir, "lua"), 0o755); err != nil {
		t.Fatal(err)
	}
	leaves := []string{"init.lua", filepath.Join("lua", "opts.lua")}
	for _, f := range leaves {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("-- "+f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := runAdd(&globals{}, "base", []string{dir}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	// (a) the directory stays a REAL directory, not a symlink.
	fi, err := os.Lstat(dir)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		t.Fatalf("%s must remain a real directory (mode=%v err=%v)", dir, fi.Mode(), err)
	}
	// (b) each leaf is a symlink into the repo, and (d) the repo source is readable.
	for _, f := range leaves {
		leaf := filepath.Join(dir, f)
		dest := filepath.Join(repo, "profiles", "base", "config", "nvim", f)
		target, err := os.Readlink(leaf)
		if err != nil {
			t.Fatalf("%s must be a symlink: %v", leaf, err)
		}
		if target != dest {
			t.Errorf("%s → %s, want %s", leaf, target, dest)
		}
		if data, err := os.ReadFile(dest); err != nil || string(data) != "-- "+f+"\n" {
			t.Errorf("repo source %s unreadable/wrong: data=%q err=%v", dest, string(data), err)
		}
	}

	// (c) a subsequent link pass is a no-op: every leaf is already correctly linked.
	l := link.NewLinker(home, link.OsFS{}, false, console.New(&bytes.Buffer{}, false))
	pairs, err := l.Targets(filepath.Join(repo, "profiles", "base"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != len(leaves) {
		t.Fatalf("expected %d leaf links, got %d (%+v)", len(leaves), len(pairs), pairs)
	}
	for _, p := range pairs {
		if st := l.Status(p); st != link.StateLinked {
			t.Errorf("after adopt, %s is %s, want linked", p.Dst, st)
		}
	}
}

// TestRunAddRejectsTraversalProfile guards against a crafted --profile escaping
// the profiles/ tree (e.g. moving adopted files outside the repo).
func TestRunAddRejectsTraversalProfile(t *testing.T) {
	home, _ := withSandbox(t)
	src := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(src, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runAdd(&globals{}, filepath.Join("..", "..", "evil"), []string{src}); err == nil {
		t.Error("expected error for a traversal profile name")
	}
	// the original file must be untouched (not moved/symlinked).
	if fi, err := os.Lstat(src); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("%s should be untouched (err=%v mode=%v)", src, err, fi.Mode())
	}
}

func TestRunAddRejectsNonHomePath(t *testing.T) {
	withSandbox(t)
	if err := runAdd(&globals{}, "base", []string{"/etc/hosts"}); err == nil {
		t.Error("expected error adopting a path outside HOME")
	}
}

// TestRunAddRejectsHomeItself guards against `dotctl add ~` walking all of $HOME.
func TestRunAddRejectsHomeItself(t *testing.T) {
	withSandbox(t)
	if err := runAdd(&globals{}, "base", []string{"~"}); err == nil {
		t.Error("expected error adopting $HOME itself")
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
