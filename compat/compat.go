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

	cmd := exec.CommandContext(runCtx, r.Binary, args...)
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
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return res, fmt.Errorf("compat: %s timed out after %s", r.Binary, timeout)
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
