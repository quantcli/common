# Issue triage policy

This document defines how inbound issues and discussions are triaged across the four `quantcli/*` repos:

- [`quantcli/common`](https://github.com/quantcli/common) — contract + shared compat library
- [`quantcli/crono-export-cli`](https://github.com/quantcli/crono-export-cli)
- [`quantcli/liftoff-export-cli`](https://github.com/quantcli/liftoff-export-cli)
- [`quantcli/withings-export-cli`](https://github.com/quantcli/withings-export-cli)

It exists so that reporters know what to expect, and so that the four repos behave the same way. Like [`CONTRACT.md`](CONTRACT.md), this is a coordination document: a change here is a change every repo agrees to follow.

## Scope and posture

- **Public-by-default.** Every comment, label, and reply on these repos is public. Security-sensitive reports are an exception — see [SECURITY.md](SECURITY.md).
- **Standards-body voice.** Replies stay neutral and technical. We don't advocate for one upstream over another, and we don't critique competing tools or formats.
- **Reporter first.** First reply acknowledges the human and names a concrete next step. Diagnosis comes second.

## Priority buckets

Triage assigns one of four priorities. Priority drives the SLA in the next section.

| Label | Meaning | Examples |
|---|---|---|
| `priority:critical` | Security, data loss, or a contract violation that breaks every conformant CLI. Block a release. | RCE in `prime`, supply-chain compromise, `--format json` emitting non-JSON across all three exporters. |
| `priority:high` | Reproducible bug that breaks a documented contract surface on at least one CLI, or a regression on `main`. | `--since 7d` returning wrong window; compat bundle failing on `main`. |
| `priority:medium` | Reproducible bug or gap that has a workaround, or a well-scoped enhancement that's clearly in charter. | Confusing error text; missing `--types` validation; small docs additions. |
| `priority:low` | Polish, cosmetic, "nice to have", or open-ended discussion with no immediate action. | Typos, wording, tracking ideas, design exploration. |

Triage that flips a priority must say why in a comment. Silent re-prioritization is not allowed.

Severity-vs-visibility check: a loud-but-cosmetic report is not automatically high. A quiet-but-blocking report is not automatically low. The priority reflects the actual impact, not the volume of the thread.

## Time-to-first-touch SLA

"First touch" is a triage comment from a maintainer — a label set, a routing decision, a request for a repro, or a substantive reply. It is **not** a thumbs-up reaction or a silent label change.

Targets are measured in **business days** (Mon–Fri, US/Eastern), excluding weekends and US federal holidays. The clock starts when the issue or discussion is opened, or when a reporter responds to a `triage:needs-info` ask.

| Priority | Issues | Discussions |
|---|---|---|
| `priority:critical` | **4 business hours** | n/a — re-file as an issue |
| `priority:high` | **1 business day** | 2 business days |
| `priority:medium` | **2 business days** | 3 business days |
| `priority:low` | **5 business days** | 5 business days |

Untriaged inbound (`triage:needs-triage`) inherits the **medium** SLA until a triage decision is made.

A PR from an external contributor gets a "thanks, here's what happens next" comment within **1 business day**, even when review will take longer. Review depth is set by [CONTRIBUTING.md](CONTRIBUTING.md), not by this SLA.

These are **first-touch** targets, not resolution targets. Time-to-fix depends on engineering scope and is tracked per-issue, not by this document.

## Triage decisions

Every new issue and discussion gets, at minimum:

1. One `priority:*` label.
2. One `kind:*` label (what the report is).
3. One or more `area:*` labels (what surface it touches).
4. A first-touch comment that either (a) routes to an owner, (b) requests a repro / clarification, or (c) closes as duplicate / out-of-scope with a pointer.

Reproducibility bar for `kind:bug`: a report without a minimal repro (command, expected vs actual, CLI version, OS) gets `triage:needs-info`. Triage does not guess at the repro — the burden is on the reporter to provide one.

Scope-creep filter: a feature request hidden inside a bug report is two issues. Split them; don't let one absorb the other.

Empathy-before-explanation order: the first comment acknowledges the reporter as a human; the second comment asks the technical question. Skip step one only for one-line dup closes that link to the canonical issue.

## Canonical label scheme

The four repos share one label vocabulary. Adding, renaming, or removing labels follows the same PR-against-`common` flow as a contract change.

### `triage:*` — state in the triage funnel

| Label | Meaning |
|---|---|
| `triage:needs-triage` | Net-new, untouched. Default for fresh inbound. |
| `triage:needs-info` | Waiting on the reporter (repro, version, logs). |
| `triage:duplicate` | Closed as a duplicate; comment links to the canonical issue. |
| `triage:wont-fix` | Closed deliberately as out-of-scope or rejected; comment names the reason. |

### `kind:*` — what the report is

| Label | Meaning |
|---|---|
| `kind:bug` | Observed behavior diverges from documented behavior. |
| `kind:enhancement` | New feature or capability inside charter. |
| `kind:question` | Usage / "how do I" — often resolves to a docs gap. |
| `kind:docs` | Docs-only change (`README.md`, `CONTRIBUTING.md`, `CONTRACT.md` prose, `prime` text). |
| `kind:discussion` | Open-ended; no specific action expected. |
| `kind:security` | Security-sensitive. Re-route privately per [SECURITY.md](SECURITY.md) **before** further public comment. |

### `area:*` — which surface

| Label | Where it applies | Meaning |
|---|---|---|
| `area:contract` | `common` | `CONTRACT.md` prose or semantics. |
| `area:compat` | `common` | The `compat/` library or its bundles. |
| `area:ci` | all four | GitHub Actions workflows, security scans, releases. |
| `area:docs` | all four | Repo docs other than `CONTRACT.md`. |
| `area:auth` | export-CLIs | `auth status` / `auth login` / env-var auth. |
| `area:format` | export-CLIs | `--format markdown` / `json` / `csv` and codec behavior. |
| `area:dates` | export-CLIs | `--since` / `--until` parsing and windowing. |
| `area:prime` | export-CLIs | The `prime` subcommand. |
| `area:crono` | cross-repo | Crono-specific issue filed against `common` (rare; usually re-routed). |
| `area:liftoff` | cross-repo | Liftoff-specific issue filed against `common`. |
| `area:withings` | cross-repo | Withings-specific issue filed against `common`. |

Cross-repo `area:{service}` labels exist for the case where someone files against `common` an issue that's actually about a specific exporter. Triage applies the label and re-routes, rather than silently closing.

### `priority:*` — SLA bucket

Defined in [the priority table above](#priority-buckets). Every triaged issue carries exactly one `priority:*` label.

### `compat:ref` — contract citation present

Applied when an issue or PR cites a CONTRACT section (e.g. `CONTRACT §3`). Signals to reviewers that the [`compat/` citation check](CONTRIBUTING.md#citing-the-contract-from-compat-code) is in play, and that the quoted text must match the contract as written.

### Retained GitHub defaults

| Label | Notes |
|---|---|
| `good first issue` | GitHub built-in. Applied by maintainers, not self-assigned. |
| `help wanted` | GitHub built-in. Signals that maintainers welcome an external PR. |

## Routing

Triage routes by `kind` + `area`, not by name:

- `kind:bug` + `area:contract` / `area:compat` → Lead Go Engineer.
- `kind:bug` + `area:{service}` on an exporter repo → that exporter's maintainer (today: Lead Go Engineer).
- `kind:enhancement` touching `CONTRACT.md` → Integration & Standards.
- `kind:security` → re-route privately per [SECURITY.md](SECURITY.md). Do not discuss publicly until the responsible disclosure window closes.
- `kind:question` or `kind:docs` → first-touch reply from Community Manager; file a `kind:docs` follow-up against Documentation & Education if the question signals a docs gap.
- `kind:discussion` → leave open; revisit if it converges on a specific action.

A repeated question — three or more independent reporters asking the same thing — is a docs bug, not a user bug. File the docs follow-up with concrete language pulled from the threads.

## Repro request template

When applying `triage:needs-info` on a `kind:bug`, the first-touch comment asks for, at minimum:

```
- Command you ran (exact argv).
- What you expected to happen.
- What actually happened (stdout, stderr, exit code).
- CLI version (`{service}-export --version`) and OS.
- Whether `--format json` produces the same divergence (helps isolate codec vs core).
```

The reporter has **14 calendar days** to respond. After 14 days with no reply, the issue is closed with a comment that invites them to reopen when they have the info. Reopened issues re-enter the funnel at their original priority.

## What triage does not do

- Set roadmap or commit to a delivery date.
- Promise upstream-maintainer outreach. If a third-party dependency is the root cause, we ship our own fix; we do not file issues or PRs against the upstream maintainer without board sign-off.
- Speak on behalf of one export-CLI against another, or against a competing format / tool.

Anything that requires those is escalated to the CEO.

## Changing this policy

Open a PR against `quantcli/common`. The flow mirrors a contract change:

1. State the current behavior, the proposed behavior, and why.
2. Land the change here.
3. Apply any label rename across all four repos in a follow-up PR / scripted change, gated on maintainer review.

Label additions, renames, and removals require maintainer review on the PR. Bulk label changes across the four repos require an explicit go-ahead in the PR thread before they are applied.
