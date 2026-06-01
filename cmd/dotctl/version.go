package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is the dotctl version. It defaults to "dev" and is overridden at build
// time via -ldflags "-X main.version=...". Using the `main` package as the target
// keeps the ldflags path independent of the module path, so renaming the repo
// needs no change here, in the Makefile, or in goreleaser (which injects
// main.version by default).
var version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the dotctl version",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println(version)
			return nil
		},
	}
}
