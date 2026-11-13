# Security policy

This document covers the [quantcli](https://github.com/quantcli) family of open-source CLIs: `common`, `crono-export-cli`, `liftoff-export-cli`, `withings-export-cli`, and any future `*-export-cli` that adopts the [contract](CONTRACT.md).

## Reporting a vulnerability

**Please do not open a public GitHub issue for security reports.** Public issue threads are searchable from the day they are filed; we'd rather give the project a chance to ship a fix before the issue is widely known.

Report security issues privately via:

- GitHub's private vulnerability reporting at <https://github.com/quantcli/common/security/advisories/new>, or
- Email to `security@quantcli.org` (PGP key forthcoming).

Include:

- The repository and version affected (or `main` and a commit SHA).
- A description of the issue and the impact you observed.
- A minimal reproduction — command-line invocation, sample input, the unexpected behaviour.
- Your name/handle for credit in the eventual advisory, if you want it.

**Out of scope:**

- Vulnerabilities in the upstream services these CLIs talk to (Cronometer, Liftoff, Withings, etc.). Report those to the upstream vendor.
- Reports that depend on an attacker already having local code execution on the user's machine.
- Reports relying on outdated dependencies in a release older than the currently supported version range.

## Response SLA

- **Acknowledgement:** within 5 business days.
- **Initial assessment** (severity + whether it's in scope): within 10 business days of acknowledgement.
- **Fix or mitigation** for confirmed vulnerabilities: best effort. Critical issues in supported releases are prioritised; low-severity issues may be batched into a regular release.

We coordinate disclosure with the reporter. Default disclosure timeline is **90 days** from the initial report, or earlier if a fix is available and shipped.

## Supported branches

We patch the **latest minor release of each CLI** on its `main` branch. Older releases are not patched; users on older versions should upgrade.

`quantcli/common` defines the contract; it is patched on `main`. If a change to the contract is required to resolve a vulnerability, it follows the contract-change flow described in [CONTRIBUTING.md](CONTRIBUTING.md), with the security review fast-tracked.

## Supply-chain policy

Every PR — in `common` and in every `*-export-cli` — is gated on a CI workflow that runs three checks:

- `govulncheck` against the Go vulnerability database.
- `osv-scanner` for transitive vulnerabilities across the OSV database.
- A license-policy check that allowlists only permissive licenses.

The same checks re-run on a daily schedule (06:17 UTC) on each repo's default branch, so an advisory published against a dependency we already ship is caught within a day rather than waiting for the next PR. That daily schedule is itself monitored — see [Scan freshness monitoring](#scan-freshness-monitoring) below.

**License allowlist** (SPDX identifiers, sorted by SPDX id; must match `ALLOWED_LICENSES` in `.github/workflows/security.yml` exactly):

- `0BSD`
- `Apache-2.0`
- `BSD-2-Clause`
- `BSD-3-Clause`
- `BSL-1.0` (Boost Software License — permissive, OSI-approved; **not** the source-available Business Source License — see denylist entry for `BUSL-*`)
- `CC0-1.0`
- `ISC`
- `MIT`
- `MPL-2.0`
- `Unlicense`

**License denylist** (blocking; not exhaustive):

- The GPL family — `GPL-*`, `LGPL-*`, `AGPL-*`.
- `SSPL-*`, `BUSL-*` (Business Source License — sometimes informally written "BSL"; not to be confused with the permissive `BSL-1.0` Boost above), and other "source-available" licenses.
- "Custom" or unidentified licenses where the SPDX identifier cannot be resolved.

A PR that introduces a denied license is blocked. To request an exception, open an issue against `quantcli/common` with the dependency name, version, license text, and the rationale. Exceptions are rare and case-by-case.

### Scan freshness monitoring

A daily scan is only a control while it is actually running. GitHub **disables scheduled workflows in a public repository after 60 days with no repository activity**, and announces that by email rather than by failing anything — so the daily re-scan above would switch itself off in exactly the case it exists for (a quiet repo), while the Actions tab kept showing green, because the last run that ever happened was green.

`quantcli/common` therefore runs a watcher: [`.github/workflows/scan-freshness.yml`](.github/workflows/scan-freshness.yml), daily at **07:42 UTC**, one hour after the scan cron. Unlike `security.yml` it is **not** copied into the export-clis — one watcher sweeps the whole org. It enumerates every repo in the `quantcli` organisation and requires each one to have:

- a `security.yml` workflow that is in state `active` (not disabled, by GitHub or by hand), and
- a **successful** completed run of it on the default branch within the **freshness window of 48 hours** — wide enough to absorb one missed daily run, tight enough that two consecutive misses are loud.

Anything else is a red build, with a reason code:

| Reason | Meaning | How to clear it |
|---|---|---|
| `stale` | The newest completed run on the default branch is older than the window. | Re-run `security.yml` in that repo; if the cron was auto-disabled, re-enable it and push something. |
| `workflow-disabled` | `security.yml` exists but is not `active` — this is the 60-day auto-disable, or a manual one. | Re-enable it from the repo's Actions tab, then dispatch a run. |
| `workflow-missing` | The repo has no `security.yml` at all — a new repo nobody wired up, or a deleted/renamed file. | Copy the workflow from `common` (see [CONTRIBUTING.md](CONTRIBUTING.md)), or add an exemption with a reason. |
| `no-completed-run` | The workflow exists but has never finished a run on the default branch. | Dispatch one. |
| `scan-red` | The scan is running on schedule but its latest verdict is a failure. Coverage is fine; the repo is not clean. | Triage the finding per [TRIAGE.md](TRIAGE.md). |
| `unreadable` | The repo's Actions metadata could not be read. An unreadable repo is treated as an unscanned repo rather than silently skipped. | Check token scope and repo visibility. |

**Alerting path.** The watcher's job goes red, and on scheduled runs it also opens — or comments on, if one is already open — a GitHub issue in `quantcli/common` titled **"Security scan freshness check failed"**, carrying the per-repo table. The issue exists because a failing *scheduled* workflow only emails the account that last touched the cron, which is a notification nobody reliably reads; an open issue in the org's main repo is visible to everyone watching it. The issue reports scan **coverage** only — repo names, reason codes, run links — never vulnerability details.

**Exemptions** live in [`.github/scan-freshness.json`](.github/scan-freshness.json) and each carries a written reason. Discovery is org-wide and opt-out, not an allowlist, so a newly created repo is covered the day it appears unless somebody argues in a PR for why it should not be.

**Residual risk, stated plainly:** the watcher is itself a scheduled workflow in a public repo, so it is subject to the same 60-day rule it exists to catch, and nothing watches the watcher. The mitigation is that `common` is the org's most active repo — 60 days of total silence there would be its own alarm — plus GitHub's warning email before a cron is disabled. A second, independent watcher outside GitHub Actions would close this; it is not worth the machinery at the current size.

To prove the check can still go red, run it from the Actions tab with the `max-age-hours` input set to `0.01`: every non-exempt repo should report `stale`.

## What's not in this policy yet

- **Signed releases / SBOM publishing.** Useful next steps; tracked as separate follow-up tickets, not yet shipped.
- **Threat model write-up.** The product surface is intentionally small (local CLIs, user owns their tokens), so a full threat model is premature. We will publish one if the surface grows materially.
- **Pen test.** Not commissioned for the current product surface.

If the policy itself needs to change — to add a category of scan, to adjust the allowlist, to revise the disclosure timeline — open a PR against this file in `quantcli/common`. Policy changes ripple across every export-cli and are reviewed accordingly.
