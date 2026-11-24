package output

import "github.com/othercodes/tmetric-cli/internal/tmetric"

// Describe is the one-line summary of an entry: "note" · project / task.
func Describe(entry *tmetric.TimeEntry) string {
	text := `"` + Note(entry) + `"`
	if entry.Project != nil {
		text += " · " + entry.Project.Name
	}
	if entry.Task != nil && entry.Task.Name != "" {
		text += " / " + entry.Task.Name
	}
	return text
}

// Note is the entry's description, or a placeholder when it has none.
func Note(entry *tmetric.TimeEntry) string {
	if entry.Note == "" {
		return "(no description)"
	}
	return entry.Note
}

// Project is the entry's project name, or "-".
func Project(entry *tmetric.TimeEntry) string {
	if entry.Project == nil {
		return "-"
	}
	return entry.Project.Name
}

// Task is the entry's task name, or "-".
func Task(entry *tmetric.TimeEntry) string {
	if entry.Task == nil || entry.Task.Name == "" {
		return "-"
	}
	return entry.Task.Name
}
