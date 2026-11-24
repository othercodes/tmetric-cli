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
				{"description", noteOf(running)},
				{"project", projectOf(running)},
				{"task", taskOf(running)},
				{"started", running.StartTime.In(session.Location).Format("15:04")},
				{"elapsed", output.Duration(session.Elapsed(running))},
			})
		},
	}
}

func newTimerStartCmd(flags *rootFlags) *cobra.Command {
	opts := app.StartOptions{}
	cmd := &cobra.Command{
		Use:   "start [description]",
		Short: "Start a timer now, stopping the one that runs",
		Long: `start starts a timer now. TMetric stops the timer that was running.

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
				fmt.Fprintf(os.Stderr, "Stopped %s %s\n", describe(stopped), styler.Dim("("+output.Duration(session.Elapsed(stopped))+")"))
			}
			fmt.Fprintf(os.Stderr, "Started %s %s\n", describe(started),
				styler.Dim("at "+started.StartTime.In(session.Location).Format("15:04")))
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "project ID or exact name")
	cmd.Flags().StringVarP(&opts.Task, "task", "t", "", "task ID or exact name, within --project")
	return cmd
}

func newTimerStopCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running timer now",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
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
			fmt.Fprintf(os.Stderr, "Stopped %s %s\n", describe(stopped.Entry),
				styler.Dim("("+output.Duration(session.Elapsed(stopped.Entry))+")"))
			if stopped.Dropped {
				fmt.Fprintln(os.Stderr, styler.Dim("TMetric did not keep it: entries shorter than a minute are dropped."))
			}
			return nil
		},
	}
}

// describe is the one-line summary of an entry: "note · project / task".
func describe(entry *tmetric.TimeEntry) string {
	text := `"` + noteOf(entry) + `"`
	if entry.Project != nil {
		text += " · " + entry.Project.Name
	}
	if entry.Task != nil && entry.Task.Name != "" {
		text += " / " + entry.Task.Name
	}
	return text
}

func noteOf(entry *tmetric.TimeEntry) string {
	if entry.Note == "" {
		return "(no description)"
	}
	return entry.Note
}

func projectOf(entry *tmetric.TimeEntry) string {
	if entry.Project == nil {
		return "-"
	}
	return entry.Project.Name
}

func taskOf(entry *tmetric.TimeEntry) string {
	if entry.Task == nil || entry.Task.Name == "" {
		return "-"
	}
	return entry.Task.Name
}
