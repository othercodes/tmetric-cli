package cmd

import (
	"os"

	"github.com/othercodes/tmetric-cli/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// uiOnTerminal asks the OS whether both ends are TTYs, and runTUI draws the
// UI; tests swap both out, since CI has no terminal.
var (
	uiOnTerminal = func() bool { return stdinIsTerminal() && term.IsTerminal(int(os.Stdout.Fd())) }
	runTUI       = tui.Run
)

func newUICmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "Track time in an interactive terminal UI",
		Long: `ui shows the running timer, today's entries and your projects, and starts
and stops timers from the keyboard:

  tab    switch between today and projects
  ↑/↓    move in the focused list
  s      start a timer on the selected project (asks for a description)
  x      stop the running timer
  r      reload
  q      quit`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Never draw into a pipe or CI log: the UI needs a real terminal
			// on both ends.
			if !uiOnTerminal() {
				return usagef("tmetric ui needs an interactive terminal; use the other commands in scripts")
			}
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			return runTUI(cmd.Context(), session)
		},
	}
}
