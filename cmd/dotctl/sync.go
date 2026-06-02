package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ved0el/dotctl/internal/engine"
	"github.com/ved0el/dotctl/internal/machine"
)

func newSyncCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "git pull the repo, then re-converge this machine",
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := g.logger()
			cx, err := g.loadCtx()
			if err != nil {
				return err
			}
			if len(cx.Cfg.Profiles) == 0 {
				return machine.ErrNotBootstrapped
			}
			runner := g.newRunner(log)
			log.Step("pulling %s", cx.Repo)
			if err := runner.Run(cmd.Context(), "git", "-C", cx.Repo, "pull", "--ff-only"); err != nil {
				return fmt.Errorf("git pull: %w", err)
			}
			deps, err := g.deps(log, cx.Repo, cx.Cfg.Profiles, cx.Cfg)
			if err != nil {
				return err
			}
			return engine.Run(cmd.Context(), engine.Options{Repo: cx.Repo, Profiles: cx.Cfg.Profiles, Overlay: filepath.Join(cx.CfgDir, "local")}, cx.Cfg, deps)
		},
	}
}

func newSaveCmd(g *globals) *cobra.Command {
	var msg string
	cmd := &cobra.Command{
		Use:   "save",
		Short: "Commit and push your dotfiles changes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := g.logger()
			cx, err := g.loadCtx()
			if err != nil {
				return err
			}
			runner := g.newRunner(log)

			if g.dryRun {
				log.Plan("run", "git -C "+cx.Repo+" add -A && commit -m "+fmt.Sprintf("%q", msg)+" && push")
				return nil
			}
			if err := runner.Run(cmd.Context(), "git", "-C", cx.Repo, "add", "-A"); err != nil {
				return fmt.Errorf("git add: %w", err)
			}
			out, _ := runner.Output(cmd.Context(), "git", "-C", cx.Repo, "status", "--porcelain")
			if strings.TrimSpace(string(out)) == "" {
				log.OK("nothing to save — working tree is clean")
				return nil
			}
			if err := runner.Run(cmd.Context(), "git", "-C", cx.Repo, "commit", "-m", msg); err != nil {
				return fmt.Errorf("git commit: %w", err)
			}
			if err := runner.Run(cmd.Context(), "git", "-C", cx.Repo, "push"); err != nil {
				return fmt.Errorf("git push: %w", err)
			}
			log.OK("saved and pushed")
			return nil
		},
	}
	cmd.Flags().StringVarP(&msg, "message", "m", "chore: update dotfiles", "commit message")
	return cmd
}
