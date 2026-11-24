// Package cmd implements the tmetric CLI commands.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/othercodes/tmetric-cli/internal/auth"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
	"github.com/spf13/cobra"
)

// version is the CLI version, overridden at release time via
// -ldflags "-X github.com/othercodes/tmetric-cli/internal/cmd.version=vX.Y.Z".
var version = "dev"

// tokenEnv holds an API token that wins over the stored login.
const tokenEnv = "TMETRIC_API_TOKEN"

// rootFlags holds the global flags shared by every command.
type rootFlags struct {
	baseURL string
	asJSON  bool
}

func newRootCmd() *cobra.Command {
	flags := &rootFlags{}
	root := &cobra.Command{
		Use:   "tmetric",
		Short: "Track time in TMetric from the command line",
		Long: `tmetric tracks time in TMetric from the command line.

Sign in with "tmetric login", which asks for your personal API token (TMetric >
My Profile > Get new API token) and stores it in your keychain. TMETRIC_API_TOKEN
also works and takes precedence.

Output adapts to where it goes: tables on a terminal, tab-separated values in
a pipe, and --json where structured output is useful.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Version = version
	root.SetVersionTemplate("tmetric version {{.Version}}\n")
	root.PersistentFlags().StringVar(&flags.baseURL, "base-url", "", "API base URL (default "+tmetric.DefaultBaseURL+", env TMETRIC_BASE_URL)")
	root.PersistentFlags().BoolVar(&flags.asJSON, "json", false, "output JSON instead of the default format")
	// A bad flag is a usage error, so it exits 2 like any other.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{msg: err.Error()}
	})

	root.AddCommand(
		newLoginCmd(flags),
		newLogoutCmd(flags),
		newWhoamiCmd(flags),
	)
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
	styler := output.NewStyler(os.Stderr)
	fmt.Fprintln(os.Stderr, styler.Red("error:")+" "+renderError(err))
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

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// renderError turns API and CLI errors into a friendly one-or-few-line
// message.
func renderError(err error) string {
	if errors.Is(err, context.Canceled) {
		return "interrupted"
	}
	lines := []string{err.Error()}
	if tmetric.IsUnauthorized(err) {
		lines = append(lines, "run `tmetric login`, or check that "+tokenEnv+" holds a valid API token")
	}
	return strings.Join(lines, "\n  ")
}

// resolveBaseURL is the API the command talks to: the flag, then the
// environment, then the production API.
func resolveBaseURL(flags *rootFlags) string {
	if flags.baseURL != "" {
		return flags.baseURL
	}
	if fromEnv := os.Getenv("TMETRIC_BASE_URL"); fromEnv != "" {
		return fromEnv
	}
	return tmetric.DefaultBaseURL
}

func userAgent() string {
	return "tmetric-cli/" + version + " (Go; " + runtime.Version() + ")"
}

// newClient builds the API client. A token in the environment wins over a
// stored login: it is the explicit choice, and it is what a script sets on
// purpose.
func newClient(flags *rootFlags) (*tmetric.Client, error) {
	baseURL := resolveBaseURL(flags)
	token := os.Getenv(tokenEnv)
	if token == "" {
		stored, err := auth.Load(baseURL)
		if errors.Is(err, auth.ErrNoCredentials) {
			return nil, errors.New("not signed in: run `tmetric login`, or export a token from TMetric > My Profile > Get new API token:\n  export " + tokenEnv + "=your-token")
		}
		if err != nil {
			return nil, err
		}
		token = stored
	}
	return tmetric.NewClient(baseURL, token, userAgent(), nil), nil
}

// credentialSource names where the token in use comes from.
func credentialSource() string {
	if os.Getenv(tokenEnv) != "" {
		return tokenEnv
	}
	return "the login stored by `tmetric login`"
}
