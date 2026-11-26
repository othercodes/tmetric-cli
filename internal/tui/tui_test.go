package tui

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMain(m *testing.M) {
	tickInterval = time.Millisecond
	os.Exit(m.Run())
}

func has(t *testing.T, view string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(view, w) {
			t.Errorf("view lacks %q:\n%s", w, view)
		}
	}
}

func lacks(t *testing.T, view string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(view, w) {
			t.Errorf("view has %q:\n%s", w, view)
		}
	}
}

func TestDayView(t *testing.T) {
	m := loaded(t, routes())
	has(t, m.View(),
		"┌─ Timer", "● 1h30m", `"standup" · CLI`, "since 13:00",
		"┌─ tmetric", "│ Time entries", "│ Projects",
		"Day │ Week", "Tue 24 Nov 2026 · today",
		"START", "09:00", "10:00", "1h00m", "review", "running",
		"2 entries · 2h30m",
		"Move: ↑/↓", "Continue: s", "Keybindings: ?")
	if !strings.HasSuffix(m.View(), "Keybindings: ?") {
		t.Error("the footer should end with the keybindings help")
	}
}

func TestNoTimerAndEmptyData(t *testing.T) {
	r := routes()
	r[latestRoute], r[todayRoute], r[weekRoute] = "null", "null", "null"
	m := loaded(t, r)
	has(t, m.View(), "No timer running", "0 entries · 0h00m")
	m = press(m, "enter", "s") // nothing to open or continue
	has(t, press(m, "]").View(), "total 0h00m")
	if m.windowTitle() != "tmetric" {
		t.Errorf("title = %q", m.windowTitle())
	}
}

func TestNarrowFooterKeepsHelp(t *testing.T) {
	m := loaded(t, routes())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	footer := next.(model).footer()
	if !strings.HasSuffix(footer, "Keybindings: ?") || strings.Contains(footer, "Stop") {
		t.Errorf("footer = %q", footer)
	}
}

func TestTick(t *testing.T) {
	m := loaded(t, routes())
	m.session.Now = func() time.Time { return time.Date(2026, 11, 24, 4, 40, 45, 0, time.UTC) }
	next, cmd := m.Update(tick{})
	m = next.(model)
	has(t, m.View(), "● 1h40m", "1h40m", "2 entries · 2h40m")
	if m.title != "● 1h40m · standup" {
		t.Errorf("title = %q", m.title)
	}
	if batch, ok := cmd().(tea.BatchMsg); !ok || len(batch) != 2 {
		t.Errorf("a changed title should be set along with the next tick, got %#v", cmd())
	}
	// Same second, same title: only the next tick.
	if _, cmd := m.Update(tick{}); cmd() != (tick{}) {
		t.Error("an unchanged title should not be set again")
	}
}

func TestStatusExpires(t *testing.T) {
	m := loaded(t, routes())
	m = m.notify("Started")
	if m.footer() != "Started" {
		t.Errorf("footer = %q", m.footer())
	}
	m.session.Now = func() time.Time { return time.Date(2026, 11, 24, 4, 31, 0, 0, time.UTC) }
	next, _ := m.Update(tick{})
	if f := next.(model).footer(); !strings.Contains(f, "Keybindings") {
		t.Errorf("the keys should come back, footer = %q", f)
	}
}

func TestLoadErrors(t *testing.T) {
	for _, broken := range []string{latestRoute, todayRoute, weekRoute, projectsRoute} {
		r := routes()
		r[broken] = "HTTP 500"
		m := loaded(t, r)
		if !m.statusErr || !strings.Contains(m.footer(), "error: the TMetric API returned HTTP 500") {
			t.Errorf("%s: footer = %q", broken, m.footer())
		}
	}
}

func TestEnterAfterAnEmptyDay(t *testing.T) {
	r := routes()
	r["GET /accounts/42/timeentries?endDate=2026-11-25&startDate=2026-11-25"] = "[]"
	r[createRoute] = `[{"id":8,"startTime":"2026-11-24T14:30:00","endTime":null}]`
	// An empty day, then back to a day with entries: enter must not read row -1.
	m := press(loaded(t, r), "right", "left", "s", "y")
	if m.dayTable.Cursor() != 0 || !strings.Contains(m.footer(), "Started") {
		t.Errorf("cursor = %d, footer = %q", m.dayTable.Cursor(), m.footer())
	}
}

func TestStaleAnswersAreDropped(t *testing.T) {
	m := loaded(t, routes())
	next, _ := m.Update(dayLoaded{day: m.day.AddDate(0, 0, -1), report: nil})
	next, _ = next.(model).Update(weekLoaded{start: m.week.AddDate(0, 0, -7), report: nil})
	has(t, next.(model).View(), "2 entries · 2h30m")
	m = press(next.(model), "]")
	has(t, m.View(), "total 4h30m")
}

