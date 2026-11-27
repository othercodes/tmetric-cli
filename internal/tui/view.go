package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// ANSI palette colors, so the UI follows the terminal theme.
var (
	accent    = lipgloss.Color("6")
	cursorFg  = lipgloss.Color("5")
	errorFg   = lipgloss.Color("1")
	dim       = lipgloss.NewStyle().Faint(true)
	accentFg  = lipgloss.NewStyle().Foreground(accent)
	selection = lipgloss.NewStyle().Reverse(true)
	running   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
)

// Fixed sizes: the timer box, the footer, the sidebar.
const (
	timerHeight  = 3
	footerHeight = 1
	sidebarWidth = 18
)

func newTable() table.Model {
	return table.New()
}

// box draws a panel whose title sits in its top border.
func box(title, body string, width, height int, focused bool) string {
	border := lipgloss.NewStyle()
	if focused {
		border = border.Foreground(accent)
	}
	dashes := max(0, width-5-lipgloss.Width(title))
	top := border.Render("┌─") + " " + title + " " + border.Render(strings.Repeat("─", dashes)+"┐")
	inner := width - 4 // borders and one space of padding on each side
	lines := strings.Split(body, "\n")
	lines = lines[:min(len(lines), height-2)]
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, inner, "…")
	}
	content := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, true, true, true).
		BorderForeground(border.GetForeground()).
		Width(inner+2).Height(height-2).Padding(0, 1).
		Render(strings.Join(lines, "\n"))
	return top + "\n" + content
}

func (m model) mainHeight() int { return max(6, m.height-timerHeight-footerHeight) }
func (m model) panelWidth() int { return max(30, m.width-sidebarWidth) }

// layout sizes and styles the tables for the window and the focus.
func (m model) layout() model {
	inner := m.panelWidth() - 4
	rows := m.mainHeight() - 2 - 2 // borders, caption and summary
	panelFocused := m.focus == panelFocus && m.screen == mainScreen

	rest := max(20, inner-(5+7+6)-2*5)
	m.dayTable.SetColumns([]table.Column{
		{Title: "START", Width: 5}, {Title: "END", Width: 7}, {Title: "TIME", Width: 6},
		{Title: "PROJECT", Width: rest * 2 / 5}, {Title: "DESCRIPTION", Width: rest - rest*2/5},
	})
	m.weekTable.SetColumns([]table.Column{
		{Title: "DAY", Width: 10}, {Title: "ENTRIES", Width: 7}, {Title: "TIME", Width: max(6, inner-17-2*3)},
	})
	rest = max(20, inner-8-2*3)
	m.projectTable.SetColumns([]table.Column{
		{Title: "ID", Width: 8}, {Title: "PROJECT", Width: rest * 3 / 5}, {Title: "CLIENT", Width: rest - rest*3/5},
	})

	m.reportTable.SetColumns(reportColumns(inner))

	tables := []*table.Model{&m.dayTable, &m.weekTable, &m.projectTable, &m.reportTable}
	active := []bool{m.view == entriesView && m.tab == dayTab, m.view == entriesView && m.tab == weekTab, m.view == projectsView,
		m.view == reportsView && m.reportTab == projectsReportTab}
	for i, t := range tables {
		t.SetWidth(inner)
		t.SetHeight(rows)
		styles := table.DefaultStyles()
		// Two spaces after each column, none before: rows line up with the caption.
		styles.Header = lipgloss.NewStyle().Padding(0, 2, 0, 0)
		styles.Cell = lipgloss.NewStyle().Padding(0, 2, 0, 0)
		styles.Selected = lipgloss.NewStyle()
		if active[i] && panelFocused {
			styles.Selected = lipgloss.NewStyle().Foreground(cursorFg)
			t.Focus()
		} else {
			t.Blur()
		}
		t.SetStyles(styles)
	}
	return m.fill()
}

