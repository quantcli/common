// Command stubcli is a tiny contract-compliant stand-in used by the
// compat library's own tests. It is NOT a usable export-cli. It exists
// only so the dates compat suite can run against a known-good binary in
// quantcli/common's CI, proving the library itself works end-to-end
// before any real exporter wires it up.
//
// stubcli has two modes, selected by the STUBCLI_MODE env var:
//
//   - "" (default) / "flat": contract surface lives on the root
//     binary, mirroring single-purpose CLIs. Root --help mentions
//     --since, --until, and --format and exits 0.
//   - "cobra": contract surface lives on a `biometrics` subcommand,
//     mirroring cobra-based exporters (crono, liftoff, withings).
//     Root --help lists subcommands but does NOT mention
//     --since/--until/--format; passing them to the root binary
//     fails. `biometrics --help` mentions them and the parse path is
//     the same.
//
// The two modes let both compat suites (dates, formats) self-test
// against both Runner shapes: a flat Runner against the default
// mode, and a Runner with Subcommands=["biometrics"] against cobra
// mode. If the subcommand dispatch in compat.Runner regresses, the
// cobra-mode self-tests fail at the help-documents-flag assertion
// because root --help no longer carries the flags.
//
// The stub responds to --format by emitting an empty data set in
// the requested codec: `[]` for json, a single header row for csv,
// nothing for markdown. That makes the formats compat suite's
// data-path assertions (JSONIsArray, CSVHasHeader, DefaultIsMarkdown)
// runnable against the stub without any upstream API.
//
// STUBCLI_FORMATS, when set, restricts the codecs the stub will
// accept at parse time. It is a comma-separated list, e.g.
// "markdown,json" to simulate a partial-codec exporter (the
// crono/liftoff shape today). Codecs not in the list are rejected
// with the same non-zero/stderr behavior as an unknown format. The
// compat/formats self-test uses this to prove that
// Runner.SupportedFormats actually skips the missing codec's subtest
// rather than running it against a stub that would reject the call.
// When unset, all three §4 codecs are accepted.
//
// STUBCLI_NEEDS_TOKEN=1, when set, makes the stub simulate a
// credentialless-CI failure: `--help` and `--format <unknown>` still
// behave normally (the hook does not fire during parse), but a
// successful parse of `--format <known codec>` does NOT emit data —
// instead the stub exits non-zero with `error: not logged in` on
// stderr and an empty stdout. This is the failure mode real
// credentialed exporters (crono / liftoff / withings) hit in CI:
// flag parsing succeeds, the data call fails because no token is
// available. The compat/formats self-test uses this to prove that
// Runner.SkipDataPath is load-bearing against the token-style
// adversarial model — not just the broader STUBCLI_FORMATS=__never__
// reject-everything model.
//
// STUBCLI_DATASET selects which data set the stub emits. Unset (the
// default) is the empty result the dates and formats suites expect.
// "parity" emits one record whose CSV header is a conforming
// projection of its JSON keys; "drift" emits the same record with a
// CSV header carrying unit-converted and computed columns the JSON
// codec has no fields for. The compat/parity self-tests use the pair
// to prove the bundle passes a conforming CLI and fails a drifting
// one — a bundle that only ever ran against a conforming stub would
// attest nothing.
//
// stubcli never makes a network request.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const cobraSubcommand = "biometrics"

func main() {
	mode := os.Getenv("STUBCLI_MODE")
	args := os.Args[1:]

	switch mode {
	case "", "flat":
		runFlat(args)
	case "cobra":
		runCobra(args)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown STUBCLI_MODE=%q\n", mode)
		os.Exit(2)
	}
}

// runFlat is the original stubcli behavior: --since, --until, and
// --format parse at the root binary. Used by the flat-mode
// self-tests.
func runFlat(args []string) {
	if len(args) > 0 && args[0] == "--help" {
		fmt.Fprintln(os.Stdout, "stubcli — contract-compliant test stand-in (flat mode)")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "  --since VALUE   inclusive lower bound (local date)")
		fmt.Fprintln(os.Stdout, "  --until VALUE   inclusive upper bound (local date)")
		fmt.Fprintln(os.Stdout, "  --format VALUE  output format (markdown|json|csv); default markdown")
		os.Exit(0)
	}
	parseAndEmit("stubcli", args)
}

