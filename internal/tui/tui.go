// Package tui is the interactive front-end launched by `tmetric ui`. It reads
// through internal/app, never through the cobra commands. docs/tui-design.md
// describes how it looks and behaves.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/othercodes/tmetric-cli/internal/app"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// Run draws the UI until the user quits or ctx is cancelled. opts let tests
// feed keys in and throw the drawing away.
func Run(ctx context.Context, session *app.Session, opts ...tea.ProgramOption) error {
	opts = append([]tea.ProgramOption{tea.WithAltScreen(), tea.WithContext(ctx)}, opts...)
	_, err := tea.NewProgram(newModel(ctx, session), opts...).Run()
	if errors.Is(err, tea.ErrProgramKilled) {
		return ctx.Err()
	}
	return err
}

// Screens, views, tabs, focus and overlays.
const (
	mainScreen = iota
	accountScreen
)

const (
	entriesView = iota
	projectsView
	reportsView
)

var viewNames = []string{"Time entries", "Projects", "Reports"}

const (
	dayTab = iota
	weekTab
)

const (
	panelFocus = iota
	sidebarFocus
)

const (
	noOverlay = iota
	formOverlay
	helpOverlay
)

// statusFor is how long a notice or an error replaces the footer.
const statusFor = 5 * time.Second

// Messages from the API calls and the clock.
type (
	timerLoaded struct {
		entry *tmetric.TimeEntry
		err   error
	}
	dayLoaded struct {
		day    time.Time
		report *app.Report
		err    error
	}
	weekLoaded struct {
		start  time.Time
		report *app.Report
		err    error
	}
	projectsLoaded struct {
		projects []tmetric.Project
		err      error
	}
	recentLoaded struct {
		recent []tmetric.RecentEntry
		err    error
	}
	tasksLoaded struct {
		projectID int64
		tasks     []tmetric.Task
		err       error
	}
	// acted is a finished start or stop, after which the data reloads.
	acted struct {
		notice string
		err    error
		// retry, on an edit that overlaps other entries, overwrites them.
		retry tea.Cmd
	}
	tick struct{}
)

// confirmation is an action that would stop the running timer, held until
// the user says yes.
type confirmation struct {
	question string
	action   tea.Cmd
}

type model struct {
	ctx     context.Context
	session *app.Session

	width, height int

	screen, view, tab, focus, overlay int

	running   *tmetric.TimeEntry
	day       time.Time
	dayReport *app.Report
	week      time.Time
	weekRep   *app.Report
	projects  []tmetric.Project

	dayTable, weekTable, projectTable table.Model

	// The Reports view: its tab, the range of the project report (an index in
	// app.Ranges) and the answers.
	reportTab, reportRange int
	report                 []tmetric.ProjectReport
	reportFrom, reportTo   time.Time
	reportReady            bool
	reportTable            table.Model
	balance                *tmetric.Balance

	filtering bool
	filter    textinput.Model

	form form

	status    string
	statusErr bool
	statusEnd time.Time

	// confirm, when set, is an action waiting for a y in the footer.
	confirm *confirmation

	title string
}

func newModel(ctx context.Context, session *app.Session) model {
	filter := textinput.New()
	filter.Prompt = "/"
	filter.Cursor.SetMode(cursor.CursorStatic)
	today := session.Midnight()
	m := model{
		ctx: ctx, session: session,
		width: 100, height: 30,
		day: today, week: session.WeekStart(today),
		dayTable: newTable(), weekTable: newTable(), projectTable: newTable(), reportTable: newTable(),
		filter: filter,
	}
	return m.layout()
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.reload(), m.loadProjects, tickEvery())
}

// tickInterval is how often the clock redraws; tests shorten it.
var tickInterval = time.Second

func tickEvery() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg { return tick{} })
}

func (m model) loadTimer() tea.Msg {
	entry, err := m.session.Running(m.ctx)
	return timerLoaded{entry, err}
}

func (m model) loadDay(day time.Time) tea.Cmd {
	return func() tea.Msg {
		report, err := m.session.Day(m.ctx, day)
		return dayLoaded{day, report, err}
	}
}

func (m model) loadWeek(start time.Time) tea.Cmd {
	return func() tea.Msg {
		report, err := m.session.WeekOf(m.ctx, start)
		return weekLoaded{start, report, err}
	}
}

