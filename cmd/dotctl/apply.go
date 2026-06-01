package main

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/engine"
	"github.com/ved0el/dotctl/internal/machine"
)

func newApplyCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Re-converge this machine to its configured state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := g.logger()
			cx, err := g.loadCtx()
			if err != nil {
				return err
			}
			if len(cx.Cfg.Profiles) == 0 {
				return machine.ErrNotBootstrapped
			}
			deps, err := g.deps(log, cx.Repo, cx.Cfg.Profiles, cx.Cfg)
			if err != nil {
				return err
			}
			return engine.Run(cmd.Context(), engine.Options{Repo: cx.Repo, Profiles: cx.Cfg.Profiles, Overlay: filepath.Join(cx.CfgDir, "local")}, cx.Cfg, deps)
		},
	}
}
