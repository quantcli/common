# Evidenced consumers

[CONTRACT §9](CONTRACT.md#the-wire-surface) requires a deprecation window for a wire-surface `major` **"where the exporter has a known consumer"**, and lets a PR skip the window under a *no known consumer* carve-out that must be **stated with its evidence**. This file is where that evidence lives, so the claim is checked against a record instead of a recollection.

## Why a file and not the GitHub counters

§9's carve-out tests four counters: stars, forks, watchers beyond the maintainers, and third-party-authored issues. They are a reasonable proxy for "is anyone reading this" and a weak one in the direction that matters. A consumer integrates by reading a README and writing code against the output; none of that touches a counter. So all four can be zero while a consumer exists, and the carve-out gets claimed in perfect good faith by someone with no way to know better.

That is not a hypothetical. The first consumer this family found had never starred, forked, watched or filed an issue on the exporter it integrates with — it was found by someone looking, and until this file existed the finding lived in one comment on one internal issue. The next person to claim the carve-out would not have seen it.

So: the counters measure attention, this file records integration, and §9 now asks for both.

## Using this file

**Before claiming the no-known-consumer carve-out**, read the table below and say in the PR that you did, naming the commit of this file you read. Zero counters plus an empty row here is a claim a reviewer can re-run. Zero counters alone is not.

**When you find a consumer**, add it — immediately, not when a `major` is next in flight. The value of this file is entirely in being written before it is needed. A found consumer that nobody wrote down is one nobody will check for.

**An entry is a measurement, so it carries its date and its method.** "Discovered via" is how we know, not where we guess. Record the exact file and line of the integration where there is one; an entry a reader cannot re-verify decays into folklore.

**This file records what we observed in public source. It is not a contact list and not an endorsement.** Nothing here authorizes contacting a consumer, filing an issue on their tracker, or citing them in outreach or marketing. Those are separate decisions with their own approval.

## Consumers

| exporter | surface consumed | consumer | evidenced | how |
|---|---|---|---|---|
| `liftoff-export-cli` | `workouts list --format json`, `routines list --format json` | identity withheld pending publication authorization — recorded internally as QUA-118 | 2026-11-23 | read in the consumer's public source: it names `liftoff-export-cli` verbatim and hard-codes our `Post` / `ExerciseData` / `SetData` / `Preset` field names. Unsolicited — no issue, star, fork or contact on our side. |

Notes on that entry, because they are what a future `major` needs:

- **It consumes field *names*, not just the container.** The integration duck-types records by field presence (`exerciseData` is an array, `startedAt` is a string or absent). So a change that §9's table calls additive can still break it: adding `startedAt` to the routine record would make routines indistinguishable from workouts, and `exerciseData: null` for an empty routine already fails recognition — see [§4's empty-collection clause](CONTRACT.md#record-shape). **"Add a field" is `minor` on the table and still needs a thought about record discriminability.**
- **It also reads `exerciseTypes` as a closed set** (`WR`, `BR`, `AB`, `WD`, `DD`, `ND`), silently dropping any value outside it. Adding or renaming an `exerciseTypes` code is therefore a wire-surface change under §9 even though it changes no key and no type — the kind the table does not have a row for.
- **It carries no unit for our weights.** It takes a weight with no unit field as already being in the reading user's configured unit. liftoff's set weights are pound-canonical (QUA-109), so a kilogram-configured user imports pound numbers as kilograms. [§4's unit-named siblings](CONTRACT.md#units-and-cross-codec-parity) (`weight_lb`, `distance_km`) are the fix on our side; the consumer's mapper reads field names, so the sibling has to exist before they can read it.

## Checked and none found

A reviewer needs to tell "we looked and found nothing" from "nobody has looked". Both are recorded; only the first supports the carve-out.

| exporter | stars / forks / watchers | third-party issues | consumer evidenced | checked |
|---|---|---|---|---|
| `withings-export-cli` | 0 / 0 / 0 | none | none found | 2026-11-23 — counters re-measured; the claim in [withings-export-cli#57](https://github.com/quantcli/withings-export-cli/pull/57) holds |
| `crono-export-cli` | 3 / 0 / 0 | none | **not searched** | 2026-11-23 — counters only |
| `liftoff-export-cli` | 12 / 2 / 2 | yes (one) | **yes, see above** | 2026-11-23 |

Two of those rows mean the carve-out is **unavailable** regardless of this file: `crono-export-cli` fails the no-stars test at 3, and `liftoff-export-cli` fails all four. Only `withings-export-cli` is eligible today, and `withings-export-cli#57` is the one PR that has claimed it.

`crono-export-cli`'s "not searched" is deliberate and is the honest state: its counters are recorded, but nobody has gone looking for code that consumes its output. That search is the next action on whichever ticket touches crono's wire surface.

## Adding an entry

One row, plus a short notes block where the integration constrains us in a way §9's table does not cover. Required: the exporter, the exact surface (subcommand and codec), the consumer, the date, and a method a reader can re-run. Public source and the read-only GitHub API are the only instruments used here.
