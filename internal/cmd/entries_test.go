package cmd

import (
	"context"
	"os"
	"testing"
)

const (
	entriesRoute  = "GET /accounts/42/timeentries"
	projectsRoute = "GET /accounts/42/timeentries/projects"
	twoEntries    = `[{"id":1,"startTime":"2020-01-06T09:00:00","endTime":"2020-01-06T10:30:00","note":"review",
		"project":{"id":5,"name":"CLI"},"task":{"id":9,"name":"Tests"}},
		{"id":2,"startTime":"2020-01-06T11:00:00","endTime":null}]`
)

func TestToday(t *testing.T) {
	timerAPI(t, map[string]string{entriesRoute: twoEntries})
	// In a pipe: tab-separated rows, the total on stderr.
	expect(t, []string{"today"}, 0,
		[]string{"09:00\t10:30\t1h30m\tCLI\tTests\treview\n", "11:00\trunning\t"},
		[]string{"total "})
	expect(t, []string{"today", "--format", "csv"}, 0,
		[]string{"START,END,DURATION,PROJECT,TASK,DESCRIPTION\n09:00,10:30,1h30m,CLI,Tests,review\n"}, []string{"total "})
	expect(t, []string{"today", "--json"}, 0, []string{`"total_seconds"`, `"note": "review"`, `"from": "`}, nil)
	expect(t, []string{"today", "--format", "xml"}, 2, nil, []string{`unknown --format "xml"`})
}

func TestWeekHasADateColumn(t *testing.T) {
	timerAPI(t, map[string]string{entriesRoute: twoEntries})
	expect(t, []string{"week"}, 0, []string{"Mon 06\t09:00\t10:30\t1h30m\tCLI\tTests\treview\n"}, nil)
	expect(t, []string{"week", "--format", "csv"}, 0, []string{"DATE,START,END"}, nil)
}

func TestReportWithoutEntries(t *testing.T) {
	// "null" leaves the list nil; --json still prints [] for scripts.
	timerAPI(t, map[string]string{entriesRoute: "null"})
	expect(t, []string{"week", "--json"}, 0, []string{`"entries": []`, `"total_seconds": 0`}, nil)
	expect(t, []string{"today"}, 0, nil, []string{"total 0h00m"})
}

func TestReportOnTerminal(t *testing.T) {
	timerAPI(t, map[string]string{entriesRoute: twoEntries})
	// /dev/null is a character device, which is what the table looks for; what
	// it prints is checked in the output package, this only takes the branch.
	tty, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = tty
	defer func() { os.Stdout = old; tty.Close() }()
	if code := run(context.Background(), []string{"today"}); code != 0 {
		t.Errorf("exit code = %d", code)
	}
}

func TestReportErrors(t *testing.T) {
	timerAPI(t, map[string]string{entriesRoute: "HTTP 500"})
	expect(t, []string{"today"}, 1, nil, []string{"HTTP 500"})
	t.Setenv(tokenEnv, "")
	expect(t, []string{"week"}, 1, nil, []string{"not signed in"})
}

func TestProjectsList(t *testing.T) {
	timerAPI(t, map[string]string{projectsRoute: `[{"id":5,"name":"CLI","client":{"name":"Acme"}},{"id":6,"name":"Docs"}]`})
	expect(t, []string{"projects", "list"}, 0, []string{"5\tCLI\tAcme\n6\tDocs\t-\n"}, nil)
	expect(t, []string{"projects", "list", "--format", "csv"}, 0, []string{"ID,NAME,CLIENT\n5,CLI,Acme\n"}, nil)
	expect(t, []string{"projects", "list", "--json"}, 0, []string{`"name": "Acme"`}, nil)

	timerAPI(t, map[string]string{projectsRoute: "null"})
	expect(t, []string{"projects", "list", "--json"}, 0, []string{"[]"}, nil)

	timerAPI(t, map[string]string{projectsRoute: "HTTP 500"})
	expect(t, []string{"projects", "list"}, 1, nil, []string{"HTTP 500"})
	t.Setenv(tokenEnv, "")
	expect(t, []string{"projects", "list"}, 1, nil, []string{"not signed in"})
}