// runCobra is the subcommand-style behavior: root --help lists
// subcommands without mentioning the date flags; only the named
// subcommand owns --since/--until. Mirrors how cobra-based CLIs (e.g.
// crono biometrics, liftoff workouts) expose the contract surface.
func runCobra(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: subcommand required")
		os.Exit(2)
	}

	switch args[0] {
	case "--help":
		// Root help intentionally omits --since/--until/--format.
		// This is what proves the subcommand dispatch is real: if
		// compat.Runner silently drops the subcommand prefix, the
		// dates and formats help assertions run against this output
		// and fail.
		fmt.Fprintln(os.Stdout, "stubcli — contract-compliant test stand-in (cobra mode)")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Available subcommands:")
		fmt.Fprintln(os.Stdout, "  "+cobraSubcommand+"   sample data subcommand (owns --since/--until/--format)")
		os.Exit(0)
	case cobraSubcommand:
		// fall through to subcommand-arg handling
	default:
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q\n", args[0])
		os.Exit(2)
	}

	subArgs := args[1:]
	if len(subArgs) > 0 && subArgs[0] == "--help" {
		fmt.Fprintln(os.Stdout, "stubcli "+cobraSubcommand+" — contract-surface subcommand")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "  --since VALUE   inclusive lower bound (local date)")
		fmt.Fprintln(os.Stdout, "  --until VALUE   inclusive upper bound (local date)")
		fmt.Fprintln(os.Stdout, "  --format VALUE  output format (markdown|json|csv); default markdown")
		os.Exit(0)
	}
	parseAndEmit("stubcli "+cobraSubcommand, subArgs)
}

