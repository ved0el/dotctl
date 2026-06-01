package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/link"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/pkg"
	"github.com/ved0el/dotctl/internal/platform"
)

func newDoctorCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose environment problems (PATH, package manager, broken links, repo state)",
		RunE:  func(cmd *cobra.Command, _ []string) error { return runDoctor(g) },
	}
}

func runDoctor(g *globals) error {
	log := g.logger()
	cx, err := g.loadCtx()
	if err != nil {
		return err
	}
	home, err := platform.HomeDir()
	if err != nil {
		return err
	}

	problems := 0
	check := func(ok bool, okMsg, failMsg string) {
		if ok {
			log.OK("%s", okMsg)
		} else {
			problems++
			log.Warn("%s", failMsg)
		}
	}

	check(isDir(cx.Repo), "repo: "+cx.Repo, "repo not found at "+cx.Repo+" (run the installer)")
	check(isDir(filepath.Join(cx.Repo, ".git")), "repo is a git checkout", "repo is not a git checkout — sync/save need git")
	check(pkg.CommandExists("dotctl"), "dotctl is on PATH", "dotctl not on PATH — add ~/.local/bin to PATH")
	check(pathHasLocalBin(home), "~/.local/bin is on PATH", "~/.local/bin not on PATH — self-installed tools (mise, sheldon) won't be found")

	if mgr, err := pkg.Select(platform.OS(), pkg.ExecRunner{}); err == nil {
		check(mgr.Available(), "package manager available: "+mgr.Name(), "package manager not found on PATH: "+mgr.Name())
	} else {
		problems++
		log.Warn("no supported package manager for %s", platform.OS())
	}

	profiles := cx.Cfg.Profiles
	if len(profiles) == 0 {
		log.Warn("no profiles configured — run 'dotctl init --profiles ...'")
		problems++
		profiles = []string{machine.DefaultProfile}
	} else {
		log.OK("profiles: %s", strings.Join(profiles, ", "))
	}

	linker, err := g.newLinker(log, cx.Repo, profiles, cx.Cfg)
	if err != nil {
		return err
	}
	broken := 0
	for _, profile := range profiles {
		pairs, err := linker.Targets(filepath.Join(cx.Repo, machine.ProfilesSubdir, profile))
		if err != nil {
			return err
		}
		for _, p := range pairs {
			if st := linker.Status(p); st == link.StateWrongTarget || st == link.StateConflict {
				broken++
			}
		}
	}
	check(broken == 0, "no broken or conflicting links", fmt.Sprintf("%d broken/conflicting link(s) — run 'dotctl status' then 'dotctl apply'", broken))

	if problems > 0 {
		return fmt.Errorf("doctor found %d problem(s)", problems)
	}
	log.OK("all checks passed")
	return nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func pathHasLocalBin(home string) bool {
	want := filepath.Join(home, ".local", "bin")
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == want {
			return true
		}
	}
	return false
}
