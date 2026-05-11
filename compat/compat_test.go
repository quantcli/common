package compat_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/quantcli/common/compat"
)

// TestWithSubcommand_PrependsArg checks that WithSubcommand causes Run
// to emit the subcommand as argv[1] in front of the caller's args.
// We use a small Go program that echoes its os.Args back so the assertion
// is independent of any system command's flag semantics.
func TestWithSubcommand_PrependsArg(t *testing.T) {
	bin := buildArgEcho(t)
	r := compat.Runner{Binary: bin}.WithSubcommand("biometrics")

	res, err := r.Run(context.Background(), "--help")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit %d; stderr=%q", res.ExitCode, res.StderrString())
	}
	got := strings.TrimRight(res.StdoutString(), "\n")
	want := "biometrics\n--help"
	if got != want {
		t.Errorf("argv mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}

// TestWithSubcommand_NestedPathSplitsOnWhitespace covers liftoff-style
// CLIs whose data-producing leaves live two levels deep
// (e.g. `liftoff-export workouts stats`). Without the split, the cobra
// resolver would receive one literal "workouts stats" argv entry and
// reject it as an unknown command. The argecho-based assertion locks
// in that each whitespace-separated word is its own argv entry, in
// order, before the caller's args.
func TestWithSubcommand_NestedPathSplitsOnWhitespace(t *testing.T) {
	bin := buildArgEcho(t)
	r := compat.Runner{Binary: bin}.WithSubcommand("workouts stats")

	res, err := r.Run(context.Background(), "--format", "json")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit %d; stderr=%q", res.ExitCode, res.StderrString())
	}
	got := strings.TrimRight(res.StdoutString(), "\n")
	want := "workouts\nstats\n--format\njson"
	if got != want {
		t.Errorf("argv mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
	if r.Subcommand() != "workouts stats" {
		t.Errorf("Subcommand() = %q; want %q", r.Subcommand(), "workouts stats")
	}
}

// TestWithSubcommand_EmptyStringClears asserts that passing the empty
// string (or whitespace-only) clears any previously-set path, which
// keeps the API reversible without an extra method.
func TestWithSubcommand_EmptyStringClears(t *testing.T) {
	bin := buildArgEcho(t)
	r := compat.Runner{Binary: bin}.WithSubcommand("biometrics").WithSubcommand("")
	if r.Subcommand() != "" {
		t.Errorf("Subcommand() = %q; want empty after clear", r.Subcommand())
	}
	res, err := r.Run(context.Background(), "--help")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimRight(res.StdoutString(), "\n"); got != "--help" {
		t.Errorf("argv mismatch: got %q want %q", got, "--help")
	}
}

// TestRun_NoSubcommandPassthrough checks that the default zero
// subcommand leaves args untouched.
func TestRun_NoSubcommandPassthrough(t *testing.T) {
	bin := buildArgEcho(t)
	r := compat.Runner{Binary: bin}

	res, err := r.Run(context.Background(), "--help")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimRight(res.StdoutString(), "\n")
	if got != "--help" {
		t.Errorf("argv mismatch: got %q want %q", got, "--help")
	}
}

// TestWithSubcommand_DoesNotMutateReceiver asserts that WithSubcommand
// returns a copy and leaves the parent runner unchanged. Section
// bundles rely on this when iterating Subcommands.
func TestWithSubcommand_DoesNotMutateReceiver(t *testing.T) {
	bin := buildArgEcho(t)
	parent := compat.Runner{Binary: bin}
	_ = parent.WithSubcommand("biometrics")
	if parent.Subcommand() != "" {
		t.Errorf("parent.Subcommand() = %q; want empty after WithSubcommand on copy", parent.Subcommand())
	}
}

// TestRun_TimeoutReturnsError exercises the timeout branch so the
// Runner's error contract (non-zero exit is not an error; timeout is)
// stays load-bearing.
func TestRun_TimeoutReturnsError(t *testing.T) {
	sleeper, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not on PATH; skipping timeout exercise")
	}
	r := compat.Runner{Binary: sleeper, Timeout: 50 * time.Millisecond}
	_, runErr := r.Run(context.Background(), "5")
	if runErr == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(runErr.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", runErr)
	}
}

// buildArgEcho compiles the tiny argecho helper into a temp dir. It is
// the simplest possible echo-args binary: it prints each os.Args[1:]
// entry on its own line to stdout. We build it instead of relying on
// /bin/echo so the test works on any GOOS.
func buildArgEcho(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "argecho")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "github.com/quantcli/common/compat/internal/argecho")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build argecho: %v\n%s", err, output)
	}
	return out
}