// fill puts the data in the tables. Durations of a running timer change
// every second, so it runs on every tick.
func (m model) fill() model {
	m.dayTable.SetRows(m.dayRows())
	m.weekTable.SetRows(m.weekRows())
	var rows []table.Row
	for _, p := range m.visibleProjects() {
		client := "-"
		if p.Client != nil {
			client = p.Client.Name
		}
		rows = append(rows, table.Row{id(p.ID), p.Name, client})
	}
	m.projectTable.SetRows(rows)
	m.reportTable.SetRows(m.reportRows())
	// An empty table leaves its cursor at -1; put it back on the first row,
	// or enter would read row -1 once the rows come back.
	for _, t := range []*table.Model{&m.dayTable, &m.weekTable, &m.projectTable, &m.reportTable} {
		if t.Cursor() < 0 {
			t.SetCursor(0)
		}
	}
	return m
}

func (m model) dayRows() []table.Row {
	if m.dayReport == nil {
		return nil
	}
	rows := make([]table.Row, len(m.dayReport.Entries))
	for i := range m.dayReport.Entries {
		entry := &m.dayReport.Entries[i]
		end := "running"
		if entry.EndTime != nil {
			end = entry.EndTime.In(m.session.Location).Format("15:04")
		}
		rows[i] = table.Row{entry.StartTime.In(m.session.Location).Format("15:04"), end,
			output.Duration(m.session.Elapsed(entry)), output.Project(entry), output.Note(entry)}
	}
	return rows
}

// weekDays sums the week's entries per day.
func (m model) weekDays() (counts [7]int, totals [7]time.Duration) {
	if m.weekRep == nil {
		return
	}
	for i := range m.weekRep.Entries {
		entry := &m.weekRep.Entries[i]
		start := entry.StartTime.In(m.session.Location)
		day := int(start.Sub(m.week).Hours() / 24)
		if day >= 0 && day < 7 {
			counts[day]++
			totals[day] += m.session.Elapsed(entry)
		}
	}
	return
}

func (m model) weekRows() []table.Row {
	counts, totals := m.weekDays()
	rows := make([]table.Row, 7)
	for i := range rows {
		time, entries := "–", "–"
		if counts[i] > 0 {
			time, entries = output.Duration(totals[i]), fmt.Sprint(counts[i])
		}
		rows[i] = table.Row{m.week.AddDate(0, 0, i).Format("Mon 02 Jan"), entries, time}
	}
	return rows
}

// liveTotal sums a report again, so a running timer keeps it moving.
func (m model) liveTotal(entries []tmetric.TimeEntry) time.Duration {
	var total time.Duration
	for i := range entries {
		total += m.session.Elapsed(&entries[i])
	}
	return total
}

func (m model) View() string {
	top := box("Timer", m.timerLine(), m.width, timerHeight, false)
	var middle string
	switch {
	case m.overlay == formOverlay:
		middle = m.center(m.formView())
	case m.overlay == helpOverlay:
		middle = m.center(m.helpView())
	case m.screen == accountScreen:
		middle = m.accountView()
	default:
		middle = lipgloss.JoinHorizontal(lipgloss.Top, m.sidebarView(), m.panelView())
	}
	return top + "\n" + middle + "\n" + m.footer()
}

func (m model) center(overlay string) string {
	return lipgloss.Place(m.width, m.mainHeight(), lipgloss.Center, lipgloss.Center, overlay)
}

func (m model) timerLine() string {
	if m.running == nil {
		return dim.Render("No timer running")
	}
	since := m.running.StartTime.In(m.session.Location).Format("15:04")
	return running.Render("● "+output.Duration(m.session.Elapsed(m.running))) + "   " +
		output.Describe(m.running) + dim.Render(" · since "+since)
}

func (m model) sidebarView() string {
	inner := sidebarWidth - 4
	lines := make([]string, len(viewNames))
	for i, name := range viewNames {
		line := fmt.Sprintf("%-*s", inner, name)
		if i == m.view {
			line = selection.Render(line)
		}
		lines[i] = line
	}
	return box("tmetric", strings.Join(lines, "\n"), sidebarWidth, m.mainHeight(), m.focus == sidebarFocus)
}

