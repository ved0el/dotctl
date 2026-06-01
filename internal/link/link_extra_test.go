package link

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDryRunReportsBackupWithoutTouchingRealFile(t *testing.T) {
	profileDir, home := scaffold(t)
	real := filepath.Join(home, ".zshrc")
	mustWrite(t, real, "keep me\n")

	if err := newLinker(t, home, true).Apply(profileDir); err != nil {
		t.Fatalf("Apply dry-run: %v", err)
	}
	// Still a real file, untouched, and no backup dir created.
	data, err := os.ReadFile(real)
	if err != nil || string(data) != "keep me\n" {
		t.Errorf("dry-run modified the real file: data=%q err=%v", string(data), err)
	}
	if _, err := os.Stat(filepath.Join(home, ".dotfiles-backup")); !os.IsNotExist(err) {
		t.Error("dry-run must not create a backup dir")
	}
}

func TestDryRunReportsRelinkForWrongSymlink(t *testing.T) {
	profileDir, home := scaffold(t)
	wrong := filepath.Join(home, ".zshrc")
	if err := os.Symlink(filepath.Join(home, "elsewhere"), wrong); err != nil {
		t.Fatal(err)
	}
	if err := newLinker(t, home, true).Apply(profileDir); err != nil {
		t.Fatalf("Apply dry-run: %v", err)
	}
	// The wrong link is left in place during a dry run.
	target, err := os.Readlink(wrong)
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join(home, "elsewhere") {
		t.Errorf("dry-run changed the symlink to %q", target)
	}
}

func TestRemoveDryRunKeepsLinks(t *testing.T) {
	profileDir, home := scaffold(t)
	if err := newLinker(t, home, false).Apply(profileDir); err != nil {
		t.Fatal(err)
	}
	if err := newLinker(t, home, true).Remove(profileDir); err != nil {
		t.Fatalf("Remove dry-run: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err != nil {
		t.Error("dry-run Remove should not delete links")
	}
}

func TestApplyErrorsWhenProfileMissing(t *testing.T) {
	_, home := scaffold(t)
	err := newLinker(t, home, false).Apply(filepath.Join(home, "no-such-profile"))
	if err == nil {
		t.Error("expected error for missing profile dir")
	}
}

func TestApplyTopLevelDirLeafLinks(t *testing.T) {
	// A top-level directory (e.g. claude/) leaf-links into ~/.claude/, and the
	// directory itself stays real.
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	home := filepath.Join(root, "home")
	mustWrite(t, filepath.Join(profileDir, "claude", "settings.json"), "{}\n")
	mustMkdirAll(t, home)

	if err := newLinker(t, home, false).Apply(profileDir); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := symlinkTarget(t, filepath.Join(home, ".claude", "settings.json")), filepath.Join(profileDir, "claude", "settings.json"); got != want {
		t.Errorf(".claude/settings.json → %s, want %s", got, want)
	}
	if fi, err := os.Lstat(filepath.Join(home, ".claude")); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Error("~/.claude should be a real directory, not a symlink")
	}
}

func TestApplyGateSkipsConfigChild(t *testing.T) {
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	home := filepath.Join(root, "home")
	mustWrite(t, filepath.Join(profileDir, "config", "bat", "config"), "--theme\n")
	mustWrite(t, filepath.Join(profileDir, "config", "yabai", "yabairc"), "# wm\n")
	mustMkdirAll(t, home)

	l := newLinker(t, home, false)
	l.Gate = func(name string) bool { return name != "yabai" } // yabai absent

	if err := l.Apply(profileDir); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "bat", "config")); err != nil {
		t.Error("config/bat should link (gated in)")
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "yabai")); !os.IsNotExist(err) {
		t.Error("config/yabai should be skipped (gated out)")
	}
}

func TestApplyGateSkipsTopLevelDir(t *testing.T) {
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	home := filepath.Join(root, "home")
	mustWrite(t, filepath.Join(profileDir, "claude", "settings.json"), "{}\n")
	mustMkdirAll(t, home)

	l := newLinker(t, home, false)
	l.Gate = func(name string) bool { return name != "claude" } // claude absent
	if err := l.Apply(profileDir); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Error("~/.claude should be skipped (gated out)")
	}
}

func TestRemoveLeafLinks(t *testing.T) {
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	home := filepath.Join(root, "home")
	mustWrite(t, filepath.Join(profileDir, "config", "bat", "config"), "x\n")
	mustWrite(t, filepath.Join(profileDir, "claude", "settings.json"), "{}\n")
	mustMkdirAll(t, home)

	l := newLinker(t, home, false)
	if err := l.Apply(profileDir); err != nil {
		t.Fatal(err)
	}
	if err := l.Remove(profileDir); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "bat", "config")); !os.IsNotExist(err) {
		t.Error("config/bat/config symlink should be removed")
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Error("claude leaf symlink should be removed")
	}
}

func TestStatus(t *testing.T) {
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	home := filepath.Join(root, "home")
	mustWrite(t, filepath.Join(profileDir, "zshrc"), "# z\n")
	mustMkdirAll(t, home)
	l := newLinker(t, home, false)

	src := filepath.Join(profileDir, "zshrc")
	dst := filepath.Join(home, ".zshrc")
	p := Pair{Src: src, Dst: dst}

	if got := l.Status(p); got != StateMissing {
		t.Errorf("missing: got %v", got)
	}
	if err := l.Apply(profileDir); err != nil {
		t.Fatal(err)
	}
	if got := l.Status(p); got != StateLinked {
		t.Errorf("linked: got %v", got)
	}
	// real file in the way → conflict
	_ = os.Remove(dst)
	mustWrite(t, dst, "real\n")
	if got := l.Status(p); got != StateConflict {
		t.Errorf("conflict: got %v", got)
	}
	// symlink elsewhere → wrong-target
	_ = os.Remove(dst)
	_ = os.Symlink(filepath.Join(home, "elsewhere"), dst)
	if got := l.Status(p); got != StateWrongTarget {
		t.Errorf("wrong-target: got %v", got)
	}
}
