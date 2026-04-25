# quantcli/common

Shared contracts and conventions for [quantcli](https://github.com/quantcli) export CLIs.

## Contents

- **[CONTRACT.md](CONTRACT.md)** — the user-facing surface every `*-export-cli` adheres to: repo naming, timezone policy, date flags, output formats, auth, the `prime` subcommand, versioning.

## Why a repo for this

Three (and counting) export CLIs in this org all share the same shape: take credentials from the environment, accept `--since` / `--until`, emit markdown by default and JSON for agents, treat dates as local. Documenting that surface in one place — instead of in each CLI's README — keeps them honestly identical.

A change to the contract is a change every CLI agrees to make. Open a PR here before changing the surface in any individual CLI.

## Repos that follow this contract

- [`crono-export-cli`](https://github.com/quantcli/crono-export-cli) — Cronometer nutrition / biometrics
- [`liftoff-export-cli`](https://github.com/quantcli/liftoff-export-cli) — Liftoff workouts / bodyweights
- [`withings-export-cli`](https://github.com/quantcli/withings-export-cli) — Withings activity / sleep / measurements / intraday
