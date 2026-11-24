package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/othercodes/tmetric-cli/internal/app"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

// The fields of the new timer form.
const (
	descField = iota
	projectField
	taskField
)

// listRows is how many matches a form list shows.
const listRows = 5

// form is the new timer overlay: a description, a project picked by typing
// part of its name, and an optional task of that project.
type form struct {
	field       int
	description textinput.Model
	project     textinput.Model
	projects    []tmetric.Project
	cursor      int // in the project matches
	tasks       []tmetric.Task
	tasksFor    int64
	taskCursor  int // 0 is "no task"
}

func newForm(projects []tmetric.Project, chosen *tmetric.Project) form {
	description := textinput.New()
	description.Prompt = ""
	description.Placeholder = "what are you working on?"
	description.CharLimit = 255
	project := textinput.New()
	project.Prompt = ""
	project.Placeholder = "type to filter"
	// A steady cursor: a blinking one redraws the screen twice a second.
	description.Cursor.SetMode(cursor.CursorStatic)
	project.Cursor.SetMode(cursor.CursorStatic)
	f := form{description: description, project: project, projects: projects}
	if chosen != nil {
		f.project.SetValue(chosen.Name)
	}
	return f
}

func (m model) openForm(chosen *tmetric.Project) (tea.Model, tea.Cmd) {
	m.overlay = formOverlay
	m.form = newForm(m.projects, chosen)
	return m, m.form.description.Focus()
}

// matches are the projects whose name holds what was typed.
func (f form) matches() []tmetric.Project {
	query := strings.ToLower(strings.TrimSpace(f.project.Value()))
	var found []tmetric.Project
	for _, p := range f.projects {
		if strings.Contains(strings.ToLower(p.Name), query) {
			found = append(found, p)
		}
	}
	return found
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
	if chosen := f.chosen(); chosen != nil && chosen.ID == projectID {
		f.tasks, f.tasksFor, f.taskCursor = tasks, projectID, 0
	}
	return f
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.form
	switch {
	case key.Matches(msg, keyBack):
		m.overlay = noOverlay
		return m, nil
	case key.Matches(msg, keyStart):
		m.overlay = noOverlay
		opts := f.options()
		return m.guard(m.start(opts), `"`+opts.Note+`"`)
	case key.Matches(msg, keyNext):
		return m.nextField()
	case msg.Type == tea.KeyUp || msg.Type == tea.KeyDown:
		delta := step(msg)
		switch f.field {
		case projectField:
			f.cursor = clamp(f.cursor+delta, len(f.matches()))
		case taskField:
			f.taskCursor = clamp(f.taskCursor+delta, len(f.tasks)+1)
		}
		m.form = f
		return m, nil
	}
	var cmd tea.Cmd
	switch f.field {
	case descField:
		f.description, cmd = f.description.Update(msg)
	case projectField:
		before := f.project.Value()
		f.project, cmd = f.project.Update(msg)
		if f.project.Value() != before {
			f.cursor, f.tasks, f.tasksFor = 0, nil, 0
		}
	}
	m.form = f
	return m, cmd
}

// nextField moves on, skipping the task field when there is no project, and
// loads the project's tasks when the task field comes up.
func (m model) nextField() (tea.Model, tea.Cmd) {
	f := m.form
	f.description.Blur()
	f.project.Blur()
	chosen := f.chosen()
	f.field = (f.field + 1) % 3
	if f.field == taskField && chosen == nil {
		f.field = descField
	}
	var cmd tea.Cmd
	switch f.field {
	case descField:
		cmd = f.description.Focus()
	case projectField:
		cmd = f.project.Focus()
	case taskField:
		if f.tasksFor != chosen.ID {
			cmd = m.loadTasks(chosen.ID)
		}
	}
	m.form = f
	return m, cmd
}

// options turn the form into a start request, by ID so names never clash.
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

// clamp keeps a cursor within n items.
func clamp(cursor, n int) int {
	return max(0, min(cursor, n-1))
}
