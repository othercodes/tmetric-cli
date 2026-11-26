package app

import (
	"context"
	"testing"
	"time"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

func TestRanges(t *testing.T) {
	// Tuesday 2026-11-24 in Brisbane; the workspace week starts on Monday.
	session, _ := newFake(t, map[string]string{})
	session.User.Accounts = nil // falls back to Monday
	for name, want := range map[string][2]string{
		"week":       {"2026-11-23", "2026-11-29"},
		"last-week":  {"2026-11-16", "2026-11-22"},
		"month":      {"2026-11-01", "2026-11-30"},
		"last-month": {"2026-10-01", "2026-10-31"},
	} {
		from, to, err := session.Range(name)
		if err != nil || from.Format(time.DateOnly) != want[0] || to.Format(time.DateOnly) != want[1] {
			t.Errorf("%s = %v..%v (%v), want %s..%s", name, from, to, err, want[0], want[1])
		}
	}
	if _, _, err := session.Range("year"); err == nil || err.Error() != `unknown range "year": use week, last-week, month or last-month` {
		t.Errorf("err = %v", err)
	}
}

func TestProjectReportBiggestFirst(t *testing.T) {
	route := "GET /accounts/42/reports/projects?endDate=2026-11-29&includeDone=true&startDate=2026-11-23&userId=7"
	session, _ := newFake(t, map[string]string{route: `[{"totalSeconds":60},{"project":{"id":5},"totalSeconds":3600},{"totalSeconds":600}]`})
	from, to, _ := session.Range("week")
	report, err := session.ProjectReport(context.Background(), from, to)
	if err != nil || len(report) != 3 || report[0].TotalSeconds != 3600 || report[2].TotalSeconds != 60 {
		t.Errorf("report = %+v, err = %v", report, err)
	}
	session, _ = newFake(t, map[string]string{route: "HTTP 500"})
	if _, err := session.ProjectReport(context.Background(), from, to); err == nil {
		t.Error("want an error")
	}
}

func TestBalance(t *testing.T) {
	session, _ := newFake(t, map[string]string{"GET /accounts/42/balance": `{"week":{"requiredSeconds":144000,"actualSeconds":57600}}`})
	balance, err := session.Balance(context.Background())
	if err != nil || balance.Week.RequiredSeconds != 144000 {
		t.Errorf("balance = %+v, err = %v", balance, err)
	}
}

func TestHasSchedule(t *testing.T) {
	if HasSchedule(&tmetric.Balance{}) {
		t.Error("no hours required this month means no schedule")
	}
	// A day off inside a schedule still has one.
	if !HasSchedule(&tmetric.Balance{Month: tmetric.TimeBalance{RequiredSeconds: 3600}}) {
		t.Error("hours required this month mean a schedule")
	}
}
