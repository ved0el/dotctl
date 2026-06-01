package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/machine"
)

func newNewCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "new",
		Short: "Scaffold a fresh dotfiles repo (profiles skeleton + README)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := g.logger()
			cx, err := g.loadCtx()
			if err != nil {
				return err
			}
			profileRoot := filepath.Join(cx.Repo, machine.ProfilesSubdir)
			if _, err := os.Stat(profileRoot); err == nil {
				return fmt.Errorf("%s already exists — nothing to scaffold", profileRoot)
			}
			if g.dryRun {
				log.Plan("scaffold", cx.Repo+" (profiles/{base,tools,develop}, README.md)")
				return nil
			}
			for _, p := range []string{"base", "tools", "develop"} {
				if err := os.MkdirAll(filepath.Join(profileRoot, p), 0o755); err != nil {
					return fmt.Errorf("create %s: %w", p, err)
				}
				// A keep file + empty manifest so the profile is real and walkable.
				if err := os.WriteFile(filepath.Join(profileRoot, p, "packages.yaml"), []byte("packages: []\n"), 0o644); err != nil {
					return fmt.Errorf("seed %s: %w", p, err)
				}
			}
			readme := "# dotfiles\n\nManaged by [dotctl](https://github.com/ved0el/dotctl).\n\n```sh\ncurl -fsSL https://tinyurl.com/get-dotctl | sh\n```\n"
			if err := os.WriteFile(filepath.Join(cx.Repo, "README.md"), []byte(readme), 0o644); err != nil {
				return fmt.Errorf("write README: %w", err)
			}
			log.OK("scaffolded %s — add dotfiles with 'dotctl add', then 'dotctl save'", cx.Repo)
			return nil
		},
	}
}
