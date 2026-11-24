package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

const base = "https://app.tmetric.com/api/v3"

// isolate gives each test an empty in-memory keyring and its own config
// directory, so nothing touches the developer's real ones.
func isolate(t *testing.T) string {
	t.Helper()
	keyring.MockInit()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path, err := fallbackPath()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func noKeyring() { keyring.MockInitWithError(keyring.ErrUnsupportedPlatform) }

func noConfigDir(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
}

func writeRaw(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestKeyringRoundTrip(t *testing.T) {
	isolate(t)
	if _, err := Load(base); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Load before Save: err = %v", err)
	}
	if err := Save(base, "secret"); err != nil {
		t.Fatal(err)
	}
	if token, err := Load(base); err != nil || token != "secret" {
		t.Fatalf("Load = %q, %v", token, err)
	}
	if err := Clear(base); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(base); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Load after Clear: err = %v", err)
	}
}

func TestTokensAreKeyedByHost(t *testing.T) {
	isolate(t)
	if err := Save("https://staging.example.com/api/v3", "staging"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(base); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("a staging login must not stand in for production: err = %v", err)
	}
	if got := accountFor("not a url"); got != "not a url" {
		t.Errorf("accountFor without a host = %q", got)
	}
}

func TestFileFallbackWithoutKeyring(t *testing.T) {
	path := isolate(t)
	noKeyring()
	if err := Save(base, "secret"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	if token, err := Load(base); err != nil || token != "secret" {
		t.Fatalf("Load = %q, %v", token, err)
	}
	if err := Clear(base); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(base); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Load after Clear: err = %v", err)
	}
}

func TestKeyringSaveRemovesStaleFile(t *testing.T) {
	isolate(t)
	noKeyring()
	if err := Save(base, "old"); err != nil {
		t.Fatal(err)
	}
	keyring.MockInit()
	if err := Save(base, "new"); err != nil {
		t.Fatal(err)
	}
	stored, err := readFile()
	if err != nil || len(stored) != 0 {
		t.Errorf("file still holds %v (err %v)", stored, err)
	}
}

func TestLockedKeyringIsNotReportedAsLoggedOut(t *testing.T) {
	isolate(t)
	keyring.MockInitWithError(errors.New("the keyring is locked"))
	_, err := Load(base)
	if err == nil || errors.Is(err, ErrNoCredentials) {
		t.Errorf("err = %v, want the keyring error", err)
	}
}

func TestNowhereSafeToStore(t *testing.T) {
	isolate(t)
	noKeyring()
	noConfigDir(t)
	if err := Save(base, "secret"); err == nil {
		t.Error("Save should fail with no keyring and no config directory")
	}
	// With nowhere to read from, nobody is logged in.
	if _, err := Load(base); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("Load: err = %v", err)
	}
}

func TestCorruptFile(t *testing.T) {
	path := isolate(t)
	writeRaw(t, path, "{not json")
	noKeyring()
	if _, err := Load(base); err == nil {
		t.Error("Load should report a corrupt file")
	}
	if err := Save(base, "secret"); err == nil {
		t.Error("Save should report a corrupt file")
	}
	keyring.MockInit()
	if err := Save(base, "secret"); err == nil {
		t.Error("Save to the keyring should still report the corrupt fallback it cannot clean")
	}
	if err := Clear(base); err == nil {
		t.Error("Clear should report a corrupt file")
	}
}

func TestUnreadableFile(t *testing.T) {
	path := isolate(t)
	// A directory where the file should be cannot be read as one.
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := readFile(); err == nil {
		t.Error("readFile should fail on a directory")
	}
}

func TestUnwritableFile(t *testing.T) {
	path := isolate(t)
	noKeyring()
	// The config directory is a file, so the tmetric folder cannot be made.
	writeRaw(t, filepath.Dir(filepath.Dir(path)), "")
	if err := writeFile(map[string]string{"h": "t"}); err == nil {
		t.Error("writeFile should fail when the folder cannot be created")
	}

	path = isolate(t)
	noKeyring()
	// The folder exists but holds a directory where the file goes, and the
	// listing is empty JSON so the read passes.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(map[string]string{"h": "t"}); err == nil {
		t.Error("writeFile should fail when the file is a directory")
	}
}
