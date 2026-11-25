package tmetric

import (
	"encoding/json"
	"fmt"
	"time"
)

// User is the owner of the API token.
type User struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Email           string    `json:"email"`
	ActiveAccountID int64     `json:"activeAccountId"`
	TimeZone        TimeZone  `json:"timeZone"`
	Accounts        []Account `json:"accounts"`
}

// TimeZone is the zone every time of the user is written in.
type TimeZone struct {
	// ID is the IANA name, such as "Europe/Madrid".
	ID string `json:"id"`
}

// Account is a workspace.
type Account struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// FirstWeekDay is 0 for Sunday through 6 for Saturday.
	FirstWeekDay time.Weekday `json:"firstWeekDay"`
}

// TimeEntry is a span of tracked time. A running timer is an entry with a
// nil EndTime.
type TimeEntry struct {
	ID        int64      `json:"id,omitempty"`
	StartTime LocalTime  `json:"startTime"`
	EndTime   *LocalTime `json:"endTime"`
	Note      string     `json:"note,omitempty"`
	Project   *Project   `json:"project,omitempty"`
	Task      *Task      `json:"task,omitempty"`
	// Tags are passed back untouched, so an update never drops them.
	Tags []json.RawMessage `json:"tags,omitempty"`
}

// RecentEntry is something the user worked on lately: what, not when.
type RecentEntry struct {
	Note    string            `json:"note,omitempty"`
	Project *Project          `json:"project,omitempty"`
	Task    *Task             `json:"task,omitempty"`
	Tags    []json.RawMessage `json:"tags,omitempty"`
	// IsPinned marks the ones pinned in TMetric to stay on the list.
	IsPinned bool `json:"isPinned,omitempty"`
}

// Project is a project an entry can be tracked on.
type Project struct {
	ID     int64     `json:"id"`
	Name   string    `json:"name,omitempty"`
	Client *Customer `json:"client,omitempty"`
}

// Customer is the client a project is for.
type Customer struct {
	Name string `json:"name"`
}

// Task is a task of a project. A task from an integration such as Jira can
// have a nil ID and an external link instead.
type Task struct {
	ID   *int64 `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	// ExternalLink and Integration are passed back untouched, so an update
	// keeps the link to the external task.
	ExternalLink json.RawMessage `json:"externalLink,omitempty"`
	Integration  json.RawMessage `json:"integration,omitempty"`
	Project      *Project        `json:"project,omitempty"`
	IsCompleted  bool            `json:"isCompleted,omitempty"`
}

// localLayout is how the API writes times: wall clock, no zone. A zone or a
// "Z" would make the API move the entry to another day.
const localLayout = "2006-01-02T15:04:05"

// LocalTime is a wall-clock time in the user's timezone, as the API reads and
// writes it.
type LocalTime struct {
	wall time.Time // in UTC, only as a carrier for the wall clock
}

// NewLocalTime keeps the wall clock of t in its own location.
func NewLocalTime(t time.Time) LocalTime {
	return LocalTime{wall: time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)}
}

// In places the wall clock in loc, the user's timezone.
func (t LocalTime) In(loc *time.Location) time.Time {
	w := t.wall
	return time.Date(w.Year(), w.Month(), w.Day(), w.Hour(), w.Minute(), w.Second(), w.Nanosecond(), loc)
}

// String is the wall clock as the API writes it.
func (t LocalTime) String() string { return t.wall.Format(localLayout) }

// MarshalJSON writes the wall clock without a zone, to the second.
func (t LocalTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON reads a wall clock, with or without fractional seconds.
func (t *LocalTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("reading a time: %w", err)
	}
	// The ".999999999" layout makes the fraction optional.
	wall, err := time.Parse(localLayout+".999999999", s)
	if err != nil {
		return fmt.Errorf("reading the time %q: %w", s, err)
	}
	t.wall = wall
	return nil
}
