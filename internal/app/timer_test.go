package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

const userJSON = `{"id":7,"activeAccountId":42,"timeZone":{"id":"Australia/Brisbane"}}`

// fakeAPI answers "METHOD path" with canned bodies and records request bodies.
type fakeAPI struct {
	t      *testing.T
	routes map[string]string
	bodies map[string]string
}

func newFake(t *testing.T, routes map[string]string) (*Session, *fakeAPI) {
	t.Helper()
	fake := &fakeAPI{t: t, routes: routes, bodies: map[string]string{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	routes["GET /user"] = userJSON
	session, err := NewSession(context.Background(), tmetric.NewClient(server.URL, "t", "", nil))
	if err != nil {
		t.Fatal(err)
	}
	// 14:30:45 in Brisbane (UTC+10, no daylight saving).
	session.Now = func() time.Time { return time.Date(2026, 11, 24, 4, 30, 45, 0, time.UTC) }
	return session, fake
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		key += "?" + r.URL.RawQuery
	}
	raw, _ := io.ReadAll(r.Body)
	f.bodies[key] = string(raw)
	body, ok := f.routes[key]
	if !ok {
		f.t.Errorf("unexpected request: %s", key)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if strings.HasPrefix(body, "HTTP ") {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, body)
}

const (
	latest   = "GET /accounts/42/timeentries/latest"
	create   = "POST /accounts/42/timeentries"
	projects = "GET /accounts/42/timeentries/projects"
	tasks    = "GET /accounts/42/tasks?ProjectList=5"
	running  = `{"id":1,"startTime":"2026-11-24T13:00:00","endTime":null,"note":"standup","project":{"id":5,"name":"CLI"},"tags":[{"id":3}]}`
	finished = `{"id":1,"startTime":"2026-11-24T13:00:00","endTime":"2026-11-24T13:30:00"}`
)

func TestNewSessionErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":7,"timeZone":{"id":"Mars/Olympus_Mons"}}`)
	}))
	t.Cleanup(server.Close)
	if _, err := NewSession(context.Background(), tmetric.NewClient(server.URL, "t", "", nil)); err == nil ||
		!strings.Contains(err.Error(), "Mars/Olympus_Mons") {
		t.Errorf("err = %v", err)
	}
	if _, err := NewSession(context.Background(), tmetric.NewClient("http://127.0.0.1:1", "t", "", nil)); err == nil {
		t.Error("an unreachable API should fail")
	}
}

func TestRunning(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       bool
	}{{"running", running, true}, {"finished", finished, false}, {"no entries", "null", false}} {
		session, _ := newFake(t, map[string]string{latest: tt.body})
		entry, err := session.Running(context.Background())
		if err != nil || (entry != nil) != tt.want {
			t.Errorf("%s: entry = %+v, err = %v", tt.name, entry, err)
		}
	}
}

func TestElapsed(t *testing.T) {
	session, _ := newFake(t, map[string]string{latest: running})
	entry, _ := session.Running(context.Background())
	if got := session.Elapsed(entry); got != 90*time.Minute+45*time.Second {
		t.Errorf("running: %v", got)
	}
	var done tmetric.TimeEntry
	_ = json.Unmarshal([]byte(finished), &done)
	if got := session.Elapsed(&done); got != 30*time.Minute {
		t.Errorf("finished: %v", got)
	}
}

