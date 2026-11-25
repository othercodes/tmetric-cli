package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/othercodes/tmetric-cli/internal/app"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
	"github.com/spf13/cobra"
)

func newEntriesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "entries",
		Short: "Edit and delete time entries",
	}
	cmd.AddCommand(newEditCmd(flags), newDeleteCmd(flags))
	return cmd
}

// entryArgs reads the entry ID and the --date it is on (today by default):
// the API has no way to fetch an entry by its ID alone.
func entryArgs(session *app.Session, args []string, date string) (int64, time.Time, error) {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return 0, time.Time{}, usagef("%q is not an entry ID: `tmetric today` and `tmetric week` show them", args[0])
	}
	if date == "" {
		return id, session.Midnight(), nil
	}
	day, err := time.ParseInLocation(time.DateOnly, date, session.Location)
	if err != nil {
		return 0, time.Time{}, usagef("%q is not a date: use YYYY-MM-DD", date)
	}
	return id, day, nil
}

func addDateFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "date", "", "the day the entry is on, YYYY-MM-DD (default today)")
}

func newEditCmd(flags *rootFlags) *cobra.Command {
	var date, note, project, task, start, end string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Change a time entry",
		Long: `edit changes the fields you pass and keeps the rest, tags and task links
included. Times are HH:MM on the entry's day, in your TMetric timezone. An
--end on the running timer stops it.

TMetric stores the edited entry under a new ID, which edit prints.

An edit that would cover other entries is refused: TMetric would cut or
remove them. Change the times so they do not overlap.`,
		Example: `  tmetric entries edit 227522038 --start 09:30 --end 11:00
  tmetric entries edit 227522038 --date 2026-11-23 --description "Bugs & Debt"
  tmetric entries edit 227522038 --project "tmetric-cli" --task Tests`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			change := app.Change{}
			for name, target := range map[string]**string{
				"description": &change.Note, "project": &change.Project, "task": &change.Task,
				"start": &change.Start, "end": &change.End,
			} {
				if cmd.Flags().Changed(name) {
					value, _ := cmd.Flags().GetString(name)
					*target = &value
				}
			}
			if change == (app.Change{}) {
				return usagef("nothing to change: pass --description, --project, --task, --start or --end")
			}
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			id, day, err := entryArgs(session, args, date)
			if err != nil {
				return err
			}
			edited, err := session.Edit(cmd.Context(), day, id, change)
			if err != nil {
				return err
			}
			if flags.asJSON {
				return output.JSON(os.Stdout, edited)
			}
			fmt.Fprintf(os.Stderr, "Updated %s %s %s\n", output.Describe(edited), span(session, edited),
				output.NewStyler(os.Stderr).Dim(fmt.Sprintf("(new ID %d)", edited.ID)))
			return nil
		},
	}
	addDateFlag(cmd, &date)
	cmd.Flags().StringVar(&note, "description", "", "new description")
	cmd.Flags().StringVarP(&project, "project", "p", "", "new project, ID or exact name")
	cmd.Flags().StringVarP(&task, "task", "t", "", "new task, ID or exact name, within the project")
	cmd.Flags().StringVar(&start, "start", "", "new start, HH:MM")
	cmd.Flags().StringVar(&end, "end", "", "new end, HH:MM")
	return cmd
}

func newDeleteCmd(flags *rootFlags) *cobra.Command {
	var date string
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a time entry",
		Long:  "delete removes a time entry. It asks first on a terminal; pass --yes in scripts.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			id, day, err := entryArgs(session, args, date)
			if err != nil {
				return err
			}
			// Found first, so a mistyped ID deletes nothing and the question
			// names what goes.
			entry, _, err := session.Find(cmd.Context(), day, id)
			if err != nil {
				return err
			}
			if err := confirm(yes, "delete "+output.Describe(entry)+" "+span(session, entry)); err != nil {
				return err
			}
			if err := session.Delete(cmd.Context(), id); err != nil {
				return err
			}
			if flags.asJSON {
				return output.JSON(os.Stdout, entry)
			}
			fmt.Fprintf(os.Stderr, "Deleted %s %s\n", output.Describe(entry), span(session, entry))
			return nil
		},
	}
	addDateFlag(cmd, &date)
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "delete without asking")
	return cmd
}

// span is an entry's "09:00–10:30", or "09:00–now" while it runs.
func span(session *app.Session, entry *tmetric.TimeEntry) string {
	end := "now"
	if entry.EndTime != nil {
		end = entry.EndTime.In(session.Location).Format("15:04")
	}
	return entry.StartTime.In(session.Location).Format("15:04") + "–" + end
}

// confirm asks on a terminal; without one, it needs --yes.
func confirm(yes bool, what string) error {
	if yes {
		return nil
	}
	if !stdinIsTerminal() {
		return usagef("refusing to %s without confirmation; pass --yes in non-interactive use", what)
	}
	fmt.Fprintf(os.Stderr, "%s? [y/N] ", strings.ToUpper(what[:1])+what[1:])
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && answer == "" {
		return errors.New("aborted")
	}
	if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
		return errors.New("aborted")
	}
	return nil
}
