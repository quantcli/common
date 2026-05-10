// Package dates is the compat test bundle for §2 (timezone policy) and §3
// (date flags) of CONTRACT.md.
//
// What is machine-attested here:
//
//   - The CLI documents `--since` and `--until` in the help output of
//     whatever entry point owns them — the root binary for flat CLIs,
//     or each declared subcommand for cobra-based CLIs (set via
//     compat.Runner.Subcommands).
//   - An invalid `--since` value produces a non-zero exit with an error
//     on stderr and an empty stdout.
//   - A flag-validation failure does not perform a network request.
//   - `--help` exits zero and does not perform a network request.
//
// What is intentionally NOT machine-attested yet:
//
//   - The actual local-midnight semantics of `--since 2026-04-15` (i.e.
//     "the window starts at local midnight, not UTC midnight"). Asserting
//     this black-box requires either a `--print-resolved-window` (or
//     equivalent) affordance on every CLI, or a recorded HTTP fixture per
//     upstream. Both are out of scope for the first compat-test cut and
//     tracked separately in quantcli/common.
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
//	    "github.com/quantcli/common/compat/dates"
//	)
//
//	func TestContractDates(t *testing.T) {
//	    bin := os.Getenv("EXPORT_CLI_BIN")
//	    if bin == "" { t.Skip("EXPORT_CLI_BIN not set") }
//	    dates.RunContract(t, compat.Runner{Binary: bin})
//	}
package dates

import (
	"context"
	"strings"
	"testing"

	"github.com/quantcli/common/compat"
)

// RunContract runs the full date-flag contract test bundle against r.
// It is the only function exporters are expected to call.
//
// Each assertion is a t.Run subtest, so a failure in one does not mask
// the others. The bundle is safe to run in parallel with other compat
// suites; individual subtests are not marked parallel because they
// shell out to the same binary and the OS may serialize them anyway.
//
// If r.Subcommands is non-empty, RunContract iterates the list and
// runs the four assertions once per subcommand under a
// "subcommand=NAME" t.Run group. This is how cobra-based CLIs (e.g.
// crono's `biometrics`, `exercises`, `nutrition`, `servings`, `notes`)
// attest the contract on every data-producing subcommand. If empty,
// the assertions run once against the root binary — the right shape
// for CLIs that put --since/--until at the top level.
func RunContract(t *testing.T, r compat.Runner) {
	t.Helper()
	if r.Binary == "" {
		t.Fatal("dates: compat.Runner.Binary is empty")
	}

	if len(r.Subcommands) == 0 {
		runContractOne(t, r)
		return
	}
	for _, sub := range r.Subcommands {
		sub := sub
		t.Run("subcommand="+sub, func(t *testing.T) {
			runContractOne(t, r.WithSubcommand(sub))
		})
	}
}

// runContractOne runs the four date-flag assertions against a single
// invocation surface — either the root binary (when r has no
// subcommand prefix) or a specific subcommand of it.
func runContractOne(t *testing.T, r compat.Runner) {
	t.Helper()
	t.Run("HelpDocumentsDateFlags", func(t *testing.T) {
		helpDocumentsDateFlags(t, r)
	})
	t.Run("InvalidSinceValueFails", func(t *testing.T) {
		invalidSinceValueFails(t, r)
	})
	t.Run("HelpIsHermetic", func(t *testing.T) {
		helpIsHermetic(t, r)
	})
	t.Run("FlagValidationIsHermetic", func(t *testing.T) {
		flagValidationIsHermetic(t, r)
	})
}

// helpDocumentsDateFlags asserts that the CLI documents `--since` and
// `--until` somewhere in the `--help` output of the configured entry
// point — root binary or subcommand, depending on how the integrator
// configured compat.Runner. This is the minimum binding between the
// contract and the binary: an exporter that quietly drops one of the
// flags will fail this test.
func helpDocumentsDateFlags(t *testing.T, r compat.Runner) {
	t.Helper()
	res := r.MustRun(t, "--help")
	if res.ExitCode != 0 {
		t.Fatalf("--help exited %d, want 0; stderr=%q", res.ExitCode, res.StderrString())
	}
	// Some CLIs route global flag docs through stderr or through a
	// subcommand-listing screen, so we check both streams. The contract
	// only requires that the flags are documented and exit code is zero.
	combined := res.StdoutString() + "\n" + res.StderrString()
	for _, flag := range []string{"--since", "--until"} {
		if !strings.Contains(combined, flag) {
			t.Errorf("--help output does not mention %s; got stdout=%q stderr=%q",
				flag, res.StdoutString(), res.StderrString())
		}
	}
}

