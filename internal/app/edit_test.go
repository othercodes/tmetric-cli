package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	dayRoute   = "GET /accounts/42/timeentries?endDate=2026-11-24&startDate=2026-11-24"
	put1       = "PUT /accounts/42/timeentries/1"
	reviewJSON = `{"id":1,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T10:00:00","note":"review","project":{"id":5,"name":"CLI"},"task":{"id":9,"name":"Tests"},"tags":[{"id":3}]}`
	bugsJSON   = `{"id":2,"startTime":"2026-11-24T11:00:00","endTime":"2026-11-24T12:00:00","note":"bugs"}`
	runJSON    = `{"id":3,"startTime":"2026-11-24T13:00:00","endTime":null,"note":"standup"}`
	theDay     = `[` + reviewJSON + `,` + bugsJSON + `,` + runJSON + `]`
)

func day() time.Time {
	loc, _ := time.LoadLocation("Australia/Brisbane")
	return time.Date(2026, 11, 24, 0, 0, 0, 0, loc)
}

func ptr(s string) *string { return &s }

func TestEditTimesAndNote(t *testing.T) {
	session, fake := newFake(t, map[string]string{
		dayRoute: theDay,
		// Stored under a new ID, next to the day's other entries.
		put1: `[{"id":7,"startTime":"2026-11-24T08:30:00","endTime":"2026-11-24T09:45:00","note":"code review"},` + bugsJSON + `]`,
	})
	edited, err := session.Edit(context.Background(), day(), 1, Change{Note: ptr("code review"), Start: ptr("08:30"), End: ptr(" 09:45 ")})
	if err != nil {
		t.Fatal(err)
	}
	if edited.ID != 7 {
		t.Errorf("edited = %+v", edited)
	}
	want := `{"startTime":"2026-11-24T08:30:00","endTime":"2026-11-24T09:45:00","note":"code review","project":{"id":5,"name":"CLI"},"task":{"id":9,"name":"Tests"},"tags":[{"id":3}]}`
	if got := fake.bodies[put1]; got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestEditRefusesOverlaps(t *testing.T) {
	session, _ := newFake(t, map[string]string{dayRoute: theDay})
	_, err := session.Edit(context.Background(), day(), 1, Change{End: ptr("11:30")})
	var overlap *OverlapError
	if !errors.As(err, &overlap) || len(overlap.Entries) != 1 || overlap.Entries[0].ID != 2 {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "2  11:00–12:00  bugs") {
		t.Errorf("message = %q", err.Error())
	}
	// The running timer reaches until now (14:30).
	_, err = session.Edit(context.Background(), day(), 2, Change{End: ptr("14:00")})
	if !errors.As(err, &overlap) || !strings.Contains(err.Error(), "3  13:00–now  standup") {
		t.Errorf("err = %v", err)
	}
	// Moving the running timer's start back covers what came before it.
	_, err = session.Edit(context.Background(), day(), 3, Change{Start: ptr("11:30")})
	if !errors.As(err, &overlap) || overlap.Entries[0].ID != 2 {
		t.Errorf("err = %v", err)
	}
}

func TestEditOverwrites(t *testing.T) {
	session, fake := newFake(t, map[string]string{
		dayRoute: theDay,
		put1:     `[{"id":7,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T11:30:00"}]`,
	})
	if _, err := session.Edit(context.Background(), day(), 1, Change{End: ptr("11:30"), Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if fake.bodies[put1] == "" {
		t.Error("an overwrite should be sent")
	}
}

func TestEditProjectAndTask(t *testing.T) {
	session, fake := newFake(t, map[string]string{
		dayRoute:                               theDay,
		projects:                               `[{"id":5,"name":"CLI"},{"id":6,"name":"Docs"}]`,
		"GET /accounts/42/tasks?ProjectList=6": `[{"id":20,"name":"Write"}]`,
		put1:                                   `[{"id":7,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T10:00:00"}]`,
	})
	// A new project drops the old task.
	if _, err := session.Edit(context.Background(), day(), 1, Change{Project: ptr("docs")}); err != nil {
		t.Fatal(err)
	}
	if body := fake.bodies[put1]; !strings.Contains(body, `"project":{"id":6,"name":"Docs"}`) || strings.Contains(body, `"task"`) {
		t.Errorf("body = %s", body)
	}
	// Or picks one of the new project.
	if _, err := session.Edit(context.Background(), day(), 1, Change{Project: ptr("6"), Task: ptr("write")}); err != nil {
		t.Fatal(err)
	}
	if body := fake.bodies[put1]; !strings.Contains(body, `"task":{"id":20,"name":"Write"}`) {
		t.Errorf("body = %s", body)
	}
	// The same project keeps the task; an empty task removes it.
	if _, err := session.Edit(context.Background(), day(), 1, Change{Project: ptr("5")}); err != nil {
		t.Fatal(err)
	}
	if body := fake.bodies[put1]; !strings.Contains(body, `"task":{"id":9`) {
		t.Errorf("body = %s", body)
	}
	if _, err := session.Edit(context.Background(), day(), 1, Change{Task: ptr("")}); err != nil {
		t.Fatal(err)
	}
	if body := fake.bodies[put1]; strings.Contains(body, `"task"`) {
		t.Errorf("body = %s", body)
	}
}

func TestEditUsageErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		id     int64
		change Change
		want   string
	}{
		{"unknown entry", 99, Change{}, "no entry 99 on 2026-11-24; pass the day it is on with --date"},
		{"bad time", 1, Change{Start: ptr("9am")}, `"9am" is not a time: use HH:MM, such as 09:30`},
		{"bad end", 1, Change{End: ptr("25:00")}, `"25:00" is not a time`},
		{"end before start", 1, Change{End: ptr("08:00")}, "the end (08:00) must be after the start (09:00)"},
		{"same minute", 1, Change{End: ptr("09:00")}, "the end (09:00) must be after the start (09:00)"},
		{"task without project", 2, Change{Task: ptr("x")}, "--task needs a project"},
		{"unknown project", 1, Change{Project: ptr("nope")}, `no project matches "nope"`},
		{"unknown task", 1, Change{Task: ptr("nope")}, `no task matches "nope"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session, _ := newFake(t, map[string]string{
				dayRoute: theDay, projects: `[{"id":5,"name":"CLI"}]`, tasks: `[]`,
			})
			_, err := session.Edit(context.Background(), day(), tt.id, tt.change)
			var usage *UsageError
			if !errors.As(err, &usage) || !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEditAPIErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		routes map[string]string
		want   string
	}{
		{"day fails", map[string]string{dayRoute: "HTTP 500"}, "HTTP 500"},
		{"update fails", map[string]string{dayRoute: theDay, put1: "HTTP 500"}, "HTTP 500"},
		{"answer missing", map[string]string{dayRoute: theDay, put1: `[` + bugsJSON + `]`}, "did not return it"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session, _ := newFake(t, tt.routes)
			if _, err := session.Edit(context.Background(), day(), 1, Change{Note: ptr("x")}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	session, fake := newFake(t, map[string]string{"DELETE /accounts/42/timeentries/1": ""})
	if err := session.Delete(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.bodies["DELETE /accounts/42/timeentries/1"]; !ok {
		t.Error("no DELETE was sent")
	}
}
