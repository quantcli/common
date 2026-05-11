// Package formats is the compat test bundle for §4 (output format) of
// CONTRACT.md.
//
// What is machine-attested here:
//
//   - The CLI documents `--format` in its `--help` output (root binary
//     for flat CLIs, or each declared subcommand for cobra-based CLIs
//     configured via compat.Runner.Subcommands).
//   - An unknown `--format` value exits non-zero with an error on
//     stderr and an empty stdout.
//   - The unknown-value parse failure performs no network request.
//   - `--format json` exits zero and emits a JSON value on stdout that
//     unmarshals as `[]any`.
//   - `--format csv` exits zero and emits at least one non-empty line
//     on stdout — the header row — even on an empty result set.
//   - The default (no `--format` flag) and `--format markdown` produce
//     byte-identical stdout. This is how the suite pins down "markdown
//     is the default" without having to parse markdown.
//
// The data-path subtests (JSONIsArray, CSVHasHeader, DefaultIsMarkdown)
// invoke the CLI with no extra args beyond `--format`. Integrators
// whose CLI requires extra args to succeed (e.g. credentials via env)
// must arrange for those to be present via Runner.Env. Exporters
// whose data path is not yet runnable from a clean CI environment
// should wire the bundle in once it is — the parse-time subtests
// alone are not enough to claim machine attestation for §4.
//
// Partial-codec exporters can declare which §4 codecs they implement
// via compat.Runner.SupportedFormats. Codec-specific subtests skip
// (with t.Skipf) when their codec is not in the list, so a CLI that
// has not yet added a CSV writer can adopt the bundle without
// failure. The parse-level subtests run regardless because they
// assert on the --format flag itself, not on a specific codec.
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
//	    "github.com/quantcli/common/compat/formats"
//	)
//
//	func TestContractFormats(t *testing.T) {
//	    bin := os.Getenv("EXPORT_CLI_BIN")
//	    if bin == "" { t.Skip("EXPORT_CLI_BIN not set") }
//	    formats.RunContract(t, compat.Runner{Binary: bin})
//	}
package formats

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/quantcli/common/compat"
)

// RunContract runs the full output-format contract test bundle against
// r. It is the only function exporters are expected to call.
//
// Each assertion is a t.Run subtest, so a failure in one does not mask
// the others. If r.Subcommands is non-empty, RunContract iterates the
// list and runs the assertions once per subcommand under a
// "subcommand=NAME" t.Run group, mirroring compat/dates.
func RunContract(t *testing.T, r compat.Runner) {
	t.Helper()
	if r.Binary == "" {
		t.Fatal("formats: compat.Runner.Binary is empty")
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

// runContractOne runs the §4 assertions against a single invocation
// surface — either the root binary (when r has no subcommand prefix)
// or a specific subcommand of it.
func runContractOne(t *testing.T, r compat.Runner) {
	t.Helper()
	t.Run("HelpDocumentsFormatFlag", func(t *testing.T) {
		helpDocumentsFormatFlag(t, r)
	})
	t.Run("UnknownFormatFails", func(t *testing.T) {
		unknownFormatFails(t, r)
	})
	t.Run("FlagValidationIsHermetic", func(t *testing.T) {
		flagValidationIsHermetic(t, r)
	})
	t.Run("JSONIsArray", func(t *testing.T) {
		jsonIsArray(t, r)
	})
	t.Run("CSVHasHeader", func(t *testing.T) {
		csvHasHeader(t, r)
	})
	t.Run("DefaultIsMarkdown", func(t *testing.T) {
		defaultIsMarkdown(t, r)
	})
}

// helpDocumentsFormatFlag asserts that the CLI documents `--format`
// somewhere in its `--help` output. Like the dates equivalent, this
// is the minimum binding between §4 and the binary: a CLI that quietly
// drops the `--format` flag will fail this test.
func helpDocumentsFormatFlag(t *testing.T, r compat.Runner) {
	t.Helper()
	res := r.MustRun(t, "--help")
	if res.ExitCode != 0 {
		t.Fatalf("--help exited %d, want 0; stderr=%q", res.ExitCode, res.StderrString())
	}
	combined := res.StdoutString() + "\n" + res.StderrString()
	if !strings.Contains(combined, "--format") {
		t.Errorf("--help output does not mention --format; got stdout=%q stderr=%q",
			res.StdoutString(), res.StderrString())
	}
}

// unknownFormatFails asserts that a value like `--format frobnicate`
// causes the CLI to exit non-zero with an error on stderr and an
// empty stdout. The empty-stdout check is the §4 "stdout is data
// only" rule: a parse failure must not contaminate the data stream.
func unknownFormatFails(t *testing.T, r compat.Runner) {
	t.Helper()
	res, err := r.Run(context.Background(), "--format", unknownFormatValue)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("unknown --format accepted (exit 0); stdout=%q stderr=%q",
			res.StdoutString(), res.StderrString())
	}
	if len(res.Stdout) != 0 {
		t.Errorf("unknown --format produced stdout output (§4 violation): %q",
			res.StdoutString())
	}
	if strings.TrimSpace(res.StderrString()) == "" {
		t.Errorf("unknown --format produced no stderr message")
	}
}

// flagValidationIsHermetic asserts that the unknown-format parse
// failure does not dial out. Mirrors the dates bundle's parse-failure
// hermetic test, but exercises the `--format` parse path explicitly
// in case the CLI looks up format codecs differently than date
// values. Pins down the `--format` parse-failure path of CONTRACT §7
// Hermeticity on every PR.
func flagValidationIsHermetic(t *testing.T, r compat.Runner) {
	t.Helper()
	res, err := r.WithEnv(noNetworkEnv()...).Run(context.Background(), "--format", unknownFormatValue)
	if err != nil {
		t.Fatalf("run failed under no-network env: %v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("unknown --format accepted under no-network env (exit 0); stderr=%q",
			res.StderrString())
	}
}

// jsonIsArray asserts that `--format json` exits zero and emits
// stdout that unmarshals as a JSON array (`[]any`). The check is
// row-count agnostic: zero rows is `[]`, N rows is `[{…},…]`, both
// pass. The §4 empty-result rule (`[]` on no data) is therefore
// covered implicitly so long as the integrator's data path returns
// successfully.
//
// Skipped (not failed) if "json" is not in Runner.SupportedFormats.
func jsonIsArray(t *testing.T, r compat.Runner) {
	t.Helper()
	if !r.SupportsFormat("json") {
		t.Skipf("--format json not declared in Runner.SupportedFormats")
	}
	res := r.MustRun(t, "--format", "json")
	if res.ExitCode != 0 {
		t.Fatalf("--format json exited %d; stderr=%q", res.ExitCode, res.StderrString())
	}
	trimmed := strings.TrimSpace(res.StdoutString())
	if trimmed == "" {
		t.Fatalf("--format json produced empty stdout; want JSON array (`[]` for empty result per §4)")
	}
	var arr []any
	if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
		t.Errorf("--format json stdout is not a JSON array: %v; stdout=%q",
			err, res.StdoutString())
	}
}

