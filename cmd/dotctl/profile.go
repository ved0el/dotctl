package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/machine"
)

func newProfileCmd(g *globals) *cobra.Command {
	c := &cobra.Command{Use: "profile", Short: "Manage which profiles this machine applies"}
	c.AddCommand(
		&cobra.Command{
			Use:   "ls",
			Short: "List the profiles this machine applies",
			RunE: func(cmd *cobra.Command, _ []string) error {
				cx, err := g.loadCtx()
				if err != nil {
					return err
				}
				if len(cx.Cfg.Profiles) == 0 {
					g.logger().Warn("no profiles configured")
					return nil
				}
				fmt.Println(strings.Join(cx.Cfg.Profiles, "\n"))
				return nil
			},
		},
		&cobra.Command{
			Use:   "add <profile>...",
			Short: "Add profiles to this machine (validated, then persisted)",
			Args:  cobra.MinimumNArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return mutateProfiles(g, args, true) },
		},
		&cobra.Command{
			Use:   "rm <profile>...",
			Short: "Remove profiles from this machine",
			Args:  cobra.MinimumNArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return mutateProfiles(g, args, false) },
		},
	)
	return c
}

func mutateProfiles(g *globals, names []string, add bool) error {
	cx, err := g.loadCtx()
	if err != nil {
		return err
	}
	set := map[string]bool{}
	for _, p := range cx.Cfg.Profiles {
		set[p] = true
	}
	if add {
		if err := machine.Validate(filepath.Join(cx.Repo, machine.ProfilesSubdir), names); err != nil {
			return err
		}
		for _, n := range names {
			set[n] = true
		}
	} else {
		for _, n := range names {
			delete(set, n)
		}
	}
	// Preserve a stable order: keep existing order, then append new ones.
	var out []string
	seen := map[string]bool{}
	for _, p := range append(append([]string{}, cx.Cfg.Profiles...), names...) {
		if set[p] && !seen[p] {
			out = append(out, p)
			seen[p] = true
		}
	}
	cx.Cfg.Profiles = out
	if cx.Cfg.Repo == "" {
		cx.Cfg.Repo = cx.Repo
	}
	if err := machine.Save(cx.CfgDir, cx.Cfg); err != nil {
		return err
	}
	g.logger().OK("profiles: [%s] — run 'dotctl apply' to converge", strings.Join(out, ", "))
	return nil
}
