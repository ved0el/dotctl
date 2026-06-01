package link

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ved0el/dotctl/internal/console"
)

func newLinker(t *testing.T, home string, dryRun bool) *Linker {
	t.Helper()
	l := NewLinker(home, OsFS{}, dryRun, console.New(&bytes.Buffer{}, false))
	l.Now = func() time.Time { return time.Unix(1700000000, 0) } // deterministic backup dir
	return l
}

// scaffold creates a profile with a top-level file and a config/ child, plus an
// empty home directory, and returns their paths.
func scaffold(t *testing.T) (profileDir, home string) {
	t.Helper()
	root := t.TempDir()
	profileDir = filepath.Join(root, "profile")
	home = filepath.Join(root, "home")
	mustMkdirAll(t, filepath.Join(profileDir, "config", "nvim"))
	mustWrite(t, filepath.Join(profileDir, "zshrc"), "# zshrc\n")
	mustWrite(t, filepath.Join(profileDir, "config", "nvim", "init.lua"), "-- nvim\n")
	mustWrite(t, filepath.Join(profileDir, "packages.yaml"), "packages: []\n")
	mustMkdirAll(t, home)
	return profileDir, home
}

func mustMkdirAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlinkTarget(t *testing.T, p string) string {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatalf("lstat %s: %v", p, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", p)
	}
	target, err := os.Readlink(p)
	if err != nil {
		t.Fatalf("readlink %s: %v", p, err)
	}
	return target
}

func TestApplyLinksFilesAndConfig(t *testing.T) {
	profileDir, home := scaffold(t)
	if err := newLinker(t, home, false).Apply(profileDir); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := symlinkTarget(t, filepath.Join(home, ".zshrc")), filepath.Join(profileDir, "zshrc"); got != want {
		t.Errorf(".zshrc → %s, want %s", got, want)
	}
	// Under config/, files are linked leaf-by-leaf; the dir itself stays real.
	if got, want := symlinkTarget(t, filepath.Join(home, ".config", "nvim", "init.lua")), filepath.Join(profileDir, "config", "nvim", "init.lua"); got != want {
		t.Errorf(".config/nvim/init.lua → %s, want %s", got, want)
	}
	if fi, err := os.Lstat(filepath.Join(home, ".config", "nvim")); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Error(".config/nvim should be a real directory, not a symlink")
	}
	if _, err := os.Lstat(filepath.Join(home, ".packages.yaml")); !os.IsNotExist(err) {
		t.Error("packages.yaml should not be linked")
	}
}

func TestApplyIdempotent(t *testing.T) {
	profileDir, home := scaffold(t)
	l := newLinker(t, home, false)
	if err := l.Apply(profileDir); err != nil {
		t.Fatal(err)
	}
	if err := l.Apply(profileDir); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	// No backup directory should have been created on a clean re-run.
	if _, err := os.Stat(filepath.Join(home, ".dotfiles-backup")); !os.IsNotExist(err) {
		t.Error("idempotent re-run should not create a backup dir")
	}
}

func TestApplyBacksUpRealFile(t *testing.T) {
	profileDir, home := scaffold(t)
	realFile := filepath.Join(home, ".zshrc")
	mustWrite(t, realFile, "original content\n")

	if err := newLinker(t, home, false).Apply(profileDir); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// .zshrc is now a symlink into the profile...
	if got, want := symlinkTarget(t, realFile), filepath.Join(profileDir, "zshrc"); got != want {
		t.Errorf(".zshrc → %s, want %s", got, want)
	}
	// ...and the original was preserved in the backup dir.
	backup := filepath.Join(home, ".dotfiles-backup", "1700000000", ".zshrc")
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != "original content\n" {
		t.Errorf("backup content = %q", string(data))
	}
}

func TestApplyReplacesDanglingSymlink(t *testing.T) {
	profileDir, home := scaffold(t)
	dangling := filepath.Join(home, ".zshrc")
	if err := os.Symlink(filepath.Join(home, "does-not-exist"), dangling); err != nil {
		t.Fatal(err)
	}
	if err := newLinker(t, home, false).Apply(profileDir); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := symlinkTarget(t, dangling), filepath.Join(profileDir, "zshrc"); got != want {
		t.Errorf(".zshrc → %s, want %s", got, want)
	}
	// No backup: a broken link is not user data.
	if _, err := os.Stat(filepath.Join(home, ".dotfiles-backup")); !os.IsNotExist(err) {
		t.Error("replacing a dangling link should not create a backup")
	}
}

func TestApplyDryRunWritesNothing(t *testing.T) {
	profileDir, home := scaffold(t)
	if err := newLinker(t, home, true).Apply(profileDir); err != nil {
		t.Fatalf("Apply dry-run: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Error("dry-run must not create symlinks")
	}
}

func TestRemoveOnlyOurLinks(t *testing.T) {
	profileDir, home := scaffold(t)
	l := newLinker(t, home, false)
	if err := l.Apply(profileDir); err != nil {
		t.Fatal(err)
	}
	// Put a real file at a different target so Remove must leave it alone.
	other := filepath.Join(home, ".gitconfig")
	mustWrite(t, other, "real\n")

	if err := l.Remove(profileDir); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Error(".zshrc symlink should be removed")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error(".gitconfig real file must be left intact")
	}
	// Idempotent second removal.
	if err := l.Remove(profileDir); err != nil {
		t.Errorf("second Remove: %v", err)
	}
}
