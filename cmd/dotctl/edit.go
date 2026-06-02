package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/platform"
)

func newEditCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Open a managed dotfile by its logical name in $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runEdit(cmd, g, args[0]) },
	}
}

// runEdit resolves a logical name (e.g. "zshrc" or "config/mise/conf.d/tools.toml")
// to its repo source path via the link convention, then opens it in $EDITOR.
// Editing the repo source (not the linked copy) means changes are ready to `save`.
func runEdit(cmd *cobra.Command, g *globals, name string) error {
	log := g.logger()
	cx, err := g.loadCtx()
	if err != nil {
		return err
	}
	home, err := platform.HomeDir()
	if err != nil {
		return err
	}
	profiles := cx.Cfg.Profiles
	if len(profiles) == 0 {
		profiles = []string{machine.DefaultProfile}
	}
	linker, err := g.newLinker(log, cx.Repo, profiles, cx.Cfg)
	if err != nil {
		return err
	}

	// Resolve via Targets; later profiles win on collision (matches apply precedence).
	var src string
	seen := map[string]bool{}
	var known []string
	for _, profile := range profiles {
		pairs, err := linker.Targets(filepath.Join(cx.Repo, machine.ProfilesSubdir, profile))
		if err != nil {
			return err
		}
		for _, p := range pairs {
			logical := logicalName(home, p.Dst)
			if !seen[logical] {
				seen[logical] = true
				known = append(known, logical)
			}
			if logical == name || filepath.Base(logical) == name {
				src = p.Src
			}
		}
	}
	if src == "" {
		sort.Strings(known)
		return fmt.Errorf("no managed file %q; known names: %s", name, strings.Join(known, ", "))
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	fields := strings.Fields(editor)
	args := append(fields[1:], src)
	if g.dryRun {
		log.Plan("edit", strings.Join(append([]string{fields[0]}, args...), " "))
		return nil
	}
	return g.newRunner(log).Run(cmd.Context(), fields[0], args...)
}

// logicalName is a link target's path relative to $HOME with the leading dot
// stripped: ~/.zshrc → "zshrc", ~/.config/x/y → "config/x/y".
func logicalName(home, dst string) string {
	rel, err := filepath.Rel(home, dst)
	if err != nil {
		rel = filepath.Base(dst)
	}
	return strings.TrimPrefix(rel, ".")
}
