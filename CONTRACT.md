# quantcli export-CLI contract

This document is the source of truth for the user-facing surface every quantcli `*-export-cli` should adhere to. Each CLI exists to export personal data from a single upstream service (Cronometer, Liftoff, Withings, …) for use by humans on a terminal and by personal LLM agents calling the CLI as a tool.

The contract exists so that, once you have used one of these CLIs, the others feel like the same tool with a different verb.

## Status

| Section | crono-export | liftoff-export | withings-export |
|---|---|---|---|
| Repo naming (`{service}-export-cli`) | ✓ | ✓ | ✓ |
| Timezone policy | ✓ | ✓ | ✓ |
| Date flags (`--since` / `--until`) | ✗ | ✓ | ✗ |
| Markdown-default output | ✗ | ✓ | ✓ |
| Single `--format` flag | ✗ | ✗ | ✓ |
| `auth status` subcommand | ✗ | ✗ | ✗ |
| `prime` subcommand | ✓ (older shape) | ✗ | ✗ |

`✗` means the CLI either doesn't conform yet or pre-dates this contract; tracked in each repo's issue list.

---

## 1. Repo naming

Each CLI lives at `github.com/quantcli/{service}-export-cli` where `{service}` is the upstream service in lowercase (`crono`, `liftoff`, `withings`). The binary is `{service}-export`. Homebrew cask is `{service}-export`.

## 2. Timezone policy

Off-by-one-day bugs are the most common timezone failure in tools like these. All three CLIs treat dates the same way to avoid them:

- **User-supplied dates** parse in `time.Local`. Use `time.ParseInLocation("2006-01-02", s, time.Local)`, never bare `time.Parse`. Bare `time.Parse` defaults to UTC and silently shifts the calendar boundary by the user's offset.
- **API timestamps** that flow into day-bucketing or human-readable output are converted with `.Local()` before formatting. Bucketing without local conversion buckets by whatever zone the API returned, which is usually UTC, which is wrong.
- **JSON output** preserves RFC3339 with whatever offset Go's default `time.Time` marshaler emits — usually local. Don't override the marshaler.
- **CSV / markdown output** renders timestamps in the user's local zone.

## 3. Date flags

Every subcommand that selects a window of data accepts:

```
--since VALUE   inclusive lower bound
--until VALUE   inclusive upper bound (omit for "now")
```

`VALUE` is one of:

| Form | Example | Meaning |
|---|---|---|
| Keyword | `today`, `yesterday` | Local-calendar day |
| Absolute | `2026-04-15` | Local midnight that calendar day |
| Relative | `7d`, `4w`, `6m`, `1y` | N units before today's local midnight |

Window semantics are half-open `[since, until_exclusive)` internally, but `--until` is inclusive of the named day from the user's point of view: the CLI adds 24h to the parsed value before filtering, so `--until 2026-04-15` includes all 24 hours of April 15.

Relative durations snap to local midnight, **not** "this many days before the current instant". `--since 30d` returns the same window whether you run it at 9am or 11pm.

When neither flag is given, each subcommand picks its own default (typical: `7d` for log-style commands, `30d` or `90d` for sparse commands like workouts/measurements, `1d` for dense commands like intraday).

## 4. Output format

```
--format markdown   default; fitdown-style human-readable
--format json       array on stdout, suitable for jq / LLM agents
--format csv        spreadsheet-friendly
```

A single `--format` flag, not separate `--json` / `--csv` flags. `markdown` is the default because terminals are humans by default; agents pass `--format json` once.

Output rules:

- **stdout**: data only, in the requested format.
- **stderr**: errors and progress messages. Pipelines using `>` to redirect stdout do not need `2>&1`.
- **Empty result**: success with empty output (`[]` for JSON, no rows for markdown/CSV), exit code 0. Empty is not an error.
- **Exit code**: 0 success, non-zero only for auth or network failure.

## 5. Auth

Auth flows differ legitimately across upstreams (env-var basic auth, OAuth2, interactive credential prompts), but two surface elements are required of every CLI:

- An `auth status` subcommand that prints one line summarizing readiness — e.g. `logged in as you@example.com (token expires 2026-05-01)` or `missing CRONOMETER_PASSWORD`. Exit code 0 if usable, non-zero otherwise.
- A headless path: where the upstream's auth model permits it, every CLI accepts environment variables that let a fresh container run without an interactive login. The variable names are `{SERVICE}_*` (e.g. `CRONOMETER_PASSWORD`, `WITHINGS_REFRESH_TOKEN`). Document them in `prime`.

Where the upstream forbids headless auth (e.g. interactive OAuth consent), `auth status` still works and the CLI still exposes `auth login` / `auth logout` / `auth refresh`.

## 6. The `prime` subcommand

Every CLI exposes a `prime` subcommand that prints a single-screen primer aimed at LLM agents calling the CLI as a tool. Same section structure across all three so an agent that has read one knows where to look in another:

```
WHAT IT IS    one paragraph
I/O CONTRACT  stdout/stderr/exit-code/format
AUTH          env vars + auth subcommands
DATE FLAGS    link to this contract; CLI-specific defaults
SUBCOMMANDS   one block per, with output schema sketch
EXAMPLES      3-5 jq recipes for common questions
GOTCHAS       non-obvious pitfalls
```

Prime is short. It is not a man page. If it grows past one terminal screen, something belongs in this contract instead.

## 7. Versioning and releases

Semantic versioning. User-visible bug fix → patch. New subcommand or flag → minor. Removed/renamed flag → major. Releases cut via `gh release create` against the relevant tag; goreleaser builds binaries for darwin/linux/windows × amd64/arm64 and publishes the cask to `quantcli/homebrew-tap`.

---

This contract is a living document. A change here is a change every CLI agrees to make. Open a PR against this repo before changing the surface in any individual CLI.
