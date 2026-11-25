package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// Change is an edit of a time entry. A nil field stays as it is.
type Change struct {
	Note *string
	// Project and Task take an ID or an exact name; an empty Task removes the
	// task. A new project drops the old task, unless Task picks one of it.
	Project, Task *string
	// Start and End are wall clocks ("15:04") on the entry's day. An End on
	// a running timer stops it.
	Start, End *string
	// Overwrite lets the edit cover other entries, which TMetric then cuts
	// or removes. Without it, an overlap is an *OverlapError.
	Overwrite bool
}

// OverlapError is an edit that would cover other entries of the day.
type OverlapError struct{ Entries []tmetric.TimeEntry }

func (e *OverlapError) Error() string {
	lines := make([]string, len(e.Entries))
	for i, entry := range e.Entries {
		end := "now"
		if entry.EndTime != nil {
			end = clock(*entry.EndTime)
		}
		lines[i] = fmt.Sprintf("%d  %s–%s  %s", entry.ID, clock(entry.StartTime), end, entry.Note)
	}
	return "the change would overwrite these entries, which TMetric would cut or remove:\n    " + strings.Join(lines, "\n    ")
}

func clock(t tmetric.LocalTime) string { return t.String()[11:16] }

// Find returns the entry id on day and the day's other entries.
func (s *Session) Find(ctx context.Context, day time.Time, id int64) (*tmetric.TimeEntry, []tmetric.TimeEntry, error) {
	entries, err := s.Client.Entries(ctx, s.accountID(), day, day)
	if err != nil {
		return nil, nil, err
	}
	var found *tmetric.TimeEntry
	var others []tmetric.TimeEntry
	for i := range entries {
		if entries[i].ID == id {
			found = &entries[i]
		} else {
			others = append(others, entries[i])
		}
	}
	if found == nil {
		return nil, nil, usagef("no entry %d on %s; pass the day it is on with --date", id, day.Format(time.DateOnly))
	}
	return found, others, nil
}

// Edit changes the entry id on day. TMetric stores the result under a new ID,
// which the returned entry carries.
func (s *Session) Edit(ctx context.Context, day time.Time, id int64, change Change) (*tmetric.TimeEntry, error) {
	entry, others, err := s.Find(ctx, day, id)
	if err != nil {
		return nil, err
	}
	if err := s.apply(ctx, entry, change); err != nil {
		return nil, err
	}
	if !change.Overwrite {
		if covered := s.overlaps(entry, others); len(covered) > 0 {
			return nil, &OverlapError{Entries: covered}
		}
	}
	entries, err := s.Client.UpdateEntry(ctx, s.accountID(), *entry)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].StartTime.String() == entry.StartTime.String() {
			return &entries[i], nil
		}
	}
	return nil, fmt.Errorf("the entry may have changed, but the API did not return it: check `tmetric today`")
}

// Delete removes the entry id.
func (s *Session) Delete(ctx context.Context, id int64) error {
	return s.Client.DeleteEntry(ctx, s.accountID(), id)
}

// apply puts change into entry, checking it as it goes.
func (s *Session) apply(ctx context.Context, entry *tmetric.TimeEntry, change Change) error {
	if change.Note != nil {
		entry.Note = *change.Note
	}
	if err := s.applyProjectTask(ctx, entry, change); err != nil {
		return err
	}
	return s.applyTimes(entry, change)
}

// applyProjectTask resolves a new project and task. A new project drops the
// old task; an empty task removes it.
func (s *Session) applyProjectTask(ctx context.Context, entry *tmetric.TimeEntry, change Change) error {
	if change.Project != nil {
		project, err := s.findProject(ctx, *change.Project)
		if err != nil {
			return err
		}
		if entry.Project == nil || entry.Project.ID != project.ID {
			entry.Task = nil
		}
		entry.Project = project
	}
	switch {
	case change.Task == nil:
		return nil
	case *change.Task == "":
		entry.Task = nil
		return nil
	case entry.Project == nil:
		return usagef("--task needs a project: the entry has none, pass --project too")
	}
	task, err := s.findTask(ctx, entry.Project.ID, *change.Task)
	if err != nil {
		return err
	}
	entry.Task = task
	return nil
}

// applyTimes sets a new start and end on the entry's day, and checks that the
// entry still lasts.
func (s *Session) applyTimes(entry *tmetric.TimeEntry, change Change) error {
	day := entry.StartTime.In(s.Location)
	if change.Start != nil {
		start, err := at(day, *change.Start)
		if err != nil {
			return err
		}
		entry.StartTime = start
	}
	if change.End != nil {
		end, err := at(day, *change.End)
		if err != nil {
			return err
		}
		entry.EndTime = &end
	}
	// TMetric drops an entry that starts and ends in the same minute.
	if entry.EndTime != nil && entry.EndTime.String() <= entry.StartTime.String() {
		return usagef("the end (%s) must be after the start (%s)", clock(*entry.EndTime), clock(entry.StartTime))
	}
	return nil
}

// at is the wall clock hh:mm on day.
func at(day time.Time, hhmm string) (tmetric.LocalTime, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(hhmm))
	if err != nil {
		return tmetric.LocalTime{}, usagef("%q is not a time: use HH:MM, such as 09:30", hhmm)
	}
	return tmetric.NewLocalTime(time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, day.Location())), nil
}

// overlaps are the other entries that entry would cover. A running entry
// reaches until now.
func (s *Session) overlaps(entry *tmetric.TimeEntry, others []tmetric.TimeEntry) []tmetric.TimeEntry {
	now := s.now().String()
	end := func(e *tmetric.TimeEntry) string {
		if e.EndTime == nil {
			return now
		}
		return e.EndTime.String()
	}
	var covered []tmetric.TimeEntry
	for i := range others {
		other := &others[i]
		if entry.StartTime.String() < end(other) && other.StartTime.String() < end(entry) {
			covered = append(covered, *other)
		}
	}
	return covered
}
