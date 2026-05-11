// Package compat is the cross-CLI conformance test library for the
// quantcli export-CLI contract. Each *-export-cli imports the subpackages
// (e.g. compat/dates) and runs them against its own built binary in CI.
//
// The library is deliberately black-box: it shells out to the binary under
// test and asserts on stdout, stderr, and exit code. It never imports a
// CLI's internal packages. Adding a new contract test means adding a new
// subpackage here; every exporter then picks it up by adding a one-line
// test entry point.
//
// See CONTRACT.md in the parent repository for the surface this library
// pins down.
package compat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Runner invokes a single *-export-cli binary in a controlled environment.
// A zero-value Runner is not usable; Binary must be set to an absolute path.
type Runner struct {
	// Binary is the absolute path to the export-cli binary under test.
	Binary string

	// Env is the environment passed to the binary. If nil, an empty
	// environment is used. Tests should set this explicitly so behavior
	// does not depend on whatever happens to be in the CI runner's
	// environment (notably PATH, HOME, TZ, and any *_TOKEN credentials).
	Env []string

	// Timeout is the per-invocation timeout. Zero means 10 seconds.
	Timeout time.Duration

	// Subcommands declares the subcommands under which the contract
	// surface lives — for CLIs (typically cobra-based) where flags like
	// --since and --until are attached to data-producing subcommands
	// rather than the root binary. Examples: crono's `biometrics`,
	// `exercises`, `nutrition`, `servings`, `notes` each accept their
	// own --since/--until.
	//
	// Each entry is one space-separated subcommand path. Single-word
	// entries like "biometrics" target a top-level subcommand; nested
	// paths like "workouts stats" target a leaf under a parent (as in
	// liftoff's `liftoff-export workouts stats`). The dispatcher splits
	// on whitespace before invoking the binary, so the cobra command
	// tree receives separate argv entries rather than one space-joined
	// argument.
	//
	// Empty means the surface is on the root binary; section bundles
	// invoke the binary directly. Non-empty means each section's
	// RunContract iterates the list and verifies the contract once per
	// subcommand via t.Run, so a regression in any single subcommand
	// surfaces as a named subtest failure rather than masking the rest.
	//
	// The Runner itself does not look at this field; section bundles
	// (e.g. compat/dates) read it and dispatch via WithSubcommand.
	Subcommands []string

	// SupportedFormats declares which §4 output codecs the exporter
	// implements. Used by compat/formats to gate the codec-specific
	// data-path subtests.
	//
	// Default (nil) means the exporter declares the full §4 surface
	// (markdown, json, csv); compat/formats runs every subtest.
	//
	// Non-nil declares an explicit subset — e.g.
	// []string{"markdown", "json"} for a CLI that has not yet added a
	// CSV writer. The bundle's JSONIsArray, CSVHasHeader, and
	// DefaultIsMarkdown subtests skip via t.Skipf when their codec is
	// not in the list, naming the missing codec so the gap is visible
	// in test output rather than masked.
	//
	// SupportedFormats does NOT relax the parse-level subtests
	// (HelpDocumentsFormatFlag, UnknownFormatFails,
	// FlagValidationIsHermetic). Those assert on the --format flag
	// itself, not on a specific codec, and run regardless.
	//
	// An empty slice declares zero supported codecs — rarely useful,
	// and effectively disables the data-path subtests. Use nil to
	// declare "all of §4".
	SupportedFormats []string

	// subcommandParts, when non-empty, is prepended to args on every Run
	// call as separate argv entries. Set via WithSubcommand from a
	// space-separated path (e.g. "workouts stats"); section bundles use
	// it to dispatch per-subcommand. Callers do not need to set it
	// directly — set Subcommands instead and let the bundle compose the
	// dispatch.
	subcommandParts []string
}

// Result captures everything observable about one CLI invocation. All
// compat-test assertions operate on these three fields.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// StdoutString returns Stdout as a string.
func (r Result) StdoutString() string { return string(r.Stdout) }

// StderrString returns Stderr as a string.
func (r Result) StderrString() string { return string(r.Stderr) }

