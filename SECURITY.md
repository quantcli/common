# Security policy

This document covers the [quantcli](https://github.com/quantcli) family of open-source CLIs: `common`, `crono-export-cli`, `liftoff-export-cli`, `withings-export-cli`, and any future `*-export-cli` that adopts the [contract](CONTRACT.md).

## Owner direction: proportionate security (2026-11-23)

This direction supersedes conflicting older security-prioritization, hardening, and follow-up instructions. Quantcli builds small local personal-data export CLIs. Working authentication, correct exports, reliable releases, understandable commands and docs, and reported user problems take priority over speculative security infrastructure.

Protect credentials and private health data with straightforward established mechanisms: avoid token and data leakage, retain TLS verification, use sensible credential-file permissions, and fix credible exploitable dependencies. Before starting security work, briefly identify the affected user asset, plausible failure or attack path, concrete consequence, and smallest useful remedy. Missing checks, imperfect coverage claims, theoretical drift, and incomplete proofs are not themselves vulnerabilities.

Urgent security work requires actual compromise, exposed credentials, or a credible exploitable defect with material user impact. Other hardening and routine dependency updates are ordinary maintenance: batch them and keep them subordinate to product delivery. Licensing and compliance are separate considerations, not automatic security emergencies.

New scanners, custom workflow parsers, cross-repository monitors, monitoring of monitors, broader credentials, and additional recurring security automations require an explicit owner decision before implementation. Do not create follow-up tasks merely because residual risk exists. Record accepted limitations briefly and stop; accepted residual risk is a valid completed outcome. Do not repeatedly notify DT about unchanged warnings.

Existing deployed checks stay enabled pending owner approval of specific operational changes. Preserve credential protections, branch protection, and independent review. The owner or an explicitly delegated maintainer makes final merge and release decisions.

## Reporting a vulnerability

**Please do not open a public GitHub issue for security reports.** Public issue threads are searchable from the day they are filed; we'd rather give the project a chance to ship a fix before the issue is widely known.

Report security issues privately via:

- GitHub's private vulnerability reporting at <https://github.com/quantcli/common/security/advisories/new>, or
- Email to `security@quantcli.org`.

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

`quantcli/common` defines the contract; it is patched on `main`. If a change to the contract is required to resolve a vulnerability, it follows the contract-change flow described in [CONTRIBUTING.md](CONTRIBUTING.md), with review expedited when the impact warrants urgency.

## Supply-chain policy

The deployed [`security.yml`](.github/workflows/security.yml) runs `govulncheck`, `osv-scanner`, and a license-policy check on pushes to `main`, PRs targeting `main`, manual dispatch, and nominally daily at **06:17 UTC**. The exporters carry copies; pending trigger corrections are reviewed separately. These jobs report CI results; the required branch-protection gate in `common` is currently `Code owner approval`, not three separately required scanner contexts.

Both vulnerability scanners inspect Go dependencies, so their coverage overlaps. `govulncheck` analyzes Go code and reports reachable vulnerabilities; OSV scans dependency manifests across its database. The workflow discovers Go modules excluding `.git`, `vendor`, and `testdata`, and fails if none are found. Neither a green result nor the presence of a workflow proves complete coverage of shipped artifacts. A red result needs assessment: it can be a scanner or toolchain failure, license-policy failure, or advisory requiring an exploitability check.

The license allowlist below remains enforced by the existing workflow. A license failure can block that workflow without being a security vulnerability.

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

The deployed [`.github/workflows/scan-freshness.yml`](.github/workflows/scan-freshness.yml) runs in `common`, nominally daily at **07:42 UTC**, on manual dispatch, and on PRs touching its checker or configuration. It uses [`tools/scanfreshness`](tools/scanfreshness) to inspect public Actions metadata for repositories returned by org discovery. For non-exempt repos it checks for an active `security.yml` and that the latest completed, non-PR run on the default branch succeeded within **48 hours**. Exemptions with reasons are in [`.github/scan-freshness.json`](.github/scan-freshness.json); currently `.github` and `homebrew-tap` are exempt.

The checker reports `stale`, `workflow-disabled`, `workflow-missing`, `no-completed-run`, `scan-red`, or `unreadable`. These describe workflow status, not confirmed vulnerabilities. On a failed scheduled run, the alert job rechecks and opens or comments on an issue titled **"Security scan freshness check failed"** if failure persists. That existing automation can repeat comments; this policy change does not alter it. Maintainers should assess the underlying finding and avoid additional repeated notifications to DT about unchanged warnings.

GitHub schedules are best effort and can be delayed or disabled after inactivity. The watcher is also scheduled, and it neither monitors itself nor proves that org discovery returned every repo or that scans covered every shipped artifact. These are accepted scope limits, not instructions to add another watcher, expand credentials, or build a parser. A concise truthful description is sufficient unless concrete uncovered shipped code and material impact are established.

## Proposed sustainable baseline — owner decision required

Keep credential and private-data protections, TLS verification, sensible credential-file permissions, branch protection, independent review, and genuine dependency fixes. Use existing PR checks and a simple periodic dependency review; batch routine findings with maintenance.

For a simpler operational baseline, prefer `govulncheck` for Go vulnerabilities and assess whether OSV contributes useful findings beyond that overlap. Consider a weekly scheduled vulnerability scan instead of daily duplicate scans, and license checks when dependencies change. Assess whether the existing freshness watcher provides enough benefit to retain. These are proposals for the owner's decision, not approved changes: current scanners, daily schedules, license checks, and the watcher remain enabled until specific operational changes are approved.

Signed releases, SBOM publication, a formal threat-model document, penetration testing, drift monitoring, and additional security automation are not standing commitments or automatic follow-up work. Reconsider them only for a concrete user benefit and with owner approval where new infrastructure is involved.

Policy changes are reviewed through a PR in `quantcli/common`; operational changes need the specific owner decision described above. The owner or an explicitly delegated maintainer makes final merge and release decisions.
