package output

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// failingWriter refuses every write, to reach the error paths.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// devNull is a character device, which is what a terminal looks like to
// IsTerminal, without needing a real terminal in CI.
func devNull(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func pipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	return r, w
}

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(devNull(t)) {
		t.Error("a character device should count as a terminal")
	}
	_, w := pipe(t)
	if IsTerminal(w) {
		t.Error("a pipe should not count as a terminal")
	}
	closed, _ := pipe(t)
	closed.Close()
	if IsTerminal(closed) {
		t.Error("a file that cannot be inspected should not count as a terminal")
	}
}

func TestColorEnabled(t *testing.T) {
	tty := devNull(t)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")
	if !ColorEnabled(tty) {
		t.Error("color should be on for a terminal")
	}
	t.Setenv("TERM", "dumb")
	if ColorEnabled(tty) {
		t.Error("TERM=dumb should turn color off")
	}
	t.Setenv("TERM", "xterm")
	t.Setenv("NO_COLOR", "1")
	if ColorEnabled(tty) {
		t.Error("NO_COLOR should turn color off")
	}
}

func TestStyler(t *testing.T) {
	on := Styler{enabled: true}
	if got := on.Bold("x"); got != "\x1b[1mx\x1b[0m" {
		t.Errorf("Bold = %q", got)
	}
	if got := on.Dim("x") + on.Red("x") + on.Green("x"); got != "\x1b[2mx\x1b[0m\x1b[31mx\x1b[0m\x1b[32mx\x1b[0m" {
		t.Errorf("Dim+Red+Green = %q", got)
	}
	if got := on.Bold(""); got != "" {
		t.Errorf("empty text should stay empty, got %q", got)
	}
	if got := (Styler{}).Bold("x"); got != "x" {
		t.Errorf("a disabled styler should not add escapes, got %q", got)
	}
	_, w := pipe(t)
	if NewStyler(w).enabled {
		t.Error("NewStyler should not color a pipe")
	}
}

func TestTablePipedIsTSVWithoutHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := Table(&buf, []string{"ID", "NAME"}, [][]string{{"1", "a b"}, {"2", "c"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "1\ta b\n2\tc\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTableOnTerminalAlignsColumns(t *testing.T) {
	var buf bytes.Buffer
	styler := Styler{enabled: true}
	rows := [][]string{{"1", styler.Green("running"), "x"}, {"200", "done", "y"}}
	if err := alignedTable(&buf, Styler{}, []string{"ID", "STATE", "NOTE"}, rows); err != nil {
		t.Fatal(err)
	}
	want := "ID   STATE    NOTE\n" +
		"1    \x1b[32mrunning\x1b[0m  x\n" +
		"200  done     y\n"
	if got := buf.String(); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestTableOnTerminalWithoutRows(t *testing.T) {
	var buf bytes.Buffer
	if err := alignedTable(&buf, Styler{}, []string{"ID"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "(none)\n" {
		t.Errorf("got %q", got)
	}
}

func TestTableDetectsTerminal(t *testing.T) {
	if err := Table(devNull(t), []string{"ID"}, [][]string{{"1"}}); err != nil {
		t.Fatal(err)
	}
}

func TestTableWriteErrors(t *testing.T) {
	rows := [][]string{{"1"}}
	if Table(failingWriter{}, []string{"ID"}, rows) == nil {
		t.Error("piped table should report the write error")
	}
	if alignedTable(failingWriter{}, Styler{}, []string{"ID"}, rows) == nil {
		t.Error("aligned table should report a header write error")
	}
	// The header fits, the row does not.
	limited := &limitedWriter{left: len("ID\n")}
	if alignedTable(limited, Styler{}, []string{"ID"}, rows) == nil {
		t.Error("aligned table should report a row write error")
	}
}

// limitedWriter accepts left bytes, then fails.
type limitedWriter struct{ left int }

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > w.left {
		return 0, errors.New("disk full")
	}
	w.left -= len(p)
	return len(p), nil
}

func TestKeyValues(t *testing.T) {
	var buf bytes.Buffer
	if err := KeyValues(&buf, [][2]string{{"name", "Ada"}, {"timezone", "Europe/Madrid"}}); err != nil {
		t.Fatal(err)
	}
	want := "name      Ada\ntimezone  Europe/Madrid\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if err := KeyValues(devNull(t), [][2]string{{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
}

func TestCSVQuoting(t *testing.T) {
	var buf bytes.Buffer
	if err := CSV(&buf, []string{"note"}, [][]string{{`a, "b"`}}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "note\n\"a, \"\"b\"\"\"\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCSVWriteErrors(t *testing.T) {
	// csv.Writer buffers 4 KiB, so only a bigger record reaches the writer.
	big := strings.Repeat("x", 5000)
	if CSV(failingWriter{}, []string{big}, nil) == nil {
		t.Error("CSV should report a header write error")
	}
	if CSV(failingWriter{}, []string{"h"}, [][]string{{big}}) == nil {
		t.Error("CSV should report a row write error")
	}
	if CSV(failingWriter{}, []string{"h"}, nil) == nil {
		t.Error("CSV should report a flush error")
	}
}

func TestJSONIndented(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "{\n  \"a\": 1\n}\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0h00m"},
		{59 * time.Second, "0h00m"},
		{65 * time.Minute, "1h05m"},
		{25*time.Hour + 30*time.Minute + 59*time.Second, "25h30m"},
		{-time.Minute, "0h00m"},
	}
	for _, tt := range tests {
		if got := Duration(tt.in); got != tt.want {
			t.Errorf("Duration(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
