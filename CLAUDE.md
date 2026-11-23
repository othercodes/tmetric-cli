# tmetric-cli

Unofficial CLI and TUI for TMetric time tracking, in Go. Binary: `tmetric`.
Module: `github.com/othercodes/tmetric-cli` (go 1.23).

## Working rules

- Answer the user in Spanish. Code, comments, commits and issues in English.
- Explain what you do and why. Do not assume the user knows everything.
- Verify before you assert. Test against the real API when the user gives a token.
- Write tests for non-trivial logic: parsing, output, HTTP client against `httptest`.
- Never commit or push unless the user asks. Before a commit, show the message first.
- Commits: Conventional Commits, lowercase, one commit per topic.
- One GitHub issue = one branch = one PR.

## Model project

Copy the structure and conventions of `~/Projects/webshare/github/webshare-cli`.
Read the matching file there before you write a new one.

- Cobra commands in `internal/cmd`. `root.go` maps errors to exit codes: 0 ok, 1 runtime/API, 2 usage (`usagef`).
- `internal/app`: what the CLI and the TUI do, free of cobra and rendering.
- `internal/output`: tables on a terminal, tab-separated in a pipe, `--json`, CSV on request; honors `NO_COLOR` and `TERM=dumb`. Errors go to stderr.
- `internal/tui`: Bubble Tea + Lipgloss, reads through `internal/app` only.
- `internal/auth`: token in the OS keyring (`zalando/go-keyring`), fallback to a 0600 file in `os.UserConfigDir()/tmetric`.
- GoReleaser + Homebrew cask + `.github/workflows/{ci,release}.yml`.

Differences from webshare-cli:

- No SDK exists. `internal/tmetric` is our own small HTTP client with only the calls we use.
- No OAuth. The auth is a personal API token (TMetric > My Profile > Get new API token, valid 1 year).
  `tmetric login` asks for it and stores it. `TMETRIC_API_TOKEN` takes precedence. No refresh, no lock.

## TMetric API v3

- Base: `https://app.tmetric.com/api/v3`. Spec: `https://app.tmetric.com/api-docs/openapi.v3.json`.
- Header: `Authorization: Bearer <token>`, `Accept: application/json`.
- Second source that uses v3: github.com/DaniilTsitavets/tmetric_mcp.

| Use | Route |
|---|---|
| user, account, timezone | `GET /user` → `id`, `name`, `email`, `activeAccountId`, `timeZone{id}`, `accounts[]{id,name,firstWeekDay}` |
| timer status | `GET /accounts/{accountId}/timeentries/latest` |
| timer start | `POST /accounts/{accountId}/timeentries` body `{startTime, endTime:null, note, project:{id}, task:{id}}` |
| timer stop | `PUT /accounts/{accountId}/timeentries/{id}` with the full entry and `endTime` set |
| entries in a range | `GET /accounts/{accountId}/timeentries?startDate=YYYY-MM-DD&endDate=YYYY-MM-DD` |
| projects you can track | `GET /accounts/{accountId}/timeentries/projects` |
| tasks of a project | `GET /accounts/{accountId}/tasks?ProjectList={id}` (PascalCase query, repeat key for arrays, no text search, 206 at 500 items) |
| delete entry (later) | `DELETE /accounts/{accountId}/timeentries/{id}` |

API facts that shape the code:

- There is no timer endpoint. A running timer is a time entry with `endTime: null`.
- Datetimes have no zone: `2026-11-20T15:00:00`, local time in `user.timeZone.id`. Never send `Z` or an offset: the entry moves to another day. Responses can have fractional seconds.
- There is no duration field. Duration = `endTime - startTime`, or `now - startTime` while running.
- `POST timeentries` returns an array with all entries of the affected days. Find the new one by `startTime`.
- A new entry overwrites the entries it overlaps. Starting a timer closes the running one.
- `PUT` replaces the whole entry: send every field back, not only `endTime`.
- The account is `activeAccountId` from `/user`. There is no separate accounts endpoint.
- `userId` is optional on every route and defaults to the token owner, so the client never sends it.
- Not verified yet: `/latest` with no entries (null or 204), and whether `endDate` is inclusive.

## Product decisions

- `--project` and `--task` take an ID or a name. A name resolves against the API; zero or many matches is a usage error.
  Never send `task:{name}`: TMetric creates the task when it does not exist.
- `week` starts on the account's `firstWeekDay`, not always Monday.
