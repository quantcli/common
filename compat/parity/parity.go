// Package parity is the compat test bundle for the cross-codec half of
// §4's "Units and cross-codec parity" clause in CONTRACT.md.
//
// What is machine-attested here:
//
//   - Every column in `--format csv` names a field that `--format json`
//     also emits for the same subcommand.
//
// That is the whole of it, and the narrowness is deliberate. The clause
// has two halves and only one of them is observable from outside the
// binary:
//
//   - CONTRACT §4: "CSV MUST NOT invent." is checkable. A column with no
//     json counterpart is visible by comparing two invocations.
//   - CONTRACT §4: "One logical field, one unit — in every codec and
//     every subcommand." is NOT checkable. A JSON number carries no unit
//     at runtime, so a bundle cannot tell seconds from minutes; it can
//     only notice when the *name* changed with the unit (`duration_sec`
//     vs `duration_min`), which the invented-column check already
//     catches. Unit correctness, and the `prime` documentation duties
//     for dispatch fields and grandfathered bare names, stay
//     review-attested.
//
// Name comparison ignores case and `_`/`-` separators, because §4 lets
// each CLI declare one case convention: crono maps a JSON key to a
// column verbatim, liftoff maps `camelCase` to `snake_case`. Both pass;
// a renamed or invented column still fails. Row cardinality is not
// compared at all — §4 explicitly permits CSV to denormalize a nested
// record into one row per deepest repeated element.
//
// The bundle needs a data path that returns at least one record. With
// zero records there are no JSON keys to compare against, so it skips
// with a named reason rather than passing vacuously — an exporter
// cannot earn this cell by exporting nothing.
//
// Exporter usage:
//
//	//go:build compat
//	package mycli_compat_test
//
//	import (
//	    "os"
//	    "testing"
//	    "github.com/quantcli/common/compat"
//	    "github.com/quantcli/common/compat/parity"
//	)
//
//	func TestContractParity(t *testing.T) {
//	    bin := os.Getenv("EXPORT_CLI_BIN")
//	    if bin == "" { t.Skip("EXPORT_CLI_BIN not set") }
//	    parity.RunCrossCodec(t, compat.Runner{Binary: bin})
//	}
package parity

import (
	"encoding/csv"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/quantcli/common/compat"
)

// RunCrossCodec runs the cross-codec parity bundle against r. It is the
// only function exporters are expected to call.
//
// If r.Subcommands is non-empty, RunCrossCodec iterates the list and
// runs the assertion once per subcommand under a "subcommand=NAME"
// t.Run group, mirroring compat/dates and compat/formats. Parity is a
// per-subcommand property — §4 scopes it to "the same subcommand" —
// so a CLI whose codecs agree on one subcommand and disagree on
// another fails only the offending subtest.
func RunCrossCodec(t *testing.T, r compat.Runner) {
	t.Helper()
	if r.Binary == "" {
		t.Fatal("parity: compat.Runner.Binary is empty")
	}
	if r.SupportedFormats != nil && (!r.SupportsFormat(compat.FormatMarkdown) || !r.SupportsFormat(compat.FormatJSON)) {
		t.Fatalf("compat/parity: Runner.SupportedFormats=%v declares less than the §4 floor {markdown, json}; see CONTRACT.md §4", r.SupportedFormats)
	}

	if len(r.Subcommands) == 0 {
		runOne(t, r)
		return
	}
	for _, sub := range r.Subcommands {
		sub := sub
		t.Run("subcommand="+sub, func(t *testing.T) {
			runOne(t, r.WithSubcommand(sub))
		})
	}
}

// runOne runs the parity assertions against a single invocation
// surface — either the root binary (when r has no subcommand prefix)
// or a specific subcommand of it.
func runOne(t *testing.T, r compat.Runner) {
	t.Helper()
	t.Run("CSVColumnsExistInJSON", func(t *testing.T) {
		csvColumnsExistInJSON(t, r)
	})
}

