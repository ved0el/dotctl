package pkg

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/ved0el/dotctl/internal/console"
)

// Runner executes external commands. ExecRunner runs them for real; DryRunner
// only logs; tests substitute a fake. Injecting this seam keeps command
// construction unit-testable and makes --dry-run a wiring choice, not a branch
// scattered through the backends.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs commands via os/exec, streaming output to the parent process.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (ExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// DryRunner logs the command it would run and performs no execution.
type DryRunner struct{ Log *console.Logger }

func (d DryRunner) Run(_ context.Context, name string, args ...string) error {
	d.Log.Plan("run", commandString(name, args))
	return nil
}

func (d DryRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	d.Log.Plan("run", commandString(name, args))
	return nil, nil
}

func commandString(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}
