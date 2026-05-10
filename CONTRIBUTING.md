# Contributing to quantcli/common

This repo holds the [contract](CONTRACT.md) and shared conventions every quantcli `*-export-cli` agrees to. A change here is a change every CLI must follow. That means PRs land slowly and deliberately. If you want to move fast, work in the per-service repo (`crono-export-cli`, `liftoff-export-cli`, `withings-export-cli`); only promote to `common` once the change has stabilized in at least one of them.

## Before you open a PR

- Open an issue or comment on an existing one first if the change touches `CONTRACT.md` or affects all three exporters. The contract is a coordination point — discussion before code saves rework.
- Trivial fixes (typos, dead links, README polish) can skip the discussion step and go straight to a PR.
- Don't add dependencies casually. The Go side is standard library first; markdown tooling stays in CI, not in the repo.

## Branch conventions

Branch off `main`. Use a short prefix:

- `docs/` — README, CONTRACT, this file, anything documentation-only.
- `chore/` — repo plumbing, CI tweaks, version bumps.
- `feat/` — new shared code or new section of `CONTRACT.md`.
- `fix/` — bug fixes in shared code.

Examples: `docs/clarify-since-keyword`, `chore/bump-go-1.23`, `feat/contract-csv-quoting`.

Branches are short-lived. Rebase on `main` rather than merging `main` back in. One branch, one PR, one logical change.

## Commit style

- Imperative subject, ≤ 72 chars: `docs: clarify --since keyword behavior`.
- Optional body explains *why*, wrapped at ~72 chars.
- One concern per commit. If a PR has unrelated cleanups, split them.
- Reference the issue or contract section when relevant: `Refs CONTRACT.md §3`.

## Pull requests

- Title mirrors the lead commit.
- Body answers three questions: **What changed? Why? What does it imply for the per-service CLIs?** That third question is the one PR authors miss most often, and it's the one reviewers care about most.
- Mark the PR as draft until CI is green.
- Squash-merge by default. Use a merge commit only if the individual commits are independently meaningful (rare).
- The Lead Go Engineer is the merge gate for this repo. Expect one round of review on most PRs.

## Proposing a contract change

`CONTRACT.md` is the user-facing surface of every export CLI. Changes here ripple. The expected flow:

1. **Open an issue** describing the proposed change. Include: the current behavior, the proposed behavior, why the change is worth the coordination cost, and a list of which `*-export-cli` repos are affected.
2. **Land the implementation in at least one exporter first**, behind whatever flag or path lets it ship without breaking the contract. This is how we avoid contract changes that look fine in prose and fall apart on contact with a real upstream API.
3. **Open a PR against `common`** that updates `CONTRACT.md` and the [Status table](CONTRACT.md#status). The PR body lists the follow-up issues filed against each affected `*-export-cli` repo so reviewers can see the rollout plan.
4. **Update compat tests** in the same PR (see below).
5. **Don't merge until** every affected exporter either has the change shipped or has a tracked, owner-assigned follow-up issue. A merged contract change with no rollout plan creates silent drift.

User-facing flag changes follow semver in each CLI: a removed or renamed flag is a major bump.

## Adding a new export-cli to the family

The bar for joining the family is conformance to `CONTRACT.md`, not feature parity. A new CLI is welcome once it satisfies all of:

1. **Repo and binary naming** match `§1`. The repo lives at `github.com/quantcli/{service}-export-cli`; the binary is `{service}-export`.
2. **Date flags** behave per `§3`. `--since` and `--until` parse local-calendar dates with the keyword, absolute, and relative forms; relative durations snap to local midnight; `--until` is inclusive of the named day.
3. **Output formats** match `§4`. `--format markdown|json|csv`, markdown default, data on stdout, errors on stderr, exit 0 on empty result.
4. **Auth** has the surface in `§5`. An `auth status` subcommand exists and exits non-zero when the CLI cannot make an authenticated request. Headless env vars are documented in `prime` where the upstream allows them.
5. **`prime` subcommand** is present with the section structure in `§6` and fits on one terminal screen.
6. **Compat tests pass** — see below.

Once all six are met, open a PR against this repo that:

- Adds the new repo to the README's "Repos that follow this contract" list.
- Adds a column to the Status table in `CONTRACT.md` (the only contract change permitted as part of onboarding a new CLI; everything else is a separate proposal).
- Links to the new CLI's first tagged release.

Keep the per-CLI PR (the one that adds the CLI to the table) trivial and reviewable. Behavioral conformance is verified by the compat tests, not by re-reading the new CLI's source from this repo.

## Compat tests

The contract is only as honest as the test that proves three CLIs behave the same. The harness lives in [`compat/`](compat/README.md) as its own Go module (`github.com/quantcli/common/compat`); each exporter imports it and runs the relevant bundles against its own built binary in CI.

Rules:

- Anyone changing `CONTRACT.md` is also expected to update or add tests under `compat/` that exercise the new behavior against every `*-export-cli`.
- The harness is deliberately black-box: it shells out to the binary and asserts on stdout, stderr, and exit code only. It must not import a CLI's internal packages.
- One subpackage per contract section. The naming convention is `compat/<section>` where `<section>` is the CONTRACT.md section being attested — currently `compat/dates` (§2–§3) and `compat/formats` (§4); `compat/auth` (§5) and `compat/prime` (§6) are expected to follow. Each subpackage exposes a single entry point — `RunContract(t, runner)` — that exporters call from one build-tagged `_test.go` file.
- Cobra-based exporters whose contract surface lives on subcommands set `compat.Runner.Subcommands`; section bundles dispatch per-subcommand under a `subcommand=NAME/...` subtree. Flat CLIs leave the field empty and the bundle runs against the root binary.
- A PR that changes the contract without touching `compat/` is incomplete. Either update the tests in the same PR or open a follow-up issue and link it from the PR body before merging — the Lead Go Engineer holds the line on this.
- Compat tests run in CI on every PR and on `main`. A failing compat test on `main` means at least one shipped CLI no longer matches the contract, and that's a release-blocker incident, not a flake.
- The Status table in `CONTRACT.md` distinguishes **machine-attested** rows (covered by `compat/`) from **human-attested** rows (still verified by reviewer judgment). Promoting a row from human to machine attestation is itself a worthwhile PR.

**Bar for a new exporter:** the exporter's CI must build its binary and run `dates.RunContract` and `formats.RunContract` against it green. See [`compat/README.md`](compat/README.md) for the one-file integration pattern.

## License and sign-off

This repo is MIT-licensed (see [LICENSE](LICENSE)). By contributing you agree your changes are under the same license. We don't require a CLA. Sign-off (`Signed-off-by:` in commit messages) is encouraged but not required.

## Questions

Open an issue. Tag the Lead Go Engineer if it's blocking you.