func (m model) loadProjects() tea.Msg {
	projects, err := m.session.Projects(m.ctx)
	return projectsLoaded{projects, err}
}

func (m model) loadRecent() tea.Msg {
	recent, err := m.session.Recent(m.ctx)
	return recentLoaded{recent, err}
}

func (m model) loadTasks(projectID int64) tea.Cmd {
	return func() tea.Msg {
		tasks, err := m.session.OpenTasks(m.ctx, projectID)
		return tasksLoaded{projectID, tasks, err}
	}
}

// reload refreshes what a start, a stop or an edit can change.
func (m model) reload() tea.Cmd {
	return tea.Batch(m.loadTimer, m.loadDay(m.day), m.loadWeek(m.week), m.loadReport(m.reportRange), m.loadBalance)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Every answer of the API carries its error: a failed call shows in the
	// footer and changes nothing else.
	if result, ok := msg.(loadResult); ok && result.failure() != nil {
		return m.fail(result.failure()), nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m.layout(), nil
	case tick:
		return m.onTick()
	case acted:
		return m.onActed(msg)
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m.apply(msg), nil
}

// loadResult is an answer of the API.
type loadResult interface{ failure() error }

func (r timerLoaded) failure() error    { return r.err }
func (r dayLoaded) failure() error      { return r.err }
func (r weekLoaded) failure() error     { return r.err }
func (r projectsLoaded) failure() error { return r.err }
func (r tasksLoaded) failure() error    { return r.err }
func (r recentLoaded) failure() error   { return r.err }

// apply stores a successful answer of the API.
func (m model) apply(msg tea.Msg) model {
	switch msg := msg.(type) {
	case timerLoaded:
		m.running = msg.entry
	case dayLoaded:
		// A slow answer for a day the user has already left is dropped.
		if msg.day.Equal(m.day) {
			m.dayReport = msg.report
		}
	case weekLoaded:
		if msg.start.Equal(m.week) {
			m.weekRep = msg.report
		}
	case projectsLoaded:
		m.projects = msg.projects
	case tasksLoaded:
		m.form = m.form.withTasks(msg.projectID, msg.tasks)
	case recentLoaded:
		// The recent projects come first in the matches: keep the highlighted one.
		chosen := m.form.chosen()
		m.form.recent = msg.recent
		if chosen != nil {
			m.form.cursor = m.form.indexOf(chosen.ID)
		}
	case reportLoaded:
		m = m.applyReport(msg)
	case balanceLoaded:
		m.balance = msg.balance
	}
	return m.fill()
}

// onTick redraws the clock, clears an old notice and keeps the window title
// in step with the timer.
func (m model) onTick() (tea.Model, tea.Cmd) {
	if !m.statusEnd.IsZero() && m.session.Now().After(m.statusEnd) {
		m.status, m.statusEnd = "", time.Time{}
	}
	m = m.fill()
	cmds := []tea.Cmd{tickEvery()}
	if title := m.windowTitle(); title != m.title {
		m.title = title
		cmds = append(cmds, tea.SetWindowTitle(title))
	}
	return m, tea.Batch(cmds...)
}

// onActed reports a finished action and reloads what it changed. An edit
// that would overwrite other entries asks before it goes ahead.
func (m model) onActed(msg acted) (tea.Model, tea.Cmd) {
	var overlap *app.OverlapError
	if errors.As(msg.err, &overlap) && msg.retry != nil {
		names := make([]string, len(overlap.Entries))
		for i := range overlap.Entries {
			names[i] = output.Describe(&overlap.Entries[i])
		}
		m.confirm = &confirmation{
			question: fmt.Sprintf("This overwrites %s; TMetric cuts or removes them. Save anyway?", strings.Join(names, ", ")),
			action:   msg.retry,
		}
		return m, nil
	}
	if msg.err != nil {
		return m.fail(msg.err), m.reload()
	}
	return m.notify(msg.notice), m.reload()
}

func (m model) notify(notice string) model {
	m.status, m.statusErr, m.statusEnd = notice, false, m.session.Now().Add(statusFor)
	return m
}

func (m model) fail(err error) model {
	m.status, m.statusErr, m.statusEnd = "error: "+err.Error(), true, m.session.Now().Add(statusFor)
	return m
}

// guard runs action now, or first asks when it would stop the running
// timer: TMetric closes it on the spot, and the CLI cannot undo that. next
// names what starts instead, if anything.
func (m model) guard(action tea.Cmd, next string) (tea.Model, tea.Cmd) {
	if m.running == nil {
		return m, action
	}
	question := "Stop " + output.Describe(m.running) + " (" + output.Duration(m.session.Elapsed(m.running)) + ")"
	if next != "" {
		question += " and start " + next
	}
	m.confirm = &confirmation{question: question + "?", action: action}
	return m, nil
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm != nil {
		action := m.confirm.action
		m.confirm = nil
		if msg.String() == "y" {
			return m, action
		}
		return m.notify("Cancelled."), nil
	}
	switch {
	case m.overlay == formOverlay:
		return m.updateForm(msg)
	case m.overlay == helpOverlay:
		if key.Matches(msg, keyBack, keyHelp, keyQuit) {
			m.overlay = noOverlay
		}
		return m, nil
	case m.filtering:
		return m.updateFilter(msg)
	}

	switch {
	case key.Matches(msg, keyQuit):
		return m, tea.Sequence(tea.SetWindowTitle("tmetric"), tea.Quit)
	case key.Matches(msg, keyHelp):
		m.overlay = helpOverlay
		return m, nil
	case key.Matches(msg, keyRefresh):
		return m, tea.Batch(m.reload(), m.loadProjects)
	case key.Matches(msg, keyStop):
		return m.guard(m.stop, "")
	case key.Matches(msg, keyNew):
		return m.openForm(nil)
	case key.Matches(msg, keyAccount):
		m.screen = accountScreen
		return m, nil
	case key.Matches(msg, keyBack):
		m.screen = mainScreen
		return m, nil
	}
	if m.screen == accountScreen {
		return m, nil
	}

	switch {
	case key.Matches(msg, keyViews):
		m.view = int(msg.String()[0] - '1')
		return m.layout(), nil
	case key.Matches(msg, keyFocus):
		m.focus = 1 - m.focus
		return m.layout(), nil
	}
	if m.focus == sidebarFocus {
		if key.Matches(msg, keyMove) {
			m.view = (m.view + step(msg) + len(viewNames)) % len(viewNames)
			return m.layout(), nil
		}
		return m, nil
	}
	switch m.view {
	case projectsView:
		return m.updateProjects(msg)
	case reportsView:
		return m.updateReports(msg)
	}
	return m.updateEntries(msg)
}

func (m model) updateEntries(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyTabs):
		m.tab = 1 - m.tab
		return m.layout(), nil
	case key.Matches(msg, keyToday):
		today := m.session.Midnight()
		return m.goTo(today, m.session.WeekStart(today))
	case key.Matches(msg, keyDay):
		if m.tab == weekTab {
			return m.goTo(m.day, m.week.AddDate(0, 0, 7*step(msg)))
		}
		day := m.day.AddDate(0, 0, step(msg))
		return m.goTo(day, m.session.WeekStart(day))
	case key.Matches(msg, keyOpenDay) && m.tab == weekTab:
		m.tab = dayTab
		day := m.week.AddDate(0, 0, m.weekTable.Cursor())
		next, cmd := m.goTo(day, m.week)
		return next.(model).layout(), cmd
	case key.Matches(msg, keyContinue) && m.tab == dayTab:
		entry := m.selectedEntry()
		if entry == nil {
			return m, nil
		}
		return m.guard(m.continueEntry(entry), output.Describe(entry))
	case key.Matches(msg, keyEdit):
		if entry := m.selectedEntry(); entry != nil {
			return m.openEdit(*entry)
		}
		return m, nil
	case key.Matches(msg, keyDelete):
		entry := m.selectedEntry()
		if entry == nil {
			return m, nil
		}
		m.confirm = &confirmation{
			question: "Delete " + output.Describe(entry) + " " + span(m.session.Location, entry) + "?",
			action:   m.deleteEntry(*entry),
		}
		return m, nil
	}
	var cmd tea.Cmd
	if m.tab == weekTab {
		m.weekTable, cmd = m.weekTable.Update(msg)
	} else {
		m.dayTable, cmd = m.dayTable.Update(msg)
	}
	return m, cmd
}

