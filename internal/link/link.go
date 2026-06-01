// Package link implements the modified-Stow symlink engine: it maps a profile
// tree into the home directory and removes those links again.
//
// Convention (dotfiles stored without leading dots in the repo):
//   - <profile>/<file>    → ~/.<file>   (top-level file, linked as a unit)
//   - <profile>/<dir>/…   → ~/.<dir>/…  (top-level dir, leaf-linked; e.g. claude/)
//   - <profile>/config/<x> → ~/.config/<x>  (XDG, gated per child)
//
// Linking is idempotent and never clobbers: a real file in a link's path is
// moved into a per-run, path-preserving backup directory before the symlink is
// created.
package link

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ved0el/dotctl/internal/console"
)

// Linker applies and removes symlinks for a single profile.
type Linker struct {
	Home   string
	FS     FS
	Now    func() time.Time
	DryRun bool
	Log    *console.Logger
	// Gate, if set, reports whether a config/<name> subtree should be linked
	// (e.g. only if the owning command is installed). nil → link everything.
	Gate func(name string) bool
}

// NewLinker builds a Linker for the given home directory, defaulting the clock
// to time.Now. Tests may override the Now field afterwards for deterministic
// backup paths.
func NewLinker(home string, fs FS, dryRun bool, log *console.Logger) *Linker {
	return &Linker{Home: home, FS: fs, Now: time.Now, DryRun: dryRun, Log: log}
}

// Pair is a single source→destination symlink mapping.
type Pair struct{ Src, Dst string }

// Targets returns the source→destination pairs a profile maps into $HOME. It is
// exported so other tools (status checks, tests) can derive the expected links
// from the repo without duplicating the convention.
//
// Resolution:
//   - A top-level FILE `name` → `~/.name` (linked as a unit; e.g. zshrc → ~/.zshrc).
//   - A top-level DIRECTORY `name` → leaf-linked into `~/.name/` (e.g. claude/ →
//     ~/.claude/), so the dir stays real and apps can write state without touching
//     the repo. `config` is the special case mapping to `~/.config/`.
//   - Each leaf under a directory maps to its matching path; intermediate dirs are
//     created real, letting multiple profiles share a target dir without clobbering.
//
// Directories are gated (see Gate): `config/<tool>` per child, other top-level
// dirs by the dir name — so a tool's config only links when the tool is active.
func (l *Linker) Targets(profileDir string) ([]Pair, error) {
	entries, err := l.FS.ReadDir(profileDir)
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", profileDir, err)
	}
	var pairs []Pair
	for _, e := range entries {
		name := e.Name()
		if name == "packages.yaml" {
			continue
		}
		if !e.IsDir() {
			// Top-level file → ~/.name (unit). Core dotfiles, not gated.
			pairs = append(pairs, Pair{Src: filepath.Join(profileDir, name), Dst: filepath.Join(l.Home, "."+name)})
			continue
		}
		dstRoot := filepath.Join(l.Home, "."+name) // claude → ~/.claude, config → ~/.config
		if name == "config" {
			// XDG dir: gate per immediate child (each child is a distinct tool).
			children, err := l.FS.ReadDir(filepath.Join(profileDir, "config"))
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", filepath.Join(profileDir, "config"), err)
			}
			for _, c := range children {
				sub, err := l.gatedLeaves(c, filepath.Join(profileDir, "config"), dstRoot)
				if err != nil {
					return nil, err
				}
				pairs = append(pairs, sub...)
			}
			continue
		}
		// Other top-level dir (e.g. claude) → gate on the dir name, leaf-link to ~/.name.
		if !l.gated(name) {
			l.Log.Debug("skip %s (command not found)", name)
			continue
		}
		leaves, err := l.walkLeaves(filepath.Join(profileDir, name), dstRoot)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, leaves...)
	}
	return pairs, nil
}

// gatedLeaves links a single child of an XDG dir (gated on the child name), as a
// leaf file or a recursively-walked subtree.
func (l *Linker) gatedLeaves(child os.DirEntry, srcDir, dstDir string) ([]Pair, error) {
	if !l.gated(child.Name()) {
		l.Log.Debug("skip config/%s (command not found)", child.Name())
		return nil, nil
	}
	src, dst := filepath.Join(srcDir, child.Name()), filepath.Join(dstDir, child.Name())
	if child.IsDir() {
		return l.walkLeaves(src, dst)
	}
	return []Pair{{Src: src, Dst: dst}}, nil
}

// gated reports whether an entry named name should be linked (Gate is optional;
// nil means link everything).
func (l *Linker) gated(name string) bool { return l.Gate == nil || l.Gate(name) }

