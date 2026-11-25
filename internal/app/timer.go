package app

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// Running returns the running timer, or nil when none runs.
func (s *Session) Running(ctx context.Context) (*tmetric.TimeEntry, error) {
	latest, err := s.Client.LatestEntry(ctx, s.accountID())
	if err != nil || latest == nil || latest.EndTime != nil {
		return nil, err
	}
	return latest, nil
}

// StartOptions describe a new timer. Project and Task take an ID or a name.
type StartOptions struct {
	Note    string
	Project string
	Task    string
}

// Start starts a timer now. TMetric closes the timer that was running, which
// comes back as stopped (nil when none ran).
func (s *Session) Start(ctx context.Context, opts StartOptions) (started, stopped *tmetric.TimeEntry, err error) {
	if opts.Task != "" && opts.Project == "" {
		return nil, nil, usagef("--task needs --project: tasks are looked up within a project")
	}
	entry := tmetric.TimeEntry{StartTime: s.now(), Note: opts.Note}
	if opts.Project != "" {
		if entry.Project, err = s.findProject(ctx, opts.Project); err != nil {
			return nil, nil, err
		}
	}
	if opts.Task != "" {
		if entry.Task, err = s.findTask(ctx, entry.Project.ID, opts.Task); err != nil {
			return nil, nil, err
		}
	}
	return s.begin(ctx, entry)
}

// Continue starts a timer now with the description, project and task of
// entry. The task goes back whole, so a link to an external task stays.
func (s *Session) Continue(ctx context.Context, entry *tmetric.TimeEntry) (started, stopped *tmetric.TimeEntry, err error) {
	next := tmetric.TimeEntry{StartTime: s.now(), Note: entry.Note}
	if entry.Project != nil {
		next.Project = &tmetric.Project{ID: entry.Project.ID, Name: entry.Project.Name}
	}
	if entry.Task != nil {
		task := *entry.Task
		task.Project = nil
		next.Task = &task
	}
	return s.begin(ctx, next)
}

// Recent is what the user worked on lately, the pinned ones first.
func (s *Session) Recent(ctx context.Context) ([]tmetric.RecentEntry, error) {
	recent, err := s.Client.RecentEntries(ctx, s.accountID())
	if err != nil {
		return nil, err
	}
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].IsPinned && !recent[j].IsPinned })
	return recent, nil
}

// begin creates the open entry and finds it in the answer, after noting the
// timer that ran, which TMetric closes.
func (s *Session) begin(ctx context.Context, entry tmetric.TimeEntry) (started, stopped *tmetric.TimeEntry, err error) {
	if stopped, err = s.Running(ctx); err != nil {
		return nil, nil, err
	}
	entries, err := s.Client.CreateEntry(ctx, s.accountID(), entry)
	if err != nil {
		return nil, nil, err
	}
	// The API answers with every entry of the day, with times cut to the
	// minute. Only one timer runs at a time, so the new one is the open entry.
	for i := range entries {
		if entries[i].EndTime == nil {
			return &entries[i], stopped, nil
		}
	}
	return nil, nil, errors.New("the timer may have started, but the API did not return it: check `tmetric timer status`")
}

// OpenTasks are the tasks of a project a timer can point at: not completed,
// and not external (an external task has no ID).
func (s *Session) OpenTasks(ctx context.Context, projectID int64) ([]tmetric.Task, error) {
	all, err := s.Client.Tasks(ctx, s.accountID(), projectID)
	if err != nil {
		return nil, err
	}
	var tasks []tmetric.Task
	for _, t := range all {
		if t.ID != nil && !t.IsCompleted {
			tasks = append(tasks, t)
		}
	}
	return tasks, nil
}

// Stopped is a timer that Stop ended.
type Stopped struct {
	Entry *tmetric.TimeEntry
	// Dropped means TMetric deleted the entry instead of keeping it, which it
	// does to entries shorter than a minute.
	Dropped bool
}

// Stop stops the running timer now, or returns nil when none runs.
func (s *Session) Stop(ctx context.Context) (*Stopped, error) {
	running, err := s.Running(ctx)
	if err != nil || running == nil {
		return nil, err
	}
	end := s.now()
	running.EndTime = &end
	// PUT replaces the entry, so it goes back whole: tags and task links stay.
	entries, err := s.Client.UpdateEntry(ctx, s.accountID(), *running)
	if err != nil {
		return nil, err
	}
	// The API stores the stopped entry under a new ID, so it is found by its
	// start, which the API already cut to the minute when the timer started.
	for i := range entries {
		if entries[i].EndTime != nil && entries[i].StartTime.String() == running.StartTime.String() {
			return &Stopped{Entry: &entries[i]}, nil
		}
	}
	return &Stopped{Entry: running, Dropped: true}, nil
}

func (s *Session) findProject(ctx context.Context, ref string) (*tmetric.Project, error) {
	projects, err := s.Client.Projects(ctx, s.accountID())
	if err != nil {
		return nil, err
	}
	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = p.Name
	}
	i, err := match(ref, "project", names, func(i int) int64 { return projects[i].ID })
	if err != nil {
		return nil, err
	}
	return &tmetric.Project{ID: projects[i].ID, Name: projects[i].Name}, nil
}

func (s *Session) findTask(ctx context.Context, projectID int64, ref string) (*tmetric.Task, error) {
	tasks, err := s.OpenTasks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.Name
	}
	i, err := match(ref, "task", names, func(i int) int64 { return *tasks[i].ID })
	if err != nil {
		return nil, err
	}
	return &tmetric.Task{ID: tasks[i].ID, Name: tasks[i].Name}, nil
}

// match finds ref among names, as an ID or as a name compared without case.
// Zero or several matches is a usage error that lists what there is.
func match(ref, kind string, names []string, id func(int) int64) (int, error) {
	var found []int
	if n, err := strconv.ParseInt(ref, 10, 64); err == nil {
		for i := range names {
			if id(i) == n {
				found = append(found, i)
			}
		}
	}
	if len(found) == 0 {
		for i, name := range names {
			if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(ref)) {
				found = append(found, i)
			}
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return 0, usagef("no %s matches %q; pass its ID or exact name, one of:\n    %s", kind, ref, list(names, id, nil))
	default:
		return 0, usagef("%d %ss match %q; pass the ID of one of:\n    %s", len(found), kind, ref, list(names, id, found))
	}
}

// list renders "ID  name" lines, for the indexes in only or for all of them.
func list(names []string, id func(int) int64, only []int) string {
	if only == nil {
		for i := range names {
			only = append(only, i)
		}
	}
	if len(only) == 0 {
		return "(none)"
	}
	lines := make([]string, len(only))
	for j, i := range only {
		lines[j] = strconv.FormatInt(id(i), 10) + "  " + names[i]
	}
	return strings.Join(lines, "\n    ")
}