// csvColumnsExistInJSON asserts the machine-checkable half of §4's
// cross-codec parity rule: every `--format csv` column names a field
// the `--format json` codec also emits for the same subcommand.
//
// Skipped (not failed) when either codec is undeclared, when
// Runner.SkipDataPath is set, or when the data path returns no
// records — each with a named reason, so a skip reads as a gap in
// attestation rather than a pass.
func csvColumnsExistInJSON(t *testing.T, r compat.Runner) {
	t.Helper()
	if r.SkipDataPath {
		t.Skipf("data-path subtests disabled via Runner.SkipDataPath")
	}
	if !r.SupportsFormat(compat.FormatJSON) {
		t.Skipf("--format json not declared in Runner.SupportedFormats")
	}
	if !r.SupportsFormat(compat.FormatCSV) {
		t.Skipf("--format csv not declared in Runner.SupportedFormats; parity has no second codec to compare against")
	}

	records := jsonRecords(t, r)
	if len(records) == 0 {
		t.Skipf("--format json returned an empty result set; no JSON keys to compare CSV columns against. " +
			"Point the bundle at a data path that yields at least one record (see compat/README.md#stubbing-the-upstream)")
	}

	// Union the keys across every record, not just the first. §4 says
	// an absent value is `null`, but an exporter may instead omit the
	// key (liftoff's csv codec documents "a key JSON omits" as a real
	// case), so a single sparse first record would under-report the
	// available field set and produce false failures.
	available := map[string]string{}
	for _, rec := range records {
		collectKeys("", rec, available)
	}

	columns := csvHeader(t, r)
	if len(columns) == 0 {
		t.Fatalf("--format csv produced no header row; nothing to compare")
	}

	var missing []string
	for _, col := range columns {
		if col == "" {
			continue
		}
		if _, ok := available[normalizeName(col)]; !ok {
			missing = append(missing, col)
		}
	}
	if len(missing) == 0 {
		return
	}

	t.Errorf("%d CSV column(s) name no field that --format json emits for this subcommand "+
		"(CONTRACT.md §4, cross-codec parity: csv is a projection of the json record, not a second dataset):\n"+
		"  invented: %s\n"+
		"  json emits: %s\n"+
		"Fix by adding the field to the json codec — additively, per the §4 posture list — not by dropping the CSV column.",
		len(missing), strings.Join(missing, ", "), strings.Join(sortedOriginals(available), ", "))
}

// jsonRecords runs `--format json` and returns the decoded top-level
// array. §4 requires the json codec to emit an array on stdout, so a
// non-array is a failure here rather than a skip.
func jsonRecords(t *testing.T, r compat.Runner) []any {
	t.Helper()
	res := r.MustRun(t, "--format", "json")
	if res.ExitCode != 0 {
		t.Fatalf("--format json exited %d; stderr=%q", res.ExitCode, res.StderrString())
	}
	trimmed := strings.TrimSpace(res.StdoutString())
	if trimmed == "" {
		t.Fatalf("--format json produced empty stdout; want a JSON array (`[]` for an empty result per §4)")
	}
	var arr []any
	if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
		t.Fatalf("--format json stdout is not a JSON array: %v; stdout=%q", err, res.StdoutString())
	}
	return arr
}

// csvHeader runs `--format csv` and returns the parsed header row.
//
// The reader is configured with FieldsPerRecord = -1 so a denormalized
// body with varying row widths does not fail the parse before the
// header is read. Only the header participates in the comparison.
func csvHeader(t *testing.T, r compat.Runner) []string {
	t.Helper()
	res := r.MustRun(t, "--format", "csv")
	if res.ExitCode != 0 {
		t.Fatalf("--format csv exited %d; stderr=%q", res.ExitCode, res.StderrString())
	}
	reader := csv.NewReader(strings.NewReader(res.StdoutString()))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		t.Fatalf("--format csv stdout is not parseable CSV: %v; stdout=%q", err, res.StdoutString())
	}
	return header
}

// collectKeys walks a decoded JSON value and records every field name a
// CSV column could legitimately be projected from, keyed by its
// normalized form with the most readable original spelling as the
// value (used only for the failure diagnostic).
//
// For each key of an object it registers both the full path joined with
// `_` (`sleep_deep_sec`) and the bare leaf name (`deep_sec`). §4 permits
// either projection: crono flattens by key name and liftoff
// denormalizes nested records to one row per deepest repeated element,
// which drops the ancestor prefix. Registering both keeps the check
// sound against invented columns without prescribing one flattening
// style.
//
// Arrays recurse with the prefix unchanged — a denormalized row's
// columns come from the element's own keys, not from an element index.
func collectKeys(prefix string, v any, out map[string]string) {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			path := k
			if prefix != "" {
				path = prefix + "_" + k
			}
			register(out, path)
			register(out, k)
			collectKeys(path, child, out)
		}
	case []any:
		for _, elem := range val {
			collectKeys(prefix, elem, out)
		}
	}
}

// register adds name to out under its normalized form. When two
// spellings normalize alike the shorter original wins, so the
// diagnostic prefers `deep_sec` over a long synthesized path.
func register(out map[string]string, name string) {
	key := normalizeName(name)
	if key == "" {
		return
	}
	if existing, ok := out[key]; ok && len(existing) <= len(name) {
		return
	}
	out[key] = name
}

// normalizeName collapses a field name to the form the comparison runs
// on: lowercase, with `_`, `-` and spaces removed.
//
// This is what lets both §4-declared case conventions pass — crono's
// verbatim `hr_average` and liftoff's `camelCase`→`snake_case`
// (`sessionDurationSeconds` ↔ `session_duration_seconds`) normalize to
// the same string. It is deliberately not a fuzzy match: `duration`
// and `duration_sec` normalize differently, so the unit-suffix drift
// behind withings-export-cli#20 still fails.
func normalizeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch r {
		case '_', '-', ' ':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sortedOriginals returns the original spellings of every collected
// field name, sorted, for the failure diagnostic.
func sortedOriginals(available map[string]string) []string {
	out := make([]string, 0, len(available))
	for _, original := range available {
		out = append(out, original)
	}
	sort.Strings(out)
	return out
}
