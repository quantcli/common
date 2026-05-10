package dates_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/quantcli/common/compat"
	"github.com/quantcli/common/compat/dates"
)

// TestRunContract_AgainstStubFlat builds the in-tree stub binary in its
// default (flat) mode and runs the compat suite against it. This is the
// library's own gate for the original Runner shape: if the stub (which
// is contract-compliant by construction) starts failing these tests,
// the library has a bug, not the stub.
func TestRunContract_AgainstStubFlat(t *testing.T) {
	bin := buildStub(t)
	dates.RunContract(t, compat.Runner{Binary: bin})
}

// TestRunContract_AgainstStubSubcommand runs the same suite against
// the stub in cobra mode, with a Subcommands declaration so the Runner
// dispatches per-subcommand. In cobra mode the stub's root --help does
// NOT mention --since/--until — only `biometrics --help` does — so this
// test fails fast if compat.Runner ever stops prepending the
// subcommand.
//
// This is the gate that pins down the contract for crono / liftoff /
// withings, whose date flags live on cobra subcommands.
func TestRunContract_AgainstStubSubcommand(t *testing.T) {
	bin := buildStub(t)
	r := compat.Runner{
		Binary:      bin,
		Env:         []string{"STUBCLI_MODE=cobra"},
		Subcommands: []string{"biometrics"},
	}
	dates.RunContract(t, r)
}

// buildStub compiles the stub CLI into a temp directory and returns the
// absolute path. It uses `go build` rather than relying on a checked-in
// binary so the test is reproducible across platforms.
func buildStub(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "stubcli")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "github.com/quantcli/common/compat/internal/stubcli")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build stubcli failed: %v\n%s", err, output)
	}
	return out
}