func TestStartWithProjectAndTask(t *testing.T) {
	session, fake := newFake(t, map[string]string{
		projects: `[{"id":5,"name":"CLI"},{"id":6,"name":"Docs"}]`,
		tasks: `[{"id":8,"name":"Tests","isCompleted":true},{"id":9,"name":"Tests"},
			{"id":null,"name":"Tests","externalLink":{"link":"x"}}]`,
		latest: running,
		// Times come back cut to the minute.
		create: `[` + finished + `,{"id":2,"startTime":"2026-11-24T14:30:00","endTime":null,"project":{"id":5,"name":"CLI"}}]`,
	})
	started, stopped, err := session.Start(context.Background(), StartOptions{Note: "review", Project: "cli", Task: " tests "})
	if err != nil {
		t.Fatal(err)
	}
	if started.ID != 2 || stopped == nil || stopped.ID != 1 {
		t.Errorf("started = %+v, stopped = %+v", started, stopped)
	}
	want := `{"startTime":"2026-11-24T14:30:45","endTime":null,"note":"review","project":{"id":5,"name":"CLI"},"task":{"id":9,"name":"Tests"}}`
	if got := fake.bodies[create]; got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestStartByIDWithoutProject(t *testing.T) {
	session, fake := newFake(t, map[string]string{
		latest: "null",
		create: `[{"id":2,"startTime":"2026-11-24T14:30:00","endTime":null}]`,
	})
	started, stopped, err := session.Start(context.Background(), StartOptions{})
	if err != nil || started.ID != 2 || stopped != nil {
		t.Fatalf("started = %+v, stopped = %+v, err = %v", started, stopped, err)
	}
	if got := fake.bodies[create]; got != `{"startTime":"2026-11-24T14:30:45","endTime":null}` {
		t.Errorf("body = %s", got)
	}
}

func TestStartProjectByID(t *testing.T) {
	session, _ := newFake(t, map[string]string{
		// A project literally named "6" must not shadow the project with ID 6.
		projects: `[{"id":5,"name":"6"},{"id":6,"name":"Docs"}]`,
		latest:   "null",
		create:   `[{"id":2,"startTime":"2026-11-24T14:30:00","endTime":null,"project":{"id":6}}]`,
	})
	started, _, err := session.Start(context.Background(), StartOptions{Project: "6"})
	if err != nil || started.Project.ID != 6 {
		t.Errorf("started = %+v, err = %v", started, err)
	}
}

func TestStartUsageErrors(t *testing.T) {
	tests := []struct {
		name  string
		opts  StartOptions
		want  string
		extra map[string]string
	}{
		{"task without project", StartOptions{Task: "x"}, "--task needs --project: tasks are looked up within a project", nil},
		{"unknown project", StartOptions{Project: "nope"}, "no project matches \"nope\"; pass its ID or exact name, one of:\n    5  CLI\n    6  CLI", nil},
		{"ambiguous project", StartOptions{Project: "cli"}, "2 projects match \"cli\"; pass the ID of one of:\n    5  CLI\n    6  CLI", nil},
		{"unknown task", StartOptions{Project: "5", Task: "nope"}, "no task matches \"nope\"; pass its ID or exact name, one of:\n    (none)",
			map[string]string{tasks: `[]`}},
		{"unknown task among open ones", StartOptions{Project: "5", Task: "8"}, "no task matches \"8\"; pass its ID or exact name, one of:\n    9  Tests",
			map[string]string{tasks: `[{"id":8,"name":"Old","isCompleted":true},{"id":9,"name":"Tests"}]`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := map[string]string{projects: `[{"id":5,"name":"CLI"},{"id":6,"name":"CLI"}]`}
			for k, v := range tt.extra {
				routes[k] = v
			}
			session, _ := newFake(t, routes)
			_, _, err := session.Start(context.Background(), tt.opts)
			var usage *UsageError
			if !errors.As(err, &usage) || err.Error() != tt.want {
				t.Errorf("err =\n%v\nwant\n%s", err, tt.want)
			}
		})
	}
}

func TestStartAPIErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		opts   StartOptions
		routes map[string]string
		want   string
	}{
		{"projects fail", StartOptions{Project: "x"}, map[string]string{projects: "HTTP 500"}, "HTTP 500"},
		{"tasks fail", StartOptions{Project: "5", Task: "x"}, map[string]string{projects: `[{"id":5,"name":"CLI"}]`, tasks: "HTTP 500"}, "HTTP 500"},
		{"latest fails", StartOptions{}, map[string]string{latest: "HTTP 500"}, "HTTP 500"},
		{"create fails", StartOptions{}, map[string]string{latest: "null", create: "HTTP 500"}, "HTTP 500"},
		{"new entry missing", StartOptions{}, map[string]string{latest: "null", create: `[` + finished + `]`}, "did not return it"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session, _ := newFake(t, tt.routes)
			if _, _, err := session.Start(context.Background(), tt.opts); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestStopSendsTheWholeEntryBack(t *testing.T) {
	update := "PUT /accounts/42/timeentries/1"
	session, fake := newFake(t, map[string]string{
		latest: running,
		// The stopped entry comes back under a new ID, next to the day's others.
		update: `[{"id":7,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T10:00:00"},
			{"id":8,"startTime":"2026-11-24T13:00:00","endTime":"2026-11-24T14:30:00"}]`,
	})
	stopped, err := session.Stop(context.Background())
	if err != nil || stopped.Dropped || stopped.Entry.ID != 8 || stopped.Entry.EndTime.String() != "2026-11-24T14:30:00" {
		t.Fatalf("stopped = %+v, err = %v", stopped, err)
	}
	want := `{"startTime":"2026-11-24T13:00:00","endTime":"2026-11-24T14:30:45","note":"standup","project":{"id":5,"name":"CLI"},"tags":[{"id":3}]}`
	if got := fake.bodies[update]; got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestStopDropped(t *testing.T) {
	session, _ := newFake(t, map[string]string{latest: running,
		"PUT /accounts/42/timeentries/1": `[{"id":7,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T10:00:00"}]`})
	stopped, err := session.Stop(context.Background())
	if err != nil || !stopped.Dropped || stopped.Entry.ID != 1 {
		t.Errorf("stopped = %+v, err = %v", stopped, err)
	}
}

func TestStopWithoutTimer(t *testing.T) {
	session, _ := newFake(t, map[string]string{latest: finished})
	if stopped, err := session.Stop(context.Background()); err != nil || stopped != nil {
		t.Errorf("stopped = %+v, err = %v", stopped, err)
	}
	session, _ = newFake(t, map[string]string{latest: running, "PUT /accounts/42/timeentries/1": "HTTP 500"})
	if _, err := session.Stop(context.Background()); err == nil {
		t.Error("a failed update should be reported")
	}
}
