package tui

import (
	"strings"
	"testing"
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

func TestNoSuggestionsWhenEditing(t *testing.T) {
	r := recentRoutes()
	r[latestRoute] = runningEntry
	m := press(loaded(t, r), "enter")
	lacks(t, m.View(), "write docs")
}

func TestRecentLoadError(t *testing.T) {
	r := recentRoutes()
	r[recentRoute] = "HTTP 500"
	if m := press(loaded(t, r), "n"); !m.statusErr {
		t.Errorf("footer = %q", m.footer())
	}
}
