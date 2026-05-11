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

// TestRunner_DataPathHermeticDefault pins down the documented
// zero-value semantics of Runner.DataPathHermetic so the formats
// bundle's skip guard cannot drift away from the contract:
//
//   - nil (the struct-literal default) is treated as hermetic, matching
//     the bundle's pre-DataPathHermetic behavior. Existing wirings
//     that omit the field continue to run the data-path subtests.
//   - &true is the explicit-hermetic claim, equivalent to nil.
//   - &false is the opt-out: the data-path subtests skip.
//
// IsDataPathHermetic is the accessor section bundles consult; this
// test asserts on it directly so the contract is checkable without
// shelling out to the stub.
func TestRunner_DataPathHermeticDefault(t *testing.T) {
	cases := []struct {
		name string
		r    compat.Runner
		want bool
	}{
		{"nil (default)", compat.Runner{Binary: "ignored"}, true},
		{"&true (explicit)", compat.Runner{Binary: "ignored", DataPathHermetic: compat.BoolPtr(true)}, true},
		{"&false (opt out)", compat.Runner{Binary: "ignored", DataPathHermetic: compat.BoolPtr(false)}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.IsDataPathHermetic(); got != tc.want {
				t.Errorf("IsDataPathHermetic() = %v; want %v", got, tc.want)
			}
		})
	}
}

// TestBoolPtr asserts the helper round-trips both bool values. It
// is small but pins down the helper's contract so callers can rely
// on it for Runner literal ergonomics.
func TestBoolPtr(t *testing.T) {
	if p := compat.BoolPtr(true); p == nil || *p != true {
		t.Errorf("BoolPtr(true) = %v; want pointer to true", p)
	}
	if p := compat.BoolPtr(false); p == nil || *p != false {
		t.Errorf("BoolPtr(false) = %v; want pointer to false", p)
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
