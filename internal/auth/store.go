// Package auth keeps the TMetric API token: in the operating system's
// keyring, or in a file only the owner can read when there is no keyring.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const keyringService = "tmetric-cli"

// ErrNoCredentials reports that nobody has logged in against this API host.
var ErrNoCredentials = errors.New("not logged in")

// accountFor keys the stored token by API host, so a login against a test
// environment never stands in for the production one.
func accountFor(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return baseURL
	}
	return parsed.Host
}

// fallbackPath is where the token goes when the operating system has no
// keyring to put it in. It never guesses: a relative path would drop a live
// token into whatever directory the command was run from.
func fallbackPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding somewhere to keep the login: %w", err)
	}
	return filepath.Join(dir, "tmetric", "credentials.json"), nil
}

// Save stores the token in the OS keyring, falling back to a file only the
// owner can read when there is no keyring to reach.
func Save(baseURL, token string) error {
	account := accountFor(baseURL)
	if keyring.Set(keyringService, account, token) == nil {
		// An earlier fallback would go stale here: Load prefers the keyring,
		// so that copy would sit on disk and never be read again.
		return removeFromFile(account)
	}
	stored, err := readFile()
	if err != nil {
		return err
	}
	stored[account] = token
	return writeFile(stored)
}

// Load returns the token stored for the API host, or ErrNoCredentials.
func Load(baseURL string) (string, error) {
	account := accountFor(baseURL)
	token, err := keyring.Get(keyringService, account)
	if err == nil {
		return token, nil
	}
	stored, fileErr := readFile()
	if fileErr != nil {
		return "", fileErr
	}
	if token, ok := stored[account]; ok {
		return token, nil
	}
	// A keyring that is there but will not answer is not the same as nobody
	// having logged in, and logging in again would not fix it.
	if !errors.Is(err, keyring.ErrNotFound) && !errors.Is(err, keyring.ErrUnsupportedPlatform) {
		return "", fmt.Errorf("reading the login from the keyring: %w", err)
	}
	return "", ErrNoCredentials
}

// Clear forgets the token for the API host. Clearing something that was
// never there is not an error.
func Clear(baseURL string) error {
	account := accountFor(baseURL)
	_ = keyring.Delete(keyringService, account)
	return removeFromFile(account)
}

func removeFromFile(account string) error {
	stored, err := readFile()
	if err != nil {
		return err
	}
	if _, ok := stored[account]; !ok {
		return nil
	}
	delete(stored, account)
	return writeFile(stored)
}

// readFile tolerates having nowhere to read from: with no config directory
// there is no fallback file, which is not an error until something needs to
// be written.
func readFile() (map[string]string, error) {
	stored := map[string]string{}
	path, err := fallbackPath()
	if err != nil {
		return stored, nil
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return stored, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(body, &stored); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return stored, nil
}

func writeFile(stored map[string]string) error {
	path, err := fallbackPath()
	if err != nil {
		return err
	}
	// A map of strings always encodes.
	body, _ := json.Marshal(stored)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	// 0600: with no keyring this file is the only thing guarding a live token.
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
