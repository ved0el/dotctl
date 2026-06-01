package main

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/engine"
	"github.com/ved0el/dotctl/internal/machine"
)

func newInitCmd(g *globals) *cobra.Command {
	var profiles []string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize this machine: install packages and link dotfiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := g.logger()
			cx, err := g.loadCtx()
			if err != nil {
				return err
			}

			// Profile precedence: --profiles flag > machine.yaml > default.
			chosen := profiles
			if len(chosen) == 0 {
				chosen = cx.Cfg.Profiles
			}
			if len(chosen) == 0 {
				chosen = []string{machine.DefaultProfile}
			}
			if err := machine.Validate(filepath.Join(cx.Repo, machine.ProfilesSubdir), chosen); err != nil {
				return err
			}

			// Persist selection on first run or when explicitly provided.
			if !g.dryRun && (len(profiles) > 0 || len(cx.Cfg.Profiles) == 0) {
				cx.Cfg.Profiles = chosen
				if cx.Cfg.Repo == "" {
					cx.Cfg.Repo = cx.Repo
				}
				if err := machine.Save(cx.CfgDir, cx.Cfg); err != nil {
					return err
				}
				log.OK("wrote machine config: profiles [%s]", strings.Join(chosen, ", "))
			}

			deps, err := g.deps(log, cx.Repo, chosen, cx.Cfg)
			if err != nil {
				return err
			}
			return engine.Run(cmd.Context(), engine.Options{Repo: cx.Repo, Profiles: chosen, Overlay: filepath.Join(cx.CfgDir, "local")}, cx.Cfg, deps)
		},
	}
	cmd.Flags().StringSliceVar(&profiles, "profiles", nil, "comma-separated profiles to apply (e.g. base,develop)")
	return cmd
}
