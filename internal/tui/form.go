package tui

import (
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/othercodes/tmetric-cli/internal/app"
	"github.com/othercodes/tmetric-cli/internal/output"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// The fields of the form. Start and end only exist when editing.
const (
	descField = iota
	projectField
	taskField
	startField
	endField
)

// listRows is how many matches a form list shows.
const listRows = 5

// form is the overlay that starts a timer (n) or edits an entry (enter): a
// description, a project picked by typing part of its name, an optional task
// of that project and, when editing, the start and end.
type form struct {
	field       int
	description textinput.Model
	project     textinput.Model
	projects    []tmetric.Project
	cursor      int // in the project matches
	tasks       []tmetric.Task
	tasksFor    int64
	taskCursor  int  // 0 is "no task"
	taskTouched bool // the user moved in the task list

	// recent feeds the suggestions: whole entries on the description, their
	// projects on an empty project field. suggest is the highlighted one, -1
	// while typing. used is the suggestion that filled the form, until the
	// form drifts from it.
	recent  []tmetric.RecentEntry
	suggest int
	used    *tmetric.RecentEntry

	// editing is the entry as it was, nil for a new timer.
	editing    *tmetric.TimeEntry
	day        time.Time
	start, end textinput.Model
}

func newInput(placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.CharLimit = 255
	// The form box is 60 wide; labels take 13 and the borders 4. Without a
	// width, an unfocused field shows only the first letter of its hint.
	input.Width = 42
	// A steady cursor: a blinking one redraws the screen twice a second.
	input.Cursor.SetMode(cursor.CursorStatic)
	return input
}

func newForm(projects []tmetric.Project, chosen *tmetric.Project) form {
	f := form{
		description: newInput("what are you working on?"),
		project:     newInput("type to filter"),
		start:       newInput("HH:MM"),
		end:         newInput("HH:MM, empty while it runs"),
		projects:    projects,
		suggest:     -1,
	}
	if chosen != nil {
		f.project.SetValue(chosen.Name)
		f.cursor = f.indexOf(chosen.ID)
	}
	return f
}

// editForm is the form filled with entry, which is on day.
func (m model) editForm(entry tmetric.TimeEntry, day time.Time) form {
	f := newForm(m.projects, entry.Project)
	f.editing, f.day = &entry, day
	f.description.SetValue(entry.Note)
	f.start.SetValue(entry.StartTime.In(m.session.Location).Format("15:04"))
	if entry.EndTime != nil {
		f.end.SetValue(entry.EndTime.In(m.session.Location).Format("15:04"))
	}
	return f
}

func (m model) openForm(chosen *tmetric.Project) (tea.Model, tea.Cmd) {
	m.overlay = formOverlay
	m.form = newForm(m.projects, chosen)
	return m, tea.Batch(m.form.description.Focus(), m.loadRecent)
}

// suggestions are the recent entries whose description holds what was typed,
// as many as the list shows, on the description field. An edit shows them
// once the description changes, or the first one would be the entry itself.
func (f form) suggestions() []tmetric.RecentEntry {
	if f.field != descField || f.editing != nil && f.description.Value() == f.editing.Note {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(f.description.Value()))
	var found []tmetric.RecentEntry
	for _, r := range f.recent {
		if strings.Contains(strings.ToLower(r.Note), query) && len(found) < listRows {
			found = append(found, r)
		}
	}
	return found
}

// recentProjects are the projects of the recent entries that can still be
// tracked, once each, in the order of the recent entries (pinned first).
func (f form) recentProjects() []tmetric.Project {
	var found []tmetric.Project
	for _, r := range f.recent {
		if r.Project == nil || slices.ContainsFunc(found, sameProject(r.Project.ID)) {
			continue
		}
		if i := slices.IndexFunc(f.projects, sameProject(r.Project.ID)); i >= 0 {
			found = append(found, f.projects[i])
		}
	}
	return found
}

func sameProject(id int64) func(tmetric.Project) bool {
	return func(p tmetric.Project) bool { return p.ID == id }
}

// projectSuggestions are the recent projects while the project field is
// focused and empty.
func (f form) projectSuggestions() []tmetric.Project {
	if f.field != projectField || strings.TrimSpace(f.project.Value()) != "" {
		return nil
	}
	return f.recentProjects()
}

// use fills the form with the highlighted suggestion; it does not start or
// save anything.
func (f form) use() form {
	if f.field == projectField {
		f = f.withProject(&f.projectSuggestions()[f.suggest])
		f.suggest = -1
		return f
	}
	r := f.suggestions()[f.suggest]
	f.description.SetValue(r.Note)
	f.description.CursorEnd()
	// An edit cannot take the project away, so a suggestion with no project
	// leaves the entry's project as it is.
	if r.Project != nil || f.editing == nil {
		f = f.withProject(r.Project)
	}
	f.taskCursor, f.taskTouched = 0, false
	f.used, f.suggest = &r, -1
	return f
}

// withProject puts project, or none, in the project field.
func (f form) withProject(project *tmetric.Project) form {
	f.project.SetValue("")
	f.cursor, f.tasks, f.tasksFor = 0, nil, 0
	if project != nil {
		f.project.SetValue(project.Name)
		f.project.CursorEnd()
		f.cursor = f.indexOf(project.ID)
	}
	return f
}

// unchanged reports whether the form still holds the suggestion it used, so
// the timer can start from it whole, a link to an external task included.
func (f form) unchanged() bool {
	if f.used == nil || f.taskTouched || strings.TrimSpace(f.description.Value()) != f.used.Note {
		return false
	}
	chosen := f.chosen()
	if f.used.Project == nil {
		return chosen == nil
	}
	return chosen != nil && chosen.ID == f.used.Project.ID
}

func (m model) openEdit(entry tmetric.TimeEntry) (tea.Model, tea.Cmd) {
	m.overlay = formOverlay
	m.form = m.editForm(entry, m.day)
	return m, tea.Batch(m.form.description.Focus(), m.loadRecent)
}

// fields is how many fields the form has.
func (f form) fields() int {
	if f.editing != nil {
		return 5
	}
	return 3
}

// matches are the projects whose name holds what was typed, the recent ones
// first.
func (f form) matches() []tmetric.Project {
	query := strings.ToLower(strings.TrimSpace(f.project.Value()))
	ordered := f.recentProjects()
	for _, p := range f.projects {
		if !slices.ContainsFunc(ordered, sameProject(p.ID)) {
			ordered = append(ordered, p)
		}
	}
	var found []tmetric.Project
	for _, p := range ordered {
		if strings.Contains(strings.ToLower(p.Name), query) {
			found = append(found, p)
		}
	}
	return found
}

// indexOf is where project id is among the matches, 0 when it is not there.
func (f form) indexOf(id int64) int {
	for i, p := range f.matches() {
		if p.ID == id {
			return i
		}
	}
	return 0
}

// chosen is the highlighted project match, or nil when the field is empty or
// matches nothing.
func (f form) chosen() *tmetric.Project {
	found := f.matches()
	if strings.TrimSpace(f.project.Value()) == "" || len(found) == 0 {
		return nil
	}
	return &found[min(f.cursor, len(found)-1)]
}

func (f form) withTasks(projectID int64, tasks []tmetric.Task) form {
	chosen := f.chosen()
	if chosen == nil || chosen.ID != projectID {
		return f
	}
	f.tasks, f.tasksFor, f.taskCursor = tasks, projectID, 0
	// When editing, the entry's own task starts highlighted.
	if f.editing != nil && f.editing.Task != nil && f.editing.Task.ID != nil {
		for i, t := range tasks {
			if *t.ID == *f.editing.Task.ID {
				f.taskCursor = i + 1
			}
		}
	}
	return f
}

// input is the text field being typed in, or nil on the task list.
func (f *form) input() *textinput.Model {
	switch f.field {
	case descField:
		return &f.description
	case projectField:
		return &f.project
	case startField:
		return &f.start
	case endField:
		return &f.end
	}
	return nil
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.form
	switch {
	case key.Matches(msg, keyBack):
		m.overlay = noOverlay
		return m, nil
	case key.Matches(msg, keyStart) && f.suggest >= 0:
		m.form = f.use()
		return m, nil
	case key.Matches(msg, keyStart):
		m.overlay = noOverlay
		if f.editing != nil {
			return m, m.edit(f.day, f.editing, f.change())
		}
		if f.unchanged() {
			entry := tmetric.TimeEntry{Note: f.used.Note, Project: f.used.Project, Task: f.used.Task}
			return m.guard(m.continueEntry(&entry), output.Describe(&entry))
		}
		opts := f.options()
		return m.guard(m.start(opts), `"`+opts.Note+`"`)
	case key.Matches(msg, keyNext):
		return m.nextField()
	case msg.Type == tea.KeyUp || msg.Type == tea.KeyDown:
		m.form = f.move(step(msg))
		return m, nil
	}
	input := f.input()
	if input == nil {
		return m, nil
	}
	before := f.project.Value()
	var cmd tea.Cmd
	*input, cmd = input.Update(msg)
	f.suggest = -1
	if f.project.Value() != before {
		f.cursor, f.tasks, f.tasksFor = 0, nil, 0
	}
	m.form = f
	return m, cmd
}

// move goes up or down the list of the field. On a suggestion list, -1 is
// back to typing.
func (f form) move(delta int) form {
	suggestions := len(f.suggestions()) + len(f.projectSuggestions())
	switch {
	case f.field == descField || suggestions > 0:
		f.suggest = max(-1, min(f.suggest+delta, suggestions-1))
	case f.field == projectField:
		f.cursor = clamp(f.cursor+delta, len(f.matches()))
	case f.field == taskField:
		f.taskCursor = clamp(f.taskCursor+delta, len(f.tasks)+1)
		f.taskTouched = true
	}
	return f
}

// nextField moves on, skipping the task field when there is no project, and
// loads the project's tasks when the task field comes up.
func (m model) nextField() (tea.Model, tea.Cmd) {
	f := m.form
	if input := f.input(); input != nil {
		input.Blur()
	}
	chosen := f.chosen()
	f.suggest = -1
	f.field = (f.field + 1) % f.fields()
	if f.field == taskField && chosen == nil {
		f.field = (f.field + 1) % f.fields()
	}
	var cmd tea.Cmd
	if input := f.input(); input != nil {
		cmd = input.Focus()
	} else if f.tasksFor != chosen.ID {
		cmd = m.loadTasks(chosen.ID)
	}
	m.form = f
	return m, cmd
}

// options turn the new timer form into a start request, by ID so names never
// clash.
func (f form) options() app.StartOptions {
	opts := app.StartOptions{Note: strings.TrimSpace(f.description.Value())}
	chosen := f.chosen()
	if chosen == nil {
		return opts
	}
	opts.Project = id(chosen.ID)
	if f.tasksFor == chosen.ID && f.taskCursor > 0 {
		opts.Task = id(*f.tasks[f.taskCursor-1].ID)
	}
	return opts
}

// change is what the edit form changed, field by field: what the user left
// alone stays as it is, a link to an external task included.
func (f form) change() app.Change {
	var change app.Change
	was := f.editing
	if note := strings.TrimSpace(f.description.Value()); note != was.Note {
		change.Note = &note
	}
	chosen := f.chosen()
	if chosen != nil && (was.Project == nil || chosen.ID != was.Project.ID) {
		project := id(chosen.ID)
		change.Project = &project
	}
	switch {
	case f.taskTouched && chosen != nil && f.tasksFor == chosen.ID:
		task := ""
		if f.taskCursor > 0 {
			task = id(*f.tasks[f.taskCursor-1].ID)
		}
		change.Task = &task
	case f.unchanged() && !external(f.used.Task):
		// The task of the suggestion it used, or none.
		task := ""
		if f.used.Task != nil {
			task = id(*f.used.Task.ID)
		}
		change.Task = &task
	}
	if start := strings.TrimSpace(f.start.Value()); start != clockOf(&was.StartTime, f.day.Location()) {
		change.Start = &start
	}
	if end := strings.TrimSpace(f.end.Value()); end != "" && end != clockOf(was.EndTime, f.day.Location()) {
		change.End = &end
	}
	return change
}

// clockOf is t as HH:MM in loc, or empty for no time.
func clockOf(t *tmetric.LocalTime, loc *time.Location) string {
	if t == nil {
		return ""
	}
	return t.In(loc).Format("15:04")
}

// edit saves change; an overlap comes back with a retry that overwrites.
func (m model) edit(day time.Time, was *tmetric.TimeEntry, change app.Change) tea.Cmd {
	return func() tea.Msg {
		edited, err := m.session.Edit(m.ctx, day, was.ID, change)
		if err != nil {
			change.Overwrite = true
			return acted{err: err, retry: m.edit(day, was, change)}
		}
		return acted{notice: "Updated " + output.Describe(edited)}
	}
}

func (m model) deleteEntry(entry tmetric.TimeEntry) tea.Cmd {
	return func() tea.Msg {
		if err := m.session.Delete(m.ctx, entry.ID); err != nil {
			return acted{err: err}
		}
		return acted{notice: "Deleted " + output.Describe(&entry)}
	}
}

// clamp keeps a cursor within n items.
func clamp(cursor, n int) int {
	return max(0, min(cursor, n-1))
}

// external reports whether task links to another tool and has no ID, so an
// edit cannot set it.
func external(task *tmetric.Task) bool { return task != nil && task.ID == nil }
