package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	latestRoute  = "GET /accounts/42/timeentries/latest"
	createRoute  = "POST /accounts/42/timeentries"
	updateRoute  = "PUT /accounts/42/timeentries/1"
	runningEntry = `{"id":1,"startTime":"2026-11-24T13:00:00","endTime":null,"note":"standup",
		"project":{"id":5,"name":"CLI"},"task":{"id":9,"name":"Tests"}}`
	bareEntry = `{"id":1,"startTime":"2026-11-24T13:00:00","endTime":null}`
)

// timerAPI serves the user plus routes ("METHOD path" → body, or "HTTP 500"),
// and signs the CLI in with an environment token.
func timerAPI(t *testing.T, routes map[string]string) {
	t.Helper()
	isolate(t)
	routes["GET /user"] = userJSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
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
	t.Setenv("TMETRIC_BASE_URL", server.URL)
	t.Setenv(tokenEnv, "good")
}

func expect(t *testing.T, args []string, code int, stdout, stderr []string) {
	t.Helper()
	out, errOut, got := runCLI(t, args...)
	if got != code {
		t.Errorf("%v: exit code = %d, want %d (stderr %q)", args, got, code, errOut)
	}
	for _, want := range stdout {
		if !strings.Contains(out, want) {
			t.Errorf("%v: stdout = %q, want %q", args, out, want)
		}
	}
	for _, want := range stderr {
		if !strings.Contains(errOut, want) {
			t.Errorf("%v: stderr = %q, want %q", args, errOut, want)
		}
	}
}

func TestTimerStatus(t *testing.T) {
	timerAPI(t, map[string]string{latestRoute: runningEntry})
	expect(t, []string{"timer", "status"}, 0,
		[]string{"standup", "CLI", "Tests", "13:00", "elapsed"}, nil)
	expect(t, []string{"timer", "status", "--json"}, 0,
		[]string{`"running": true`, `"note": "standup"`, `"elapsed_seconds"`}, nil)

	timerAPI(t, map[string]string{latestRoute: bareEntry})
	expect(t, []string{"timer", "status"}, 0, []string{"(no description)", "project      -", "task         -"}, nil)

	timerAPI(t, map[string]string{latestRoute: "null"})
	expect(t, []string{"timer", "status"}, 0, []string{"No timer running."}, nil)
	expect(t, []string{"timer", "status", "--json"}, 0, []string{`"entry": null`, `"running": false`}, nil)
}

func TestTimerStart(t *testing.T) {
	timerAPI(t, map[string]string{
		latestRoute: runningEntry,
		createRoute: `[{"id":2,"startTime":"2026-11-24T14:30:00","endTime":null,"note":"review"}]`,
	})
	expect(t, []string{"timer", "start", "review", "PR", "--yes"}, 0, nil,
		[]string{`Stopped "standup" · CLI / Tests (1h30m)`, `Started "review" at 14:30`})

	timerAPI(t, map[string]string{
		latestRoute: "null",
		createRoute: `[{"id":2,"startTime":"2026-11-24T14:30:00","endTime":null}]`,
	})
	expect(t, []string{"timer", "start", "--json"}, 0, []string{`"running": true`, `"id": 2`}, nil)
	expect(t, []string{"timer", "start", "--task", "x"}, 2, nil, []string{"--task needs --project"})
}

func TestTimerStop(t *testing.T) {
	timerAPI(t, map[string]string{
		latestRoute: runningEntry,
		updateRoute: `[{"id":1,"startTime":"2026-11-24T13:00:00","endTime":"2026-11-24T14:00:00","note":"standup"}]`,
	})
	expect(t, []string{"timer", "stop", "--yes"}, 0, nil, []string{`Stopped "standup" (1h00m)`})
	expect(t, []string{"timer", "stop", "-y", "--json"}, 0, []string{`"running": false`, `"dropped": false`, `"elapsed_seconds": 3600`}, nil)

	timerAPI(t, map[string]string{latestRoute: runningEntry, updateRoute: `[]`})
	expect(t, []string{"timer", "stop", "--yes"}, 0, nil, []string{"entries shorter than a minute are dropped"})

	timerAPI(t, map[string]string{latestRoute: "null"})
	expect(t, []string{"timer", "stop"}, 0, nil, []string{"No timer running."})
	expect(t, []string{"timer", "stop", "--json"}, 0, []string{`"entry": null`}, nil)
}