// parseAndEmit is the shared flag validator + (empty) data emitter.
// It is used by both modes so the parse-error behavior (non-zero
// exit, stderr message, empty stdout) and the per-format output
// shape are identical regardless of where the flags live.
//
// Validation order is `--format` first, then `--since` / `--until`.
// The dates compat suite passes `--since obviously-not-a-date` with
// no `--format`, so the default-markdown format passes the format
// check and the date check still fires. The formats compat suite
// passes `--format obviously-not-a-format` with no date flags, so
// the format check fires before reaching the date check.
//
// `--since` and `--until` are optional in stub mode: an empty value
// is treated as "no window filter" so the formats suite's data-path
// subtests (which pass no date flags) reach the emitter.
func parseAndEmit(progName string, args []string) {
	fs := flag.NewFlagSet(progName, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	since := fs.String("since", "", "inclusive lower bound")
	until := fs.String("until", "", "inclusive upper bound")
	window := fs.Bool("print-resolved-window", false, "print resolved window")
	format := fs.String("format", "markdown", "output format (markdown|json|csv)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	if !formatAccepted(*format) {
		fmt.Fprintf(os.Stderr, "error: invalid value for --format: %q\n", *format)
		os.Exit(2)
	}
	if *since != "" && !validDate(*since) {
		fmt.Fprintf(os.Stderr, "error: invalid value for --since: %q\n", *since)
		os.Exit(2)
	}
	if *until != "" && !validDate(*until) {
		fmt.Fprintf(os.Stderr, "error: invalid value for --until: %q\n", *until)
		os.Exit(2)
	}

	if *window {
		start, err := time.ParseInLocation("2006-01-02", *since, time.Local)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		end, err := time.ParseInLocation("2006-01-02", *until, time.Local)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Printf("since=%s until_exclusive=%s\n", start.Format(time.RFC3339), end.AddDate(0, 0, 1).Format(time.RFC3339))
		return
	}

	// STUBCLI_NEEDS_TOKEN=1 simulates a credentialless-CI failure:
	// flags parsed cleanly, but the data call fails because no token
	// is available. Mirrors the shape real credentialed exporters
	// (crono / liftoff / withings) hit when run from a CI environment
	// without secrets. Exit code 1 (data-call failure) is distinct
	// from the 2 used for parse failures above, so a test that
	// expected a parse-rejection but accidentally took the data path
	// sees a different shape.
	if os.Getenv("STUBCLI_NEEDS_TOKEN") == "1" {
		fmt.Fprintln(os.Stderr, "error: not logged in")
		os.Exit(1)
	}

	emitDataset(*format)
}

// emitDataset writes the stub's data set in the requested codec.
//
// With STUBCLI_DATASET unset the stub reports the empty data set, which
// is what the dates and formats suites expect. Per CONTRACT §4: `[]`
// for json, header-only for csv, nothing for markdown.
//
// The "parity" and "drift" datasets exist for the compat/parity
// suite, which needs at least one record to have any JSON keys to
// compare CSV columns against. Both emit the same JSON record; they
// differ only in the CSV header, so the parity bundle is the single
// variable between a pass and a fail.
func emitDataset(format string) {
	switch os.Getenv("STUBCLI_DATASET") {
	case "parity":
		emitParityDataset(format, parityCSVHeader)
	case "drift":
		emitParityDataset(format, driftCSVHeader)
	case "camel":
		emitDatasetPair(format, camelRecordJSON, camelCSVHeader, "90 kg  x5")
	case "":
		switch format {
		case "json":
			fmt.Fprintln(os.Stdout, "[]")
		case "csv":
			fmt.Fprintln(os.Stdout, "date,value")
		case "markdown":
			// empty stdout
		}
	default:
		fmt.Fprintf(os.Stderr, "error: unknown STUBCLI_DATASET=%q\n", os.Getenv("STUBCLI_DATASET"))
		os.Exit(2)
	}
}

// parityRecordJSON is one record exercising every projection shape
// CONTRACT §4 permits a CSV column to be derived from: a top-level
// scalar (`date`), a top-level unit-named number (`duration_sec`), a
// nested object (`sleep.deep_sec`, reachable as either
// `sleep_deep_sec` or `deep_sec`), and an object inside an array
// (`tags[].name`, reachable as `name` after denormalization).
const parityRecordJSON = `[{"date":"2026-11-20","duration_sec":3600,` +
	`"sleep":{"deep_sec":1200,"hr_average":52},"tags":[{"name":"nap"}]}]`

// parityCSVHeader is conforming: every column names a field
// parityRecordJSON emits. It deliberately mixes a full flattened path
// (`sleep_deep_sec`), a bare nested leaf (`hr_average`), and an
// array-element leaf (`name`) so the bundle's acceptance of all three
// projections is pinned down rather than assumed.
const parityCSVHeader = "date,duration_sec,sleep_deep_sec,hr_average,name"

// driftCSVHeader is the withings `sleep` failure reproduced in
// miniature: `duration_min` is a unit-converted column the json codec
// has no field for, and `total_sleep_min` is a computed convenience
// column with no json equivalent at all. Both are the "CSV MUST NOT
// invent" case, and the parity bundle must fail on them.
const driftCSVHeader = "date,duration_min,total_sleep_min"

// camelRecordJSON is the liftoff shape: camelCase JSON keys, including
// the `exerciseTypes` dispatch field and the polymorphic `inputOne`
// that CONTRACT §4's unit-dispatch rule names explicitly.
const camelRecordJSON = `[{"sessionDurationSeconds":3600,"exerciseTypes":"WR","inputOne":110.2}]`

// camelCSVHeader is the snake_case projection of camelRecordJSON. §4
// permits a CLI to declare one case convention, so this header is
// conforming even though no column matches a JSON key byte-for-byte.
// It is the case that would fail if the parity bundle compared names
// literally.
const camelCSVHeader = "session_duration_seconds,exercise_types,input_one"

func emitParityDataset(format, header string) {
	emitDatasetPair(format, parityRecordJSON, header, "2026-11-20  3600s")
}

// emitDatasetPair writes one record in the requested codec: the JSON
// array verbatim, or the CSV header plus one empty body row. The
// parity bundle reads only the header, but emitting a row keeps the
// stub a plausible CSV producer for any future suite.
func emitDatasetPair(format, recordJSON, header, markdown string) {
	switch format {
	case "json":
		fmt.Fprintln(os.Stdout, recordJSON)
	case "csv":
		fmt.Fprintln(os.Stdout, header)
		fmt.Fprintln(os.Stdout, strings.Repeat(",", strings.Count(header, ",")))
	case "markdown":
		fmt.Fprintln(os.Stdout, markdown)
	}
}

// formatAccepted reports whether the stub should treat name as a
// valid --format value. It honors STUBCLI_FORMATS for partial-codec
// simulation; when unset, all three §4 codecs are accepted.
//
// Unknown codecs (anything outside §4) are always rejected so the
// formats bundle's UnknownFormatFails subtest still fires regardless
// of STUBCLI_FORMATS.
func formatAccepted(name string) bool {
	allowed := os.Getenv("STUBCLI_FORMATS")
	if allowed == "" {
		switch name {
		case "markdown", "json", "csv":
			return true
		}
		return false
	}
	for _, candidate := range strings.Split(allowed, ",") {
		if strings.TrimSpace(candidate) == name {
			return true
		}
	}
	return false
}

// validDate is intentionally narrow: it accepts only what the stub needs
// to fail the compat suite's "obviously-not-a-date" probe. It is not a
// faithful implementation of CONTRACT.md §3 parsing.
func validDate(s string) bool {
	if s == "" {
		return false
	}
	switch s {
	case "today", "yesterday":
		return true
	}
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return true // YYYY-MM-DD shape; we do not validate the calendar.
	}
	return false
}
