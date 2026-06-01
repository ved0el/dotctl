package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/console"
	"github.com/ved0el/dotctl/internal/engine"
	"github.com/ved0el/dotctl/internal/link"
	"github.com/ved0el/dotctl/internal/machine"
	"github.com/ved0el/dotctl/internal/pkg"
	"github.com/ved0el/dotctl/internal/platform"
)

const defaultRepoName = ".dotfiles"

// globals holds flags shared by every command.
type globals struct {
	dryRun  bool
	verbose bool
}

func newRootCmd() *cobra.Command {
	g := &globals{}
	root := &cobra.Command{
		Use:           "dotctl",
		Short:         "Profile-based dotfiles & environment manager",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Bare `dotctl` reports status (exits non-zero on drift — prompt-friendly).
		RunE: func(cmd *cobra.Command, _ []string) error { return runStatus(cmd, g) },
	}
	root.PersistentFlags().BoolVarP(&g.dryRun, "dry-run", "n", false, "preview actions without making changes")
	root.PersistentFlags().BoolVarP(&g.verbose, "verbose", "v", false, "verbose output")

	root.AddCommand(
		newInitCmd(g),
		newApplyCmd(g),
		newSyncCmd(g),
		newSaveCmd(g),
		newStatusCmd(g),
		newAddCmd(g),
		newDoctorCmd(g),
		newProfileCmd(g),
		newNewCmd(g),
		newLinkCmd(g),
		newUnlinkCmd(g),
		newPkgCmd(g),
		newVersionCmd(),
	)
	return root
}

func (g *globals) logger() *console.Logger { return console.New(os.Stdout, g.verbose) }

// cmdCtx is the resolved context every sub-command needs.
type cmdCtx struct {
	Repo   string
	CfgDir string
	Cfg    machine.Config
}

// loadCtx resolves the repo path, config dir, and machine config. It applies no
// default-profile logic — that differs per command (bootstrap writes, apply
// requires, link/pkg default) and stays in the command bodies.
func (g *globals) loadCtx() (cmdCtx, error) {
	repo, err := repoPath()
	if err != nil {
		return cmdCtx{}, err
	}
	cfgDir, err := configDir()
	if err != nil {
		return cmdCtx{}, err
	}
	cfg, err := machine.Load(cfgDir)
	if err != nil {
		return cmdCtx{}, err
	}
	return cmdCtx{Repo: repo, CfgDir: cfgDir, Cfg: cfg}, nil
}

// repoPath resolves the dotfiles repo root (DOTCTL_REPO or ~/.dotfiles).
func repoPath() (string, error) {
	if v := os.Getenv("DOTCTL_REPO"); v != "" {
		return v, nil
	}
	home, err := platform.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultRepoName), nil
}

// configDir resolves the local config directory (~/.config/dotctl).
func configDir() (string, error) {
	home, err := platform.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dotctl"), nil
}

// newLinker builds a Linker for the current home and dry-run setting, gated so a
// config/<tool> only links when its tool is active on this machine (declared in
// the selected profiles, or the command is already installed).
func (g *globals) newLinker(log *console.Logger, repo string, profiles []string, cfg machine.Config) (*link.Linker, error) {
	home, err := platform.HomeDir()
	if err != nil {
		return nil, err
	}
	l := link.NewLinker(home, link.OsFS{}, g.dryRun, log)
	l.Gate = toolGate(repo, profiles, cfg)
	return l, nil
}

// toolGate reports whether a config/<name> should be linked: true if the tool is
// declared by the selected profiles, or its command is on PATH.
func toolGate(repo string, profiles []string, cfg machine.Config) func(string) bool {
	active := machine.ActiveTools(filepath.Join(repo, machine.ProfilesSubdir), profiles, cfg)
	return func(name string) bool {
		return active[name] || pkg.CommandExists(name)
	}
}

// newRunner returns a real or dry-run command runner.
func (g *globals) newRunner(log *console.Logger) pkg.Runner {
	if g.dryRun {
		return pkg.DryRunner{Log: log}
	}
	return pkg.ExecRunner{}
}

// deps assembles the collaborators reconcile needs, ensuring Manager and Runner
// share the same dry-ness so a dry run never executes a real install.
func (g *globals) deps(log *console.Logger, repo string, profiles []string, cfg machine.Config) (engine.Deps, error) {
	runner := g.newRunner(log)
	mgr, err := pkg.Select(runner)
	if err != nil {
		return engine.Deps{}, err
	}
	linker, err := g.newLinker(log, repo, profiles, cfg)
	if err != nil {
		return engine.Deps{}, err
	}
	return engine.Deps{Linker: linker, Manager: mgr, Runner: runner, Log: log}, nil
}
