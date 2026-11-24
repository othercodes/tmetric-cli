package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/othercodes/tmetric-cli/internal/auth"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// stdinIsTerminal asks the OS whether stdin is a TTY. output.IsTerminal is not
// enough here: /dev/null is a character device too, and has no prompt to type
// into. Tests swap both out, since CI has no terminal.
var stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// readPassword reads a line from the terminal without echoing it.
var readPassword = func() ([]byte, error) { return term.ReadPassword(int(os.Stdin.Fd())) }

func newLoginCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Store your TMetric API token",
		Long: `login asks for your personal TMetric API token, checks it against the API
and stores it in your operating system's keychain. When there is no keychain
it falls back to a file only you can read.

Get a token in TMetric > My Profile > Get new API token. It lasts a year, and
getting a new one revokes the old one.

On a terminal the token is read without echo; in a pipe it is read from stdin:
  pass show tmetric | tmetric login

` + tokenEnv + ` keeps working and takes precedence over a stored login.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := readToken()
			if err != nil {
				return err
			}
			baseURL := resolveBaseURL(flags)
			// Checked before it is stored, so a typo never reaches the keychain.
			user, err := tmetric.NewClient(baseURL, token, userAgent(), nil).User(cmd.Context())
			if err != nil {
				return fmt.Errorf("checking the token: %w", err)
			}
			if err := auth.Save(baseURL, token); err != nil {
				return err
			}
			if flags.asJSON {
				return output.JSON(os.Stdout, user)
			}
			fmt.Fprintf(os.Stderr, "Signed in as %s (%s).\n", user.Name, user.Email)
			warnEnvToken()
			return nil
		},
	}
}

// readToken reads the token without echo on a terminal, or as the first line
// of stdin in a pipe.
func readToken() (string, error) {
	var raw string
	if stdinIsTerminal() {
		fmt.Fprint(os.Stderr, "TMetric API token: ")
		typed, err := readPassword()
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("reading the token: %w", err)
		}
		raw = string(typed)
	} else {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", usagef("no token on stdin: pipe one in, or run `tmetric login` on a terminal")
		}
		raw = line
	}
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", usagef("the token is empty")
	}
	return token, nil
}

func newLogoutCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored API token from this machine",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseURL := resolveBaseURL(flags)
			_, err := auth.Load(baseURL)
			if errors.Is(err, auth.ErrNoCredentials) {
				fmt.Fprintln(os.Stderr, "Not signed in.")
				warnEnvToken()
				return nil
			}
			if err != nil {
				return err
			}
			if err := auth.Clear(baseURL); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Signed out. The token itself stays valid until you get a new one in TMetric.")
			warnEnvToken()
			return nil
		},
	}
}

// warnEnvToken says when the stored login is not what commands will use.
func warnEnvToken() {
	if os.Getenv(tokenEnv) != "" {
		fmt.Fprintln(os.Stderr, output.NewStyler(os.Stderr).Dim(tokenEnv+" is set and takes precedence over the stored login."))
	}
}

func newWhoamiCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the user and workspace these credentials belong to",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(flags)
			if err != nil {
				return err
			}
			user, err := client.User(cmd.Context())
			if err != nil {
				return err
			}
			if flags.asJSON {
				return output.JSON(os.Stdout, user)
			}
			if err := output.KeyValues(os.Stdout, [][2]string{
				{"name", user.Name},
				{"email", user.Email},
				{"workspace", activeAccountName(user)},
				{"timezone", user.TimeZone.ID},
			}); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, output.NewStyler(os.Stderr).Dim("via "+credentialSource()))
			return nil
		},
	}
}

// activeAccountName is the name of the workspace commands act on, with its
// ID, which is what --account-style flags and the API take.
func activeAccountName(user *tmetric.User) string {
	for _, account := range user.Accounts {
		if account.ID == user.ActiveAccountID {
			return fmt.Sprintf("%s (%d)", account.Name, account.ID)
		}
	}
	return fmt.Sprintf("%d", user.ActiveAccountID)
}
