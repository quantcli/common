# Getting Started with quantcli export CLIs

This guide is for anyone who wants to pull personal health and fitness data from Cronometer, Liftoff, or Withings into a terminal, a script, or an LLM agent. By the end you will have a working export running on your machine.

## What is this?

quantcli is a set of open-source CLIs that export your personal data from health and fitness services. Each CLI targets one upstream service and follows the [CONTRACT.md](CONTRACT.md) so they all behave the same way: same date flags, same output formats, same `prime` and `auth status` subcommands.

**Available CLIs:**

| CLI | Service | Install |
|---|---|---|
| `crono-export` | Cronometer (nutrition, food log, biometrics) | `brew install quantcli/tap/crono-export` |
| `liftoff-export` | Liftoff / gymbros.com (gym workouts, bodyweight) | `brew install quantcli/tap/liftoff-export` |
| `withings-export` | Withings (activity, sleep, body measurements, intraday) | `brew install quantcli/tap/withings-export` |

## Pick your CLI

- **Cronometer data** (food log, macros, biometrics you enter manually) → `crono-export`
- **Gym workouts, bodyweight tracking via Liftoff / gymbros.com** → `liftoff-export`
- **Withings device data** (scale, sleep tracker, activity watch) → `withings-export`

## Install

```sh
brew install quantcli/tap/crono-export      # or liftoff-export or withings-export
```

No Homebrew? Build from source:

```sh
git clone https://github.com/quantcli/crono-export-cli
cd crono-export-cli
go build -o crono-export .
```

Replace `crono-export-cli` / `crono-export` with the CLI name for your service.

## Orient yourself with `prime`

Every CLI has a `prime` subcommand that prints a one-screen orientation aimed at both humans and LLM agents:

```
crono-export prime
liftoff-export prime
withings-export prime
```

`prime` covers: what the CLI exports, the I/O contract, auth requirements, date flags, every subcommand with output schema, jq examples, and known gotchas.

## Authenticate

### crono-export — env-var credentials

```sh
export CRONOMETER_USERNAME="you@example.com"
export CRONOMETER_PASSWORD="yourpassword"
crono-export auth status          # exit 0 means ready
```

### liftoff-export — stored OAuth token

```sh
liftoff-export auth login         # opens a browser tab; stores token locally
liftoff-export auth status        # exit 0 means ready
```

### withings-export — stored OAuth token

```sh
withings-export auth login        # opens a browser tab; stores token locally
withings-export auth status       # exit 0 means ready
```

`auth status` always exits 0 when credentials are usable, non-zero otherwise. It never makes a network call.

## Run your first export

All subcommands follow the same shape:

```
<cli> <subcommand> [--since VALUE] [--until VALUE] [--format markdown|json|csv]
```

Date values: `today`, `yesterday`, `YYYY-MM-DD`, `7d`, `4w`, `3m`, `1y`.

```sh
crono-export nutrition --since 7d
liftoff-export workouts list --since 7d
withings-export activity --since 7d
```

Default output is human-readable markdown. Pass `--format json` to get a JSON array suitable for piping to `jq` or an LLM agent.

```sh
crono-export nutrition --since 7d --format json | jq '.[0]'
```

## Verify contract conformance

Each CLI ships a conformance test suite. After building the binary:

```sh
# crono-export — parse-level conformance (data-path tests need live credentials)
cd crono-export-cli
EXPORT_CLI_BIN=/path/to/crono-export go test -tags=compat ./...

# withings-export — full conformance (stub server handles auth)
cd withings-export-cli
WITHINGS_EXPORT_BIN=/path/to/withings-export go test -tags=compat ./...
```

All green means the CLI conforms to [CONTRACT.md §3–§4 and §7](CONTRACT.md).

## Going further

- **Worked examples with expected output:**
  - [crono-export example](docs/example-crono.md) — nutrition totals, food log, biometrics
  - [liftoff-export example](docs/example-liftoff.md) — workouts list, bodyweight trend
  - [withings-export example](docs/example-withings.md) — activity, sleep, measurements
- **Contract reference:** [CONTRACT.md](CONTRACT.md) — date flags, output formats, auth model, conformance
- **Conformance library:** [compat/README.md](compat/README.md) — how the machine-attested test bundles work
- **Security policy:** [SECURITY.md](SECURITY.md)
