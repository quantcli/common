// Command stubcli is a tiny contract-compliant stand-in used by the
// compat library's own tests. It is NOT a usable export-cli. It exists
// only so the dates compat suite can run against a known-good binary in
// quantcli/common's CI, proving the library itself works end-to-end
// before any real exporter wires it up.
//
// Two behaviors:
//   - `--help`: prints a help string mentioning --since and --until, exits 0.
//   - anything else: parses --since/--until; rejects "obviously-not-a-date"
//     with a stderr message and exit code 2.
//
// It never makes a network request.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Fprintln(os.Stdout, "stubcli — contract-compliant test stand-in")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "  --since VALUE   inclusive lower bound (local date)")
		fmt.Fprintln(os.Stdout, "  --until VALUE   inclusive upper bound (local date)")
		os.Exit(0)
	}

	fs := flag.NewFlagSet("stubcli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	since := fs.String("since", "", "inclusive lower bound")
	until := fs.String("until", "", "inclusive upper bound")
	if err := fs.Parse(os.Args[1:]); err != nil {
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
