package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	lnk "github.com/ved0el/dotctl/internal/link"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/manifest"
	"github.com/ved0el/dotctl/internal/pkg"
	"github.com/ved0el/dotctl/internal/platform"
)

func newStatusCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Aliases: []string{"st"},
		Short:   "Show drift: which links and packages are present, missing, or wrong",
		RunE:    func(cmd *cobra.Command, _ []string) error { return runStatus(cmd, g) },
	}
}

// runStatus reports drift without changing anything and returns a non-nil error
// when the machine has drifted (so it exits non-zero — handy in a shell prompt).
func runStatus(cmd *cobra.Command, g *globals) error {
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

	// Links: classify every declared target.
	drift := 0
	for _, profile := range profiles {
		pairs, err := linker.Targets(filepath.Join(cx.Repo, machine.ProfilesSubdir, profile))
		if err != nil {
			return err
		}
		for _, p := range pairs {
			switch st := linker.Status(p); st {
			case lnk.StateLinked:
				log.Debug("ok    %s", p.Dst)
			default:
				drift++
				log.Warn("%-12s %s", st, p.Dst)
			}
		}
	}

	// Packages: which declared packages are missing.
	pkgs, err := machine.ResolvePackages(filepath.Join(cx.Repo, machine.ProfilesSubdir), profiles, cx.Cfg)
	if err != nil {
		return err
	}
	mgr, mgrErr := pkg.Select(platform.OS(), pkg.ExecRunner{})
	missingPkgs := 0
	for _, p := range pkgs {
		if !packageInstalled(cmd, mgr, mgrErr, p) {
			missingPkgs++
			log.Warn("%-12s %s", "pkg-missing", p.Name)
		}
	}

	if drift == 0 && missingPkgs == 0 {
		log.OK("in sync — %d links, %d packages", countLinks(linker, cx.Repo, profiles), len(pkgs))
		return nil
	}
	return fmt.Errorf("drift: %d link(s), %d package(s) need attention — run 'dotctl apply'", drift, missingPkgs)
}

// packageInstalled checks a package's presence: custom-install packages by
// command on PATH, managed packages via the platform manager.
func packageInstalled(cmd *cobra.Command, mgr pkg.Manager, mgrErr error, p manifest.Package) bool {
	if p.Custom() {
		return pkg.CommandExists(p.Name)
	}
	if mgrErr != nil {
		return false
	}
	ok, _ := mgr.IsInstalled(cmd.Context(), p)
	return ok
}

func countLinks(l *lnk.Linker, repo string, profiles []string) int {
	n := 0
	for _, profile := range profiles {
		if pairs, err := l.Targets(filepath.Join(repo, machine.ProfilesSubdir, profile)); err == nil {
			n += len(pairs)
		}
	}
	return n
}