// Run invokes the binary with the given args and returns its observable
// output. A non-zero exit code is NOT returned as an error — compat tests
// frequently assert on non-zero exits, so the caller decides what counts
// as a failure. ctx cancellation, process-start failure, and timeouts are
// returned as errors.
func (r Runner) Run(ctx context.Context, args ...string) (Result, error) {
	if r.Binary == "" {
		return Result{}, errors.New("compat: Runner.Binary is empty")
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fullArgs := args
	if len(r.subcommandParts) > 0 {
		fullArgs = append(append([]string(nil), r.subcommandParts...), args...)
	}
	cmd := exec.CommandContext(runCtx, r.Binary, fullArgs...)
	// Default to an empty env so tests are hermetic. Callers opt into
	// passing TZ, HOME, etc. via Runner.Env.
	if r.Env != nil {
		cmd.Env = append([]string(nil), r.Env...)
	} else {
		cmd.Env = []string{}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		res.ExitCode = 0
		return res, nil
	}
	// Timeout check first. exec.CommandContext kills the process when
	// the deadline expires, which surfaces as an *exec.ExitError on the
	// signal path. The package contract promises a non-nil error on
	// timeout, so we must detect that case before falling through to
	// the ExitError handler — otherwise a hung CLI looks like a clean
	// non-zero exit to the caller.
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return res, fmt.Errorf("compat: %s timed out after %s", r.Binary, timeout)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, fmt.Errorf("compat: failed to run %s: %w", r.Binary, err)
}

// MustRun is the testing-helper equivalent of Run: it fails the test on
// any non-exit-code error (timeout, missing binary, etc.) and returns the
// Result on success.
func (r Runner) MustRun(t *testing.T, args ...string) Result {
	t.Helper()
	res, err := r.Run(context.Background(), args...)
	if err != nil {
		t.Fatalf("compat: %v", err)
	}
	return res
}

// WithEnv returns a copy of r with environment variable KEY=VALUE pairs
// appended. Useful for setting TZ on a per-test basis without mutating
// the receiver.
func (r Runner) WithEnv(kv ...string) Runner {
	out := r
	out.Env = append(append([]string(nil), r.Env...), kv...)
	return out
}

// WithSubcommand returns a copy of r whose Run prepends sub as a
// command-line path before the caller's args. Section bundles use this
// internally to dispatch per-subcommand when Runner.Subcommands is
// non-empty; integrators normally set Subcommands and let the bundle
// do it.
//
// sub is a space-separated path: "biometrics" targets a single
// top-level subcommand; "workouts stats" targets a leaf under a parent
// (each whitespace-separated word becomes its own argv entry, so the
// cobra command tree resolves them as separate names rather than one
// space-joined argument). Leading/trailing whitespace is ignored; the
// empty string clears any previously-set path.
//
// Calling WithSubcommand again replaces (not stacks) the previous
// path.
func (r Runner) WithSubcommand(sub string) Runner {
	out := r
	out.subcommandParts = strings.Fields(sub)
	return out
}

// Subcommand returns the subcommand path that Run will prepend to
// args, or the empty string if none is set. Nested paths come back
// joined with single spaces (e.g. "workouts stats"). Section bundles
// use this in subtest names so failures point at the offending
// subcommand.
func (r Runner) Subcommand() string { return strings.Join(r.subcommandParts, " ") }

// SupportsFormat reports whether name is one of the §4 codecs the
// exporter declares it implements.
//
// A nil SupportedFormats is treated as "all of §4" — SupportsFormat
// returns true for any name. When SupportedFormats is non-nil
// (including empty), only names present in the slice return true.
// String comparison is exact: callers pass the canonical CONTRACT.md
// codec names ("markdown", "json", "csv").
//
// Section bundles consult this when deciding whether to skip a
// codec-specific subtest. Integrators normally do not call it
// directly — populate SupportedFormats and let the bundle dispatch.
func (r Runner) SupportsFormat(name string) bool {
	if r.SupportedFormats == nil {
		return true
	}
	for _, f := range r.SupportedFormats {
		if f == name {
			return true
		}
	}
	return false
}
