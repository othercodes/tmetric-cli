package tmetric

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// request is what the fake API saw.
type request struct {
	method, path, query, body string
	header                    http.Header
}

// fakeAPI answers every request with status and body, and records it.
func fakeAPI(t *testing.T, status int, body string) (*Client, *request) {
	t.Helper()
	seen := &request{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*seen = request{r.Method, r.URL.Path, r.URL.RawQuery, string(raw), r.Header.Clone()}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return NewClient(server.URL+"/api/v3/", "secret", "tmetric-cli/test", nil), seen
}

func TestNewClientDefaults(t *testing.T) {
	c := NewClient("", "t", "", nil)
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.httpClient.Timeout != requestTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, requestTimeout)
	}
}

func TestUser(t *testing.T) {
	c, seen := fakeAPI(t, 200, `{"id":7,"name":"Ada","email":"ada@example.com","activeAccountId":42,
		"timeZone":{"id":"Europe/Madrid"},"accounts":[{"id":42,"name":"Work","firstWeekDay":1}]}`)
	user, err := c.User(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if seen.method != "GET" || seen.path != "/api/v3/user" {
		t.Errorf("request = %s %s", seen.method, seen.path)
	}
	if got := seen.header.Get("Authorization"); got != "Bearer secret" {
		t.Errorf("Authorization = %q", got)
	}
	if seen.header.Get("Accept") != "application/json" || seen.header.Get("User-Agent") != "tmetric-cli/test" {
		t.Errorf("headers = %v", seen.header)
	}
	if seen.header.Get("Content-Type") != "" {
		t.Error("a request without a body should not set Content-Type")
	}
	if user.ID != 7 || user.ActiveAccountID != 42 || user.TimeZone.ID != "Europe/Madrid" ||
		len(user.Accounts) != 1 || user.Accounts[0].FirstWeekDay != time.Monday {
		t.Errorf("user = %+v", user)
	}
}

func TestUserError(t *testing.T) {
	c, _ := fakeAPI(t, 401, ``)
	if _, err := c.User(context.Background()); !IsUnauthorized(err) {
		t.Errorf("err = %v, want a 401", err)
	}
}

func TestLatestEntry(t *testing.T) {
	c, seen := fakeAPI(t, 200, `{"id":1,"startTime":"2026-11-23T09:00:00.123","endTime":null,"note":"standup"}`)
	entry, err := c.LatestEntry(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if seen.path != "/api/v3/accounts/42/timeentries/latest" || seen.query != "" {
		t.Errorf("request = %s?%s", seen.path, seen.query)
	}
	if entry.ID != 1 || entry.EndTime != nil || entry.StartTime.String() != "2026-11-23T09:00:00" {
		t.Errorf("entry = %+v", entry)
	}
}

func TestLatestEntryWithoutEntries(t *testing.T) {
	for _, tt := range []struct {
		status int
		body   string
	}{{200, "null"}, {200, ""}, {204, ""}} {
		c, _ := fakeAPI(t, tt.status, tt.body)
		entry, err := c.LatestEntry(context.Background(), 42)
		if err != nil || entry != nil {
			t.Errorf("HTTP %d %q: entry = %+v, err = %v", tt.status, tt.body, entry, err)
		}
	}
}

func TestLatestEntryError(t *testing.T) {
	c, _ := fakeAPI(t, 500, ``)
	if _, err := c.LatestEntry(context.Background(), 42); err == nil {
		t.Error("want an error")
	}
}

func TestEntries(t *testing.T) {
	c, seen := fakeAPI(t, 200, `[{"id":1,"startTime":"2026-11-23T09:00:00","endTime":"2026-11-23T10:30:00",
		"project":{"id":5,"name":"CLI","client":{"name":"Acme"}},"task":{"id":9,"name":"Tests"},"tags":[{"id":3,"name":"dev"}]}]`)
	loc := time.FixedZone("test", 3600)
	entries, err := c.Entries(context.Background(), 42,
		time.Date(2026, 11, 23, 0, 0, 0, 0, loc), time.Date(2026, 11, 29, 23, 0, 0, 0, loc))
	if err != nil {
		t.Fatal(err)
	}
	if seen.path != "/api/v3/accounts/42/timeentries" || seen.query != "endDate=2026-11-29&startDate=2026-11-23" {
		t.Errorf("request = %s?%s", seen.path, seen.query)
	}
	if len(entries) != 1 || entries[0].Project.Client.Name != "Acme" || *entries[0].Task.ID != 9 ||
		entries[0].EndTime.String() != "2026-11-23T10:30:00" || len(entries[0].Tags) != 1 {
		t.Errorf("entries = %+v", entries)
	}
}

func TestCreateEntry(t *testing.T) {
	c, seen := fakeAPI(t, 200, `[{"id":10,"startTime":"2026-11-23T09:00:00","endTime":null}]`)
	taskID := int64(9)
	entries, err := c.CreateEntry(context.Background(), 42, TimeEntry{
		ID:        99, // never sent: the API assigns it
		StartTime: NewLocalTime(time.Date(2026, 11, 23, 9, 0, 0, 500, time.UTC)),
		Note:      "standup",
		Project:   &Project{ID: 5},
		Task:      &Task{ID: &taskID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen.method != "POST" || seen.path != "/api/v3/accounts/42/timeentries" {
		t.Errorf("request = %s %s", seen.method, seen.path)
	}
	if seen.header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", seen.header.Get("Content-Type"))
	}
	want := `{"startTime":"2026-11-23T09:00:00","endTime":null,"note":"standup","project":{"id":5},"task":{"id":9}}`
	if seen.body != want {
		t.Errorf("body =\n%s\nwant\n%s", seen.body, want)
	}
	if len(entries) != 1 || entries[0].ID != 10 {
		t.Errorf("entries = %+v", entries)
	}
}

func TestUpdateEntrySendsEverythingBack(t *testing.T) {
	// The API answers with every entry of the day, the updated one among them.
	c, seen := fakeAPI(t, 200, `[{"id":9,"startTime":"2026-11-23T08:00:00","endTime":"2026-11-23T09:00:00"},
		{"id":10,"startTime":"2026-11-23T09:00:00","endTime":"2026-11-23T10:00:00"}]`)
	var entry TimeEntry
	// An external task and tags must survive the round trip untouched.
	original := `{"id":10,"startTime":"2026-11-23T09:00:00","endTime":null,"note":"n",
		"project":{"id":5,"name":"CLI"},"task":{"name":"TMET-1","externalLink":{"link":"https://x"},"integration":{"id":1}},
		"tags":[{"id":3,"name":"dev"}]}`
	if err := json.Unmarshal([]byte(original), &entry); err != nil {
		t.Fatal(err)
	}
	end := NewLocalTime(time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC))
	entry.EndTime = &end
	updated, err := c.UpdateEntry(context.Background(), 42, entry)
	if err != nil {
		t.Fatal(err)
	}
	if seen.method != "PUT" || seen.path != "/api/v3/accounts/42/timeentries/10" {
		t.Errorf("request = %s %s", seen.method, seen.path)
	}
	want := `{"startTime":"2026-11-23T09:00:00","endTime":"2026-11-23T10:00:00","note":"n","project":{"id":5,"name":"CLI"},` +
		`"task":{"name":"TMET-1","externalLink":{"link":"https://x"},"integration":{"id":1}},"tags":[{"id":3,"name":"dev"}]}`
	if seen.body != want {
		t.Errorf("body =\n%s\nwant\n%s", seen.body, want)
	}
	if len(updated) != 2 || updated[1].EndTime == nil {
		t.Errorf("updated = %+v", updated)
	}
}

func TestUpdateEntryError(t *testing.T) {
	c, _ := fakeAPI(t, 403, `{"statusCode":403,"message":"Restricted workspace access."}`)
	_, err := c.UpdateEntry(context.Background(), 42, TimeEntry{ID: 1})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 403 {
		t.Fatalf("err = %v", err)
	}
	if got := err.Error(); got != "the TMetric API returned HTTP 403: Restricted workspace access." {
		t.Errorf("message = %q", got)
	}
}

func TestProjects(t *testing.T) {
	c, seen := fakeAPI(t, 200, `[{"id":5,"name":"CLI"}]`)
	projects, err := c.Projects(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if seen.path != "/api/v3/accounts/42/timeentries/projects" {
		t.Errorf("path = %s", seen.path)
	}
	if len(projects) != 1 || projects[0].Name != "CLI" {
		t.Errorf("projects = %+v", projects)
	}
}

func TestTasks(t *testing.T) {
	// 206 is a partial list (the first 500), still a success.
	c, seen := fakeAPI(t, 206, `[{"id":9,"name":"Tests","project":{"id":5},"isCompleted":true}]`)
	tasks, err := c.Tasks(context.Background(), 42, 5)
	if err != nil {
		t.Fatal(err)
	}
	if seen.path != "/api/v3/accounts/42/tasks" || seen.query != "ProjectList=5" {
		t.Errorf("request = %s?%s", seen.path, seen.query)
	}
	if len(tasks) != 1 || !tasks[0].IsCompleted || tasks[0].Project.ID != 5 {
		t.Errorf("tasks = %+v", tasks)
	}
}

func TestErrorWithoutMessage(t *testing.T) {
	c, _ := fakeAPI(t, 502, `<html>Bad gateway</html>`)
	_, err := c.Projects(context.Background(), 42)
	if err == nil || err.Error() != "the TMetric API returned HTTP 502" {
		t.Errorf("err = %v", err)
	}
	if IsUnauthorized(err) {
		t.Error("a 502 is not a 401")
	}
}

func TestUnreadableAnswer(t *testing.T) {
	c, _ := fakeAPI(t, 200, `{"id":"not a number"}`)
	if _, err := c.User(context.Background()); err == nil || !strings.Contains(err.Error(), "reading the answer to GET /user") {
		t.Errorf("err = %v", err)
	}
}

func TestUnreachableAPI(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "t", "", nil)
	_, err := c.User(context.Background())
	var reqErr *RequestError
	if !errors.As(err, &reqErr) || !strings.HasPrefix(err.Error(), "could not reach the TMetric API: ") {
		t.Fatalf("err = %v", err)
	}
	if errors.Unwrap(reqErr) == nil {
		t.Error("RequestError should unwrap to the cause")
	}
}

func TestBodyCutShort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "{")
	}))
	t.Cleanup(server.Close)
	_, err := NewClient(server.URL, "t", "", nil).User(context.Background())
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		t.Errorf("err = %v, want a RequestError", err)
	}
}

func TestBadRequests(t *testing.T) {
	c := NewClient("http://example.com", "t", "", nil)
	// A value JSON cannot encode.
	if _, err := c.do(context.Background(), "POST", "/x", nil, func() {}, nil); err == nil || !strings.HasPrefix(err.Error(), "encoding the request") {
		t.Errorf("err = %v", err)
	}
	// A method net/http refuses.
	if _, err := c.do(context.Background(), "BAD METHOD", "/x", nil, nil, nil); err == nil || !strings.HasPrefix(err.Error(), "building the request") {
		t.Errorf("err = %v", err)
	}
}

func TestDoWithoutOut(t *testing.T) {
	c, _ := fakeAPI(t, 200, `{"ignored":true}`)
	found, err := c.do(context.Background(), "DELETE", "/x", nil, nil, nil)
	if err != nil || !found {
		t.Errorf("found = %v, err = %v", found, err)
	}
}
