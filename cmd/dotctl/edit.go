package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/console"
	"github.com/ved0el/dotctl/internal/link"
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

	src, known, err := resolveManaged(linker, cx.Repo, profiles, home, name)
	if err != nil {
		return err
	}
	if src == "" {
		return g.editNotFound(log, cx.Repo, profiles, home, name, known)
	}

	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	fields := strings.Fields(editor) // non-empty: editor is non-empty after trim
	args := append(fields[1:], src)
	if g.dryRun {
		log.Plan("edit", strings.Join(append([]string{fields[0]}, args...), " "))
		return nil
	}
	if _, err := exec.LookPath(fields[0]); err != nil {
		return fmt.Errorf("editor %q not found — check $EDITOR (a path with spaces needs a no-space wrapper binary): %w", fields[0], err)
	}
	return g.newRunner(log).Run(cmd.Context(), fields[0], args...)
}

// resolveManaged maps a logical name to a repo source path. An exact logical-path
// match wins (later profile overrides, matching apply precedence); otherwise a
// unique leaf basename match is used. A basename matching two distinct files is
// ambiguous and errors rather than silently opening the wrong one. Returns the
// resolved src ("" if none) and the sorted set of known logical names.
func resolveManaged(linker *link.Linker, repo string, profiles []string, home, name string) (string, []string, error) {
	var exactSrc string
	byBasename := map[string]string{} // logical name -> src (later profile wins)
	seen := map[string]bool{}
	var known []string
	for _, profile := range profiles {
		pairs, err := linker.Targets(filepath.Join(repo, machine.ProfilesSubdir, profile))
		if err != nil {
			return "", nil, err
		}
		for _, p := range pairs {
			logical := logicalName(home, p.Dst)
			if !seen[logical] {
				seen[logical] = true
				known = append(known, logical)
			}
			switch {
			case logical == name:
				exactSrc = p.Src
			case filepath.Base(logical) == name:
				byBasename[logical] = p.Src
			}
		}
	}
	sort.Strings(known)
	if exactSrc != "" {
		return exactSrc, known, nil
	}
	switch len(byBasename) {
	case 0:
		return "", known, nil
	case 1:
		for _, s := range byBasename {
			return s, known, nil
		}
	}
	ambiguous := make([]string, 0, len(byBasename))
	for l := range byBasename {
		ambiguous = append(ambiguous, l)
	}
	sort.Strings(ambiguous)
	return "", known, fmt.Errorf("ambiguous name %q matches %s — use the full path", name, strings.Join(ambiguous, ", "))
}

// editNotFound builds the "no such file" error, distinguishing a name that exists
// in the repo but was gated off (its tool inactive on this machine) via an ungated
// re-scan, so the message points the user at the real cause.
func (g *globals) editNotFound(log *console.Logger, repo string, profiles []string, home, name string, known []string) error {
	if ungated, err := g.newOverlayLinker(log); err == nil {
		for _, profile := range profiles {
			pairs, err := ungated.Targets(filepath.Join(repo, machine.ProfilesSubdir, profile))
			if err != nil {
				continue
			}
			for _, p := range pairs {
				l := logicalName(home, p.Dst)
				if l == name || filepath.Base(l) == name {
					return fmt.Errorf("%q exists but is gated off (its tool isn't installed or declared in an active profile); add it to a profile's packages.yaml or install the tool", name)
				}
			}
		}
	}
	return fmt.Errorf("no managed file %q; known names: %s", name, strings.Join(known, ", "))
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
