package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/othercodes/tmetric-cli/internal/app"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
	"github.com/spf13/cobra"
)

// addFormatFlag registers --format on commands that print a list.
func addFormatFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "format", "", "csv for CSV with a header (default: a table on a terminal, tab-separated in a pipe)")
}

// renderList writes rows as CSV or, by default, as a table that turns into
// tab-separated values in a pipe.
func renderList(format string, header []string, rows [][]string) error {
	switch format {
	case "":
		return output.Table(os.Stdout, header, rows)
	case "csv":
		return output.CSV(os.Stdout, header, rows)
	default:
		return usagef("unknown --format %q: use csv, or leave it out", format)
	}
}

func newTodayCmd(flags *rootFlags) *cobra.Command {
	return newReportCmd(flags, "today", "Show today's time entries and total", false,
		func(ctx context.Context, s *app.Session) (*app.Report, error) { return s.Today(ctx) })
}

func newWeekCmd(flags *rootFlags) *cobra.Command {
	return newReportCmd(flags, "week", "Show this week's time entries and total", true,
		func(ctx context.Context, s *app.Session) (*app.Report, error) { return s.Week(ctx) })
}

// reportJSON is the --json shape of today and week.
type reportJSON struct {
	From         string              `json:"from"`
	To           string              `json:"to"`
	TotalSeconds int64               `json:"total_seconds"`
	Entries      []tmetric.TimeEntry `json:"entries"`
}

func newReportCmd(flags *rootFlags, use, short string, withDate bool,
	fetch func(context.Context, *app.Session) (*app.Report, error)) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long: short + `. A running timer counts up to now.

On a terminal the total follows the table; in a pipe or with --format csv it
goes to stderr, so stdout holds only the entries.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			report, err := fetch(cmd.Context(), session)
			if err != nil {
				return err
			}
			if flags.asJSON {
				entries := report.Entries
				if entries == nil {
					entries = []tmetric.TimeEntry{}
				}
				return output.JSON(os.Stdout, reportJSON{
					From:         report.From.Format(time.DateOnly),
					To:           report.To.Format(time.DateOnly),
					TotalSeconds: int64(report.Total.Seconds()),
					Entries:      entries,
				})
			}
			header, rows := entryRows(session, report.Entries, withDate)
			if err := renderList(format, header, rows); err != nil {
				return err
			}
			total := output.Duration(report.Total)
			if format == "" && output.IsTerminal(os.Stdout) {
				fmt.Println(output.NewStyler(os.Stdout).Bold("Total " + total))
				return nil
			}
			fmt.Fprintln(os.Stderr, "total "+total)
			return nil
		},
	}
	addFormatFlag(cmd, &format)
	return cmd
}

// entryRows lays entries out as table rows, with a date column for reports
// that span several days.
func entryRows(session *app.Session, entries []tmetric.TimeEntry, withDate bool) ([]string, [][]string) {
	header := []string{"ID", "START", "END", "DURATION", "PROJECT", "TASK", "DESCRIPTION"}
	if withDate {
		header = append([]string{"DATE"}, header...)
	}
	rows := make([][]string, 0, len(entries))
	for i := range entries {
		entry := &entries[i]
		start := entry.StartTime.In(session.Location)
		end := "running"
		if entry.EndTime != nil {
			end = entry.EndTime.In(session.Location).Format("15:04")
		}
		row := []string{fmt.Sprint(entry.ID), start.Format("15:04"), end, output.Duration(session.Elapsed(entry)),
			output.Project(entry), output.Task(entry), output.Note(entry)}
		if withDate {
			row = append([]string{start.Format("Mon 02")}, row...)
		}
		rows = append(rows, row)
	}
	return header, rows
}

func newProjectsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List the projects you can track time on",
	}
	var format string
	list := &cobra.Command{
		Use:   "list",
		Short: "List the projects you can track time on",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			projects, err := session.Projects(cmd.Context())
			if err != nil {
				return err
			}
			if flags.asJSON {
				if projects == nil {
					projects = []tmetric.Project{}
				}
				return output.JSON(os.Stdout, projects)
			}
			rows := make([][]string, len(projects))
			for i, p := range projects {
				client := "-"
				if p.Client != nil {
					client = p.Client.Name
				}
				rows[i] = []string{fmt.Sprint(p.ID), p.Name, client}
			}
			return renderList(format, []string{"ID", "NAME", "CLIENT"}, rows)
		},
	}
	addFormatFlag(list, &format)
	cmd.AddCommand(list)
	return cmd
}
