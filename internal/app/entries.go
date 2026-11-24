package app

import (
	"context"
	"time"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// Report is the time tracked over a range of days.
type Report struct {
	// From and To are midnights in the user's timezone; both days count.
	From, To time.Time
	Entries  []tmetric.TimeEntry
	// Total counts a running timer up to now.
	Total time.Duration
}

// Today is what was tracked today, in the user's timezone.
func (s *Session) Today(ctx context.Context) (*Report, error) {
	today := s.today()
	return s.report(ctx, today, today)
}

// Week is what was tracked this week, which starts on the workspace's first
// day of the week.
func (s *Session) Week(ctx context.Context) (*Report, error) {
	from := WeekStart(s.today(), s.firstWeekDay())
	return s.report(ctx, from, from.AddDate(0, 0, 6))
}

// Projects are the projects the user can track time on.
func (s *Session) Projects(ctx context.Context) ([]tmetric.Project, error) {
	return s.Client.Projects(ctx, s.accountID())
}

// WeekStart is the most recent first day of the week on or before day.
func WeekStart(day time.Time, first time.Weekday) time.Time {
	back := (int(day.Weekday()) - int(first) + 7) % 7
	return day.AddDate(0, 0, -back)
}

func (s *Session) report(ctx context.Context, from, to time.Time) (*Report, error) {
	entries, err := s.Client.Entries(ctx, s.accountID(), from, to)
	if err != nil {
		return nil, err
	}
	report := &Report{From: from, To: to, Entries: entries}
	for i := range entries {
		report.Total += s.Elapsed(&entries[i])
	}
	return report, nil
}

// today is midnight today in the user's timezone.
func (s *Session) today() time.Time {
	now := s.Now().In(s.Location)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.Location)
}

// firstWeekDay is the active workspace's first day of the week, Monday when
// the workspace is not in the user's list.
func (s *Session) firstWeekDay() time.Weekday {
	for _, account := range s.User.Accounts {
		if account.ID == s.User.ActiveAccountID {
			return account.FirstWeekDay
		}
	}
	return time.Monday
}
