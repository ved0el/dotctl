package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/console"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/platform"
)

func newAddCmd(g *globals) *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   "add <path>...",
		Short: "Adopt existing dotfiles into a profile (move into the repo, then symlink back)",
		Args:  cobra.MinimumNArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runAdd(g, profile, args) },
	}
	cmd.Flags().StringVar(&profile, "profile", machine.DefaultProfile, "profile to adopt the files into")
	return cmd
}

func runAdd(g *globals, profile string, paths []string) error {
	log := g.logger()
	cx, err := g.loadCtx()
	if err != nil {
		return err
	}
	if err := validateProfileName(profile); err != nil {
		return err
	}
	home, err := platform.HomeDir()
	if err != nil {
		return err
	}
	profileDir := filepath.Join(cx.Repo, machine.ProfilesSubdir, profile)

	for _, raw := range paths {
		abs, err := resolveHomePath(raw, home)
		if err != nil {
			return err
		}
		rel, err := homeRel(home, abs)
		if err != nil {
			return err
		}
		fi, err := os.Lstat(abs)
		if err != nil {
			return fmt.Errorf("%s: %w", abs, err)
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			log.Warn("skip %s (already a symlink)", abs)
		case fi.IsDir():
			// Adopt a directory the way the link engine maps one: leaf-by-leaf,
			// leaving the directory itself real. A whole-directory symlink would
			// make a later `apply` back up the repo's own files through the link.
			if err := adoptDir(g, log, abs, home, profileDir); err != nil {
				return err
			}
		default:
			if err := adoptFile(g, log, abs, repoDest(profileDir, rel)); err != nil {
				return err
			}
		}
	}
	return nil
}

// homeRel returns abs relative to home, requiring it to be a dotfile under $HOME.
func homeRel(home, abs string) (string, error) {
	rel, err := filepath.Rel(home, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%s is not under %s", abs, home)
	}
	if !strings.HasPrefix(rel, ".") {
		return "", fmt.Errorf("%s is not a dotfile (no leading dot)", abs)
	}
	return rel, nil
}

// repoDest maps a $HOME-relative dotfile path to its repo location, dropping the
// leading dot: ~/.config/x → <profile>/config/x, ~/.zshrc → <profile>/zshrc.
func repoDest(profileDir, rel string) string {
	return filepath.Join(profileDir, strings.TrimPrefix(rel, "."))
}

// adoptDir walks a real directory and adopts each leaf file individually, so the
// directory stays a real dir (matching link.walkLeaves). Existing symlinks inside
// it are skipped (already adopted). The file list is collected before any move so
// mutation never disturbs the walk.
func adoptDir(g *globals, log *console.Logger, dir, home, profileDir string) error {
	var leaves []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			log.Warn("skip %s (already a symlink)", p)
			return nil
		}
		leaves = append(leaves, p)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", dir, err)
	}
	for _, leaf := range leaves {
		rel, err := homeRel(home, leaf)
		if err != nil {
			return err
		}
		if err := adoptFile(g, log, leaf, repoDest(profileDir, rel)); err != nil {
			return err
		}
	}
	return nil
}

// adoptFile moves a single real file into the repo and symlinks the original back
// to it, creating real intermediate directories on both sides.
func adoptFile(g *globals, log *console.Logger, abs, dest string) error {
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("%s already exists in the repo — remove it first", dest)
	}
	if g.dryRun {
		log.Plan("adopt", fmt.Sprintf("%s → %s", abs, dest))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
	}
	if err := os.Rename(abs, dest); err != nil {
		return fmt.Errorf("move %s into repo (must be on the same filesystem): %w", abs, err)
	}
	if err := os.Symlink(dest, abs); err != nil {
		return fmt.Errorf("symlink %s (the original is now at %s): %w", abs, dest, err)
	}
	log.OK("adopted %s → %s", abs, dest)
	return nil
}

// resolveHomePath expands a leading ~ and makes the path absolute.
func resolveHomePath(raw, home string) (string, error) {
	switch {
	case raw == "~":
		return home, nil
	case strings.HasPrefix(raw, "~/"):
		return filepath.Join(home, raw[2:]), nil
	}
	return filepath.Abs(raw)
}
