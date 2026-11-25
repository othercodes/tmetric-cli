package tui

import (
	"strings"
	"testing"
)

const (
	put3     = "PUT /accounts/42/timeentries/3"
	delete3  = "DELETE /accounts/42/timeentries/3"
	tasksCLI = "GET /accounts/42/tasks?ProjectList=5"
)

func editRoutes() map[string]string {
	r := routes()
	r[tasksCLI] = `[{"id":9,"name":"Tests"},{"id":10,"name":"Docs"}]`
	return r
}

func TestEditFormOpensFilled(t *testing.T) {
	m := loaded(t, editRoutes())
	has(t, m.footer(), "Edit: enter", "Continue: s", "Delete: d")
	m = press(m, "enter")
	has(t, m.View(), "┌─ Edit entry", "review", "▸ CLI", "Tests", "Start", "09:00", "End", "10:00", "Save: enter")
	m = press(m, "esc")
	if m.overlay != noOverlay {
		t.Error("esc should close the form")
	}
	if press(m, "e").overlay != formOverlay {
		t.Error("e should open the form too")
	}
}

func TestEditSaves(t *testing.T) {
	r := editRoutes()
	r[put3] = `[{"id":30,"startTime":"2026-11-24T08:30:00","endTime":"2026-11-24T10:00:00","note":"code review","project":{"id":5,"name":"CLI"}}]`
	m := press(loaded(t, r), "enter")
	m = press(m, " ", "x", "backspace", "backspace") // the description stays "review"
	m = press(m, "tab", "tab", "tab")                // project, task (loads them), start
	m = press(m, "backspace", "backspace", "backspace", "backspace", "backspace", "0", "8", ":", "3", "0", "enter")
	if !strings.Contains(m.footer(), `Updated "code review" · CLI`) {
		t.Errorf("footer = %q", m.footer())
	}
	// Only the start changed: the task, tags and the rest go back as they were.
	want := `{"startTime":"2026-11-24T08:30:00","endTime":"2026-11-24T10:00:00","note":"review","project":{"id":5,"name":"CLI"},"task":{"id":9,"name":"Tests"}}`
	if got := bodies[put3]; got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestEditTaskAndProject(t *testing.T) {
	r := editRoutes()
	r[put3] = `[{"id":30,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T10:00:00"}]`
	m := press(loaded(t, r), "enter", "tab", "tab")
	has(t, m.View(), "▸ Tests") // the entry's own task starts highlighted
	m = press(m, "down", "x", "enter")
	if got := bodies[put3]; !strings.Contains(got, `"task":{"id":10,"name":"Docs"}`) {
		t.Errorf("body = %s", got)
	}
	m = press(m, "enter", "tab", "tab", "up", "up", "enter") // back to no task
	if got := bodies[put3]; strings.Contains(got, `"task"`) {
		t.Errorf("body = %s", got)
	}
	r["GET /accounts/42/tasks?ProjectList=6"] = `[]`
	press(m, "enter", "tab", "backspace", "backspace", "backspace", "d", "o", "c", "enter")
	if got := bodies[put3]; !strings.Contains(got, `"project":{"id":6,"name":"Docs"}`) {
		t.Errorf("body = %s", got)
	}
}

func TestEditRunningTimer(t *testing.T) {
	r := editRoutes()
	r["PUT /accounts/42/timeentries/1"] = `[{"id":31,"startTime":"2026-11-24T13:00:00","endTime":"2026-11-24T14:00:00","note":"standup"}]`
	m := press(loaded(t, r), "down", "enter")
	has(t, m.View(), "standup", "13:00", "HH:MM, empty while it runs")
	m = press(m, "tab", "tab", "tab", "tab") // description → project → task → start → end
	m = press(m, "1", "4", ":", "0", "0", "enter")
	if got := bodies["PUT /accounts/42/timeentries/1"]; !strings.Contains(got, `"endTime":"2026-11-24T14:00:00"`) {
		t.Errorf("body = %s", got)
	}
	m = press(m, "down", "enter", "tab", "tab", "tab", "tab", "tab")
	if m.form.field != descField {
		t.Errorf("tab should wrap through the five fields, field = %d", m.form.field)
	}
}

func TestEditOverlapAsksToOverwrite(t *testing.T) {
	r := editRoutes()
	r[put3] = `[{"id":30,"startTime":"2026-11-24T09:00:00","endTime":"2026-11-24T13:30:00"}]`
	m := press(loaded(t, r), "enter", "tab", "tab", "tab", "tab")
	m = press(m, "backspace", "backspace", "backspace", "backspace", "backspace", "1", "3", ":", "3", "0", "enter")
	if !strings.HasPrefix(m.footer(), `This overwrites "standup" · CLI; TMetric cuts or removes them. Save anyway? y/N`) {
		t.Fatalf("footer = %q", m.footer())
	}
	if bodies[put3] != "" {
		t.Fatal("nothing should be saved before the answer")
	}
	m = press(m, "y")
	if bodies[put3] == "" || !strings.Contains(m.footer(), "Updated") {
		t.Errorf("y should save; footer = %q", m.footer())
	}
}

func TestEditErrorShows(t *testing.T) {
	r := editRoutes()
	r[put3] = "HTTP 500"
	m := press(loaded(t, r), "enter", "x", "enter")
	if !m.statusErr || m.confirm != nil {
		t.Errorf("a failed save should show, not ask; footer = %q", m.footer())
	}
}

func TestTaskListIgnoresTyping(t *testing.T) {
	m := press(loaded(t, editRoutes()), "enter", "tab", "tab", "z")
	if m.form.field != taskField || strings.Contains(m.form.description.Value(), "z") {
		t.Error("typing on the task list should do nothing")
	}
}

func TestDeleteEntry(t *testing.T) {
	r := editRoutes()
	r[delete3] = ""
	m := press(loaded(t, r), "d")
	if got := m.footer(); got != `Delete "review" · CLI / Tests 09:00–10:00? y/N` {
		t.Fatalf("footer = %q", got)
	}
	m = press(m, "y")
	if _, sent := bodies[delete3]; !sent || !strings.Contains(m.footer(), `Deleted "review"`) {
		t.Errorf("footer = %q", m.footer())
	}
	r[delete3] = "HTTP 500"
	if m := press(loaded(t, r), "d", "y"); !m.statusErr {
		t.Errorf("a failed delete should show; footer = %q", m.footer())
	}
	// The running timer reads "now" as its end.
	if m := press(loaded(t, r), "down", "d"); !strings.Contains(m.footer(), "13:00–now?") {
		t.Errorf("footer = %q", m.footer())
	}
}

func TestEditAndDeleteNeedAnEntry(t *testing.T) {
	r := editRoutes()
	r[todayRoute] = "[]"
	m := press(loaded(t, r), "enter", "e", "d")
	if m.overlay != noOverlay || m.confirm != nil {
		t.Error("with no entries, enter, e and d should do nothing")
	}
	m = press(loaded(t, editRoutes()), "]", "e", "d")
	if m.overlay != noOverlay || m.confirm != nil {
		t.Error("e and d only work on the Day tab")
	}
}

func TestEditEntryOfAnUnlistedProject(t *testing.T) {
	r := editRoutes()
	r[projectsRoute] = `[{"id":6,"name":"Docs"}]` // CLI is no longer offered
	m := press(loaded(t, r), "enter")
	if m.form.cursor != 0 || m.form.chosen() != nil {
		t.Errorf("an unlisted project should match nothing; cursor %d", m.form.cursor)
	}
	has(t, m.View(), "no project matches")
}
