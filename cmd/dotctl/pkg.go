package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/engine"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/manifest"
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
			runner := g.newRunner(log)
			mgr, err := pkg.Select(runner)
			if err != nil {
				return err
			}
			// Reuse the engine's custom/managed split so a custom-install package
			// (sheldon, mise) runs its own command instead of being misrouted to
			// the platform manager (brew/apt/dnf).
			if _, failed := engine.InstallSet(cmd.Context(), pkgs, mgr, runner, log); len(failed) > 0 {
				return errors.Join(failed...)
			}
			return nil
		},
	}
	var addProfile string
	add := &cobra.Command{
		Use:   "add <name>...",
		Short: "Add packages to a profile's manifest and install them",
		Args:  cobra.MinimumNArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return pkgMutate(cmd, g, addProfile, args, true) },
	}
	add.Flags().StringVar(&addProfile, "profile", machine.DefaultProfile, "profile to add the packages to")

	var rmProfile string
	rm := &cobra.Command{
		Use:   "rm <name>...",
		Short: "Remove packages from a profile's manifest (does not uninstall)",
		Args:  cobra.MinimumNArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return pkgMutate(cmd, g, rmProfile, args, false) },
	}
	rm.Flags().StringVar(&rmProfile, "profile", machine.DefaultProfile, "profile to remove the packages from")

	pkgCmd.AddCommand(install, add, rm)
	return pkgCmd
}

// pkgMutate adds or removes packages in a profile's packages.yaml. On add it
// also installs the new packages; rm leaves the binaries in place (uninstall is
// destructive and per-machine).
func pkgMutate(cmd *cobra.Command, g *globals, profile string, names []string, add bool) error {
	log := g.logger()
	cx, err := g.loadCtx()
	if err != nil {
		return err
	}
	if err := validateProfileName(profile); err != nil {
		return err
	}
	profileDir := filepath.Join(cx.Repo, machine.ProfilesSubdir, profile)
	pkgs, err := manifest.WalkProfile(profileDir)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, p := range pkgs {
		have[p.Name] = true
	}

	if add {
		var added []manifest.Package
		for _, n := range names {
			if have[n] {
				log.Debug("%s already declared in %s", n, profile)
				continue
			}
			p := manifest.Package{Name: n}
			pkgs = append(pkgs, p)
			added = append(added, p)
		}
		if g.dryRun {
			log.Plan("add packages", fmt.Sprintf("%v → %s", names, profile))
			return nil
		}
		if len(added) == 0 {
			log.OK("all of %v already declared in %s — nothing to do", names, profile)
			return nil
		}
		if err := manifest.WriteProfile(profileDir, pkgs); err != nil {
			return err
		}
		addedNames := make([]string, len(added))
		for i, p := range added {
			addedNames[i] = p.Name
		}
		log.OK("added %v to %s", addedNames, profile)
		mgr, err := pkg.Select(g.newRunner(log))
		if err != nil {
			return err
		}
		return mgr.Install(cmd.Context(), added)
	}

	remove := map[string]bool{}
	for _, n := range names {
		remove[n] = true
	}
	kept := pkgs[:0]
	for _, p := range pkgs {
		if !remove[p.Name] {
			kept = append(kept, p)
		}
	}
	if g.dryRun {
		log.Plan("remove packages", fmt.Sprintf("%v from %s", names, profile))
		return nil
	}
	if err := manifest.WriteProfile(profileDir, kept); err != nil {
		return err
	}
	log.OK("removed %v from %s (not uninstalled)", names, profile)
	return nil
}
