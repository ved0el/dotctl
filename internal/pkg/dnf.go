package pkg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ved0el/dotctl/internal/manifest"
)

type dnfManager struct{ r Runner }

func (dnfManager) Name() string { return "dnf" }

func (dnfManager) Available() bool {
	_, err := exec.LookPath("dnf")
	return err == nil
}

func (m dnfManager) Install(ctx context.Context, pkgs []manifest.Package) error {
	names := pkgNames(supported(pkgs, "dnf"), func(p manifest.Package) string { return p.Dnf })
	if len(names) == 0 {
		return nil
	}
	name, args := "dnf", append([]string{"install", "-y"}, names...)
	if os.Geteuid() != 0 {
		name, args = "sudo", append([]string{"dnf", "install", "-y"}, names...)
	}
	if err := m.r.Run(ctx, name, args...); err != nil {
		return fmt.Errorf("dnf install: %w", err)
	}
	return nil
}

func (m dnfManager) IsInstalled(ctx context.Context, p manifest.Package) (bool, error) {
	if p.Skipped("dnf") {
		return true, nil
	}
	name := p.Dnf
	if name == "" {
		name = p.Name
	}
	out, err := m.r.Output(ctx, "rpm", "-q", name)
	if err != nil {
		return false, nil // not installed (rpm -q exits non-zero)
	}
	return !strings.Contains(string(out), "not installed"), nil
}
