package cmd

import (
	"testing"
)

const (
	putRoute    = "PUT /accounts/42/timeentries/1"
	deleteRoute = "DELETE /accounts/42/timeentries/1"
	dayEntries  = `[{"id":1,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T10:00:00","note":"review","project":{"id":5,"name":"CLI"}},
		{"id":2,"startTime":"2026-11-24T11:00:00","endTime":"2026-11-24T12:00:00","note":"bugs"}]`
)

// confirmWith makes stdin a terminal that answers typed.
func confirmWith(t *testing.T, typed string) {
	t.Helper()
	withStdin(t, typed)
	old := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = old })
}

func TestEdit(t *testing.T) {
	timerAPI(t, map[string]string{
		entriesRoute: dayEntries,
		putRoute:     `[{"id":7,"startTime":"2026-11-24T08:30:00","endTime":"2026-11-24T10:00:00","note":"review","project":{"id":5,"name":"CLI"}}]`,
	})
	expect(t, []string{"entries", "edit", "1", "--date", "2026-11-24", "--start", "08:30"}, 0, nil,
		[]string{`Updated "review" · CLI 08:30–10:00 (new ID 7)`})
	// An empty description is a change too: it clears the description.
	timerAPI(t, map[string]string{
		entriesRoute: dayEntries,
		putRoute:     `[{"id":8,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T10:00:00"}]`,
	})
	expect(t, []string{"entries", "edit", "1", "--description", "", "--json"}, 0, []string{`"id": 8`}, nil)
}

func TestEditErrors(t *testing.T) {
	timerAPI(t, map[string]string{entriesRoute: dayEntries})
	expect(t, []string{"entries", "edit", "1"}, 2, nil, []string{"nothing to change"})
	expect(t, []string{"entries", "edit", "x", "--end", "10:30"}, 2, nil, []string{`"x" is not an entry ID`})
	expect(t, []string{"entries", "edit", "1", "--end", "10:30", "--date", "24/11"}, 2, nil, []string{`"24/11" is not a date`})
	expect(t, []string{"entries", "edit", "1", "--end", "11:30", "--date", "2026-11-24"}, 2, nil,
		[]string{"would overwrite these entries", "2  11:00–12:00  bugs"})
	expect(t, []string{"entries", "edit", "9", "--end", "11:30"}, 2, nil, []string{"no entry 9 on"})

	timerAPI(t, map[string]string{entriesRoute: "HTTP 500"})
	expect(t, []string{"entries", "edit", "1", "--end", "10:30"}, 1, nil, []string{"HTTP 500"})
	t.Setenv(tokenEnv, "")
	expect(t, []string{"entries", "edit", "1", "--end", "10:30"}, 1, nil, []string{"not signed in"})
}

func TestDelete(t *testing.T) {
	timerAPI(t, map[string]string{entriesRoute: dayEntries, deleteRoute: ""})
	expect(t, []string{"entries", "delete", "1", "--yes"}, 0, nil, []string{`Deleted "review" · CLI 09:00–10:00`})
	expect(t, []string{"entries", "delete", "1", "-y", "--json"}, 0, []string{`"note": "review"`}, nil)

	confirmWith(t, "y\n")
	expect(t, []string{"entries", "delete", "1", "--date", "2026-11-24"}, 0, nil,
		[]string{`Delete "review" · CLI 09:00–10:00? [y/N]`, "Deleted"})
}

func TestDeleteRefusals(t *testing.T) {
	timerAPI(t, map[string]string{entriesRoute: dayEntries})
	// No terminal to ask on.
	withStdin(t, "")
	expect(t, []string{"entries", "delete", "1"}, 2, nil, []string{"refusing to delete \"review\" · CLI 09:00–10:00 without confirmation"})
	for _, typed := range []string{"n\n", "", "maybe\n"} {
		confirmWith(t, typed)
		expect(t, []string{"entries", "delete", "1"}, 1, nil, []string{"aborted"})
	}
	expect(t, []string{"entries", "delete", "9", "--yes"}, 2, nil, []string{"no entry 9 on"})
	expect(t, []string{"entries", "delete", "x"}, 2, nil, []string{`"x" is not an entry ID`})
}

func TestDeleteErrors(t *testing.T) {
	timerAPI(t, map[string]string{entriesRoute: dayEntries, deleteRoute: "HTTP 500"})
	expect(t, []string{"entries", "delete", "1", "--yes"}, 1, nil, []string{"HTTP 500"})
	timerAPI(t, map[string]string{entriesRoute: "HTTP 500"})
	expect(t, []string{"entries", "delete", "1", "--yes"}, 1, nil, []string{"HTTP 500"})
	t.Setenv(tokenEnv, "")
	expect(t, []string{"entries", "delete", "1", "--yes"}, 1, nil, []string{"not signed in"})
}
