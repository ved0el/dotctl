package main

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/engine"
	"github.com/ved0el/dotctl/internal/machine"
)

func newUpgradeCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade installed packages to their latest versions, then re-apply",
		Long: "Upgrade the packages this machine has installed to their latest versions " +
			"(managed via the platform manager; sheldon/mise refresh themselves), then " +
			"re-link and re-run hooks. Unlike 'sync', it does not git pull — run 'sync' " +
			"to update the repo, 'upgrade' to update package versions.",
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
			return engine.Upgrade(cmd.Context(), engine.Options{
				Repo:     cx.Repo,
				Profiles: cx.Cfg.Profiles,
				Overlay:  filepath.Join(cx.CfgDir, "local"),
			}, cx.Cfg, deps)
		},
	}
}
