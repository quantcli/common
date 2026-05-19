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

**License allowlist** (SPDX identifiers):

- `Apache-2.0`
- `MIT`
- `BSD-2-Clause`
- `BSD-3-Clause`
- `MPL-2.0`
- `ISC`
- `Unlicense`

**License denylist** (blocking; not exhaustive):

- The GPL family — `GPL-*`, `LGPL-*`, `AGPL-*`.
- `SSPL-*`, `BUSL-*` / `BSL-*`, and other "source-available" licenses.
- "Custom" or unidentified licenses where the SPDX identifier cannot be resolved.

A PR that introduces a denied license is blocked. To request an exception, open an issue against `quantcli/common` with the dependency name, version, license text, and the rationale. Exceptions are rare and case-by-case.

## What's not in this policy yet

- **Signed releases / SBOM publishing.** Useful next steps; tracked as separate follow-up tickets, not yet shipped.
- **Threat model write-up.** The product surface is intentionally small (local CLIs, user owns their tokens), so a full threat model is premature. We will publish one if the surface grows materially.
- **Pen test.** Not commissioned for the current product surface.

If the policy itself needs to change — to add a category of scan, to adjust the allowlist, to revise the disclosure timeline — open a PR against this file in `quantcli/common`. Policy changes ripple across every export-cli and are reviewed accordingly.
