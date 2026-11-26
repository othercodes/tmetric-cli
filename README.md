# tmetric

[![Test](https://github.com/othercodes/tmetric-cli/actions/workflows/test.yml/badge.svg)](https://github.com/othercodes/tmetric-cli/actions/workflows/test.yml)
[![Quality gate status](https://sonarcloud.io/api/project_badges/measure?project=othercodes_tmetric-cli&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=othercodes_tmetric-cli)

An unofficial command-line interface and terminal UI for [TMetric](https://tmetric.com) time tracking.
Start and stop timers, review and fix your entries, and see where your week went, without leaving the terminal.

> Not affiliated with TMetric. It talks to the public [TMetric REST API v3](https://app.tmetric.com/api-docs).

## Install

With Go 1.26 or later:

```sh
go install github.com/othercodes/tmetric-cli/cmd/tmetric@latest
```

This installs `tmetric` into `$(go env GOPATH)/bin`; make sure that folder is on your `PATH`.

## Usage

### Sign in

Get a personal API token in TMetric: **My Profile → Get new API token**. It lasts a year, and getting a new one revokes the old one.

```sh
tmetric login     # asks for the token without echo, checks it, stores it in your keychain
tmetric whoami    # name, email, workspace and timezone
tmetric logout    # removes the stored token from this machine
```

The token goes to the operating system keychain, or to a file only you can read (`0600`) when there is no keychain.
`TMETRIC_API_TOKEN` takes precedence over the stored token, which is handy in scripts:

```sh
pass show tmetric | tmetric login        # read the token from a pipe
TMETRIC_API_TOKEN=… tmetric today        # or skip the login altogether
```

### Track time

```sh
tmetric timer start "Review PR #12" --project "My project" --task Tests
tmetric timer status
tmetric timer stop
```

`--project` and `--task` take an ID or an exact name (case does not matter); `tmetric projects list` shows them.
Starting a timer stops the one that runs, and the CLI cannot undo a stop, so both ask first when a timer runs:

```
Stop "Review PR #12" · My project (1h05m)? [y/N]
```

Pass `--yes` to skip the question; without a terminal it is required.
With shell completion on, `tmetric timer start <TAB>` suggests the descriptions you used lately.

### Review your time

```sh
tmetric today                              # today's entries and total
tmetric week                               # this week, from your workspace's first day
tmetric reports projects                   # time per project this week
tmetric reports projects --range last-month
tmetric reports projects --from 2026-11-01 --to 2026-11-15
tmetric reports balance                    # tracked against your work schedule
```

Ranges are `week`, `last-week`, `month` and `last-month`. Amounts and costs show when your workspace gives you access to financial information; totals of money are summed per currency.

### Fix your entries

`tmetric today` and `tmetric week` show each entry's ID in the first column.

```sh
tmetric entries edit 227522038 --start 09:30 --end 11:00
tmetric entries edit 227522038 --date 2026-11-23 --description "Bugs & Debt"
tmetric entries delete 227522038
```

- `edit` changes only what you pass and keeps the rest, tags and links to external tasks included. Times are `HH:MM` on the entry's day, in your TMetric timezone.
- TMetric stores an edited entry under a new ID, which `edit` prints.
- The API cannot look an entry up by its ID alone: pass `--date` for an entry that is not from today.
- An edit that would cover other entries is refused, because TMetric would cut or remove them.
- `delete` asks first; pass `--yes` in scripts.

### Terminal UI

```sh
tmetric ui
```

The running timer stays on top, ticking. Views sit on the left (`1`…`3`), their tabs on the right:

| View | What it shows |
|---|---|
| Time entries | `Day` and `Week` tabs; move by day or week, edit, delete and continue entries |
| Projects | The projects you can track time on, filtered with `/`; start a timer on one |
| Reports | `Projects` (time per project, with a share bar) and `Balance` (against your work schedule) tabs |

| Key | Action |
|---|---|
| `n` | New timer: description (with recent suggestions), project filtered as you type, optional task |
| `s` | Continue the selected entry: a new timer with its description, project and task |
| `enter` / `e` | Edit the selected entry |
| `d` | Delete the selected entry |
| `x` | Stop the running timer |
| `←` / `→`, `t` | Previous / next day or week, back to today |
| `[` / `]` | Previous / next tab |
| `tab` | Move the focus between the views and the panel |
| `a` | Your account and workspace |
| `r` | Refresh |
| `?` | Every key, for the view you are on |
| `q` | Quit |

Anything that would stop the running timer or delete time asks `y/N` in the footer first.
While the UI runs, the terminal window title shows the timer.

### Output and scripting

Output adapts to where it goes:

- On a terminal: aligned tables, with the total underneath.
- In a pipe: tab-separated rows with no header, and the total on stderr, so `cut` and `awk` see only rows.
- `--format csv` (on `today`, `week`, `projects list` and the reports): CSV with a header.
- `--json`: the full data, on every command that shows data.

```sh
tmetric week | awk -F'\t' '{print $5}'          # durations of this week's entries
tmetric reports projects --format csv > week.csv
tmetric today --json | jq '.total_seconds'
```

Colors follow your terminal theme and switch off with `NO_COLOR` or `TERM=dumb`. Errors go to stderr.

| Exit code | Meaning |
|---|---|
| `0` | Success |
| `1` | The operation failed (the API, the network, the keychain) |
| `2` | The command was used wrongly: a bad flag, an unknown project, a refused overlap, a missing `--yes` |

### Shell completion

```sh
source <(tmetric completion zsh)     # also bash, fish and powershell
```

Add the line to your shell's startup file to keep it.

## Environment

| Variable | Use |
|---|---|
| `TMETRIC_API_TOKEN` | API token; takes precedence over the stored login |
| `TMETRIC_BASE_URL` | Another API base URL (default `https://app.tmetric.com/api/v3`); same as `--base-url` |
| `NO_COLOR` | Turns colors off |

## Development

```sh
go test ./...
go build -o tmetric ./cmd/tmetric
```

CI runs `gofmt`, `go vet`, `staticcheck`, the tests with `-race`, a 100% coverage gate and SonarCloud.
The TUI follows [docs/tui-design.md](docs/tui-design.md).

## License

[MIT](LICENSE)
