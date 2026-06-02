package pkg

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/ved0el/dotctl/internal/manifest"
)

type brewManager struct{ r Runner }

func (brewManager) Name() string { return "brew" }

func (brewManager) Available() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

func (m brewManager) Install(ctx context.Context, pkgs []manifest.Package) error {
	names := pkgNames(supported(pkgs, "brew"), func(p manifest.Package) string { return p.Brew })
	if len(names) == 0 {
		return nil
	}
	if err := m.r.Run(ctx, "brew", append([]string{"install"}, names...)...); err != nil {
		return fmt.Errorf("brew install: %w", err)
	}
	return nil
}

func (m brewManager) Upgrade(ctx context.Context, pkgs []manifest.Package) error {
	names := pkgNames(supported(pkgs, "brew"), func(p manifest.Package) string { return p.Brew })
	if len(names) == 0 {
		return nil
	}
	if err := m.r.Run(ctx, "brew", append([]string{"upgrade"}, names...)...); err != nil {
		return fmt.Errorf("brew upgrade: %w", err)
	}
	return nil
}

func (m brewManager) IsInstalled(ctx context.Context, p manifest.Package) (bool, error) {
	if p.Skipped("brew") {
		return true, nil // not managed here
	}
	name := p.Brew
	if name == "" {
		name = p.Name
	}
	// `brew list --versions <name>` exits non-zero when the formula is absent.
	_, err := m.r.Output(ctx, "brew", "list", "--versions", name)
	return err == nil, nil
}