func TestGlobalKeys(t *testing.T) {
	m := loaded(t, routes())
	if _, cmd := m.Update(keyMsg("q")); cmd == nil {
		t.Error("q should quit")
	}
	lacks(t, m.footer(), "1/2", "Views")
	m = press(m, "?")
	has(t, m.View(), "┌─ Keybindings", "Global", "This view", "Continue", "Views", "1        Time entries", "2        Projects")
	for _, closer := range []string{"esc", "?", "q"} {
		if press(press(m, "x"), closer).overlay != noOverlay && closer != "x" {
			t.Errorf("%s should close the help", closer)
		}
	}
	m = press(m, "esc", "a")
	has(t, m.View(), "┌─ Account", "ada@example.com", "Australia/Brisbane", "┌─ Workspace", "Week starts on", "Monday")
	lacks(t, m.footer(), "Focus")
	m = press(m, "2", "tab", "esc")
	if m.screen != mainScreen || m.view != entriesView || m.focus != panelFocus {
		t.Errorf("account should ignore view and focus keys; screen %d view %d focus %d", m.screen, m.view, m.focus)
	}
	r := routes()
	m = loaded(t, r)
	r[latestRoute] = "HTTP 500"
	m = press(m, "r")
	if !m.statusErr {
		t.Error("r should reload, and the broken route should show")
	}
}

func TestAccountWithoutWorkspace(t *testing.T) {
	m := loaded(t, routes())
	m.session.User.Accounts = nil
	m = press(m, "a")
	has(t, m.View(), "ID              42")
}

func TestGroupsAndFocus(t *testing.T) {
	m := loaded(t, routes())
	m = press(m, "2")
	has(t, m.View(), "┌─ Projects", "projects you can track time on", "Acme", "3 projects", "Filter: /")
	m = press(m, "1", "tab")
	if m.focus != sidebarFocus || !strings.HasPrefix(m.footer(), "Move: ↑/↓ | Focus: tab") {
		t.Errorf("focus = %d, footer = %q", m.focus, m.footer())
	}
	m = press(m, "down")
	if m.view != projectsView {
		t.Error("down in the sidebar should pick the next view")
	}
	m = press(m, "up", "up")
	if m.view != reportsView {
		t.Error("up should wrap around to the last view")
	}
	m = press(m, "t", "tab")
	if m.focus != panelFocus {
		t.Error("other keys should do nothing in the sidebar, and tab should come back")
	}
}

func TestDayNavigation(t *testing.T) {
	r := routes()
	r[mondayRoute] = `[` + mondayEntry + `]`
	m := loaded(t, r)
	m = press(m, "left")
	has(t, m.View(), "Mon 23 Nov 2026", "11:00", "bugs", "1 entries · 2h00m")
	lacks(t, m.View(), "· today")
	m = press(m, "t")
	has(t, m.View(), "Tue 24 Nov 2026 · today")
	r["GET /accounts/42/timeentries?endDate=2026-11-25&startDate=2026-11-25"] = "[]"
	m = press(m, "right", "t", "down")
	if m.dayTable.Cursor() != 1 {
		t.Errorf("cursor = %d", m.dayTable.Cursor())
	}
}

func TestViewWhileLoading(t *testing.T) {
	m := loaded(t, routes())
	// The key moves to the next day and week, but their answers are not in yet.
	next, _ := m.Update(keyMsg("right"))
	has(t, next.(model).View(), "Wed 25 Nov 2026", "0 entries · 0h00m")
	next, _ = press(m, "]").Update(keyMsg("right"))
	has(t, next.(model).View(), "week of Mon 30 Nov 2026", "total 0h00m")
}

func TestWeekNavigation(t *testing.T) {
	r := routes()
	r[lastWeekRoute] = "[]"
	r[mondayRoute] = `[` + mondayEntry + `]`
	m := loaded(t, r)
	m = press(m, "]")
	has(t, m.View(), "week of Mon 23 Nov 2026", "Mon 23 Nov", "2h00m", "Tue 24 Nov", "2h30m", "Wed 25 Nov", "total 4h30m", "Open day: enter")
	m = press(m, "left")
	has(t, m.View(), "week of Mon 16 Nov 2026", "total 0h00m")
	m = press(m, "t", "down", "up", "enter")
	if m.tab != dayTab {
		t.Fatal("enter on a week row should open the Day tab")
	}
	has(t, m.View(), "Mon 23 Nov 2026", "bugs")
}

func TestWeekIgnoresEntriesOutsideIt(t *testing.T) {
	r := routes()
	r[weekRoute] = `[{"id":5,"startTime":"2026-11-30T09:00:00","endTime":"2026-11-30T10:00:00"}]`
	m := press(loaded(t, r), "]")
	has(t, m.View(), "total 1h00m")
	if strings.Contains(m.View(), "1h00m  ") && strings.Count(m.View(), "1h00m") > 1 {
		t.Error("an entry after the week should not land on a day")
	}
}

