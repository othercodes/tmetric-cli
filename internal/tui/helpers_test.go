package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/othercodes/tmetric-cli/internal/app"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// Routes of the fake API. "Now" is Tuesday 2026-11-24 14:30:45 in Brisbane;
// the workspace week starts on Monday 23.
const (
	userJSON      = `{"id":7,"name":"Ada","email":"ada@example.com","activeAccountId":42,"timeZone":{"id":"Australia/Brisbane"},"accounts":[{"id":42,"name":"Work","firstWeekDay":1}]}`
	latestRoute   = "GET /accounts/42/timeentries/latest"
	todayRoute    = "GET /accounts/42/timeentries?endDate=2026-11-24&startDate=2026-11-24"
	mondayRoute   = "GET /accounts/42/timeentries?endDate=2026-11-23&startDate=2026-11-23"
	weekRoute     = "GET /accounts/42/timeentries?endDate=2026-11-29&startDate=2026-11-23"
	lastWeekRoute = "GET /accounts/42/timeentries?endDate=2026-11-22&startDate=2026-11-16"
	projectsRoute = "GET /accounts/42/timeentries/projects"
	tasksRoute    = "GET /accounts/42/tasks?ProjectList=6"
	createRoute   = "POST /accounts/42/timeentries"
	updateRoute   = "PUT /accounts/42/timeentries/1"
	recentRoute   = "GET /accounts/42/timeentries/recent"
	runningEntry  = `{"id":1,"startTime":"2026-11-24T13:00:00","endTime":null,"note":"standup","project":{"id":5,"name":"CLI"}}`
	doneEntry     = `{"id":3,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T10:00:00","note":"review","project":{"id":5,"name":"CLI"},"task":{"id":9,"name":"Tests"}}`
	mondayEntry   = `{"id":4,"startTime":"2026-11-23T09:00:00","endTime":"2026-11-23T11:00:00","note":"bugs"}`
	projectsJSON  = `[{"id":5,"name":"CLI","client":{"name":"Acme"}},{"id":6,"name":"Docs"},{"id":7,"name":"Design"}]`
)

// bodies holds the request bodies the fake API received, by route.
var bodies map[string]string

func routes() map[string]string {
	return map[string]string{
		latestRoute:   runningEntry,
		todayRoute:    `[` + doneEntry + `,` + runningEntry + `]`,
		weekRoute:     `[` + mondayEntry + `,` + doneEntry + `,` + runningEntry + `]`,
		projectsRoute: projectsJSON,
		recentRoute:   "[]",
	}
}

// fakeSession is a session against a fake API that answers routes
// ("METHOD path?query" → body, or "HTTP 500").
func fakeSession(t *testing.T, routes map[string]string) *app.Session {
	t.Helper()
	bodies = map[string]string{}
	routes["GET /user"] = userJSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		raw, _ := io.ReadAll(r.Body)
		bodies[key] = string(raw)
		body, ok := routes[key]
		if !ok {
			t.Errorf("unexpected request: %s", key)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if body == "HTTP 500" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	session, err := app.NewSession(context.Background(), tmetric.NewClient(server.URL, "t", "", nil))
	if err != nil {
		t.Fatal(err)
	}
	session.Now = func() time.Time { return time.Date(2026, 11, 24, 4, 30, 45, 0, time.UTC) }
	return session
}

// run executes cmd and every command it batches or sequences, feeding each
// message back into the model, the way the Bubble Tea runtime does. Ticks
// and window titles are not fed back: they would loop forever.
func run(m model, cmd tea.Cmd) model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = run(m, c)
		}
	case tea.QuitMsg, tick, nil:
	default:
		if strings.HasPrefix(typeName(msg), "tea.") {
			return m
		}
		next, more := m.Update(msg)
		m = run(next.(model), more)
	}
	return m
}

func typeName(v any) string { return typeString(v) }

// loaded is a model of 100×30 that has loaded everything Init asks for.
func loaded(t *testing.T, r map[string]string) model {
	t.Helper()
	m := newModel(context.Background(), fakeSession(t, r))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(model)
	return run(m, m.Init())
}

// press feeds keys one by one, running what each one asks for.
func press(m model, keys ...string) model {
	for _, k := range keys {
		next, cmd := m.Update(keyMsg(k))
		m = run(next.(model), cmd)
	}
	return m
}

func keyMsg(k string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		"tab": tea.KeyTab, "enter": tea.KeyEnter, "esc": tea.KeyEsc, "up": tea.KeyUp, "down": tea.KeyDown,
		"left": tea.KeyLeft, "right": tea.KeyRight, "backspace": tea.KeyBackspace, "ctrl+c": tea.KeyCtrlC,
	}
	if t, ok := named[k]; ok {
		return tea.KeyMsg{Type: t}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func typeString(v any) string { return fmt.Sprintf("%T", v) }
