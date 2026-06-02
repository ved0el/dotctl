package pkg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ved0el/dotctl/internal/manifest"
)

type aptManager struct{ r Runner }

func (aptManager) Name() string { return "apt" }

func (aptManager) Available() bool {
	_, err := exec.LookPath("apt-get")
	return err == nil
}

func (m aptManager) Install(ctx context.Context, pkgs []manifest.Package) error {
	names := pkgNames(supported(pkgs, "apt"), func(p manifest.Package) string { return p.Apt })
	if len(names) == 0 {
		return nil
	}
	// apt-get needs root; prefix sudo when we are not already root.
	name, args := "apt-get", append([]string{"install", "-y"}, names...)
	if os.Geteuid() != 0 {
		name, args = "sudo", append([]string{"apt-get", "install", "-y"}, names...)
	}
	if err := m.r.Run(ctx, name, args...); err != nil {
		return fmt.Errorf("apt-get install: %w", err)
	}
	return nil
}

func (m aptManager) Upgrade(ctx context.Context, pkgs []manifest.Package) error {
	names := pkgNames(supported(pkgs, "apt"), func(p manifest.Package) string { return p.Apt })
	if len(names) == 0 {
		return nil
	}
	// --only-upgrade upgrades the named packages without installing new ones.
	name, args := "apt-get", append([]string{"install", "--only-upgrade", "-y"}, names...)
	if os.Geteuid() != 0 {
		name, args = "sudo", append([]string{"apt-get", "install", "--only-upgrade", "-y"}, names...)
	}
	if err := m.r.Run(ctx, name, args...); err != nil {
		return fmt.Errorf("apt-get upgrade: %w", err)
	}
	return nil
}

func (m aptManager) IsInstalled(ctx context.Context, p manifest.Package) (bool, error) {
	if p.Skipped("apt") {
		return true, nil // not managed here
	}
	name := p.Apt
	if name == "" {
		name = p.Name
	}
	out, err := m.r.Output(ctx, "dpkg-query", "-W", "-f=${Status}", name)
	if err != nil {
		return false, nil // unknown package → not installed
	}
	return strings.Contains(string(out), "install ok installed"), nil
}
