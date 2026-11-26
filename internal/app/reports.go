package app

import (
	"context"
	"sort"
	"time"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// Ranges are the names a report range takes, in the order the TUI cycles them.
var Ranges = []string{"week", "last-week", "month", "last-month"}

// Range is the first and last day of a named range, in the user's timezone.
func (s *Session) Range(name string) (from, to time.Time, err error) {
	today := s.Midnight()
	week := s.WeekStart(today)
	month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, s.Location)
	switch name {
	case "week":
		return week, week.AddDate(0, 0, 6), nil
	case "last-week":
		return week.AddDate(0, 0, -7), week.AddDate(0, 0, -1), nil
	case "month":
		return month, month.AddDate(0, 1, -1), nil
	case "last-month":
		return month.AddDate(0, -1, 0), month.AddDate(0, 0, -1), nil
	}
	return time.Time{}, time.Time{}, usagef("unknown range %q: use week, last-week, month or last-month", name)
}

// ProjectReport is the time the user tracked on each project from the day of
// from to the day of to, the biggest first. It asks for the user by ID: with
// no user the API may answer for the whole team.
func (s *Session) ProjectReport(ctx context.Context, from, to time.Time) ([]tmetric.ProjectReport, error) {
	report, err := s.Client.ProjectReport(ctx, s.accountID(), s.User.ID, from, to)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(report, func(i, j int) bool { return report[i].TotalSeconds > report[j].TotalSeconds })
	return report, nil
}

// Balance is the tracked time against the work schedule, today, this week and
// this month.
func (s *Session) Balance(ctx context.Context) (*tmetric.Balance, error) {
	return s.Client.Balance(ctx, s.accountID())
}

// HasSchedule reports whether the workspace has a work schedule. Without one
// TMetric requires no hours at all, and a difference against zero means
// nothing; a single day off (zero required) still has a schedule around it.
func HasSchedule(b *tmetric.Balance) bool {
	return b.Month.RequiredSeconds > 0
}
