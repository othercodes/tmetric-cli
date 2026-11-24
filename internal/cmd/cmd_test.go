package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/othercodes/tmetric-cli/internal/auth"
	"github.com/othercodes/tmetric-cli/internal/tmetric"
	"github.com/zalando/go-keyring"
)

// TestMain keeps the whole suite away from the developer's own keychain and
// token: the CLI reads both whenever they are there.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Unsetenv(tokenEnv)
	os.Unsetenv("TMETRIC_BASE_URL")
	os.Exit(m.Run())
}

// isolate gives a test an empty keyring and config directory.
func isolate(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

// runCLI executes the CLI in-process against args and returns captured
// stdout, stderr and the exit code.
func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	capture := func(target **os.File) func() string {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		old := *target
		*target = w
		done := make(chan string)
		go func() { out, _ := io.ReadAll(r); done <- string(out) }()
		return func() string { *target = old; w.Close(); return <-done }
	}
	stopOut, stopErr := capture(&os.Stdout), capture(&os.Stderr)
	code := run(context.Background(), args)
	return stopOut(), stopErr(), code
}

// withStdin feeds input to the command as a pipe.
func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, input)
	w.Close()
	swapStdin(t, r)
}

// withTerminal makes stdin look like a terminal and types typed into the
// hidden prompt.
func withTerminal(t *testing.T, typed string, err error) {
	t.Helper()
	oldIsTerminal, oldRead := stdinIsTerminal, readPassword
	stdinIsTerminal = func() bool { return true }
	readPassword = func() ([]byte, error) { return []byte(typed), err }
	t.Cleanup(func() { stdinIsTerminal, readPassword = oldIsTerminal, oldRead })
}

func swapStdin(t *testing.T, f *os.File) {
	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = old; f.Close() })
}

const userJSON = `{"id":7,"name":"Ada","email":"ada@example.com","activeAccountId":42,
	"timeZone":{"id":"Europe/Madrid"},"accounts":[{"id":42,"name":"Work","firstWeekDay":1}]}`

// fakeAPI serves GET /user: userJSON for the token "good", 401 otherwise.
func fakeAPI(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, userJSON)
	}))
	t.Cleanup(server.Close)
	t.Setenv("TMETRIC_BASE_URL", server.URL)
	return server.URL
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
			out, _, code := runCLI(t, tt.args...)
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

func TestInterruptedIsShort(t *testing.T) {
	if got := renderError(context.Canceled); got != "interrupted" {
		t.Errorf("renderError = %q", got)
	}
}

func TestLoginFromTerminal(t *testing.T) {
	isolate(t)
	base := fakeAPI(t)
	withTerminal(t, "  good \n", nil)
	_, stderr, code := runCLI(t, "login")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "TMetric API token: ") || !strings.Contains(stderr, "Signed in as Ada (ada@example.com).") {
		t.Errorf("stderr = %q", stderr)
	}
	if token, err := auth.Load(base); err != nil || token != "good" {
		t.Errorf("stored token = %q, %v", token, err)
	}
}

func TestLoginFromPipeWithJSON(t *testing.T) {
	isolate(t)
	base := fakeAPI(t)
	withStdin(t, "good")
	stdout, _, code := runCLI(t, "login", "--json")
	if code != 0 || !strings.Contains(stdout, `"email": "ada@example.com"`) {
		t.Fatalf("code = %d, stdout = %s", code, stdout)
	}
	if token, _ := auth.Load(base); token != "good" {
		t.Errorf("stored token = %q", token)
	}
}

func TestLoginRejectsBadTokensBeforeStoringThem(t *testing.T) {
	isolate(t)
	base := fakeAPI(t)
	withStdin(t, "typo\n")
	_, stderr, code := runCLI(t, "login")
	if code != 1 || !strings.Contains(stderr, "checking the token: the TMetric API returned HTTP 401") ||
		!strings.Contains(stderr, "run `tmetric login`") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if _, err := auth.Load(base); !errors.Is(err, auth.ErrNoCredentials) {
		t.Errorf("a rejected token was stored: err = %v", err)
	}
}

func TestLoginInputErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
		code  int
		want  string
	}{
		{"empty pipe", func(t *testing.T) { withStdin(t, "") }, 2, "no token on stdin"},
		{"blank token", func(t *testing.T) { withStdin(t, "   \n") }, 2, "the token is empty"},
		{"terminal read fails", func(t *testing.T) { withTerminal(t, "", errors.New("no tty")) }, 1, "reading the token: no tty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			fakeAPI(t)
			tt.setup(t)
			_, stderr, code := runCLI(t, "login")
			if code != tt.code || !strings.Contains(stderr, tt.want) {
				t.Errorf("code = %d, stderr = %q", code, stderr)
			}
		})
	}
}

