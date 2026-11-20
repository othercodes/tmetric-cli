// Package cmd implements the tmetric CLI commands.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is the CLI version, overridden at release time via
// -ldflags "-X github.com/othercodes/tmetric-cli/internal/cmd.version=vX.Y.Z".
var version = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "tmetric",
		Short:         "Track time in TMetric from the command line",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Version = version
	root.SetVersionTemplate("tmetric version {{.Version}}\n")
	// A bad flag is a usage error, so it exits 2 like any other.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{msg: err.Error()}
	})
	return root
}

// Execute runs the CLI and returns the process exit code: 0 on success, 1 on
// runtime/API errors, 2 on usage errors.
func Execute(ctx context.Context) int {
	return run(ctx, os.Args[1:])
}

func run(ctx context.Context, args []string) int {
	root := newRootCmd()
	root.SetArgs(args)
	return exitCode(root.ExecuteContext(ctx))
}

// exitCode reports err on stderr and maps it to the process exit code.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, "error: "+err.Error())
	var usageErr *usageError
	if errors.As(err, &usageErr) {
		return 2
	}
	return 1
}

// usageError marks errors caused by invalid invocation rather than a failed
// operation; they exit with code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }
