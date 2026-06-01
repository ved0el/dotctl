package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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
		rel, err := filepath.Rel(home, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("%s is not under %s", abs, home)
		}
		if !strings.HasPrefix(rel, ".") {
			return fmt.Errorf("%s is not a dotfile (no leading dot)", abs)
		}
		dest := filepath.Join(profileDir, strings.TrimPrefix(rel, ".")) // ~/.config/x → <profile>/config/x

		fi, err := os.Lstat(abs)
		if err != nil {
			return fmt.Errorf("%s: %w", abs, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			log.Warn("skip %s (already a symlink)", abs)
			continue
		}
		if _, err := os.Lstat(dest); err == nil {
			return fmt.Errorf("%s already exists in the repo — remove it first", dest)
		}

		if g.dryRun {
			log.Plan("adopt", fmt.Sprintf("%s → %s", abs, dest))
			continue
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
	}
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
