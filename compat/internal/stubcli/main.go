// Command stubcli is a tiny contract-compliant stand-in used by the
// compat library's own tests. It is NOT a usable export-cli. It exists
// only so the dates compat suite can run against a known-good binary in
// quantcli/common's CI, proving the library itself works end-to-end
// before any real exporter wires it up.
//
// stubcli has two modes, selected by the STUBCLI_MODE env var:
//
//   - "" (default) / "flat": date flags live on the root binary,
//     mirroring single-purpose CLIs. Root --help mentions --since and
//     --until and exits 0.
//   - "cobra": date flags live on a `biometrics` subcommand, mirroring
//     cobra-based exporters (crono, liftoff, withings). Root --help
//     lists subcommands but does NOT mention --since/--until; passing
//     --since to the root binary fails. `biometrics --help` mentions
//     them and the parse path is the same.
//
// The two modes let the dates compat suite self-test both Runner
// shapes: a flat Runner against the default mode, and a Runner with
// Subcommands=["biometrics"] against cobra mode. If the subcommand
// dispatch in compat.Runner regresses, the cobra-mode self-test fails
// at HelpDocumentsDateFlags because root --help no longer carries the
// flags.
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

// runFlat is the original stubcli behavior: --since and --until parse
// at the root binary. Used by the flat-mode self-test.
func runFlat(args []string) {
	if len(args) > 0 && args[0] == "--help" {
		fmt.Fprintln(os.Stdout, "stubcli — contract-compliant test stand-in (flat mode)")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "  --since VALUE   inclusive lower bound (local date)")
		fmt.Fprintln(os.Stdout, "  --until VALUE   inclusive upper bound (local date)")
		os.Exit(0)
	}
	parseDateFlags("stubcli", args)
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
		// Root help intentionally omits --since/--until. This is what
		// proves the subcommand dispatch is real: if compat.Runner
		// silently drops the subcommand prefix, HelpDocumentsDateFlags
		// runs against this output and fails.
		fmt.Fprintln(os.Stdout, "stubcli — contract-compliant test stand-in (cobra mode)")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Available subcommands:")
		fmt.Fprintln(os.Stdout, "  "+cobraSubcommand+"   sample data subcommand (owns --since/--until)")
		os.Exit(0)
	case cobraSubcommand:
		// fall through to subcommand-arg handling
	default:
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q\n", args[0])
		os.Exit(2)
	}

	subArgs := args[1:]
	if len(subArgs) > 0 && subArgs[0] == "--help" {
		fmt.Fprintln(os.Stdout, "stubcli "+cobraSubcommand+" — date-flag-owning subcommand")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "  --since VALUE   inclusive lower bound (local date)")
		fmt.Fprintln(os.Stdout, "  --until VALUE   inclusive upper bound (local date)")
		os.Exit(0)
	}
	parseDateFlags("stubcli "+cobraSubcommand, subArgs)
}

// parseDateFlags is the shared --since/--until validator. It is used
// by both modes so the parse-error behavior (non-zero exit, stderr
// message, empty stdout) is identical regardless of where the flags
// live.
func parseDateFlags(progName string, args []string) {
	fs := flag.NewFlagSet(progName, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	since := fs.String("since", "", "inclusive lower bound")
	until := fs.String("until", "", "inclusive upper bound")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	if !validDate(*since) {
		fmt.Fprintf(os.Stderr, "error: invalid value for --since: %q\n", *since)
		os.Exit(2)
	}
	if !validDate(*until) {
		fmt.Fprintf(os.Stderr, "error: invalid value for --until: %q\n", *until)
		os.Exit(2)
	}
	// Real export-CLIs would emit data here. The stub stays silent;
	// no compat test in this package exercises the data path.
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
