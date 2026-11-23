# quantcli/common

Shared contracts and conventions for [quantcli](https://github.com/quantcli) export CLIs.

## Contents

- **[CONTRACT.md](CONTRACT.md)** — the user-facing surface every `*-export-cli` adheres to: repo naming, timezone policy, date flags, output formats, auth, the `prime` subcommand, versioning.
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — branch and commit style, how to propose a contract change, how to add a new export-cli, the compat-test expectation.
- **[CONSUMERS.md](CONSUMERS.md)** — known third-party consumers of each exporter's `--format json` surface. Read it before claiming [§9](CONTRACT.md#the-wire-surface)'s no-known-consumer carve-out; the GitHub counters §9 tests cannot see an integration that never starred the repo.
- **[TRIAGE.md](TRIAGE.md)** — issue-triage SLA, label scheme, and routing for the four `quantcli/*` repos.
- **[LICENSE](LICENSE)** — MIT.

## Why a repo for this

Three (and counting) export CLIs in this org all share the same shape: take credentials from the environment, accept `--since` / `--until`, emit markdown by default and JSON for agents, treat dates as local. Documenting that surface in one place — instead of in each CLI's README — keeps them honestly identical.

A change to the contract is a change every CLI agrees to make. Open a PR here before changing the surface in any individual CLI.

## Repos that follow this contract

- [`crono-export-cli`](https://github.com/quantcli/crono-export-cli) — Cronometer nutrition / biometrics
- [`liftoff-export-cli`](https://github.com/quantcli/liftoff-export-cli) — Liftoff workouts / bodyweights
- [`withings-export-cli`](https://github.com/quantcli/withings-export-cli) — Withings activity / sleep / measurements / intraday