// selectedEntry is the entry under the cursor of the Day tab, nil when the
// day has none or another tab shows.
func (m model) selectedEntry() *tmetric.TimeEntry {
	entries := m.dayEntries()
	if m.tab != dayTab || len(entries) == 0 {
		return nil
	}
	entry := entries[m.dayTable.Cursor()]
	return &entry
}

// span is an entry's "09:00–10:30", or "09:00–now" while it runs.
func span(loc *time.Location, entry *tmetric.TimeEntry) string {
	end := "now"
	if entry.EndTime != nil {
		end = entry.EndTime.In(loc).Format("15:04")
	}
	return entry.StartTime.In(loc).Format("15:04") + "–" + end
}

// goTo shows day in the Day tab and the week starting on week, loading what
// changed.
func (m model) goTo(day, week time.Time) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if !day.Equal(m.day) {
		m.day, m.dayReport = day, nil
		cmds = append(cmds, m.loadDay(day))
	}
	if !week.Equal(m.week) {
		m.week, m.weekRep = week, nil
		cmds = append(cmds, m.loadWeek(week))
	}
	return m.fill(), tea.Batch(cmds...)
}

func (m model) updateProjects(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyFilter):
		m.filtering = true
		return m, m.filter.Focus()
	case key.Matches(msg, keyStart):
		project := m.selectedProject()
		if project == nil {
			return m, nil
		}
		return m.openForm(project)
	}
	var cmd tea.Cmd
	m.projectTable, cmd = m.projectTable.Update(msg)
	return m, cmd
}

