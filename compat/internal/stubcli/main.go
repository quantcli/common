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
// stubcli never makes a network request.
package main

import (
	"flag"
	"fmt"
	"os"
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
	format := fs.String("format", "markdown", "output format (markdown|json|csv)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	switch *format {
	case "markdown", "json", "csv":
		// ok
	default:
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

	// The stub has no upstream, so it always reports the empty data
	// set. Per CONTRACT §4: `[]` for json, header-only for csv,
	// nothing for markdown.
	switch *format {
	case "json":
		fmt.Fprintln(os.Stdout, "[]")
	case "csv":
		fmt.Fprintln(os.Stdout, "date,value")
	case "markdown":
		// empty stdout
	}
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
