// Package app holds what the CLI and the TUI do against the TMetric API, free
// of cobra and of any rendering, so both front-ends share one behaviour.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// UsageError is a request the caller has to change, such as a project name
// that matches nothing; the CLI exits 2 on it.
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

func usagef(format string, args ...any) error {
	return &UsageError{Msg: fmt.Sprintf(format, args...)}
}

// Session is a signed-in user and the workspace and timezone commands act in.
type Session struct {
	Client *tmetric.Client
	User   *tmetric.User
	// Location is the user's TMetric timezone. Every time the API reads or
	// writes is a wall clock there, whatever zone this machine is in.
	Location *time.Location
	// Now is the clock; tests replace it.
	Now func() time.Time
}

// NewSession reads the user behind the client's token.
func NewSession(ctx context.Context, client *tmetric.Client) (*Session, error) {
	user, err := client.User(ctx)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(user.TimeZone.ID)
	if err != nil {
		return nil, fmt.Errorf("reading the TMetric timezone %q: %w", user.TimeZone.ID, err)
	}
	return &Session{Client: client, User: user, Location: location, Now: time.Now}, nil
}

func (s *Session) accountID() int64 { return s.User.ActiveAccountID }

// now is the current wall clock in the user's timezone.
func (s *Session) now() tmetric.LocalTime {
	return tmetric.NewLocalTime(s.Now().In(s.Location))
}

// Elapsed is how long an entry lasts, or has lasted so far while it runs.
func (s *Session) Elapsed(entry *tmetric.TimeEntry) time.Duration {
	end := s.Now()
	if entry.EndTime != nil {
		end = entry.EndTime.In(s.Location)
	}
	return end.Sub(entry.StartTime.In(s.Location))
}