func TestContinue(t *testing.T) {
	r := routes()
	r[createRoute] = `[{"id":8,"startTime":"2026-11-24T14:30:00","endTime":null,"note":"review","project":{"id":5,"name":"CLI"}}]`
	m := press(loaded(t, r), "enter", "esc")
	if bodies[createRoute] != "" {
		t.Fatal("opening an entry and closing it must not change data")
	}
	m = press(m, "s") // the 09:00 "review" entry, while "standup" runs
	if got := m.footer(); got != `Stop "standup" · CLI (1h30m) and start "review" · CLI / Tests? y/N` {
		t.Fatalf("footer = %q", got)
	}
	m = press(m, "y")
	if !strings.Contains(m.footer(), `Started "review" · CLI, stopped "standup" · CLI`) {
		t.Errorf("footer = %q", m.footer())
	}
	want := `{"startTime":"2026-11-24T14:30:45","endTime":null,"note":"review","project":{"id":5,"name":"CLI"},"task":{"id":9,"name":"Tests"}}`
	if got := bodies[createRoute]; got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestStop(t *testing.T) {
	tests := []struct{ name, latest, update, want string }{
		{"kept", runningEntry, `[{"id":9,"startTime":"2026-11-24T13:00:00","endTime":"2026-11-24T14:30:00","note":"standup"}]`, `Stopped "standup" (1h30m)`},
		{"dropped", runningEntry, `[]`, "TMetric dropped it"},
		{"none", "null", "", "No timer running."},
		{"error", runningEntry, "HTTP 500", "error: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := routes()
			r[latestRoute] = tt.latest
			if tt.update != "" {
				r[updateRoute] = tt.update
			}
			keys := []string{"x", "y"}
			if tt.latest == "null" {
				keys = keys[:1] // nothing runs: nothing to confirm
			}
			if f := press(loaded(t, r), keys...).footer(); !strings.Contains(f, tt.want) {
				t.Errorf("footer = %q, want %q", f, tt.want)
			}
		})
	}
}

func TestProjectsFilter(t *testing.T) {
	m := press(loaded(t, routes()), "2", "/", "d", "e")
	has(t, m.View(), "/de", "Design", "1 projects")
	lacks(t, m.View(), "Docs")
	if !strings.HasPrefix(m.footer(), "Start: enter | Back: esc") {
		t.Errorf("footer = %q", m.footer())
	}
	m = press(m, "enter")
	if m.filtering || !strings.Contains(m.View(), "/de") {
		t.Error("enter should keep the filter and leave typing")
	}
	m = press(m, "/", "esc")
	has(t, m.View(), "3 projects", "projects you can track time on")
	m = press(m, "/", "z", "z", "enter", "enter")
	if m.overlay != noOverlay {
		t.Error("enter with no project shown should do nothing")
	}
	m = press(m, "/", "esc", "down")
	if m.projectTable.Cursor() != 1 {
		t.Errorf("cursor = %d", m.projectTable.Cursor())
	}
}

func TestStartFromProject(t *testing.T) {
	r := routes()
	r[createRoute] = `[{"id":8,"startTime":"2026-11-24T14:30:00","endTime":null,"note":"docs","project":{"id":6,"name":"Docs"}}]`
	m := press(loaded(t, r), "2", "down", "enter")
	has(t, m.View(), "┌─ New timer", "Docs")
	m = press(m, "d", "o", "c", "s", "enter", "y")
	if m.overlay != noOverlay || !strings.Contains(m.footer(), `Started "docs" · Docs`) {
		t.Errorf("footer = %q", m.footer())
	}
	if !strings.Contains(bodies[createRoute], `"note":"docs","project":{"id":6,"name":"Docs"}`) {
		t.Errorf("body = %s", bodies[createRoute])
	}
}

func TestNewTimerForm(t *testing.T) {
	r := routes()
	r[tasksRoute] = `[{"id":11,"name":"Write docs"},{"id":12,"name":"Review"},{"id":13,"name":"Old","isCompleted":true}]`
	r[createRoute] = `[{"id":8,"startTime":"2026-11-24T14:30:00","endTime":null,"note":"fix","project":{"id":6,"name":"Docs"},"task":{"id":12,"name":"Review"}}]`
	m := press(loaded(t, r), "n")
	has(t, m.View(), "┌─ New timer", "Description", "choose a project first", "Start: enter | Next field: tab")
	m = press(m, "f", "i", "x", "tab", "d")
	has(t, m.View(), "▸ Docs", "Design", "optional")
	m = press(m, "down", "down") // two matches: the cursor stops on the last one
	if m.form.cursor != 1 {
		t.Errorf("project cursor = %d", m.form.cursor)
	}
	m = press(m, "o") // "do": only Docs; typing resets the cursor
	if m.form.cursor != 0 {
		t.Error("typing should reset the project cursor")
	}
	m = press(m, "tab")
	has(t, m.View(), "(none)", "▸ (none)", "Write docs", "Review")
	lacks(t, m.View(), "Old")
	m = press(m, "down", "down", "down", "enter", "y")
	if !strings.Contains(m.footer(), `Started "fix" · Docs / Review`) {
		t.Errorf("footer = %q", m.footer())
	}
	if !strings.Contains(bodies[createRoute], `"task":{"id":12,"name":"Review"}`) {
		t.Errorf("body = %s", bodies[createRoute])
	}
}

