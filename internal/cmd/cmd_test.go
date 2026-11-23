package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// runCLI executes the CLI in-process against args and returns captured
// stdout and the exit code.
func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	code := run(context.Background(), args)
	os.Stdout = old
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading stdout: %v", err)
	}
	return string(out), code
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		out  string
	}{
		{"version", []string{"--version"}, 0, "tmetric version dev"},
		{"unknown flag is a usage error", []string{"--nope"}, 2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, code := runCLI(t, tt.args...)
			if code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if !strings.Contains(out, tt.out) {
				t.Errorf("stdout = %q, want it to contain %q", out, tt.out)
			}
		})
	}
}

func TestExecuteReadsOSArgs(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"tmetric", "--version"}
	if code := Execute(context.Background()); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestExitCodeForRuntimeError(t *testing.T) {
	if code := exitCode(errors.New("the API returned HTTP 500")); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}
