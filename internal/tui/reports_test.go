package tui

import (
	"strings"
	"testing"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

const lastWeekReport = "GET /accounts/42/reports/projects?endDate=2026-11-22&includeDone=true&startDate=2026-11-16&userId=7"

func TestProjectsReport(t *testing.T) {
	m := press(loaded(t, routes()), "3")
	has(t, m.View(), "│ Reports", "Projects │ Balance", "this week · Mon 23 Nov to Sun 29 Nov 2026",
		"CLI", "1h30m", "60%", "100.00 EUR", "(no project)", "total 2h30m · amount 100.00 EUR")
	if !strings.HasPrefix(m.footer(), "Move: ↑/↓ | Range: t | Tabs: [ ]") {
		t.Errorf("footer = %q", m.footer())
	}
	m = press(m, "down")
	if m.reportTable.Cursor() != 1 {
		t.Errorf("cursor = %d", m.reportTable.Cursor())
	}
}

func TestReportRangeCycles(t *testing.T) {
	r := routes()
	r[lastWeekReport] = `[{"project":{"id":6,"name":"Docs"},"totalSeconds":7200}]`
	m := loaded(t, r)
	m = press(m, "3")
	// The key moves to last week; until the answer comes, the report waits.
	next, _ := m.Update(keyMsg("t"))
	has(t, next.(model).View(), "last week", "loading…")
	m = press(m, "t")
	has(t, m.View(), "last week · Mon 16 Nov to Sun 22 Nov 2026", "Docs", "2h00m", "100%", "total 2h00m")
	// An answer for a range already left is dropped.
	next, _ = m.Update(reportLoaded{rangeIndex: 0, report: nil})
	has(t, next.(model).View(), "Docs")
}

func TestReportRangeWraps(t *testing.T) {
	r := routes()
	r[lastWeekReport] = "[]"
	r["GET /accounts/42/reports/projects?endDate=2026-11-30&includeDone=true&startDate=2026-11-01&userId=7"] = "[]"
	r["GET /accounts/42/reports/projects?endDate=2026-10-31&includeDone=true&startDate=2026-10-01&userId=7"] = "[]"
	m := press(loaded(t, r), "3", "t", "t", "t")
	has(t, m.View(), "last month · Thu 01 Oct to Sat 31 Oct 2026", "total 0h00m")
	m = press(m, "t")
	has(t, m.View(), "this week")
}

func TestBalanceTab(t *testing.T) {
	m := press(loaded(t, routes()), "3", "]")
	has(t, m.View(), "Projects │ Balance", "tracked against your work schedule",
		"Today", "4h00m", "of 8h00m", "-4h00m", "This week", "41h15m", "of 40h00m", "+1h15m", "This month", "-80h00m")
	if !strings.HasPrefix(m.footer(), "Tabs: [ ] | Focus: tab") {
		t.Errorf("footer = %q", m.footer())
	}
	m = press(m, "t", "down") // no range and no list on this tab
	if m.reportRange != 0 {
		t.Error("t should not change the range on the balance tab")
	}
	if press(m, "[").reportTab != projectsReportTab {
		t.Error("[ should go back to the projects tab")
	}
}

func TestBalanceWithoutSchedule(t *testing.T) {
	r := routes()
	r[balanceRoute] = `{"today":{"actualSeconds":10560},"week":{"actualSeconds":64200},"month":{"actualSeconds":477240}}`
	m := press(loaded(t, r), "3", "]")
	has(t, m.View(), "2h56m", "132h34m", "No work schedule in this workspace")
	lacks(t, m.View(), "█", " of ")
}

func TestBalanceDayOff(t *testing.T) {
	r := routes()
	// Sunday inside a schedule: nothing required today, a full bar.
	r[balanceRoute] = `{"today":{"requiredSeconds":0,"actualSeconds":3600},"week":{"requiredSeconds":144000,"actualSeconds":3600},"month":{"requiredSeconds":576000,"actualSeconds":3600}}`
	has(t, press(loaded(t, r), "3", "]").View(), "of 0h00m", strings.Repeat("█", barWidth)+"  +1h00m")
}

func TestReportsWhileLoading(t *testing.T) {
	m := press(loaded(t, routes()), "3")
	m.balance = nil
	has(t, press(m, "]").View(), "loading…")
}

func TestReportErrors(t *testing.T) {
	for _, broken := range []string{reportRoute, balanceRoute} {
		r := routes()
		r[broken] = "HTTP 500"
		if m := loaded(t, r); !m.statusErr {
			t.Errorf("%s: footer = %q", broken, m.footer())
		}
	}
}

func TestMoneyTotalsAndBars(t *testing.T) {
	eur, usd := 10.0, 5.0
	got := moneyTotals([]tmetric.ProjectReport{
		{BillableAmount: &usd, BillableCurrency: "USD"}, {BillableAmount: &eur, BillableCurrency: "EUR"},
		{BillableAmount: &eur, BillableCurrency: "EUR"}, {},
	})
	if strings.Join(got, " · ") != "amount 20.00 EUR · amount 5.00 USD" {
		t.Errorf("totals = %v", got)
	}
	if bar(2, 4) != "████" || bar(-1, 4) != "░░░░" || bar(0.5, 4) != "██░░" {
		t.Error("bar is off")
	}
}
