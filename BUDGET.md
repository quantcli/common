# BUDGET.md — quantcli operating budget

A single public source of truth for quantcli's money: how much we operate on, what it pays for, where contributions come from, and where they go.

**Update cadence:** monthly, via PR to `quantcli/common`, reviewed before merge. Each month's snapshot is appended to [Monthly snapshots](#monthly-snapshots). The operating envelope and firewall sections are static unless explicitly changed (see [Governance of this page](#governance-of-this-page)).

**Status at first publish:** neither funding platform is live yet. The numbers below are therefore a pre-launch baseline ($0 in, $0 out). The platform links become real once GitHub Sponsors and Open Collective are enabled.

---

## What this page is

quantcli is a small open-source project that maintains a shared data contract ([CONTRACT.md](CONTRACT.md)) and a family of MIT-licensed Go CLIs that export your own quantified-self data in a uniform, agent-friendly format. This page documents the money that keeps that running.

It is written so a skeptical donor can reconstruct every number without asking us. If a figure on this page is not linked to a primary source — an Open Collective ledger entry, a GitHub Sponsors export, an invoice, or a receipt — it does not belong on this page.

## Operating envelope

**$300 / month.**

This is the ceiling on monthly operating spend across all categories below. It is not a target, a fundraising goal, or an aspiration — it is what we have committed to keep the project running on.

Why $300/mo and not more:

- It is the smallest credible envelope that covers the current cost of running the contract, the compat suite, the four `quantcli/*` repos, and a modest maintainer-agent runtime budget.
- It is small enough that we can sustain the project for months on existing reserves if all contribution inflows go to zero — runway over heroics.
- Raising it requires an explicit board sign-off, a corresponding line on this page, and a public note in the next monthly snapshot.

If contributions consistently exceed the envelope, the surplus accrues to a reserve fund (tracked here) rather than triggering automatic new spend.

## Sponsor-influence firewall

This section is the most important one on this page and is non-negotiable.

> **Sponsors fund our continued operation. They do not buy influence over the contract.**

Specifically, no contribution at any tier on any platform buys:

- **Governance influence.** Sponsors do not get board seats, voting rights, technical-steering positions, or any formal or informal say over project direction.
- **Roadmap priority.** Sponsors do not get their bug, feature, or contract change prioritized over anyone else's. The triage SLA in [TRIAGE.md](TRIAGE.md) applies equally to sponsors and non-sponsors.
- **Contract carve-outs.** Sponsors do not get amendments to [CONTRACT.md](CONTRACT.md) that favor their product, their vendor, or any specific export-CLI. The contract is the same surface for everyone who implements it.
- **Vendor-specific concessions.** Sponsors do not get their export-CLI labeled, blessed, recommended, or placed ahead of any other export-CLI that conforms to the contract. We do not bless specific tools; we maintain a neutral, shared contract.
- **Exclusive support.** Sponsors do not get a private support channel, a dedicated maintainer, an SLA, or guaranteed response times. Contributors are donors, not customers.
- **Logo placement implying endorsement.** Sponsor names are listed alphabetically by tier on a public acknowledgements page. No logos sized by tier. No placement that implies the project endorses any specific product.
- **Private side agreements.** All sponsor terms live in public tier copy. There are no private side agreements, no NDAs, no off-ledger arrangements.

If a prospective sponsor asks for any of the above, the answer is **no**, and the request is escalated to the board. A polite refusal is preferred to an arrangement that would compromise the project's neutrality.

This firewall applies equally to individual sponsors, corporate sponsors, and infrastructure-credit donors.

## What the money pays for

The $300/mo envelope is split across the following categories. Specific dollar amounts in each category are reported in the monthly snapshot below; the categories themselves are stable.

| Category | What it covers | Typical fraction |
|---|---|---|
| **Domains & DNS** | `quantcli.*` domain registrations and DNS | Single-digit % |
| **CI / build minutes** | GitHub Actions runner minutes above the free tier across the four `quantcli/*` repos | Up to ~20% |
| **Agent runtime** | LLM-API spend for the maintainer agents that triage issues, maintain the compat suite, and reconcile this page | Up to ~50% |
| **Platform fees** | Stripe processing, GitHub Sponsors fees, Open Source Collective's 10% fiscal-host fee | Whatever the platforms charge |
| **Security & compliance** | Vulnerability scanning, license-audit tooling, any incident-response tooling | Variable, often $0 |
| **Contributor stipends** | Post-hoc, capped honoraria for specific shipped artifacts. Capped at 25% of trailing-3-month inflows. Requires board sign-off per disbursement. | $0 by default |
| **Reserve** | Surplus from inflows above the envelope. Held to extend runway, not for new spend. | Whatever is left |

Out of envelope (we do not spend on): travel, swag, conference booths, ads, lawyer retainers, paid endorsements, or payments to vendors whose services we export.

## Where the money comes from

Two public ledgers; both are linked from the monthly snapshot once live.

- **GitHub Sponsors** — `https://github.com/sponsors/quantcli` *(planned — not yet live)*.
- **Open Collective** — `https://opencollective.com/quantcli` *(planned — not yet live)*.

We do not accept off-ledger contributions. If you would like to contribute and your organization cannot use either platform, open a public issue on `quantcli/common` and we will discuss whether a third public-ledger option is worth standing up.

## Monthly snapshots

Each month's snapshot is a separate subsection added by PR. The first real snapshot will be added the month after platform launch; until then, the pre-launch baseline below applies.

### 2026-05 (pre-launch baseline)

- **Inflows this month:** $0.00. No platforms live yet.
- **Outflows this month:** $0.00 logged against this envelope. Operating costs to date have been absorbed by the founder.
- **Reserve balance:** $0.00.
- **Notes:** The sustainability function is being stood up. The first real snapshot will be the month after both platforms (GitHub Sponsors + Open Collective) are live.

Future snapshots follow this template:

```
### YYYY-MM

- Inflows this month: $X.XX
  - GitHub Sponsors: $X.XX (link to GH Sponsors export commit)
  - Open Collective: $X.XX (link to OC ledger range)
- Outflows this month: $X.XX
  - [Category]: $X.XX (link to invoice/receipt or OC expense)
  - …
- Net: $X.XX
- Reserve balance: $X.XX
- Notes: anomalies, one-time items, anything that needs explanation.
```

Every dollar figure links to a primary source. No "approximately", no "rough order of magnitude".

## How to flag an error

If you spot a number on this page that does not match its linked primary source, or a primary source that does not exist or has changed, open an issue on `quantcli/common` with `budget` in the title. It will be triaged under the [TRIAGE.md](TRIAGE.md) SLA, and the page will either be corrected or the discrepancy explained in the next monthly snapshot.

## Governance of this page

- This page is maintained by the project's sustainability function and reviewed by the board before each monthly publish.
- Changes to the operating envelope require board sign-off recorded in the next monthly snapshot.
- Changes to the [Sponsor-influence firewall](#sponsor-influence-firewall) section require board sign-off **and** a 14-day public-comment window opened via a tracked issue on `quantcli/common` before merge.
- The git edit history of this file is the audit trail. There is no private copy of this page.
