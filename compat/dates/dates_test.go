package dates_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/quantcli/common/compat"
	"github.com/quantcli/common/compat/dates"
)

// TestRunContract_AgainstStub builds the in-tree stub binary and runs
// the compat suite against it. This is the library's own gate: if the
// stub (which is contract-compliant by construction) starts failing
// these tests, the library has a bug, not the stub.
func TestRunContract_AgainstStub(t *testing.T) {
	bin := buildStub(t)
	dates.RunContract(t, compat.Runner{Binary: bin})
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
