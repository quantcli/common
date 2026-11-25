# quantcli export-CLI contract

This document is the source of truth for the user-facing surface every quantcli `*-export-cli` should adhere to. Each CLI exists to export personal data from a single upstream service (Cronometer, Liftoff, Withings, …) for use by humans on a terminal and by personal LLM agents calling the CLI as a tool.

The contract exists so that, once you have used one of these CLIs, the others feel like the same tool with a different verb.

## Status

Each cell shows whether the section is implemented and how it is attested for that CLI:

- **machine** — a [`compat/`](compat/README.md) bundle covers this cell, self-tests green in `quantcli/common`'s own CI on every PR here, *and* the exporter's own CI runs **that cell's subtests** against its built binary on every PR to its repo. All three are required: a bundle nobody runs attests nothing, and a subtest that skips — or that the exporter's CI never selects — attests nothing about the cell it would have covered.
- **local** — implemented, and the exporter's own CI runs an automated test of this cell on every PR to its repo, but that test lives in the exporter rather than in [`compat/`](compat/README.md). It sits deliberately between the two tiers around it. Stronger than **human**: it catches a regression on its own, without a reviewer having to notice. Weaker than **machine**: the test and the behavior it checks were written from one author's reading of this contract, so it cannot catch a *misreading*, and `quantcli/common`'s CI never exercises it — nothing here fails when the contract moves underneath it. Two duties come with the cell: it MUST name its test in [§8](#8-conformance-the-compat-library)'s work list, and it MUST be lifted into a `compat/` bundle once a second exporter wants the same assertion. **local** is a promotion candidate, not a destination, and where a bundle already covers the cell, wiring that bundle is the fix — the tier is not a way to decline it.
- **human** — implemented; verified only by review at merge time. A cell stays **human** when no bundle covers it, when the exporter's CI does not run the bundle function that covers it, or when it runs that bundle with this cell's subtests switched off. `Runner.SkipDataPath` and `Runner.SupportedFormats` are today's switches; both skip with a named reason rather than silently.
- **—** — not implemented yet.
- **n/a** — does not apply to this CLI's upstream (e.g. precedence rules for a token file the CLI never writes).

| Section | crono-export | liftoff-export | withings-export |
|---|---|---|---|
| Repo naming (`{service}-export-cli`) | human | human | human |
| Timezone policy | human | human | human |
| Date flags (`--since` / `--until`) — surface | **machine** | human | human |
| Date flags — local-midnight semantics | human | **machine** | **machine** |
| `--format` flag surface (documented, rejects unknown) | **machine** | **machine** | **machine** |
| `--format markdown` (default) | human | **machine** | **machine** |
| `--format json` | human | **machine** | **machine** |
| `--format csv` | — | — | **machine** |
| JSON scalar typing (§4) | — | human | — |
| Record shape — fixed key set (§4) | — | — | — |
| Units declared in key names (§4) | — | — | — |
| `json`/`csv` cross-codec parity (§4) | human | — | — |
| `auth status` subcommand | human | human | human |
| Headless auth precedence (env over token file) | n/a | human | human |
| `prime` subcommand | human | human | human |
| Hermetic `--help` / flag-validation (§7) | **machine** | human | human |

