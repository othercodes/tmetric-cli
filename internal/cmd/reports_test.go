package cmd

import (
	"context"
	"os"
	"testing"
)

const (
	reportRoute  = "GET /accounts/42/reports/projects"
	balanceRoute = "GET /accounts/42/balance"
	reportBody   = `[{"project":{"id":6,"name":"Docs"},"totalSeconds":1800,"billableSeconds":0,"billableAmount":20,"billableCurrency":"USD"},
		{"project":{"id":5,"name":"CLI","client":{"name":"Acme"}},"totalSeconds":5400,"billableSeconds":3600,
		 "billableAmount":100,"billableCurrency":"EUR","costAmount":40,"costCurrency":"EUR"},
		{"totalSeconds":1800}]`
)

func TestReportsProjects(t *testing.T) {
	timerAPI(t, map[string]string{reportRoute: reportBody})
	// Biggest first; money per currency, never mixed; no project, no amounts.
	expect(t, []string{"reports", "projects"}, 0, []string{
		"CLI\tAcme\t1h30m\t1h00m\t60%\t100.00 EUR\t40.00 EUR\n",
		"Docs\t-\t0h30m\t0h00m\t20%\t20.00 USD\t-\n",
		"(no project)\t-\t0h30m\t0h00m\t20%\t-\t-\n",
	}, []string{"total 2h30m · amount 100.00 EUR · amount 20.00 USD · cost 40.00 EUR"})
	expect(t, []string{"reports", "projects", "--format", "csv"}, 0, []string{"PROJECT,CLIENT,TIME,BILLABLE,SHARE,AMOUNT,COST\nCLI,Acme"}, nil)
	expect(t, []string{"reports", "projects", "--json", "--range", "last-month"}, 0,
		[]string{`"total_seconds": 9000`, `"billableCurrency": "EUR"`, `"from": "`}, nil)
	expect(t, []string{"reports", "projects", "--from", "2026-11-01", "--to", "2026-11-15"}, 0, nil, []string{"2026-11-01 to 2026-11-15"})
	expect(t, []string{"reports", "projects", "--format", "xml"}, 2, nil, []string{`unknown --format "xml"`})
}

func TestReportsProjectsEmpty(t *testing.T) {
	timerAPI(t, map[string]string{reportRoute: "null"})
	expect(t, []string{"reports", "projects", "--json"}, 0, []string{`"projects": []`, `"total_seconds": 0`}, nil)
	// A project with no time has no share to show.
	timerAPI(t, map[string]string{reportRoute: `[{"project":{"id":1,"name":"Idle"},"totalSeconds":0}]`})
	expect(t, []string{"reports", "projects"}, 0, []string{"Idle\t-\t0h00m\t0h00m\t-\t-\t-\n"}, []string{"total 0h00m"})
}

func TestReportsProjectsOnTerminal(t *testing.T) {
	timerAPI(t, map[string]string{reportRoute: reportBody})
	tty, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = tty
	defer func() { os.Stdout = old; tty.Close() }()
	if code := run(context.Background(), []string{"reports", "projects"}); code != 0 {
		t.Errorf("exit code = %d", code)
	}
}

func TestReportsProjectsErrors(t *testing.T) {
	timerAPI(t, map[string]string{reportRoute: "HTTP 500"})
	expect(t, []string{"reports", "projects"}, 1, nil, []string{"HTTP 500"})
	expect(t, []string{"reports", "projects", "--range", "year"}, 2, nil, []string{`unknown range "year"`})
	expect(t, []string{"reports", "projects", "--from", "2026-11-01"}, 2, nil, []string{"pass both --from and --to"})
	expect(t, []string{"reports", "projects", "--from", "x", "--to", "2026-11-01"}, 2, nil, []string{`"x" is not a date`})
	expect(t, []string{"reports", "projects", "--from", "2026-11-01", "--to", "y"}, 2, nil, []string{`"y" is not a date`})
	expect(t, []string{"reports", "projects", "--from", "2026-11-15", "--to", "2026-11-01"}, 2, nil, []string{"--to (2026-11-01) is before --from (2026-11-15)"})
	t.Setenv(tokenEnv, "")
	expect(t, []string{"reports", "projects"}, 1, nil, []string{"not signed in"})
}

func TestReportsBalance(t *testing.T) {
	timerAPI(t, map[string]string{balanceRoute: `{"today":{"requiredSeconds":28800,"actualSeconds":14400},
		"week":{"requiredSeconds":144000,"actualSeconds":148500},"month":{"requiredSeconds":576000,"actualSeconds":576000}}`})
	expect(t, []string{"reports", "balance"}, 0, []string{
		"today\t4h00m\t8h00m\t-4h00m\n", "week\t41h15m\t40h00m\t+1h15m\n", "month\t160h00m\t160h00m\t0h00m\n",
	}, nil)
	expect(t, []string{"reports", "balance", "--json"}, 0, []string{`"requiredSeconds": 144000`}, nil)
	timerAPI(t, map[string]string{balanceRoute: `{"today":{"actualSeconds":10560},"week":{"actualSeconds":64200},"month":{"actualSeconds":477240}}`})
	expect(t, []string{"reports", "balance"}, 0, []string{"today\t2h56m\t-\t-\n", "month\t132h34m\t-\t-\n"},
		[]string{"no work schedule in this workspace"})
	expect(t, []string{"reports", "balance", "--format", "xml"}, 2, nil, []string{`unknown --format "xml"`})
	timerAPI(t, map[string]string{balanceRoute: "HTTP 500"})
	expect(t, []string{"reports", "balance"}, 1, nil, []string{"HTTP 500"})
	t.Setenv(tokenEnv, "")
	expect(t, []string{"reports", "balance"}, 1, nil, []string{"not signed in"})
}
