# quantcli export-CLI contract

This document is the source of truth for the user-facing surface every quantcli `*-export-cli` should adhere to. Each CLI exists to export personal data from a single upstream service (Cronometer, Liftoff, Withings, …) for use by humans on a terminal and by personal LLM agents calling the CLI as a tool.

The contract exists so that, once you have used one of these CLIs, the others feel like the same tool with a different verb.

## Status

| Section | crono-export | liftoff-export | withings-export | Attestation |
|---|---|---|---|---|
| Repo naming (`{service}-export-cli`) | ✓ | ✓ | ✓ | human |
| Timezone policy | ✓ | ✓ | ✓ | human |
| Date flags (`--since` / `--until`) — surface | ✓ | ✓ | ✓ | **machine** ([`compat/dates`](compat/README.md)) |
| Date flags — local-midnight semantics | ✓ | ✓ | ✓ | human |
| Markdown-default output | ✓ | ✓ | ✓ | human |
| Single `--format` flag | ✓ | ✓ | ✓ | human |
| `auth status` subcommand | ✓ | ✓ | ✓ | human |
| `prime` subcommand | ✓ | ✓ | ✓ | human |

All sections shipped across all three CLIs (April 25, 2026). The "Attestation" column tracks whether a contract section is verified by an automated [compat test](compat/README.md) on every PR or only by human review at merge time. Rows marked "human" are candidates for promotion to "machine" as the compat library grows.

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

CLIs MAY implement a subset of these codecs. A CLI MUST implement `markdown` and `json`; `csv` is recommended but optional. The Status table tracks per-CLI implementation per codec, and `compat.Runner.SupportedFormats` lets the conformance bundle target the declared subset so a partial-codec exporter can adopt the bundle without an immediate `--format csv` failure. Declaring less than `{markdown, json}` is non-conforming — markdown is required for human terminal use, json is required for agent/script use, and either alone leaves one of the two use cases unserved.

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

## 7. Conformance (the compat library)

Conformance to this contract is verified by [`compat/`](compat/README.md), a small black-box Go test library that lives in this repo and is imported by every `*-export-cli` from its own CI. The current bundles:

- [`compat/dates`](compat/README.md) — pins down §3: that `--since` / `--until` are documented in `--help`, and that an invalid value exits non-zero with a stderr-only error. The bundle additionally asserts that `--help` and flag-validation failures make no network request — that is a harness invariant the framework defends on every PR, not a property §3 itself promises.
- [`compat/formats`](compat/README.md) — pins down §4: that `--format` is documented, an unknown value exits non-zero with a stderr-only error, `--format json` emits a parseable JSON array, `--format csv` emits at least a header row, and the default is byte-identical to `--format markdown`. The `--format` parse-failure path is hermetic on the same harness-invariant basis as the dates bundle.

A new exporter is not "in" the family until its CI runs at least the `dates` bundle green. The `formats` bundle ships, but the full §4 surface (particularly `--format csv`) is not yet implemented across all three CLIs, so the §4 row in the Status table remains human-attested until exporter parity catches up. Existing exporters that have not yet wired up a bundle are tracked in the Status table's Attestation column.

## 8. Versioning and releases

Semantic versioning. User-visible bug fix → patch. New subcommand or flag → minor. Removed/renamed flag → major. Releases cut via `gh release create` against the relevant tag; goreleaser builds binaries for darwin/linux/windows × amd64/arm64 and publishes the cask to `quantcli/homebrew-tap`.

---

This contract is a living document. A change here is a change every CLI agrees to make. Open a PR against this repo before changing the surface in any individual CLI.
