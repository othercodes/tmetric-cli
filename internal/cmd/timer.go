package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/othercodes/tmetric-cli/internal/app"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
	"github.com/spf13/cobra"
)

// newSession signs in and reads the user, workspace and timezone.
func newSession(ctx context.Context, flags *rootFlags) (*app.Session, error) {
	client, err := newClient(flags)
	if err != nil {
		return nil, err
	}
	return app.NewSession(ctx, client)
}

func newTimerCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timer",
		Short: "Start, stop and check the running timer",
	}
	cmd.AddCommand(newTimerStatusCmd(flags), newTimerStartCmd(flags), newTimerStopCmd(flags))
	return cmd
}

// timerJSON is the --json shape of the timer commands.
type timerJSON struct {
	Entry          *tmetric.TimeEntry `json:"entry"`
	ElapsedSeconds int64              `json:"elapsed_seconds"`
	Running        bool               `json:"running"`
}

func toJSON(session *app.Session, entry *tmetric.TimeEntry) timerJSON {
	if entry == nil {
		return timerJSON{}
	}
	return timerJSON{
		Entry:          entry,
		ElapsedSeconds: int64(session.Elapsed(entry).Seconds()),
		Running:        entry.EndTime == nil,
	}
}

// stopJSON adds whether TMetric dropped the stopped entry.
func stopJSON(session *app.Session, stopped *app.Stopped) any {
	if stopped == nil {
		return toJSON(session, nil)
	}
	return struct {
		timerJSON
		Dropped bool `json:"dropped"`
	}{toJSON(session, stopped.Entry), stopped.Dropped}
}

func newTimerStatusCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the running timer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			running, err := session.Running(cmd.Context())
			if err != nil {
				return err
			}
			if flags.asJSON {
				return output.JSON(os.Stdout, toJSON(session, running))
			}
			if running == nil {
				fmt.Println("No timer running.")
				return nil
			}
			return output.KeyValues(os.Stdout, [][2]string{
				{"description", output.Note(running)},
				{"project", output.Project(running)},
				{"task", output.Task(running)},
				{"started", running.StartTime.In(session.Location).Format("15:04")},
				{"elapsed", output.Duration(session.Elapsed(running))},
			})
		},
	}
}

// confirmStop asks before a command stops the running timer: TMetric closes it
// on the spot and nothing undoes that. next names what starts instead.
func confirmStop(ctx context.Context, session *app.Session, yes bool, next string) error {
	running, err := session.Running(ctx)
	if err != nil || running == nil {
		return err
	}
	what := "stop " + output.Describe(running) + " (" + output.Duration(session.Elapsed(running)) + ")"
	if next != "" {
		what += " and start " + next
	}
	return confirm(yes, what)
}

func newTimerStartCmd(flags *rootFlags) *cobra.Command {
	opts := app.StartOptions{}
	var yes bool
	cmd := &cobra.Command{
		Use:   "start [description]",
		Short: "Start a timer now, stopping the one that runs",
		Long: `start starts a timer now. TMetric stops the timer that was running, so
when one runs, start asks first; pass --yes to skip the question, which is
required without a terminal.

--project and --task take an ID or an exact name (case does not matter);
"tmetric projects list" shows them. --task needs --project.`,
		Example: `  tmetric timer start "Review PR #12" --project "tmetric-cli"
  tmetric timer start standup --project 500001 --task 300001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Note = strings.Join(args, " ")
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			if err := confirmStop(cmd.Context(), session, yes, `"`+opts.Note+`"`); err != nil {
				return err
			}
			started, stopped, err := session.Start(cmd.Context(), opts)
			if err != nil {
				return err
			}
			if flags.asJSON {
				return output.JSON(os.Stdout, toJSON(session, started))
			}
			styler := output.NewStyler(os.Stderr)
			if stopped != nil {
				// The API closed it at our start time, which is now.
				stopped.EndTime = &started.StartTime
				fmt.Fprintf(os.Stderr, "Stopped %s %s\n", output.Describe(stopped), styler.Dim("("+output.Duration(session.Elapsed(stopped))+")"))
			}
			fmt.Fprintf(os.Stderr, "Started %s %s\n", output.Describe(started),
				styler.Dim("at "+started.StartTime.In(session.Location).Format("15:04")))
			return nil
		},
	}
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		return recentNotes(cmd, flags, args)
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "stop the running timer without asking")
	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "project ID or exact name")
	cmd.Flags().StringVarP(&opts.Task, "task", "t", "", "task ID or exact name, within --project")
	return cmd
}

// recentNotes completes the description of `timer start` with what the user
// worked on lately, each with its project as a hint. A failed call completes
// nothing rather than printing an error into the shell.
func recentNotes(cmd *cobra.Command, flags *rootFlags, args []string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	session, err := newSession(cmd.Context(), flags)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	recent, err := session.Recent(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]bool{}
	var notes []string
	for _, r := range recent {
		if r.Note == "" || seen[r.Note] {
			continue
		}
		seen[r.Note] = true
		hint := output.Project(&tmetric.TimeEntry{Project: r.Project})
		notes = append(notes, r.Note+"\t"+hint)
	}
	return notes, cobra.ShellCompDirectiveNoFileComp
}

func newTimerStopCmd(flags *rootFlags) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running timer now",
		Long:  "stop stops the running timer now. It asks first; pass --yes to skip the question, which is required without a terminal.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			if err := confirmStop(cmd.Context(), session, yes, ""); err != nil {
				return err
			}
			stopped, err := session.Stop(cmd.Context())
			if err != nil {
				return err
			}
			if flags.asJSON {
				return output.JSON(os.Stdout, stopJSON(session, stopped))
			}
			if stopped == nil {
				fmt.Fprintln(os.Stderr, "No timer running.")
				return nil
			}
			styler := output.NewStyler(os.Stderr)
			fmt.Fprintf(os.Stderr, "Stopped %s %s\n", output.Describe(stopped.Entry),
				styler.Dim("("+output.Duration(session.Elapsed(stopped.Entry))+")"))
			if stopped.Dropped {
				fmt.Fprintln(os.Stderr, styler.Dim("TMetric did not keep it: entries shorter than a minute are dropped."))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "stop without asking")
	return cmd
}
