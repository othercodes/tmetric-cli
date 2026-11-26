package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/othercodes/tmetric-cli/internal/app"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
	"github.com/spf13/cobra"
)

func newReportsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reports",
		Short: "Report tracked time by project, and against your work schedule",
	}
	cmd.AddCommand(newProjectsReportCmd(flags), newBalanceCmd(flags))
	return cmd
}

// reportRange is the range of a report: --from and --to, or a named --range.
func reportRange(session *app.Session, name, from, to string) (time.Time, time.Time, error) {
	if from == "" && to == "" {
		return session.Range(name)
	}
	if from == "" || to == "" {
		return time.Time{}, time.Time{}, usagef("pass both --from and --to")
	}
	start, err := time.ParseInLocation(time.DateOnly, from, session.Location)
	if err != nil {
		return time.Time{}, time.Time{}, usagef("%q is not a date: use YYYY-MM-DD", from)
	}
	end, err := time.ParseInLocation(time.DateOnly, to, session.Location)
	if err != nil {
		return time.Time{}, time.Time{}, usagef("%q is not a date: use YYYY-MM-DD", to)
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, usagef("--to (%s) is before --from (%s)", to, from)
	}
	return start, end, nil
}

// projectsReportJSON is the --json shape of `reports projects`.
type projectsReportJSON struct {
	From         string                  `json:"from"`
	To           string                  `json:"to"`
	TotalSeconds float64                 `json:"total_seconds"`
	Projects     []tmetric.ProjectReport `json:"projects"`
}

func newProjectsReportCmd(flags *rootFlags) *cobra.Command {
	var name, from, to, format string
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Show the time you tracked on each project",
		Long: `projects shows the time you tracked on each project over a range: this week
by default, --range for another one, or --from and --to for any days.

Amounts and costs only show when your workspace gives you access to
financial information. Totals of money are summed per currency.`,
		Example: `  tmetric reports projects
  tmetric reports projects --range last-month
  tmetric reports projects --from 2026-11-01 --to 2026-11-15 --format csv`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			start, end, err := reportRange(session, name, from, to)
			if err != nil {
				return err
			}
			report, err := session.ProjectReport(cmd.Context(), start, end)
			if err != nil {
				return err
			}
			var total float64
			for _, r := range report {
				total += r.TotalSeconds
			}
			if flags.asJSON {
				if report == nil {
					report = []tmetric.ProjectReport{}
				}
				return output.JSON(os.Stdout, projectsReportJSON{
					From: start.Format(time.DateOnly), To: end.Format(time.DateOnly), TotalSeconds: total, Projects: report,
				})
			}
			header := []string{"PROJECT", "CLIENT", "TIME", "BILLABLE", "SHARE", "AMOUNT", "COST"}
			rows := make([][]string, len(report))
			for i, r := range report {
				entry := &tmetric.TimeEntry{Project: r.Project}
				client := "-"
				if r.Project != nil && r.Project.Client != nil {
					client = r.Project.Client.Name
				}
				name := output.Project(entry)
				if r.Project == nil {
					name = "(no project)"
				}
				rows[i] = []string{name, client, seconds(r.TotalSeconds), seconds(r.BillableSeconds), share(r.TotalSeconds, total),
					output.Money(r.BillableAmount, r.BillableCurrency), output.Money(r.CostAmount, r.CostCurrency)}
			}
			if err := renderList(format, header, rows); err != nil {
				return err
			}
			summary := []string{"total " + seconds(total)}
			summary = append(summary, sums("amount", report, func(r tmetric.ProjectReport) (*float64, string) { return r.BillableAmount, r.BillableCurrency })...)
			summary = append(summary, sums("cost", report, func(r tmetric.ProjectReport) (*float64, string) { return r.CostAmount, r.CostCurrency })...)
			printSummary(format, start.Format(time.DateOnly)+" to "+end.Format(time.DateOnly)+" · "+strings.Join(summary, " · "))
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "range", "week", "week, last-week, month or last-month")
	cmd.Flags().StringVar(&from, "from", "", "first day, YYYY-MM-DD (with --to)")
	cmd.Flags().StringVar(&to, "to", "", "last day, YYYY-MM-DD (with --from)")
	addFormatFlag(cmd, &format)
	return cmd
}

// printSummary puts a totals line after a list: on stdout under a table on a
// terminal, on stderr in a pipe or CSV, so stdout holds only the rows.
func printSummary(format, line string) {
	if format == "" && output.IsTerminal(os.Stdout) {
		fmt.Println(output.NewStyler(os.Stdout).Bold(line))
		return
	}
	fmt.Fprintln(os.Stderr, line)
}

// sums totals amounts per currency: money in different currencies never adds up.
func sums(label string, report []tmetric.ProjectReport, pick func(tmetric.ProjectReport) (*float64, string)) []string {
	totals := map[string]float64{}
	for _, r := range report {
		if amount, currency := pick(r); amount != nil {
			totals[currency] += *amount
		}
	}
	currencies := make([]string, 0, len(totals))
	for currency := range totals {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	lines := make([]string, len(currencies))
	for i, currency := range currencies {
		amount := totals[currency]
		lines[i] = label + " " + output.Money(&amount, currency)
	}
	return lines
}

func seconds(s float64) string { return output.Duration(time.Duration(s) * time.Second) }

// share is part of total as a whole percentage.
func share(part, total float64) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*part/total)
}

func newBalanceCmd(flags *rootFlags) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "balance",
		Short: "Show your tracked time against your work schedule",
		Long: `balance shows, for today, this week and this month, the time you tracked,
the time your work schedule requires, and the difference: positive when you
are ahead, negative when you are behind.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := newSession(cmd.Context(), flags)
			if err != nil {
				return err
			}
			balance, err := session.Balance(cmd.Context())
			if err != nil {
				return err
			}
			if flags.asJSON {
				return output.JSON(os.Stdout, balance)
			}
			periods := []struct {
				name string
				tb   tmetric.TimeBalance
			}{{"today", balance.Today}, {"week", balance.Week}, {"month", balance.Month}}
			scheduled := app.HasSchedule(balance)
			rows := make([][]string, len(periods))
			for i, p := range periods {
				tracked := time.Duration(p.tb.ActualSeconds) * time.Second
				required := time.Duration(p.tb.RequiredSeconds) * time.Second
				rows[i] = []string{p.name, output.Duration(tracked), "-", "-"}
				if scheduled {
					rows[i][2], rows[i][3] = output.Duration(required), output.SignedDuration(tracked-required)
				}
			}
			if err := renderList(format, []string{"PERIOD", "TRACKED", "REQUIRED", "DIFFERENCE"}, rows); err != nil {
				return err
			}
			if !scheduled {
				fmt.Fprintln(os.Stderr, output.NewStyler(os.Stderr).Dim("no work schedule in this workspace: TMetric requires no hours"))
			}
			return nil
		},
	}
	addFormatFlag(cmd, &format)
	return cmd
}