// walkLeaves recurses srcDir, returning one Pair per file mapped under dstDir.
// Intermediate directories become real dirs (created by linkOne's MkdirAll), not
// symlinks, so sibling files from other profiles coexist in the same target dir.
// It does not follow symlinks in the source tree (ReadDir reports symlinks as
// non-dirs), which is intentional — repo profiles contain real files and dirs.
func (l *Linker) walkLeaves(srcDir, dstDir string) ([]Pair, error) {
	entries, err := l.FS.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", srcDir, err)
	}
	var pairs []Pair
	for _, e := range entries {
		src, dst := filepath.Join(srcDir, e.Name()), filepath.Join(dstDir, e.Name())
		if e.IsDir() {
			sub, err := l.walkLeaves(src, dst)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, sub...)
			continue
		}
		pairs = append(pairs, Pair{Src: src, Dst: dst})
	}
	return pairs, nil
}

// Apply links every entry of profileDir into $HOME, backing up any conflicts into
// a single per-run backup directory.
func (l *Linker) Apply(profileDir string) error {
	pairs, err := l.Targets(profileDir)
	if err != nil {
		return err
	}
	backupDir := filepath.Join(l.Home, ".dotfiles-backup", strconv.FormatInt(l.Now().Unix(), 10))
	for i, p := range pairs {
		if err := l.linkOne(p.Src, p.Dst, backupDir); err != nil {
			// Partial apply: $HOME is half-converged. Surface exactly where it
			// stopped and where to recover any moved originals, then re-run.
			l.Log.Warn("link failed after %d/%d entries (at %s)", i, len(pairs), p.Dst)
			if _, e := l.FS.Lstat(backupDir); e == nil {
				l.Log.Warn("displaced originals are under %s — recover them there; re-run to retry", backupDir)
			}
			return fmt.Errorf("apply %s: %w", profileDir, err)
		}
	}
	return nil
}

func (l *Linker) linkOne(src, dst, backupDir string) error {
	fi, err := l.FS.Lstat(dst)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// nothing in the way
	case err != nil:
		return fmt.Errorf("stat %s: %w", dst, err)
	case fi.Mode()&os.ModeSymlink != 0:
		// existing symlink: skip if already correct, else replace (handles dangling)
		target, err := l.FS.Readlink(dst)
		if err != nil {
			return fmt.Errorf("readlink %s: %w", dst, err)
		}
		if target == src {
			l.Log.Debug("skip %s (already linked)", dst)
			return nil
		}
		if l.DryRun {
			l.Log.Plan("relink", dst)
			return nil
		}
		if err := l.FS.Remove(dst); err != nil {
			return fmt.Errorf("remove stale link %s: %w", dst, err)
		}
	default:
		// a real file or directory: back it up first
		if l.DryRun {
			l.Log.Plan("backup+link", dst)
			return nil
		}
		if err := l.backup(dst, backupDir); err != nil {
			return err
		}
	}

	if l.DryRun {
		l.Log.Plan("link", dst)
		return nil
	}
	if err := l.FS.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent of %s: %w", dst, err)
	}
	if err := l.FS.Symlink(src, dst); err != nil {
		return fmt.Errorf("link %s -> %s: %w", src, dst, err)
	}
	l.Log.OK("linked %s", dst)
	return nil
}

func (l *Linker) backup(dst, backupDir string) error {
	// Preserve the path relative to $HOME so leaf files that share a basename
	// (e.g. config/mise/conf.d/tools.toml) don't collide in the backup dir.
	rel, err := filepath.Rel(l.Home, dst)
	if err != nil {
		rel = filepath.Base(dst)
	}
	target := filepath.Join(backupDir, rel)
	if err := l.FS.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	if err := l.FS.Rename(dst, target); err != nil {
		return fmt.Errorf("back up %s: %w", dst, err)
	}
	l.Log.Warn("backed up %s -> %s", dst, target)
	return nil
}

// Remove deletes symlinks that point into profileDir. It is idempotent (missing
// links are ignored) and never touches real files. Intermediate directories that
// Apply created (e.g. ~/.config/mise/conf.d/) are left in place — other tools may
// rely on them, and an empty dir is harmless.
func (l *Linker) Remove(profileDir string) error {
	pairs, err := l.Targets(profileDir)
	if err != nil {
		return err
	}
	for _, p := range pairs {
		fi, err := l.FS.Lstat(p.Dst)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("stat %s: %w", p.Dst, err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			continue // a real file, not ours
		}
		target, err := l.FS.Readlink(p.Dst)
		if err != nil {
			return fmt.Errorf("readlink %s: %w", p.Dst, err)
		}
		if target != p.Src {
			continue // points elsewhere
		}
		if l.DryRun {
			l.Log.Plan("unlink", p.Dst)
			continue
		}
		if err := l.FS.Remove(p.Dst); err != nil {
			return fmt.Errorf("unlink %s: %w", p.Dst, err)
		}
		l.Log.OK("unlinked %s", p.Dst)
	}
	return nil
}
