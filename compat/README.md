# `compat/` — cross-CLI conformance library

This module is the machine-attested half of [`CONTRACT.md`](../CONTRACT.md). Every `*-export-cli` is expected to wire it into CI so the contract's "✓" status table stops being purely human-attested.

It is a Go module (`github.com/quantcli/common/compat`) under the `compat/` subdirectory of `quantcli/common`. Stdlib only.

## Design at a glance

- **Black-box.** The library never imports a CLI's internal packages. It shells out to the binary and asserts on stdout, stderr, and exit code.
- **One subpackage per contract section.** `dates/` (CONTRACT §2–§3) and `formats/` (CONTRACT §4) ship today. `auth/` and `prime/` are expected to follow. The convention is `compat/<section>` where `<section>` is the CONTRACT.md section being attested; each subpackage exposes a single `RunContract(t, runner)` entry point.
- **Exporters consume via a build-tagged `_test.go`.** No production import; compat tests do not ship in the released binary.
- **Hermetic by default.** `compat.Runner` runs the binary with an empty environment unless callers opt into specific variables. Subtests that need to assert "no network call" set proxy env vars to an unreachable address.

## Usage from an exporter

Add a single file in your repo (e.g. `compat_test.go`) gated behind a build tag so it does not run as part of the default `go test ./...`:

```go
//go:build compat

package main_test

import (
    "os"
    "testing"

    "github.com/quantcli/common/compat"
    "github.com/quantcli/common/compat/dates"
    "github.com/quantcli/common/compat/formats"
)

func TestContractDates(t *testing.T) {
    bin := os.Getenv("EXPORT_CLI_BIN")
    if bin == "" {
        t.Skip("EXPORT_CLI_BIN not set; skipping compat suite")
    }
    dates.RunContract(t, compat.Runner{Binary: bin})
}

func TestContractFormats(t *testing.T) {
    bin := os.Getenv("EXPORT_CLI_BIN")
    if bin == "" {
        t.Skip("EXPORT_CLI_BIN not set; skipping compat suite")
    }
    formats.RunContract(t, compat.Runner{Binary: bin})
}
```

### Cobra-based CLIs: date flags on subcommands

When `--since`/`--until` live on subcommands (the crono / liftoff / withings pattern), set `Subcommands` and the suite will dispatch per-subcommand:

```go
func TestContractDates(t *testing.T) {
    bin := os.Getenv("EXPORT_CLI_BIN")
    if bin == "" {
        t.Skip("EXPORT_CLI_BIN not set; skipping compat suite")
    }
    dates.RunContract(t, compat.Runner{
        Binary: bin,
        Subcommands: []string{
            "biometrics", "exercises", "nutrition", "servings", "notes",
        },
    })
}
```

Each subcommand is verified under a `subcommand=NAME/...` subtree, so a regression in any single one fails as a named subtest instead of masking the rest.

### CI workflow

```yaml
- name: build
  run: go build -o /tmp/cli .
- name: compat tests
  env:
    EXPORT_CLI_BIN: /tmp/cli
  run: go test -tags=compat ./...
```

The exporter does not need a separate `go.mod` for compat tests — the standard `require github.com/quantcli/common/compat vX.Y.Z` line in the exporter's existing `go.mod` is enough.

## What `dates.RunContract` covers today

| Subtest | Contract | What it asserts |
|---|---|---|
| `HelpDocumentsDateFlags` | §3 | `--help` mentions `--since` and `--until` and exits 0. |
| `InvalidSinceValueFails` | §3, §4 | `--since obviously-not-a-date` exits non-zero, writes to stderr, leaves stdout empty. |
| `HelpIsHermetic` | harness invariant | `--help` succeeds with all HTTP proxies pointed at an unreachable address. |
| `FlagValidationIsHermetic` | harness invariant | A parse failure also produces no successful outbound request. |

When `compat.Runner.Subcommands` is set, every row above runs once per declared subcommand under `subcommand=NAME/...`.

## What `formats.RunContract` covers today

| Subtest | Contract | What it asserts |
|---|---|---|
| `HelpDocumentsFormatFlag` | §4 | `--help` mentions `--format` and exits 0. |
| `UnknownFormatFails` | §4 | `--format obviously-not-a-format` exits non-zero, writes to stderr, leaves stdout empty. |
| `FlagValidationIsHermetic` | harness invariant | The unknown-format parse failure makes no successful outbound request. |
| `JSONIsArray` | §4 | `--format json` exits 0 and emits stdout that unmarshals as `[]any`. |
| `CSVHasHeader` | §4 | `--format csv` exits 0 and emits at least one non-empty line on stdout (the header row, present even on an empty result). |
| `DefaultIsMarkdown` | §4 | No `--format` flag produces byte-identical stdout to `--format markdown`. |

`JSONIsArray`, `CSVHasHeader`, and `DefaultIsMarkdown` invoke the data path with no extra args beyond `--format`. Integrators whose data path needs credentials or other env to succeed pass them via `compat.Runner.Env`. As with `dates`, `subcommand=NAME/...` subtest groups fire when `Subcommands` is set.

**Exporter parity note (as of bundle landing):** §4 lists three required codecs (markdown, json, csv), but only `withings-export-cli` implements all three today; `crono-export-cli` and `liftoff-export-cli` reject `--format csv`. Until the framework grows a `Runner.SupportedFormats` affordance (or those CLIs add CSV writers), the `CSVHasHeader` subtest fails against them. That is why the §4 row in CONTRACT.md's Status table remains human-attested at bundle landing — it will flip to **machine** once exporter parity catches up.

## What it does NOT cover yet

The actual local-midnight semantics of `--since 2026-04-15` (the harmonization that just landed across crono/liftoff/withings) is still **human-attested** in the status table. Asserting it black-box requires either:

- a `--print-resolved-window`-style affordance on every CLI (a substantive contract change, intentionally out of scope of the first compat-test cut), or
- per-upstream recorded HTTP fixtures (heavy, and pinned to specific API shapes).

When that affordance lands, the test belongs here as `dates.LocalMidnightSemantics`.

## Adding a new contract test

1. Decide which CONTRACT.md section it pins down. If no subpackage exists for that section, create one (`compat/<section>/`).
2. Write the assertion as a function that takes `*testing.T` and `compat.Runner`.
3. Wire it into the section's `RunContract`. Subtests are `t.Run`-scoped so one failure does not mask the rest.
4. Update this README's table and `CONTRACT.md`'s status-attestation note in the same PR.

## Self-test

This module has its own tests that run each bundle against a stub CLI in `internal/stubcli/`. The stub is intentionally narrow — it exists so `go test ./...` from this module's root proves the library compiles and the assertions fire correctly, without depending on any of the real export-CLIs. Failures in the self-test mean the library has a bug; failures in an exporter's compat test mean the exporter drifted from the contract.

The stub has two modes (`STUBCLI_MODE=flat` and `STUBCLI_MODE=cobra`). The flat-mode self-tests exercise the original Runner shape; the cobra-mode self-tests exercise `Subcommands`-based dispatch. In cobra mode, the stub's root `--help` deliberately omits `--since/--until/--format`, so the cobra-mode self-tests fail fast if `compat.Runner` ever stops prepending the subcommand. The stub emits an empty data set per `--format` codec (`[]` for json, a single header row for csv, nothing for markdown) so the formats bundle's data-path subtests run hermetically. There is also a focused unit test for `Runner.WithSubcommand` using an `argecho` helper that just prints `os.Args`.
