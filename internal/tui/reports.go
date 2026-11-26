package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/othercodes/tmetric-cli/internal/app"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// The tabs of the Reports view.
const (
	projectsReportTab = iota
	balanceTab
)

// rangeLabels name app.Ranges for people.
var rangeLabels = map[string]string{
	"week": "this week", "last-week": "last week", "month": "this month", "last-month": "last month",
}

// barWidth is how many cells a bar takes.
const barWidth = 24

type (
	reportLoaded struct {
		rangeIndex int
		from, to   time.Time
		report     []tmetric.ProjectReport
		err        error
	}
	balanceLoaded struct {
		balance *tmetric.Balance
		err     error
	}
)

func (r reportLoaded) failure() error  { return r.err }
func (r balanceLoaded) failure() error { return r.err }

func (m model) loadReport(rangeIndex int) tea.Cmd {
	return func() tea.Msg {
		// The name comes from app.Ranges, which Range always knows.
		from, to, _ := m.session.Range(app.Ranges[rangeIndex])
		report, err := m.session.ProjectReport(m.ctx, from, to)
		return reportLoaded{rangeIndex, from, to, report, err}
	}
}

func (m model) loadBalance() tea.Msg {
	balance, err := m.session.Balance(m.ctx)
	return balanceLoaded{balance, err}
}

// applyReport stores a report answer, unless the range moved on meanwhile.
func (m model) applyReport(msg reportLoaded) model {
	if msg.rangeIndex == m.reportRange {
		m.report, m.reportFrom, m.reportTo, m.reportReady = msg.report, msg.from, msg.to, true
	}
	return m
}

func (m model) updateReports(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyTabs):
		m.reportTab = 1 - m.reportTab
		return m.layout(), nil
	case key.Matches(msg, keyRange) && m.reportTab == projectsReportTab:
		m.reportRange = (m.reportRange + 1) % len(app.Ranges)
		m.reportReady = false
		return m.fill(), m.loadReport(m.reportRange)
	}
	var cmd tea.Cmd
	if m.reportTab == projectsReportTab {
		m.reportTable, cmd = m.reportTable.Update(msg)
	}
	return m, cmd
}

// shareWidth fits "100% " and its bar.
const shareWidth = 5 + barWidth

// reportColumns lays the project report out for a panel inner width.
func reportColumns(inner int) []table.Column {
	rest := max(20, inner-6-shareWidth-14-2*4)
	return []table.Column{
		{Title: "PROJECT", Width: rest}, {Title: "TIME", Width: 6},
		{Title: "SHARE", Width: shareWidth}, {Title: "AMOUNT", Width: 14},
	}
}

func (m model) reportRows() []table.Row {
	total := m.reportTotal()
	rows := make([]table.Row, len(m.report))
	for i, r := range m.report {
		name := "(no project)"
		if r.Project != nil {
			name = r.Project.Name
		}
		part := 0.0
		if total > 0 {
			part = r.TotalSeconds / total
		}
		rows[i] = table.Row{name, seconds(r.TotalSeconds), fmt.Sprintf("%-5s", fmt.Sprintf("%.0f%%", 100*part)) + bar(part, barWidth),
			output.Money(r.BillableAmount, r.BillableCurrency)}
	}
	return rows
}

func (m model) reportTotal() float64 {
	var total float64
	for _, r := range m.report {
		total += r.TotalSeconds
	}
	return total
}

// bar draws part (0 to 1, more is cut) as filled cells out of width. Plain
// characters: a colored cell would throw the table's widths off.
func bar(part float64, width int) string {
	filled := int(math.Round(math.Max(0, math.Min(1, part)) * float64(width)))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func seconds(s float64) string { return output.Duration(time.Duration(s) * time.Second) }

// reportsPanel is the title, caption, body and summary of the Reports view.
func (m model) reportsPanel() (title, caption, body, summary string) {
	if m.reportTab == balanceTab {
		return dim.Render("Projects") + " │ " + accentFg.Render("Balance"),
			dim.Render("tracked against your work schedule"), m.balanceBody(), ""
	}
	title = accentFg.Render("Projects") + " │ " + dim.Render("Balance")
	label := rangeLabels[app.Ranges[m.reportRange]]
	if !m.reportReady {
		return title, dim.Render(label), "", "loading…"
	}
	caption = dim.Render(label + " · " + m.reportFrom.Format("Mon 02 Jan") + " to " + m.reportTo.Format("Mon 02 Jan 2006"))
	parts := []string{"total " + seconds(m.reportTotal())}
	parts = append(parts, moneyTotals(m.report)...)
	return title, caption, m.reportTable.View(), strings.Join(parts, " · ")
}

// moneyTotals sums the billable amounts per currency: money in different
// currencies never adds up.
func moneyTotals(report []tmetric.ProjectReport) []string {
	totals := map[string]float64{}
	for _, r := range report {
		if r.BillableAmount != nil {
			totals[r.BillableCurrency] += *r.BillableAmount
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
		lines[i] = "amount " + output.Money(&amount, currency)
	}
	return lines
}

// balanceBody shows today, the week and the month against the schedule, with
// a bar each; without a schedule there is nothing to measure against.
func (m model) balanceBody() string {
	b := m.balance
	if b == nil {
		return dim.Render("loading…")
	}
	periods := []struct {
		name string
		tb   tmetric.TimeBalance
	}{{"Today", b.Today}, {"This week", b.Week}, {"This month", b.Month}}
	scheduled := app.HasSchedule(b)
	lines := []string{""}
	for _, p := range periods {
		tracked := time.Duration(p.tb.ActualSeconds) * time.Second
		line := dim.Render(fmt.Sprintf("%-12s", p.name)) + fmt.Sprintf("%-9s", output.Duration(tracked))
		if scheduled {
			required := time.Duration(p.tb.RequiredSeconds) * time.Second
			part := 1.0
			if required > 0 {
				part = float64(tracked) / float64(required)
			}
			line += fmt.Sprintf("of %-9s %s  %s", output.Duration(required), bar(part, barWidth), output.SignedDuration(tracked-required))
		}
		lines = append(lines, line)
	}
	if !scheduled {
		lines = append(lines, "", dim.Render("No work schedule in this workspace: TMetric requires no hours."))
	}
	return strings.Join(lines, "\n")
}