// csvHasHeader asserts that `--format csv` exits zero and emits at
// least one non-empty line on stdout. §4 says an empty result is
// success with "no rows" for CSV — the header row is still required,
// so even a zero-row CSV must have one line.
//
// Skipped (not failed) if "csv" is not in Runner.SupportedFormats.
// crono-export-cli and liftoff-export-cli are partial-codec exporters
// today; the bundle becomes adoptable for them by declaring
// SupportedFormats: []string{"markdown", "json"}.
func csvHasHeader(t *testing.T, r compat.Runner) {
	t.Helper()
	if !r.SupportsFormat("csv") {
		t.Skipf("--format csv not declared in Runner.SupportedFormats")
	}
	res := r.MustRun(t, "--format", "csv")
	if res.ExitCode != 0 {
		t.Fatalf("--format csv exited %d; stderr=%q", res.ExitCode, res.StderrString())
	}
	if len(nonEmptyLines(res.StdoutString())) == 0 {
		t.Errorf("--format csv produced no header row on stdout; got %q",
			res.StdoutString())
	}
}

// defaultIsMarkdown asserts that the default format (no `--format`
// flag) produces byte-identical stdout to `--format markdown`. This is
// the strongest behavioral statement of "markdown is the default"
// available without parsing markdown.
//
// Skipped (not failed) if "markdown" is not in
// Runner.SupportedFormats. A CLI that does not declare markdown
// cannot be expected to default to it, and forcing the equality check
// would just measure noise.
func defaultIsMarkdown(t *testing.T, r compat.Runner) {
	t.Helper()
	if !r.SupportsFormat("markdown") {
		t.Skipf("--format markdown not declared in Runner.SupportedFormats")
	}
	noFlag := r.MustRun(t)
	explicit := r.MustRun(t, "--format", "markdown")
	if noFlag.ExitCode != 0 {
		t.Fatalf("default (no --format) exited %d; stderr=%q",
			noFlag.ExitCode, noFlag.StderrString())
	}
	if explicit.ExitCode != 0 {
		t.Fatalf("--format markdown exited %d; stderr=%q",
			explicit.ExitCode, explicit.StderrString())
	}
	if noFlag.StdoutString() != explicit.StdoutString() {
		t.Errorf("default stdout differs from --format markdown stdout:\n no-flag:  %q\n markdown: %q",
			noFlag.StdoutString(), explicit.StdoutString())
	}
}

// unknownFormatValue is a sentinel that no contract-compliant CLI
// should accept as a `--format` value. Kept as a named constant so a
// future codec adoption (e.g. `yaml`) does not collide silently with
// the negative-path probe.
const unknownFormatValue = "obviously-not-a-format"

// nonEmptyLines splits s on '\n' and drops blank/whitespace-only
// entries — including the trailing empty string from a final newline.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// noNetworkEnv mirrors compat/dates: point every common HTTP proxy
// env var at an unreachable address so any flag-validation path that
// accidentally opens a connection fails or stalls. Kept locally
// rather than exported from compat to keep the two bundles
// independently auditable.
func noNetworkEnv() []string {
	const unreachable = "http://127.0.0.1:1"
	return []string{
		"HTTP_PROXY=" + unreachable,
		"HTTPS_PROXY=" + unreachable,
		"http_proxy=" + unreachable,
		"https_proxy=" + unreachable,
		"NO_PROXY=",
		"no_proxy=",
		"TZ=UTC",
	}
}
