// Package tmetric is a small client for the TMetric REST API v3
// (https://app.tmetric.com/api-docs), with only the calls the CLI uses.
package tmetric

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production API.
const DefaultBaseURL = "https://app.tmetric.com/api/v3"

// requestTimeout bounds a single call. Not http.DefaultClient: it has no
// timeout, so a server that accepts the connection and then says nothing
// would hang the command for good.
const requestTimeout = 30 * time.Second

// Client calls the API as the owner of a personal API token. Calls without a
// userId act on that owner, so the client never sends one.
type Client struct {
	baseURL    string
	token      string
	userAgent  string
	httpClient *http.Client
}

// NewClient returns a client for baseURL (DefaultBaseURL when empty). A nil
// httpClient gets one with a request timeout.
func NewClient(baseURL, token, userAgent string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
		userAgent:  userAgent,
		httpClient: httpClient,
	}
}

// Error is a response from the API with a non-2xx status.
type Error struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("the TMetric API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("the TMetric API returned HTTP %d: %s", e.StatusCode, e.Message)
}

// RequestError is a call that never got an answer from the API.
type RequestError struct{ Err error }

func (e *RequestError) Error() string { return "could not reach the TMetric API: " + e.Err.Error() }

func (e *RequestError) Unwrap() error { return e.Err }

// do sends one request and decodes the JSON answer into out (when not nil).
// It reports whether the API answered with a body at all.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) (bool, error) {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return false, fmt.Errorf("encoding the request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return false, fmt.Errorf("building the request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		request.Header.Set("User-Agent", c.userAgent)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return false, &RequestError{Err: err}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return false, &RequestError{Err: err}
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		apiErr := &Error{}
		// A body that is not the documented error shape still leaves the status.
		_ = json.Unmarshal(raw, apiErr)
		apiErr.StatusCode = response.StatusCode
		return false, apiErr
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false, nil
	}
	if out != nil {
		if err := json.Unmarshal(trimmed, out); err != nil {
			return true, fmt.Errorf("reading the answer to %s %s: %w", method, path, err)
		}
	}
	return true, nil
}

// User returns the owner of the token, with their workspaces and timezone.
func (c *Client) User(ctx context.Context) (*User, error) {
	user := &User{}
	if _, err := c.do(ctx, http.MethodGet, "/user", nil, nil, user); err != nil {
		return nil, err
	}
	return user, nil
}

// LatestEntry returns the most recent time entry, or nil when there is none.
// It is the running timer when its EndTime is nil.
func (c *Client) LatestEntry(ctx context.Context, accountID int64) (*TimeEntry, error) {
	entry := &TimeEntry{}
	found, err := c.do(ctx, http.MethodGet, accountPath(accountID, "/timeentries/latest"), nil, nil, entry)
	if err != nil || !found {
		return nil, err
	}
	return entry, nil
}

// Entries returns the time entries from the day of from to the day of to.
func (c *Client) Entries(ctx context.Context, accountID int64, from, to time.Time) ([]TimeEntry, error) {
	query := url.Values{
		"startDate": {from.Format(time.DateOnly)},
		"endDate":   {to.Format(time.DateOnly)},
	}
	var entries []TimeEntry
	_, err := c.do(ctx, http.MethodGet, accountPath(accountID, "/timeentries"), query, nil, &entries)
	return entries, err
}

// CreateEntry adds a time entry; an entry with a nil EndTime starts the timer.
// The API answers with every entry of the days the new one touches, so the
// caller picks the new one out by its start time.
func (c *Client) CreateEntry(ctx context.Context, accountID int64, entry TimeEntry) ([]TimeEntry, error) {
	entry.ID = 0
	var entries []TimeEntry
	_, err := c.do(ctx, http.MethodPost, accountPath(accountID, "/timeentries"), nil, entry, &entries)
	return entries, err
}

// UpdateEntry replaces a time entry with entry: send every field back, not
// only the ones that change. The API stores it under a new ID and, like
// CreateEntry, answers with every entry of the days involved; an entry
// shorter than a minute is dropped instead of stored.
func (c *Client) UpdateEntry(ctx context.Context, accountID int64, entry TimeEntry) ([]TimeEntry, error) {
	path := accountPath(accountID, "/timeentries/"+strconv.FormatInt(entry.ID, 10))
	entry.ID = 0
	var entries []TimeEntry
	_, err := c.do(ctx, http.MethodPut, path, nil, entry, &entries)
	return entries, err
}

// RecentEntries returns what the user worked on lately, without times.
func (c *Client) RecentEntries(ctx context.Context, accountID int64) ([]RecentEntry, error) {
	var recent []RecentEntry
	_, err := c.do(ctx, http.MethodGet, accountPath(accountID, "/timeentries/recent"), nil, nil, &recent)
	return recent, err
}

// DeleteEntry removes a time entry.
func (c *Client) DeleteEntry(ctx context.Context, accountID, entryID int64) error {
	_, err := c.do(ctx, http.MethodDelete, accountPath(accountID, "/timeentries/"+strconv.FormatInt(entryID, 10)), nil, nil, nil)
	return err
}

// Projects returns the projects the user can track time on.
func (c *Client) Projects(ctx context.Context, accountID int64) ([]Project, error) {
	var projects []Project
	_, err := c.do(ctx, http.MethodGet, accountPath(accountID, "/timeentries/projects"), nil, nil, &projects)
	return projects, err
}

// Tasks returns the tasks of a project. The API stops at 500 tasks (HTTP
// 206) and has no text search.
func (c *Client) Tasks(ctx context.Context, accountID, projectID int64) ([]Task, error) {
	query := url.Values{"ProjectList": {strconv.FormatInt(projectID, 10)}}
	var tasks []Task
	_, err := c.do(ctx, http.MethodGet, accountPath(accountID, "/tasks"), query, nil, &tasks)
	return tasks, err
}

func accountPath(accountID int64, rest string) string {
	return "/accounts/" + strconv.FormatInt(accountID, 10) + rest
}

// IsUnauthorized reports whether err is the API refusing the token.
func IsUnauthorized(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized
}
