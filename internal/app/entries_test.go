package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

func TestWeekStart(t *testing.T) {
	// Wednesday 2026-11-25.
	wednesday := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	for first, want := range map[time.Weekday]string{
		time.Sunday:    "2026-11-22",
		time.Monday:    "2026-11-23",
		time.Tuesday:   "2026-11-24",
		time.Wednesday: "2026-11-25", // today is the first day
		time.Thursday:  "2026-11-19",
		time.Friday:    "2026-11-20",
		time.Saturday:  "2026-11-21",
	} {
		if got := WeekStart(wednesday, first).Format(time.DateOnly); got != want {
			t.Errorf("first day %v: got %s, want %s", first, got, want)
		}
	}
}

func TestToday(t *testing.T) {
	session, _ := newFake(t, map[string]string{
		// 14:30:45 in Brisbane is still 2026-11-24 there, whatever this machine's zone.
		"GET /accounts/42/timeentries?endDate=2026-11-24&startDate=2026-11-24": `[` + finished + `,` + running + `]`,
	})
	report, err := session.Today(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.From.Format(time.DateOnly) != "2026-11-24" || !report.From.Equal(report.To) {
		t.Errorf("range = %v..%v", report.From, report.To)
	}
	// 13:00–13:30 plus 13:00–now (14:30:45).
	if want := 30*time.Minute + 90*time.Minute + 45*time.Second; report.Total != want || len(report.Entries) != 2 {
		t.Errorf("total = %v, entries = %d", report.Total, len(report.Entries))
	}
}

func TestWeekUsesTheWorkspaceFirstDay(t *testing.T) {
	session, _ := newFake(t, map[string]string{
		"GET /accounts/42/timeentries?endDate=2026-11-28&startDate=2026-11-22": `[]`,
	})
	session.User.Accounts = []tmetric.Account{{ID: 1, FirstWeekDay: time.Saturday}, {ID: 42, FirstWeekDay: time.Sunday}}
	report, err := session.Week(context.Background())
	if err != nil || report.Total != 0 {
		t.Fatalf("report = %+v, err = %v", report, err)
	}
}

func TestWeekDefaultsToMonday(t *testing.T) {
	session, _ := newFake(t, map[string]string{
		"GET /accounts/42/timeentries?endDate=2026-11-29&startDate=2026-11-23": `[]`,
	})
	if _, err := session.Week(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReportError(t *testing.T) {
	session, _ := newFake(t, map[string]string{
		"GET /accounts/42/timeentries?endDate=2026-11-24&startDate=2026-11-24": "HTTP 500",
	})
	if _, err := session.Today(context.Background()); err == nil {
		t.Error("want an error")
	}
}

func TestProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			_, _ = io.WriteString(w, userJSON)
			return
		}
		_, _ = io.WriteString(w, `[{"id":5,"name":"CLI"}]`)
	}))
	t.Cleanup(server.Close)
	session, err := NewSession(context.Background(), tmetric.NewClient(server.URL, "t", "", nil))
	if err != nil {
		t.Fatal(err)
	}
	projects, err := session.Projects(context.Background())
	if err != nil || len(projects) != 1 {
		t.Errorf("projects = %+v, err = %v", projects, err)
	}
}