All implemented sections shipped across all three CLIs (April 25, 2026). The table is per-cell rather than per-row so partial-codec adoption (e.g. crono/liftoff implementing markdown+json but not csv) is visible without losing fidelity. Cells marked "human" are candidates for promotion to **machine** as the compat library grows and as exporters make more of an already-wired bundle actually run — see [§8](#8-conformance-the-compat-library), which lists the cells that are one small exporter change away.

---

## 1. Repo naming

Each CLI lives at `github.com/quantcli/{service}-export-cli` where `{service}` is the upstream service in lowercase (`crono`, `liftoff`, `withings`). The binary is `{service}-export`. Homebrew cask is `{service}-export`.

## 2. Timezone policy

Off-by-one-day bugs are the most common timezone failure in tools like these. All three CLIs treat dates the same way to avoid them:

- **User-supplied dates** parse in `time.Local`. Use `time.ParseInLocation("2006-01-02", s, time.Local)`, never bare `time.Parse`. Bare `time.Parse` defaults to UTC and silently shifts the calendar boundary by the user's offset.
- **API timestamps** that flow into day-bucketing or human-readable output are converted with `.Local()` before formatting. Bucketing without local conversion buckets by whatever zone the API returned, which is usually UTC, which is wrong.
- **JSON output** preserves RFC3339 with whatever offset Go's default `time.Time` marshaler emits — usually local. Don't override the marshaler.
- **CSV / markdown output** renders timestamps in the user's local zone.

## 3. Date flags

Every subcommand that selects a window of data accepts:

```
--since VALUE   inclusive lower bound
--until VALUE   inclusive upper bound (omit for "now")
```

`VALUE` is one of:

| Form | Example | Meaning |
|---|---|---|
| Keyword | `today`, `yesterday` | Local-calendar day |
| Absolute | `2026-04-15` | Local midnight that calendar day |
| Relative | `7d`, `4w`, `6m`, `1y` | N units before today's local midnight |

Window semantics are half-open `[since, until_exclusive)` internally, but `--until` is inclusive of the named day from the user's point of view: the exclusive bound is the next local calendar midnight. A local day may be 23 or 25 hours when daylight saving time changes.

### Resolved window diagnostic

Every window-selecting subcommand accepts `--print-resolved-window` (default
false). When set, it resolves its normal date arguments and defaults, prints
exactly `since=VALUE until_exclusive=VALUE` followed by a newline, and exits
without credentials or network requests. Each finite VALUE is an RFC3339
timestamp with its local offset; an absent bound is `unbounded`. The reported
half-open window must describe the same dates used by the export path. For an
inclusive calendar-day API, the exclusive end represents the following local
midnight. Invalid dates fail with a nonzero exit and no diagnostic on stdout.

Relative durations snap to local midnight, **not** "this many days before the current instant". `--since 30d` returns the same window whether you run it at 9am or 11pm.

When neither flag is given, each subcommand picks its own default (typical: `7d` for log-style commands, `30d` or `90d` for sparse commands like workouts/measurements, `1d` for dense commands like intraday).

## 4. Output format

```
--format markdown   default; fitdown-style human-readable
--format json       array on stdout, suitable for jq / LLM agents
--format csv        spreadsheet-friendly
```

A single `--format` flag, not separate `--json` / `--csv` flags. `markdown` is the default because terminals are humans by default; agents pass `--format json` once.

CLIs MAY implement a subset of these codecs. A CLI MUST implement `markdown` and `json`; `csv` is recommended but optional. The Status table tracks per-CLI implementation per codec, and `compat.Runner.SupportedFormats` lets the conformance bundle target the declared subset so a partial-codec exporter can adopt the bundle without an immediate `--format csv` failure. Declaring less than `{markdown, json}` is non-conforming — markdown is required for human terminal use, json is required for agent/script use, and either alone leaves one of the two use cases unserved.

Output rules:

- **stdout**: data only, in the requested format.
- **stderr**: errors and progress messages. Pipelines using `>` to redirect stdout do not need `2>&1`.
- **Empty result**: success with empty output (`[]` for JSON, no rows for markdown/CSV), exit code 0. Empty is not an error.
- **Exit code**: 0 when the export was delivered; non-zero when it was not — auth or network failure, an invalid flag, or output that could not be written.

**Shipped output is not reshaped for conformance.** Scalar typing and Record shape below bind new subcommands and new fields. An existing key, its nesting, or a subcommand's top-level container changes only to fix a bug a user would notice — a wrong, missing, or misleading value — and then as narrowly as the fix allows, preferring an additive field to a rename. Not yet matching these clauses is not by itself a reason to change what a CLI already emits.

### Scalar typing

§4 above pins down *which* codecs exist and the stdout/stderr split; §2 pins down timestamp formatting. This clause pins down the JSON type of an individual field, so a `jq` expression or an LLM agent can rely on the shape across every subcommand of every CLI. It governs the `json` codec; markdown and CSV are rendered text and carry no type.

- **Numeric quantities are JSON numbers, not quoted strings** — even when the upstream API sends them as strings. Decoding MAY keep the upstream shape; output MUST NOT. `"weight": 175` (`"weight": "175"` is wrong).
- **A logical field has one JSON type across every subcommand that emits it.** If `bodyweight` is a number in one place it is a number everywhere. When the value is absent it is `null` — never `""`, never `0`, so a missing measurement can't be read as a real one.
- **A human-formatted quantity carries a machine-readable sibling.** A field whose value is a display phrase (`"01 hours 06 minutes"`) MUST ship a sibling in a canonical unit next to it (`sessionDurationSeconds`). The unit belongs in the key name. This is additive — keep the display field.

Timestamps are the one deliberate exception: §2 fixes their formatting (RFC3339 with offset), so a `date` field MAY be a formatted string. Even then it is the *same* representation across every subcommand of a given CLI — one logical field, one shape.

### Record shape

Scalar typing pins down a field's *type*. This clause pins down which fields a record *has*, and where they sit. Both exist for one consumer: a `jq` path or an agent tool-call that worked yesterday works today, against a different account and a different device.

**A subcommand's key set is fixed.** The keys a record carries are a property of the subcommand, not of what the upstream happened to return. Every record in one `--format json` array carries the same keys, and so does the same subcommand run tomorrow for someone whose device reports less. A field with no value is present and `null` (scalar typing above) — not omitted. Omitting it makes "this device does not measure that" indistinguishable from "this exporter does not emit that", and leaves a consumer no field to read at all. An upstream whose response shape varies by device is the normal case, not an excuse: the exporter declares the key set and fills the gaps with `null`.

**Declared fields live at the top level of the record.** A consumer should not have to know how deep a quantity sits. Nesting is permitted where the data is genuinely a nested collection (a session containing laps), and in that case the nested records follow this clause too — fixed key set, `null` for absent.

**A raw upstream passthrough is not part of the key set.** Re-emitting an upstream blob verbatim (withings `sleep`/`workouts` `data{}`) is permitted and useful — nothing is lost in transit — but the keys inside it are upstream's choice, vary by device and account, and therefore carry no fixed-key-set promise. A quantity that appears *only* inside such a blob is not an emitted field of this contract. That cuts both ways: it is why a blob cannot satisfy parity below, and it is why a blob can later be replaced (§9, [the wire surface](#the-wire-surface)) where a declared field could not.

### Units and cross-codec parity

Scalar typing above pins down a field's JSON *type*. It does not say what the number *means*, and it explicitly disclaims the other codecs. This clause closes both gaps: what unit a number is in, and how the `json` and `csv` views of one subcommand relate.

**A dimensioned number declares its unit in its key name.** Durations, masses, distances and energies are unreadable as bare numbers — `duration` requires a guess, `duration_sec` does not. New fields MUST carry the unit in the key (`_sec`, `_min`, `_m`, `_km`, `_kg`, `_lb`, `_kcal`). This generalizes the display-value rule above: that rule says a formatted phrase needs a machine-readable sibling, and this one says a bare number needs a declared unit. Dimensionless counts (`steps`, `reps`, `wakeup_count`) and scores carry no suffix.

**One logical field, one unit — in every codec and every subcommand.** The companion to "one logical field, one JSON type". A codec MUST NOT convert on the way out: a duration that is seconds in `json` is seconds in `csv`, not minutes. Converting per codec is what makes two outputs of one subcommand disagree about the same quantity, and it leaves a consumer no way to tell a unit change from a data change.

**Where a sibling field selects the unit, the dispatch is documented.** Some upstreams reuse one column for different quantities — liftoff's `inputOne` is pounds for a weight-and-reps exercise and kilometres for a distance-and-duration one, selected by `exerciseTypes`. Re-emitting that is permitted; the upstream's shape is not ours to choose. But the dispatch field and its full mapping MUST appear in `prime` under SUBCOMMANDS, and the preferred fix is additive unit-named siblings (`weight_lb` / `distance_km`) beside the polymorphic field.

**Existing unitless names are grandfathered, not blessed.** A field already emitted under a bare name MUST have its canonical unit documented in `prime`, and SHOULD gain a unit-named sibling. Renaming the bare field is a §9 major.

**`csv` is a projection of the `json` record, not a second dataset.** `json` is the authoritative view; `csv` is a flat view of the same fields. Both exporters that added a `csv` codec after withings reached for this rule on their own and wrote it into their source, because this contract was silent — [crono](https://github.com/quantcli/crono-export-cli/blob/main/cmd/csv.go) ("one column per JSON key, named exactly as the JSON key … two views of the same rows") and [liftoff](https://github.com/quantcli/liftoff-export-cli/blob/main/cmd/csv.go) ("each column is the snake_case form of the JSON key it carries"). Neither had anything to hold it to: liftoff's codec does not in fact uphold its own stated rule on three of its four subcommands. A convention two repos describe and one of them keeps is exactly what a contract is for:

- **Every CSV column names a field the `json` codec also emits for that subcommand.** The match is on the field name, under one case convention declared per CLI in `prime` — crono maps a key to a column verbatim, liftoff maps `camelCase` to `snake_case`. Either is fine; picking neither is not.
- **CSV MAY omit and MAY denormalize.** A field that does not fit a flat row may be absent, and nested records may be flattened or expanded to one row per deepest repeated element with the parent's scalars repeated. Row *cardinality* is therefore not constrained; field *names and units* are.
- **CSV MUST NOT invent.** A column with no `json` counterpart means the `json` codec is missing a field, and the fix is to add it to `json` — not to accept two datasets. A computed convenience column (withings `sleep`'s `total_sleep_min`, which `json` has no equivalent for) is exactly this case.
- **A raw upstream passthrough does not satisfy parity.** Per [Record shape](#record-shape) the quantities inside a verbatim upstream blob are not emitted fields of this contract, so a CSV column cannot be projected from one. The named, unit-declaring, projectable fields live at the top level — either alongside the raw blob, or in place of it (see the posture list below).

**Posture when an exporter is out of conformance.** Close the gap additively, in this order, and never by reshaping what consumers already read:

1. Add the missing top-level field or unit-named sibling to `json`. Additive, `minor` per §9. For withings `sleep` this alone closes the parity gap.
2. Document the mapping in `prime` where the shapes genuinely cannot converge.
3. Rename or remove an emitted key only in a `major`, under [§9's wire-surface rules](#the-wire-surface) — release notes always, and a deprecation window unless the exporter has evidenced no consumer.

Reshaping or dropping a **declared** field to reach parity is explicitly not preferred: it breaks every consumer reading it today to solve a problem an additive sibling already solves.

A raw passthrough is the exception, and it is a one-time one. Because a blob carries no fixed-key-set promise ([Record shape](#record-shape)), an exporter MAY **replace** it with declared, unit-named top-level fields in a `major` — provided every quantity the blob carried is reachable from the new fields, with the mapping enumerated in the PR and in `prime`. Replacing with no data loss is a different act from dropping: the first moves a quantity onto stable ground, the second removes it, and only the first is permitted here. The alternative — keeping both forever — means double-reporting every quantity under two names, two shapes and (where the blob is upstream's) two unit conventions, which is the disagreement this clause exists to end. No exporter has taken this path yet: the first attempt, [withings-export-cli#57](https://github.com/quantcli/withings-export-cli/pull/57), was closed unmerged on 2026-11-24. Enumerating the mapping is still the test a replacement has to pass; there is just no landed example of one.

## 5. Auth

Auth flows differ legitimately across upstreams (env-var basic auth, OAuth2, interactive credential prompts), but two surface elements are required of every CLI:

- An `auth status` subcommand that prints one line summarizing readiness — e.g. `logged in as you@example.com (token expires 2026-05-01)` or `missing CRONOMETER_PASSWORD`. Exit code 0 if usable, non-zero otherwise.
- A headless path: where the upstream's auth model permits it, every CLI accepts environment variables that let a fresh container run without an interactive login. The variable names are `{SERVICE}_*` (e.g. `CRONOMETER_PASSWORD`, `WITHINGS_REFRESH_TOKEN`). Document them in `prime`.

### Precedence

Where a CLI has both a headless variable and a saved token file, the environment wins:

- **`{SERVICE}_REFRESH_TOKEN` (or the equivalent headless variable) takes precedence over any saved token file when it is set.** A container with a stale mounted config and a freshly injected secret must use the secret. Precedence is uniform across the family — an invocation that honors the variable in one CLI must not silently ignore it in another.
- `auth status` reports the environment as the source and exits 0 without a network call. Exit 0 there means "a token was supplied", not "the token works" — validity is unknowable locally.
- Where the upstream **rotates** refresh tokens on each use, the headless path writes the rotated value back to the token file on a best-effort basis, so a repeat caller can read it and re-inject it. A failed write there is a stderr warning, not an error (§4): a minted access token is usable whether or not the cache write landed, and read-only rootfs is a normal container shape.

Where the upstream forbids headless auth (e.g. interactive OAuth consent), `auth status` still works and the CLI still exposes `auth login` / `auth logout` / `auth refresh`.

## 6. The `prime` subcommand

Every CLI exposes a `prime` subcommand that prints a single-screen primer aimed at LLM agents calling the CLI as a tool. Same section structure across all three so an agent that has read one knows where to look in another:

```
WHAT IT IS    one paragraph
I/O CONTRACT  stdout/stderr/exit-code/format
AUTH          env vars + auth subcommands
DATE FLAGS    link to this contract; CLI-specific defaults
SUBCOMMANDS   one block per, with output schema sketch
EXAMPLES      3-5 jq recipes for common questions
GOTCHAS       non-obvious pitfalls
```

Prime is short. It is not a man page. If it grows past one terminal screen, something belongs in this contract instead.

## 7. Hermeticity

A `--help` invocation and any flag-validation failure that exits non-zero before subcommand work begins must make no network requests. This lets an LLM agent or an offline user introspect the CLI — discover what it does and how to call it — without an authenticated session or a working network. "Before subcommand work" includes flag-parse failures inside subcommands (e.g. `liftoff-export workouts list --since lol`): cobra parses subcommand flags inside the subcommand layer, but that parse runs before any data fetch, so it is also a §7 case.

The [`compat/`](compat/README.md) library pins this down by running the `--help` path and the flag-parse-failure path under a no-network environment; per-repo wiring of that library is described in [§8](#8-conformance-the-compat-library).

Out of scope: subcommands that do real work (`auth status`, `prime`, data subcommands) — by construction they need network access, and this section does not constrain them. `--version` is not yet specified here; it is a candidate for a future revision once a compat test pins it down. When that revision lands, `--version` joins this rule — version banners must not dial out (e.g. for update checks) without explicit user opt-in.

## 8. Conformance (the compat library)

Conformance to this contract is verified by [`compat/`](compat/README.md), a small black-box Go test library that lives in this repo and is imported by every `*-export-cli` from its own CI. All three exporters run a bundle today. What still differs per cell is how much of each bundle actually asserts against a given exporter. Two things narrow it: a bundle wired in with `Runner.SkipDataPath: true` runs only the parse-level subtests, so the codec cells it would otherwise cover stay **human** until that exporter's data path runs in CI; and each bundle entry point (`dates.RunContract`, `dates.RunResolvedWindow`, `formats.RunContract`) attests only where an exporter's test calls it *and* its CI `-run` filter selects that test. The current bundles:

- [`compat/dates`](compat/README.md) — `dates.RunContract` pins down §3's surface: that `--since` / `--until` are documented in `--help`, and that an invalid value exits non-zero with a stderr-only error. It also pins down [§7](#7-hermeticity) for both the `--help` path and the date-parse-failure path. `dates.RunResolvedWindow` is a separate entry point that pins down §3's local-midnight semantics and nothing else.
- [`compat/formats`](compat/README.md) — pins down §4: that `--format` is documented, an unknown value exits non-zero with a stderr-only error, `--format json` emits a parseable JSON array, `--format csv` emits at least a header row, and the default is byte-identical to `--format markdown`. The bundle additionally pins down [§7](#7-hermeticity) for the `--format` parse-failure path.
- [`compat/parity`](compat/README.md) — `parity.RunCrossCodec` pins down the machine-checkable parts of two §4 clauses. `CSVColumnsExistInJSON` covers [Units and cross-codec parity](#units-and-cross-codec-parity): every `--format csv` column names a field `--format json` also emits for the same subcommand. It compares names only, ignoring case and `_` separators, so both declared case conventions pass and only a genuinely invented or renamed column fails. `JSONKeySetIsUniform` covers [Record shape](#record-shape): every record in one `--format json` array carries the same top-level keys, which is the device-dependent-key-set defect in the form a black-box test can see it. What stays review-attested: the unit half of the parity clause (a number carries no unit at runtime, so no bundle can tell seconds from minutes), nested key sets, key-set stability across accounts, and the `prime` documentation duties. The bundle needs a working data path with at least one record — two for the key-set subtest — and skips with a named reason otherwise, so an exporter cannot pass it vacuously by exporting nothing.

A new exporter is not "in" the family until its CI runs at least the `dates` bundle green. The `formats` bundle ships with a `Runner.SupportedFormats` affordance so partial-codec exporters can adopt it without an immediate `--format csv` failure: declare the codecs you actually implement and the bundle skips codec-specific subtests for the rest. A per-codec cell flips from `human` to **machine** when the bundle's subtest for that codec actually *runs* against the exporter — not merely when the bundle is wired in. Codecs not yet implemented stay at `—` until the writer lands.

Where the Status table stands against that rule today, so the remaining `human` cells in bundle-covered rows read as a work list rather than a mystery:

- **crono-export** calls all three entry points, but its CI filter (`-run 'TestContract(Dates|Formats)'`) does not select its `TestContractResolvedWindow`, so the local-midnight cell is `human`; and it runs `formats.RunContract` with `SkipDataPath: true`, so its codec cells are `human` while its `--format` flag-surface cell is **machine**.
- **liftoff-export** and **withings-export** call `formats.RunContract` and `dates.RunResolvedWindow` but not `dates.RunContract`, so their date-flag surface cells are `human`. Their §7 cells are `human` for the same reason: only the `--format` parse-failure path is exercised there, not the `--help` path or the date-parse-failure path.

`dates.RunContract` is entirely parse-level and needs no credentials or stub, so each of those gaps is a one-function or one-regex change in the exporter's repo.

The §4 record-shape, unit-declaration and parity rows are `—` across the board because they are new, and `—` there is a statement about attestation, not a verdict:

These are recorded, not scheduled: shipped output is grandfathered ([§4](#4-output-format)).

- **liftoff-export**'s `json` codec may omit keys where [Record shape](#record-shape) asks for `null`; its `csv` codec handles that as an empty cell.
- **withings-export**'s `sleep`/`workouts` re-emit upstream `data{}`, whose keys follow the user's device. Its `csv` codec leaves a value the device did not report empty rather than `0`.
- **crono-export** has not been audited against these clauses.

### Running the data path without credentials

The codec subtests need the binary to reach a successful export. An exporter SHOULD make that possible without credentials, by providing an API-base override and a headless auth path that a test can point at an in-process stub of the upstream — the binary under test stays the one CI ships. `Runner.SkipDataPath: true` is the explicit fallback and leaves §4 attested at parse level only. See [`compat/README.md`](compat/README.md#stubbing-the-upstream) for the pattern and the per-exporter specifics.

`compat/parity` raises that bar one step: a stub that answers with the *empty* result is enough for the `formats` bundle but not for `parity`, which has no JSON keys to compare CSV columns against and skips. An exporter adopting `parity` needs its stub to return at least one record per subcommand. Crono's and liftoff's stubs can already do this (liftoff's `stubapi.SamplePosts` ships a non-empty fixture; crono's `cronotest.Fake` takes per-subcommand CSV bodies); withings's returns `{"status":0,"body":{}}` to every call and needs a non-empty fixture before its cells can move.

## 9. Versioning and releases

Semantic versioning. User-visible bug fix → patch. New subcommand or flag → minor. Removed/renamed flag → major. Releases cut via `gh release create` against the relevant tag; goreleaser builds binaries for darwin/linux/windows × amd64/arm64 and publishes the cask to `quantcli/homebrew-tap`.

### The wire surface

Every case above is a **flag**. The record a subcommand emits is a surface too, and a tighter one: a renamed flag breaks a shell script, a renamed `json` key breaks every `jq` pipeline and every agent calling the CLI as a tool. Classify a change to what a subcommand emits as:

| change to the emitted record | bump |
|---|---|
| add a field | `minor` |
| make a key that was previously omitted always present ([§4 record shape](#record-shape)) | `minor` |
| rename or remove a `json` key | `major` |
| move a declared field between nesting levels (flatten or re-nest) | `major` |
| change the top-level container of a subcommand's output (object ↔ array) | `major`, window impossible |
| replace a raw upstream passthrough with declared fields, no data loss ([§4 posture](#units-and-cross-codec-parity)) | `major` |
| change an existing field's unit under the same key | **not permitted** — see below |

CSV columns follow the `json` field they project — a column's bump is its field's bump. Dropping a column while its `json` field stays is a `minor`, because §4 lets `csv` omit.

**Every `major` on this list carries release notes** naming the old path and the new one, per change, as a table. Hand-written GitHub release notes are the family's mechanism and are sufficient; no repo keeps a `CHANGELOG.md` and this clause does not ask for one. (If files are ever wanted they are a separate change to this section, not an obligation inherited from it.)

**A deprecation window — both spellings emitted for a release — is REQUIRED where the exporter has a known consumer.** Note this is a *new* obligation, not one inherited from the flag rules above: §9's flag cases carry no window today, only a bump. The asymmetry is deliberate rather than an oversight. A removed flag fails loudly and immediately on the next invocation, and the caller is usually a person who can read the error; a renamed `json` key fails inside a `jq` pipeline or an agent's tool-call, where the consumer is a program and the symptom is a missing value rather than a message. Whether the flag cases should grow the same window is a real question and not one this clause answers — it is scoped to the wire surface.

Two carve-outs, each of which must be *stated with its evidence* in the PR and repeated in the release notes:

- **No known consumer.** Where the repo has no stars, forks, watchers beyond the maintainers, and no third-party-authored issues, a `major` MAY replace the old spellings directly. The PR states those numbers as of that day. The carve-out is about evidence, not convenience: it turns "nobody is reading this" into a recorded, checkable claim that a reviewer can re-run, and it expires the moment the numbers change.
- **A window is physically impossible.** One document cannot be both an object and an array, so the container-change row above cannot emit both spellings at once. State that, and carry the before/after shape in the release notes instead.

Nothing here lets a popular exporter skip the window. The two carve-outs are narrow and independent: an impossible window does not imply no consumers, and no consumers does not make a window impossible.

**A unit change is performed as a rename, never in place.** Because §4 puts the unit in the key name, seconds → minutes *is* `duration_sec` → `duration_min`: a rename, a `major`, and a missing key the first time a consumer runs it. Changing the number under a key that keeps its name is the only change on this list that no version number protects against — the output still parses, every arithmetic result is silently wrong by a constant factor, and a consumer who did not read the release notes has no way to find out. So it is not a `major`; it is not done. For a field grandfathered under a bare name (§4), the unit documented in `prime` is thereby fixed: if it is wrong, add a unit-named sibling and deprecate the bare field rather than redefining what the bare field means.

---

This contract is a living document. A change here is a change every CLI agrees to make. Open a PR against this repo before changing the surface in any individual CLI.
