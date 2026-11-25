# tmetric-cli

Unofficial CLI and TUI for TMetric time tracking, in Go. Binary: `tmetric`.
Module: `github.com/othercodes/tmetric-cli` (go 1.26).

## Working rules

- Code, comments, commits, issues and PRs in English.
- Verify before you assert. Test against the real API, not only against fakes.
- Write tests for non-trivial logic: parsing, output, HTTP client against `httptest`. CI gates coverage at 100% (`main.go` excluded).
- Never commit or push unless the user asks. Before a commit, show the message first.
- Commits: Conventional Commits, lowercase, one commit per topic.
- One GitHub issue = one branch (`<type>/<issue>-<slug>`) = one PR, squash-merged into `master`.

## Structure

- `main.go` at the root, everything else in `internal/` (the official Go layout for a single command).
- `internal/cmd`: Cobra commands. `root.go` maps errors to exit codes: 0 ok, 1 runtime/API, 2 usage (`usagef`).
- `internal/app`: what the CLI and the TUI do, free of cobra and rendering.
- `internal/tmetric`: our own small HTTP client for the TMetric API, with only the calls we use.
- `internal/output`: tables on a terminal, tab-separated in a pipe, `--json`, CSV on request; honors `NO_COLOR` and `TERM=dumb`. Errors go to stderr.
- `internal/tui`: Bubble Tea + Lipgloss, reads through `internal/app` only. Follow @docs/tui-design.md.
- `internal/auth`: token in the OS keyring (`zalando/go-keyring`), fallback to a 0600 file in `os.UserConfigDir()/tmetric`.
- GoReleaser + GitHub Actions (`test.yml`, `release.yml`) + SonarCloud.

Auth is a personal API token (TMetric > My Profile > Get new API token, valid 1 year).
`tmetric login` asks for it and stores it. `TMETRIC_API_TOKEN` takes precedence.

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
- The API cuts every time to the minute: send `14:24:37`, get `14:24:00` back.
- `POST timeentries` returns an array with all entries of the affected days. The new timer is the one open entry.
- A new entry overwrites the entries it overlaps. Starting a timer closes the running one.
- `PUT` replaces the whole entry: send every field back, not only `endTime`. It answers with an array like `POST`
  (not a single entry, whatever the spec says), and the entry comes back under a **new ID**: find it by `startTime`.
- An entry shorter than a minute (start and end in the same minute) is dropped, not stored.
- The account is `activeAccountId` from `/user`. There is no separate accounts endpoint.
- `userId` is optional on every route and defaults to the token owner, so the client never sends it.
- Verified on a real account (2026-11-24): `endDate` is inclusive; a running timer comes back from `/latest` with `endTime: null`; an entry with no task has no `task` key.
- `GET timeentries/recent` (verified 2026-11-25): recent note/project/task combinations, no times, `isPinned` for pinned ones.
- Not verified yet: `/latest` on an account with no entries (the client accepts null, an empty body and 204).

## Product decisions

- `--project` and `--task` take an ID or a name. A name resolves against the API; zero or many matches is a usage error.
  Never send `task:{name}`: TMetric creates the task when it does not exist.
- `week` starts on the account's `firstWeekDay`, not always Monday.
- The API cannot fetch an entry by ID alone, so `entries edit|delete <id>` look it up on `--date` (today by default).
- An edit that would cover other entries is refused (TMetric would cut or remove them); the TUI asks before it overwrites.
- Anything that stops or deletes tracked time asks first: `y/N` in the TUI footer, a prompt or `--yes` in the CLI.
