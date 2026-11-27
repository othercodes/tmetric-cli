package tui

import (
	"strings"
	"testing"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

const recentJSON = `[
	{"note":"TMET-1 fix login","project":{"id":5,"name":"CLI"},"task":{"name":"TMET-1","externalLink":{"link":"https://jira"}},"isPinned":true},
	{"note":"write docs","project":{"id":6,"name":"Docs"}},
	{"note":"planning"}]`

func recentRoutes() map[string]string {
	r := routes()
	r[recentRoute] = recentJSON
	r[latestRoute] = "null"
	r[createRoute] = `[{"id":8,"startTime":"2026-11-24T14:30:00","endTime":null}]`
	return r
}

func TestSuggestionsShowAndFilter(t *testing.T) {
	m := press(loaded(t, recentRoutes()), "n")
	has(t, m.View(), `"TMET-1 fix login" · CLI / TMET-1`, `"write docs" · Docs`, `"planning"`)
	lacks(t, m.View(), "▸ \"")
	m = press(m, "d", "o", "c")
	has(t, m.View(), `"write docs" · Docs`)
	lacks(t, m.View(), "planning", "TMET-1")
	m = press(m, "tab")
	lacks(t, m.View(), `"write docs" · Docs`) // only on the description field
}

func TestUseSuggestionKeepsExternalTask(t *testing.T) {
	m := press(loaded(t, recentRoutes()), "n", "down")
	has(t, m.View(), `▸ "TMET-1 fix login"`)
	if !strings.HasPrefix(m.footer(), "Use: enter") {
		t.Errorf("footer = %q", m.footer())
	}
	m = press(m, "enter")
	if m.overlay != formOverlay {
		t.Fatal("using a suggestion must not start anything")
	}
	if m.form.description.Value() != "TMET-1 fix login" || m.form.chosen() == nil || m.form.chosen().ID != 5 {
		t.Errorf("form = %q / %+v", m.form.description.Value(), m.form.chosen())
	}
	has(t, m.View(), "▸ CLI", "TMET-1") // the task shows as the hint
	m = press(m, "enter")
	want := `{"startTime":"2026-11-24T14:30:45","endTime":null,"note":"TMET-1 fix login","project":{"id":5,"name":"CLI"},"task":{"name":"TMET-1","externalLink":{"link":"https://jira"}}}`
	if got := bodies[createRoute]; got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestSuggestionWithoutProject(t *testing.T) {
	press(loaded(t, recentRoutes()), "n", "p", "l", "a", "n", "down", "enter", "enter")
	if got := bodies[createRoute]; got != `{"startTime":"2026-11-24T14:30:45","endTime":null,"note":"planning"}` {
		t.Errorf("body = %s", got)
	}
}

func TestChangedSuggestionStartsAsTyped(t *testing.T) {
	// Another project after using the suggestion: the external task goes.
	press(loaded(t, recentRoutes()), "n", "down", "enter", "tab", "backspace", "backspace", "backspace", "d", "o", "c", "enter")
	if got := bodies[createRoute]; strings.Contains(got, "TMET-1\",\"externalLink") || !strings.Contains(got, `"project":{"id":6`) {
		t.Errorf("body = %s", got)
	}
	// A project added to a suggestion that had none.
	press(loaded(t, recentRoutes()), "n", "p", "l", "a", "n", "down", "enter", "tab", "c", "l", "i", "enter")
	if got := bodies[createRoute]; !strings.Contains(got, `"project":{"id":5`) {
		t.Errorf("body = %s", got)
	}
}

func TestSuggestionCursor(t *testing.T) {
	m := press(loaded(t, recentRoutes()), "n", "down", "down", "down", "down")
	if m.form.suggest != 2 {
		t.Errorf("suggest = %d, want the last of 3", m.form.suggest)
	}
	m = press(m, "up", "up", "up", "up")
	if m.form.suggest != -1 {
		t.Errorf("suggest = %d, want -1 (typing)", m.form.suggest)
	}
	m = press(m, "down", "x")
	if m.form.suggest != -1 {
		t.Error("typing should leave the suggestions")
	}
}

func TestProjectSuggestions(t *testing.T) {
	m := press(loaded(t, recentRoutes()), "n", "tab")
	has(t, m.View(), "CLI", "Docs")
	lacks(t, m.View(), "Design", "▸") // only recent projects, none highlighted
	m = press(m, "down", "down", "down")
	has(t, m.View(), "▸ Docs")
	if !strings.HasPrefix(m.footer(), "Use: enter") {
		t.Errorf("footer = %q", m.footer())
	}
	m = press(m, "up", "enter")
	if m.overlay != formOverlay || m.form.chosen() == nil || m.form.chosen().ID != 5 {
		t.Fatalf("form = %+v", m.form.chosen())
	}
	m = press(m, "enter")
	if got := bodies[createRoute]; !strings.Contains(got, `"project":{"id":5,"name":"CLI"}`) {
		t.Errorf("body = %s", got)
	}
}

func TestRecentProjectsComeFirst(t *testing.T) {
	r := recentRoutes()
	// Design is recent; project 99 is recent but can no longer be tracked.
	r[recentRoute] = `[{"note":"gone","project":{"id":99,"name":"Old"}},{"note":"logo","project":{"id":7,"name":"Design"}},{"note":"logo v2","project":{"id":7,"name":"Design"}}]`
	m := press(loaded(t, r), "n", "tab")
	lacks(t, m.View(), "Old")
	m = press(m, "d")
	has(t, m.View(), "▸ Design", "Docs")
}

func TestRecentKeepsTheHighlightedProject(t *testing.T) {
	m := press(loaded(t, routes()), "n", "tab", "d", "down") // Docs, ▸ Design
	m = m.apply(recentLoaded{recent: []tmetric.RecentEntry{{Project: &tmetric.Project{ID: 7, Name: "Design"}}}})
	if chosen := m.form.chosen(); chosen == nil || chosen.ID != 7 {
		t.Errorf("chosen = %+v, want Design", chosen)
	}
}

func TestNextFieldLeavesSuggestions(t *testing.T) {
	m := press(loaded(t, recentRoutes()), "n", "down", "tab")
	if m.form.suggest != -1 {
		t.Errorf("suggest = %d, want -1", m.form.suggest)
	}
}

const editRecentJSON = `[
	{"note":"pair on tests","project":{"id":5,"name":"CLI"},"task":{"id":10,"name":"Docs"}},
	{"note":"TMET-1 fix login","project":{"id":5,"name":"CLI"},"task":{"name":"TMET-1","externalLink":{"link":"https://jira"}}},
	{"note":"DOC-2 spec","project":{"id":6,"name":"Docs"},"task":{"name":"DOC-2"}},
	{"note":"write docs","project":{"id":6,"name":"Docs"}},
	{"note":"planning"}]`

// editWithSuggestion edits the entry "review" (CLI / Tests) with the first
// suggestion for query, and returns the form before it saves.
func editWithSuggestion(t *testing.T, query string) model {
	t.Helper()
	r := editRoutes()
	r[recentRoute] = editRecentJSON
	r[put3] = `[{"id":30,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T10:00:00"}]`
	m := press(loaded(t, r), "enter")
	lacks(t, m.View(), "pair on tests") // nothing until the description changes
	m = press(m, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	for _, k := range query {
		m = press(m, string(k))
	}
	return press(m, "down", "enter")
}

func TestEditWithSuggestion(t *testing.T) {
	cases := []struct {
		query, hint string
		want, not   []string
	}{
		{"pair", "Docs", []string{`"note":"pair on tests"`, `"task":{"id":10,"name":"Docs"}`}, nil},
		{"TMET", "Tests (TMET-1 is external, not set)", []string{`"task":{"id":9,"name":"Tests"}`}, []string{"TMET-1\""}},
		{"DOC", "no task (DOC-2 is external, not set)", []string{`"project":{"id":6,"name":"Docs"}`}, []string{`"task"`}},
		{"write", "optional", []string{`"project":{"id":6,"name":"Docs"}`}, []string{`"task"`}},
		// No project in the suggestion: the entry keeps its project and task.
		{"plan", "Tests", []string{`"note":"planning"`, `"project":{"id":5,"name":"CLI"}`, `"task":{"id":9,"name":"Tests"}`}, nil},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			m := editWithSuggestion(t, c.query)
			has(t, m.View(), "Task         "+c.hint)
			press(m, "enter")
			has(t, bodies[put3], c.want...)
			lacks(t, bodies[put3], c.not...)
		})
	}
}

func TestRecentLoadError(t *testing.T) {
	r := recentRoutes()
	r[recentRoute] = "HTTP 500"
	if m := press(loaded(t, r), "n"); !m.statusErr {
		t.Errorf("footer = %q", m.footer())
	}
}
