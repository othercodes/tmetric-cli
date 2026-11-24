package output

import (
	"testing"

	"github.com/othercodes/tmetric-cli/internal/tmetric"
)

func TestEntryText(t *testing.T) {
	full := &tmetric.TimeEntry{Note: "review", Project: &tmetric.Project{Name: "CLI"}, Task: &tmetric.Task{Name: "Tests"}}
	if got := Describe(full); got != `"review" · CLI / Tests` {
		t.Errorf("Describe = %q", got)
	}
	if Note(full) != "review" || Project(full) != "CLI" || Task(full) != "Tests" {
		t.Errorf("fields = %q %q %q", Note(full), Project(full), Task(full))
	}
	bare := &tmetric.TimeEntry{Task: &tmetric.Task{}}
	if got := Describe(bare); got != `"(no description)"` {
		t.Errorf("Describe = %q", got)
	}
	if Project(bare) != "-" || Task(bare) != "-" || Task(&tmetric.TimeEntry{}) != "-" {
		t.Errorf("placeholders = %q %q", Project(bare), Task(bare))
	}
}
