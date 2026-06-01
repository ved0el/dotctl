package main

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/machine"
)

func newLinkCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "link",
		Short: "Create symlinks for configured profiles (no package install)",
		RunE:  func(cmd *cobra.Command, _ []string) error { return linkAll(g, false) },
	}
}

func newUnlinkCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "unlink",
		Short: "Remove symlinks created from configured profiles",
		RunE:  func(cmd *cobra.Command, _ []string) error { return linkAll(g, true) },
	}
}

// linkAll applies or removes symlinks for every configured profile.
func linkAll(g *globals, remove bool) error {
	log := g.logger()
	cx, err := g.loadCtx()
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
	for _, profile := range profiles {
		dir := filepath.Join(cx.Repo, machine.ProfilesSubdir, profile)
		if remove {
			err = linker.Remove(dir)
		} else {
			err = linker.Apply(dir)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