func (m model) panelView() string {
	var title, caption, summary string
	var body string
	switch {
	case m.view == reportsView:
		title, caption, body, summary = m.reportsPanel()
	case m.view == projectsView:
		title = accentFg.Render("Projects")
		caption = dim.Render("projects you can track time on")
		if m.filtering || m.filter.Value() != "" {
			caption = m.filter.View()
		}
		body = m.projectTable.View()
		summary = fmt.Sprintf("%d projects", len(m.visibleProjects()))
	case m.tab == weekTab:
		title = dim.Render("Day") + " │ " + accentFg.Render("Week")
		caption = dim.Render("week of " + m.week.Format("Mon 02 Jan 2006"))
		body = m.weekTable.View()
		summary = "total " + output.Duration(m.liveTotal(m.weekEntries()))
	default:
		title = accentFg.Render("Day") + " │ " + dim.Render("Week")
		label := m.day.Format("Mon 02 Jan 2006")
		if m.day.Equal(m.session.Midnight()) {
			label += " · today"
		}
		caption = dim.Render(label)
		body = m.dayTable.View()
		entries := m.dayEntries()
		summary = fmt.Sprintf("%d entries · %s", len(entries), output.Duration(m.liveTotal(entries)))
	}
	height := m.mainHeight()
	// The summary sits on the last line inside the box.
	lines := strings.Split(caption+"\n"+body, "\n")
	lines = append(lines, make([]string, max(0, height-3-len(lines)))...)
	lines = append(lines[:height-3], dim.Render(summary))
	return box(title, strings.Join(lines, "\n"), m.panelWidth(), height, m.focus == panelFocus)
}

func (m model) dayEntries() []tmetric.TimeEntry {
	if m.dayReport == nil {
		return nil
	}
	return m.dayReport.Entries
}

func (m model) weekEntries() []tmetric.TimeEntry {
	if m.weekRep == nil {
		return nil
	}
	return m.weekRep.Entries
}

// keyValues renders dimmed labels and their values.
func keyValues(pairs [][2]string) string {
	lines := make([]string, len(pairs))
	for i, p := range pairs {
		lines[i] = dim.Render(fmt.Sprintf("%-16s", p[0])) + p[1]
	}
	return strings.Join(lines, "\n")
}

func (m model) accountView() string {
	user := m.session.User
	profile := keyValues([][2]string{{"Name", user.Name}, {"Email", user.Email}, {"Timezone", user.TimeZone.ID}})
	workspace := [][2]string{{"ID", id(user.ActiveAccountID)}}
	if account := m.session.Account(); account != nil {
		workspace = [][2]string{{"Name", account.Name}, {"ID", id(account.ID)}, {"Week starts on", account.FirstWeekDay.String()}}
	}
	first := box("Account", profile, m.width, 5, true)
	second := box("Workspace", keyValues(workspace), m.width, m.mainHeight()-5, false)
	return first + "\n" + second
}

func (m model) formView() string {
	f := m.form
	label := func(field int, text string) string {
		style := dim
		if f.field == field {
			style = accentFg
		}
		return style.Render(fmt.Sprintf("%-13s", text))
	}
	lines := []string{label(descField, "Description") + f.description.View()}
	if found := f.suggestions(); len(found) > 0 {
		names := make([]string, len(found))
		for i, r := range found {
			names[i] = output.Describe(&tmetric.TimeEntry{Note: r.Note, Project: r.Project, Task: r.Task})
		}
		lines = append(lines, pickList(names, f.suggest, "")...)
	}
	lines = append(lines, label(projectField, "Project")+f.project.View())
	if found := f.projectSuggestions(); len(found) > 0 {
		lines = append(lines, pickList(projectNames(found), f.suggest, "")...)
	} else if strings.TrimSpace(f.project.Value()) != "" {
		lines = append(lines, pickList(projectNames(f.matches()), f.cursor, "no project matches")...)
	}
	chosen := f.chosen()
	switch {
	case chosen == nil:
		lines = append(lines, label(taskField, "Task")+dim.Render("choose a project first"))
	case f.field == taskField && f.tasksFor == chosen.ID:
		names := []string{"(none)"}
		for _, t := range f.tasks {
			names = append(names, t.Name)
		}
		lines = append(lines, label(taskField, "Task"))
		lines = append(lines, pickList(names, f.taskCursor, "")...)
	default:
		lines = append(lines, label(taskField, "Task")+dim.Render(taskHint(f)))
	}
	title := "New timer"
	if f.editing != nil {
		title = "Edit entry"
		lines = append(lines, label(startField, "Start")+f.start.View(), label(endField, "End")+f.end.View())
	}
	return box(accentFg.Render(title), strings.Join(lines, "\n"), 60, len(lines)+2, true)
}

