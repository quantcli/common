# Worked example: liftoff-export

**Audience:** Liftoff (gymbros.com) users who want workout history or bodyweight trends in a terminal or piped to an LLM agent.

**Prerequisites:** `liftoff-export` installed (see [GETTING_STARTED.md](../GETTING_STARTED.md)), a Liftoff account, `liftoff-export auth login` completed.

---

## 1. Authenticate

Liftoff uses a stored OAuth token. Run the login flow once:

```sh
liftoff-export auth login
```

This opens a browser tab. Complete the OAuth flow and the token is stored locally (`~/.config/liftoff-export/auth.json`).

Check readiness:

```sh
liftoff-export auth status
```

**Expected when logged in:**

```
logged in
```

Exit 0. **Expected when not logged in:**

```
Error: not logged in — run: liftoff-export auth login
```

Exit 1.

---

## 2. Orient with `prime`

```sh
liftoff-export prime
```

**Output** (reproduced at HEAD, 2026-05-19):

```
liftoff-export — primer for LLM agents
======================================

WHAT IT IS
  CLI for personal Liftoff (gymbros.com) data: gym workouts with sets/
  reps/weights and recorded bodyweights.

I/O
  stdout: data in --format markdown (default; fitdown set notation) or json.
  stderr: errors. Exit 0 on success including empty results.

DATE FLAGS  (every subcommand)
  --since VALUE / --until VALUE
  VALUE: today | yesterday | YYYY-MM-DD | Nd/Nw/Nm/Ny
  See https://github.com/quantcli/common/blob/main/CONTRACT.md#3-date-flags

SUBCOMMANDS
  workouts list                Every workout in the window
  workouts show DATE           Workouts on one specific day
  workouts stats               Per-exercise PR/recent + monthly bar charts
                               Filters: --exercise NAME, --detail
  bodyweights list             Recorded bodyweights, one per line
  bodyweights stats            Current/high/low + monthly trend + plateau

  Inspect any subcommand's row schema with: <subcommand> --since 1d --format json

EXAMPLES
  liftoff-export workouts show today
  liftoff-export workouts stats --since 30d --format json |
    jq '.[] | select(.type == "WR") | {name, vol: ([.sessions[].volume] | add)}'
  liftoff-export bodyweights list --since 90d --format json |
    jq '[.[]] | (.[-1].weight - .[0].weight)'

GOTCHAS
  - Workout dates are LOCAL — 11pm workouts bucket on the day you logged them.
  - API hosts rotate; set LIFTOFF_API_BASE=https://vX-Y-Z.api.getgymbros.com
    if data calls fail with "server is deprecated".
  - Bodyweight is read off Post.bodyweight (the value you entered for that
    workout). No workout that day means no bodyweight that day.
  - 'workouts stats' bins exercises by name. Renaming an exercise in
    Liftoff splits it into two summaries.
```

---

## 3. List recent workouts

```sh
liftoff-export workouts list --since 7d
```

**Expected output** (markdown, one row per workout):

```
| Date       | Title            | Exercises | Duration |
|------------|------------------|-----------|----------|
| 2026-05-14 | Push A           | 5         | 58m      |
| 2026-05-17 | Pull A           | 4         | 52m      |
```

For the full set-level detail, use `--format json`:

```sh
liftoff-export workouts list --since 7d --format json | jq '.[0]'
```

---

## 4. Show a specific day's workout

```sh
liftoff-export workouts show today
```

Shows all workouts logged on today's local calendar date, including each exercise's sets, reps, and weights in fitdown-style markdown.

---

## 5. Track bodyweight trend

```sh
liftoff-export bodyweights list --since 90d
```

To compute weight change over the period:

```sh
liftoff-export bodyweights list --since 90d --format json \
  | jq '[.[]] | (.[-1].weight - .[0].weight)'
```

---

## 6. Check flag validation (contract §3, §7)

These run without credentials (hermetic by [CONTRACT.md §7](../CONTRACT.md#7-hermeticity)):

```sh
liftoff-export workouts list --help    # exits 0; no network call
```

**Expected:** help text including `--since`, `--until`, `--format` flags; exit 0.

---

## 7. Run the contract conformance suite

```sh
git clone https://github.com/quantcli/liftoff-export-cli
cd liftoff-export-cli
go build -o /tmp/liftoff-export .
LIFTOFF_EXPORT_BIN=/tmp/liftoff-export go test -tags=compat ./...
```

**Expected:**

```
ok  	github.com/quantcli/liftoff-export-cli	0.003s
```

The `compat` suite covers CONTRACT.md §4 parse-level conformance (the `--format` flag, `UnknownFormatFails`, `FlagValidationIsHermetic`) across all four data-producing subcommands (`workouts list`, `workouts stats`, `bodyweights list`, `bodyweights stats`). Data-path subtests (`--format json` with live data) are skipped because the suite does not provision a Liftoff OAuth token; the parse-level subtests run without credentials.

---

## What to look at next

- `liftoff-export workouts stats --help` — per-exercise PR summaries and filters
- `liftoff-export prime` — jq recipes and API host rotation gotcha
- [CONTRACT.md §3](../CONTRACT.md#3-date-flags) — date flag semantics
- [CONTRACT.md §4](../CONTRACT.md#4-output-format) — output format contract
- [crono-export example](example-crono.md) — if you also track nutrition
- [withings-export example](example-withings.md) — if you have Withings devices
