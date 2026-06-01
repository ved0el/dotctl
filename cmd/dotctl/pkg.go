package main

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/pkg"
)

func newPkgCmd(g *globals) *cobra.Command {
	pkgCmd := &cobra.Command{
		Use:   "pkg",
		Short: "Manage packages",
	}
	install := &cobra.Command{
		Use:   "install",
		Short: "Install packages for configured profiles (no symlinking)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := g.logger()
			cx, err := g.loadCtx()
			if err != nil {
				return err
			}
			profiles := cx.Cfg.Profiles
			if len(profiles) == 0 {
				profiles = []string{machine.DefaultProfile}
			}
			profileRoot := filepath.Join(cx.Repo, machine.ProfilesSubdir)
			pkgs, err := machine.ResolvePackages(profileRoot, profiles, cx.Cfg)
			if err != nil {
				return err
			}
			mgr, err := pkg.Select(g.newRunner(log))
			if err != nil {
				return err
			}
			log.Step("installing %d package(s) via %s", len(pkgs), mgr.Name())
			return mgr.Install(cmd.Context(), pkgs)
		},
	}
	pkgCmd.AddCommand(install)
	return pkgCmd
}