func projectNames(projects []tmetric.Project) []string {
	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = p.Name
	}
	return names
}

// taskHint stands for the task list until it opens: the task of the
// suggestion the form used, the entry's own task when editing, otherwise a
// reminder that a task is optional. An edit cannot set an external task.
func taskHint(f form) string {
	task := (*tmetric.Task)(nil)
	switch {
	case f.editing != nil && f.unchanged() && external(f.used.Task):
		return keptTask(f) + " (" + f.used.Task.Name + " is external, not set)"
	case f.unchanged():
		task = f.used.Task
	case f.editing != nil:
		task = f.editing.Task
	}
	if task != nil && task.Name != "" && !f.taskTouched {
		return task.Name
	}
	return "optional"
}

// pickList shows up to listRows names around the cursor, the highlighted one
// marked, or empty when there are none.
func pickList(names []string, cursor int, empty string) []string {
	if len(names) == 0 {
		return []string{strings.Repeat(" ", 13) + dim.Render(empty)}
	}
	from := max(0, cursor-listRows+1)
	var lines []string
	for i := from; i < min(len(names), from+listRows); i++ {
		lines = append(lines, pick(names[i], i == cursor))
	}
	return lines
}

// pick is one row of a form list, marked when it is the highlighted one.
func pick(name string, highlighted bool) string {
	if highlighted {
		return strings.Repeat(" ", 11) + lipgloss.NewStyle().Foreground(cursorFg).Render("▸ "+name)
	}
	return strings.Repeat(" ", 13) + name
}

func (m model) helpView() string {
	column := func(title string, keys []key.Binding) []string {
		lines := []string{accentFg.Render(title)}
		for _, k := range keys {
			// Pad, then color: escape codes would count as width.
			lines = append(lines, accentFg.Render(fmt.Sprintf("%-8s", k.Help().Key))+" "+k.Help().Desc)
		}
		return lines
	}
	under := m
	under.overlay = noOverlay
	global := column("Global", under.globalKeys())
	if m.screen == mainScreen {
		global = append(global, "", accentFg.Render("Views"))
		for i, name := range viewNames {
			global = append(global, accentFg.Render(fmt.Sprintf("%-8s", string(rune('1'+i))))+" "+name)
		}
	}
	left := strings.Join(global, "\n")
	right := strings.Join(column("This view", under.viewKeys()), "\n")
	body := lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(28).Render(left), right)
	return box(accentFg.Render("Keybindings"), body, 60, lipgloss.Height(body)+2, true)
}

// footer lists the keys that work here, most useful first, as many as fit,
// always ending with the keybindings help. A notice or an error takes its
// place for a few seconds.
func (m model) footer() string {
	if m.confirm != nil {
		return ansi.Truncate(m.confirm.question+" "+accentFg.Render("y")+"/N", m.width, "…")
	}
	if m.status != "" {
		if m.statusErr {
			return lipgloss.NewStyle().Foreground(errorFg).Render(ansi.Truncate(m.status, m.width, "…"))
		}
		return ansi.Truncate(m.status, m.width, "…")
	}
	format := func(k key.Binding) string { return k.Help().Desc + ": " + accentFg.Render(k.Help().Key) }
	last := format(keyHelp)
	seen := map[string]bool{keyHelp.Help().Desc: true}
	var parts []string
	keys := m.viewKeys()
	if m.overlay == noOverlay && !m.filtering {
		keys = append(keys, m.globalKeys()...)
	}
	for _, k := range keys {
		if seen[k.Help().Desc] {
			continue
		}
		seen[k.Help().Desc] = true
		next := append(append([]string{}, parts...), format(k))
		if lipgloss.Width(strings.Join(append(next, last), " | ")) > m.width {
			break
		}
		parts = next
	}
	return strings.Join(append(parts, last), " | ")
}

// keptTask is the task an edit leaves on the entry: its own while the project
// stays, none when the project changes.
func keptTask(f form) string {
	was, chosen := f.editing, f.chosen()
	if was.Task == nil || was.Project == nil || chosen == nil || chosen.ID != was.Project.ID {
		return "no task"
	}
	return was.Task.Name
}