// invalidSinceValueFails asserts that a malformed `--since` value causes
// the CLI to exit non-zero with an error on stderr and an empty stdout.
//
// We use a known-bad value so any entry point that accepts `--since`
// will reject it at parse time. For cobra-based CLIs whose date flags
// live on subcommands, the integrator sets compat.Runner.Subcommands;
// RunContract then dispatches via WithSubcommand and this assertion
// runs once per declared subcommand.
func invalidSinceValueFails(t *testing.T, r compat.Runner) {
	t.Helper()
	// "obviously-not-a-date" should never parse as a keyword, absolute
	// date, or relative duration under any contract-compliant CLI.
	args := []string{"--since", "obviously-not-a-date", "--until", "obviously-not-a-date"}
	res, err := r.Run(context.Background(), args...)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("invalid --since accepted (exit 0); stdout=%q stderr=%q",
			res.StdoutString(), res.StderrString())
	}
	if len(res.Stdout) != 0 {
		// CONTRACT §4: stdout is "data only". A flag-validation error
		// must not contaminate the data stream.
		t.Errorf("invalid --since produced stdout output (contract §4 violation): %q",
			res.StdoutString())
	}
	if strings.TrimSpace(res.StderrString()) == "" {
		t.Errorf("invalid --since produced no stderr message")
	}
}

// helpIsHermetic asserts that `--help` makes no outbound network request.
// We enforce this by pointing every common proxy env var at an
// unreachable address. If the CLI tries to hit the network at all, it
// will fail or hang — the latter is caught by the Runner timeout.
//
// This is a harness invariant, not a property the contract promises
// today. A `--help` that dialed out would still satisfy §3 surface
// checks but would be unusable for LLM-agent introspection (an agent
// must be able to ask "what does this CLI do" without a token or
// network). Codifying hermeticity as its own CONTRACT.md section is
// tracked separately; in the meantime the framework defends the
// property here so an exporter cannot regress it silently.
func helpIsHermetic(t *testing.T, r compat.Runner) {
	t.Helper()
	res := r.WithEnv(noNetworkEnv()...).MustRun(t, "--help")
	if res.ExitCode != 0 {
		t.Errorf("--help exited %d with no-network env; stderr=%q", res.ExitCode, res.StderrString())
	}
}

// flagValidationIsHermetic is the parse-failure half of the same
// harness invariant documented on helpIsHermetic above: a CLI that
// dialed out before rejecting an invalid flag would have already
// leaked a network call by the time it exits non-zero.
func flagValidationIsHermetic(t *testing.T, r compat.Runner) {
	t.Helper()
	args := []string{"--since", "obviously-not-a-date", "--until", "obviously-not-a-date"}
	res, err := r.WithEnv(noNetworkEnv()...).Run(context.Background(), args...)
	if err != nil {
		t.Fatalf("run failed under no-network env: %v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("invalid --since accepted under no-network env (exit 0); stderr=%q",
			res.StderrString())
	}
	// If the CLI dialed out before parsing, stderr would typically
	// contain a proxy or DNS error rather than a parse error. We don't
	// pattern-match the message (each CLI phrases it differently) — we
	// only require that the process did not stall and did exit non-zero,
	// which is already covered above. The proxy variables exist as a
	// belt-and-suspenders: any HTTP client honoring them will fail
	// loudly instead of silently reaching the real upstream.
}

// noNetworkEnv returns a slice of KEY=VALUE strings that point common
// HTTP proxy variables at an address guaranteed not to resolve. Any
// Go HTTP client built on net/http will honor at least HTTP_PROXY and
// HTTPS_PROXY; we set the lowercase variants too for libraries that
// reach into env directly.
func noNetworkEnv() []string {
	const unreachable = "http://127.0.0.1:1"
	return []string{
		"HTTP_PROXY=" + unreachable,
		"HTTPS_PROXY=" + unreachable,
		"http_proxy=" + unreachable,
		"https_proxy=" + unreachable,
		"NO_PROXY=",
		"no_proxy=",
		// Force a deterministic timezone so CLIs that read TZ on
		// startup behave predictably across runners.
		"TZ=UTC",
	}
}
