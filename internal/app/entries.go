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
	return s.Day(ctx, s.Midnight())
}

// Week is what was tracked this week.
func (s *Session) Week(ctx context.Context) (*Report, error) {
	return s.WeekOf(ctx, s.Midnight())
}

// Day is what was tracked on the day of day.
func (s *Session) Day(ctx context.Context, day time.Time) (*Report, error) {
	return s.report(ctx, day, day)
}

// WeekOf is what was tracked in the week that holds day, which starts on the
// workspace's first day of the week.
func (s *Session) WeekOf(ctx context.Context, day time.Time) (*Report, error) {
	from := s.WeekStart(day)
	return s.report(ctx, from, from.AddDate(0, 0, 6))
}

// WeekStart is the first day of the workspace's week that holds day.
func (s *Session) WeekStart(day time.Time) time.Time {
	return WeekStart(day, s.FirstWeekDay())
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

// Midnight is midnight today in the user's timezone.
func (s *Session) Midnight() time.Time {
	now := s.Now().In(s.Location)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.Location)
}

// Account is the active workspace, or nil when it is not in the user's list.
func (s *Session) Account() *tmetric.Account {
	for i := range s.User.Accounts {
		if s.User.Accounts[i].ID == s.User.ActiveAccountID {
			return &s.User.Accounts[i]
		}
	}
	return nil
}

// FirstWeekDay is the active workspace's first day of the week, Monday when
// the workspace is not in the user's list.
func (s *Session) FirstWeekDay() time.Weekday {
	if account := s.Account(); account != nil {
		return account.FirstWeekDay
	}
	return time.Monday
}