func TestTimerErrors(t *testing.T) {
	timerAPI(t, map[string]string{latestRoute: "HTTP 500"})
	for _, sub := range []string{"status", "start", "stop"} {
		expect(t, []string{"timer", sub}, 1, nil, []string{"HTTP 500"})
	}
	timerAPI(t, map[string]string{})
	t.Setenv(tokenEnv, "")
	for _, sub := range []string{"status", "start", "stop"} {
		expect(t, []string{"timer", sub}, 1, nil, []string{"not signed in"})
	}
}

func TestStartCompletesRecentDescriptions(t *testing.T) {
	recent := "GET /accounts/42/timeentries/recent"
	timerAPI(t, map[string]string{recent: `[{"note":"review","project":{"id":5,"name":"CLI"}},
		{"note":"review","project":{"id":6,"name":"Docs"}},{"note":""},{"note":"bugs"}]`})
	// Unique, non-empty descriptions with their project as a hint; no files.
	expect(t, []string{"__complete", "timer", "start", ""}, 0, []string{"review\tCLI\nbugs\t-\n:4\n"}, nil)
	expect(t, []string{"__complete", "timer", "start", "review", ""}, 0, []string{":4\n"}, nil)

	timerAPI(t, map[string]string{recent: "HTTP 500"})
	expect(t, []string{"__complete", "timer", "start", ""}, 0, []string{":4\n"}, nil)
	t.Setenv(tokenEnv, "")
	expect(t, []string{"__complete", "timer", "start", ""}, 0, []string{":4\n"}, nil)
}

func TestTimerAsksBeforeStopping(t *testing.T) {
	timerAPI(t, map[string]string{
		latestRoute: runningEntry,
		createRoute: `[{"id":2,"startTime":"2026-11-24T14:30:00","endTime":null,"note":"review"}]`,
		updateRoute: `[{"id":1,"startTime":"2026-11-24T13:00:00","endTime":"2026-11-24T14:00:00","note":"standup"}]`,
	})
	// No terminal to ask on: refused, nothing stopped.
	withStdin(t, "")
	expect(t, []string{"timer", "start", "review"}, 2, nil,
		[]string{`refusing to stop "standup" · CLI / Tests (`, `and start "review" without confirmation; pass --yes`})
	expect(t, []string{"timer", "stop"}, 2, nil, []string{`refusing to stop "standup" · CLI / Tests (`})

	confirmWith(t, "n\n")
	expect(t, []string{"timer", "stop"}, 1, nil, []string{"? [y/N] ", "aborted"})
	confirmWith(t, "y\n")
	expect(t, []string{"timer", "start", "review"}, 0, nil, []string{`Stop "standup" · CLI / Tests (`, `Started "review"`})
	confirmWith(t, "y\n")
	expect(t, []string{"timer", "stop"}, 0, nil, []string{`Stopped "standup"`})
}

func TestTimerDoesNotAskWhenNothingRuns(t *testing.T) {
	timerAPI(t, map[string]string{
		latestRoute: "null",
		createRoute: `[{"id":2,"startTime":"2026-11-24T14:30:00","endTime":null,"note":"review"}]`,
	})
	withStdin(t, "")
	expect(t, []string{"timer", "start", "review"}, 0, nil, []string{`Started "review"`})
	expect(t, []string{"timer", "stop"}, 0, nil, []string{"No timer running."})
}

func TestTimerStopUpdateFails(t *testing.T) {
	timerAPI(t, map[string]string{latestRoute: runningEntry, updateRoute: "HTTP 500"})
	expect(t, []string{"timer", "stop", "--yes"}, 1, nil, []string{"HTTP 500"})
}