func TestLoginWithNowhereToStore(t *testing.T) {
	isolate(t)
	fakeAPI(t)
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	withStdin(t, "good\n")
	if _, stderr, code := runCLI(t, "login"); code != 1 || !strings.Contains(stderr, "finding somewhere to keep the login") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestLoginWarnsAboutEnvToken(t *testing.T) {
	isolate(t)
	fakeAPI(t)
	t.Setenv(tokenEnv, "other")
	withStdin(t, "good\n")
	if _, stderr, _ := runCLI(t, "login"); !strings.Contains(stderr, tokenEnv+" is set and takes precedence") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestLogout(t *testing.T) {
	isolate(t)
	base := fakeAPI(t)
	if err := auth.Save(base, "good"); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runCLI(t, "logout"); code != 0 || !strings.Contains(stderr, "Signed out.") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if _, err := auth.Load(base); !errors.Is(err, auth.ErrNoCredentials) {
		t.Errorf("token still stored: err = %v", err)
	}
	if _, stderr, code := runCLI(t, "logout"); code != 0 || !strings.Contains(stderr, "Not signed in.") {
		t.Errorf("second logout: code = %d, stderr = %q", code, stderr)
	}
}

func TestLogoutErrors(t *testing.T) {
	isolate(t)
	keyring.MockInitWithError(errors.New("the keyring is locked"))
	if _, stderr, code := runCLI(t, "logout"); code != 1 || !strings.Contains(stderr, "the keyring is locked") {
		t.Errorf("locked keyring: code = %d, stderr = %q", code, stderr)
	}

	// The keyring answers, but the fallback file cannot be cleaned.
	isolate(t)
	base := fakeAPI(t)
	if err := auth.Save(base, "good"); err != nil {
		t.Fatal(err)
	}
	dir, _ := os.UserConfigDir()
	if err := os.MkdirAll(filepath.Join(dir, "tmetric"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmetric", "credentials.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runCLI(t, "logout"); code != 1 {
		t.Errorf("corrupt fallback: code = %d", code)
	}
}

func TestWhoami(t *testing.T) {
	isolate(t)
	base := fakeAPI(t)
	if err := auth.Save(base, "good"); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLI(t, "whoami")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	for _, want := range []string{"Ada", "ada@example.com", "Work (42)", "Europe/Madrid"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}
	if !strings.Contains(stderr, "via the login stored by `tmetric login`") {
		t.Errorf("stderr = %q", stderr)
	}
	stdout, _, _ = runCLI(t, "whoami", "--json", "--base-url", base)
	if !strings.Contains(stdout, `"activeAccountId": 42`) {
		t.Errorf("json = %s", stdout)
	}
}

func TestWhoamiEnvTokenWins(t *testing.T) {
	isolate(t)
	base := fakeAPI(t)
	if err := auth.Save(base, "stale"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(tokenEnv, "good")
	if _, stderr, code := runCLI(t, "whoami"); code != 0 || !strings.Contains(stderr, "via "+tokenEnv) {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestWhoamiErrors(t *testing.T) {
	isolate(t)
	fakeAPI(t)
	if _, stderr, code := runCLI(t, "whoami"); code != 1 || !strings.Contains(stderr, "not signed in") {
		t.Errorf("no login: code = %d, stderr = %q", code, stderr)
	}
	t.Setenv(tokenEnv, "bad")
	if _, stderr, code := runCLI(t, "whoami"); code != 1 || !strings.Contains(stderr, "HTTP 401") {
		t.Errorf("bad token: code = %d, stderr = %q", code, stderr)
	}
	t.Setenv(tokenEnv, "")
	keyring.MockInitWithError(errors.New("the keyring is locked"))
	if _, stderr, code := runCLI(t, "whoami"); code != 1 || !strings.Contains(stderr, "the keyring is locked") {
		t.Errorf("locked keyring: code = %d, stderr = %q", code, stderr)
	}
}

func TestWhoamiCannotWrite(t *testing.T) {
	isolate(t)
	fakeAPI(t)
	t.Setenv(tokenEnv, "good")
	readOnly, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = readOnly
	defer func() { os.Stdout = old; readOnly.Close() }()
	if code := run(context.Background(), []string{"whoami"}); code != 1 {
		t.Errorf("code = %d, want 1 when stdout refuses writes", code)
	}
}

func TestActiveAccountNameFallsBackToID(t *testing.T) {
	user := &tmetric.User{ActiveAccountID: 99, Accounts: []tmetric.Account{{ID: 1, Name: "Other"}}}
	if got := activeAccountName(user); got != "99" {
		t.Errorf("got %q", got)
	}
}

func TestReadPasswordNeedsATerminal(t *testing.T) {
	// The real reader, not the swapped one: a pipe is no terminal, so it fails
	// instead of hanging or echoing.
	withStdin(t, "secret\n")
	if _, err := readPassword(); err == nil {
		t.Error("readPassword should fail without a terminal")
	}
}

func TestDevNullIsNoTerminal(t *testing.T) {
	// The real check, not the swapped one: /dev/null is a character device, but
	// login must read it as an empty pipe, not prompt on it.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	swapStdin(t, f)
	if stdinIsTerminal() {
		t.Error("/dev/null should not count as a terminal")
	}
	isolate(t)
	fakeAPI(t)
	if _, stderr, code := runCLI(t, "login"); code != 2 || !strings.Contains(stderr, "no token on stdin") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}