func (m model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyBack):
		m.filtering = false
		m.filter.Blur()
		m.filter.SetValue("")
		return m.fill(), nil
	case key.Matches(msg, keyStart):
		m.filtering = false
		m.filter.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.projectTable.SetCursor(0)
	return m.fill(), cmd
}

// visibleProjects are the projects whose name holds the filter.
func (m model) visibleProjects() []tmetric.Project {
	query := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	var shown []tmetric.Project
	for _, p := range m.projects {
		if strings.Contains(strings.ToLower(p.Name), query) {
			shown = append(shown, p)
		}
	}
	return shown
}

func (m model) selectedProject() *tmetric.Project {
	shown := m.visibleProjects()
	if len(shown) == 0 {
		return nil
	}
	return &shown[m.projectTable.Cursor()]
}

func (m model) stop() tea.Msg {
	stopped, err := m.session.Stop(m.ctx)
	switch {
	case err != nil:
		return acted{err: err}
	case stopped == nil:
		return acted{notice: "No timer running."}
	case stopped.Dropped:
		return acted{notice: "Stopped " + output.Describe(stopped.Entry) + "; TMetric dropped it (shorter than a minute)."}
	default:
		return acted{notice: "Stopped " + output.Describe(stopped.Entry) + " (" + output.Duration(m.session.Elapsed(stopped.Entry)) + ")"}
	}
}

func (m model) continueEntry(entry *tmetric.TimeEntry) tea.Cmd {
	return func() tea.Msg {
		started, stopped, err := m.session.Continue(m.ctx, entry)
		return startedMsg(started, stopped, err)
	}
}

func (m model) start(opts app.StartOptions) tea.Cmd {
	return func() tea.Msg {
		started, stopped, err := m.session.Start(m.ctx, opts)
		return startedMsg(started, stopped, err)
	}
}

func startedMsg(started, stopped *tmetric.TimeEntry, err error) acted {
	if err != nil {
		return acted{err: err}
	}
	notice := "Started " + output.Describe(started)
	if stopped != nil {
		notice += ", stopped " + output.Describe(stopped)
	}
	return acted{notice: notice}
}

// windowTitle is what the terminal window shows while the UI runs.
func (m model) windowTitle() string {
	if m.running == nil {
		return "tmetric"
	}
	return "● " + output.Duration(m.session.Elapsed(m.running)) + " · " + output.Note(m.running)
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

// step is -1 for the keys that go back (up, left) and 1 for the others.
func step(msg tea.KeyMsg) int {
	switch msg.String() {
	case "up", "k", "left", "h":
		return -1
	}
	return 1
}