func TestFormEdges(t *testing.T) {
	r := routes()
	r[createRoute] = `[{"id":8,"startTime":"2026-11-24T14:30:00","endTime":null}]`
	r[tasksRoute] = "HTTP 500"
	m := press(loaded(t, r), "n", "tab", "tab")
	if m.form.field != descField {
		t.Error("tab should skip the task field when no project is chosen")
	}
	m = press(m, "tab", "z", "z")
	has(t, m.View(), "no project matches", "choose a project first")
	m = press(m, "up", "esc")
	if m.overlay != noOverlay {
		t.Error("esc should close the form")
	}
	m = press(m, "n", "tab", "d", "o", "tab")
	if !m.statusErr {
		t.Error("a failed task load should show")
	}
	m = press(m, "tab", "tab", "enter", "y")
	if bodies[createRoute] != `{"startTime":"2026-11-24T14:30:45","endTime":null,"project":{"id":6,"name":"Docs"}}` {
		t.Errorf("body = %s", bodies[createRoute])
	}
	// Tasks for a project that is no longer chosen are dropped.
	f := m.form
	if got := f.withTasks(99, nil); got.tasksFor == 99 {
		t.Error("tasks for another project should be dropped")
	}
}

func TestFormTaskListScrolls(t *testing.T) {
	r := routes()
	r[tasksRoute] = `[{"id":1,"name":"t1"},{"id":2,"name":"t2"},{"id":3,"name":"t3"},{"id":4,"name":"t4"},{"id":5,"name":"t5"},{"id":6,"name":"t6"}]`
	m := press(loaded(t, r), "n", "tab", "d", "o", "tab")
	m = press(m, "down", "down", "down", "down", "down", "down")
	has(t, m.View(), "▸ t6")
	lacks(t, m.View(), "(none)")
	m = press(m, "tab")
	if m.form.field != descField {
		t.Error("tab from the task field should wrap to the description")
	}
	m = press(m, "tab", "tab") // tasks are already loaded for Docs, the cursor kept
	has(t, m.View(), "▸ t6")
}

func TestStartErrorShows(t *testing.T) {
	r := routes()
	r[createRoute] = "HTTP 500"
	m := press(loaded(t, r), "n", "enter", "y")
	if !m.statusErr {
		t.Errorf("footer = %q", m.footer())
	}
}

func TestUnknownMessageAndHelpers(t *testing.T) {
	m := loaded(t, routes())
	if _, cmd := m.Update(struct{}{}); cmd != nil {
		t.Error("an unknown message should do nothing")
	}
	if step(keyMsg("k")) != -1 || step(keyMsg("j")) != 1 || clamp(9, 3) != 2 || clamp(-1, 3) != 0 {
		t.Error("step or clamp is off")
	}
}

func TestRun(t *testing.T) {
	session := fakeSession(t, routes())
	if err := Run(context.Background(), session, tea.WithInput(strings.NewReader("q")), tea.WithOutput(io.Discard)); err != nil {
		t.Errorf("Run = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, session, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard)); err != context.Canceled {
		t.Errorf("Run with a cancelled context = %v", err)
	}
}

func TestConfirmationCancels(t *testing.T) {
	m := press(loaded(t, routes()), "x")
	if !strings.HasPrefix(m.footer(), `Stop "standup" · CLI (1h30m)?`) {
		t.Fatalf("footer = %q", m.footer())
	}
	m = press(m, "n")
	if m.confirm != nil || m.footer() != "Cancelled." || bodies[updateRoute] != "" {
		t.Errorf("any key but y should cancel without a call; footer = %q", m.footer())
	}
}

func TestStartsWithoutConfirmationWhenNothingRuns(t *testing.T) {
	r := routes()
	r[latestRoute] = "null"
	r[createRoute] = `[{"id":8,"startTime":"2026-11-24T14:30:00","endTime":null,"note":"review"}]`
	m := press(loaded(t, r), "s")
	if m.confirm != nil || !strings.Contains(m.footer(), `Started "review"`) {
		t.Errorf("footer = %q", m.footer())
	}
}
