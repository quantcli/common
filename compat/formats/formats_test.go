package formats_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/quantcli/common/compat"
	"github.com/quantcli/common/compat/formats"
)

// TestRunContract_AgainstStubFlat builds the in-tree stub binary in
// its default (flat) mode and runs the formats compat suite against
// it. This is the library's own gate for the original Runner shape:
// if the stub (which is contract-compliant by construction) starts
// failing these tests, the library has a bug, not the stub.
func TestRunContract_AgainstStubFlat(t *testing.T) {
	bin := buildStub(t)
	formats.RunContract(t, compat.Runner{Binary: bin})
}

// TestRunContract_AgainstStubSubcommand runs the same suite against
// the stub in cobra mode, with a Subcommands declaration so the
// Runner dispatches per-subcommand. In cobra mode the stub's root
// --help does NOT mention --format — only `biometrics --help` does —
// so this test fails fast if compat.Runner ever stops prepending the
// subcommand.
//
// This is the gate that pins down §4 for crono / liftoff / withings,
// whose contract surface lives on cobra subcommands.
func TestRunContract_AgainstStubSubcommand(t *testing.T) {
	bin := buildStub(t)
	r := compat.Runner{
		Binary:      bin,
		Env:         []string{"STUBCLI_MODE=cobra"},
		Subcommands: []string{"biometrics"},
	}
	formats.RunContract(t, r)
}

// TestRunContract_PartialCodec_SkipsCSV pins down the
// Runner.SupportedFormats affordance: an exporter that declares only
// markdown+json must pass the bundle even though one of the §4
// codecs is unimplemented.
//
// The proof is adversarial. We build the stub with
// STUBCLI_FORMATS=markdown,json so it actively rejects --format csv
// with a non-zero exit — the partial-codec exporter shape
// (crono/liftoff). Then we set
// SupportedFormats: []string{"markdown", "json"} on the Runner so
// the bundle skips CSVHasHeader. The whole suite must pass.
//
// If anyone deletes the skip guard in csvHasHeader, this test fails
// because the stub rejects --format csv and the subtest runs against
// it.
func TestRunContract_PartialCodec_SkipsCSV(t *testing.T) {
	bin := buildStub(t)
	r := compat.Runner{
		Binary:           bin,
		Env:              []string{"STUBCLI_FORMATS=markdown,json"},
		SupportedFormats: []string{"markdown", "json"},
	}
	formats.RunContract(t, r)
}

// TestSupportsFormat documents the nil-vs-empty-vs-subset behavior
// of compat.Runner.SupportsFormat at the package level. The formats
// bundle's skip guards rely on these semantics being stable.
func TestSupportsFormat(t *testing.T) {
	nilR := compat.Runner{Binary: "ignored"}
	for _, codec := range []string{"markdown", "json", "csv", "yaml"} {
		if !nilR.SupportsFormat(codec) {
			t.Errorf("nil SupportedFormats: SupportsFormat(%q) = false; want true (nil = all)", codec)
		}
	}

	emptyR := compat.Runner{Binary: "ignored", SupportedFormats: []string{}}
	for _, codec := range []string{"markdown", "json", "csv"} {
		if emptyR.SupportsFormat(codec) {
			t.Errorf("empty SupportedFormats: SupportsFormat(%q) = true; want false", codec)
		}
	}

	subsetR := compat.Runner{Binary: "ignored", SupportedFormats: []string{"markdown", "json"}}
	cases := map[string]bool{"markdown": true, "json": true, "csv": false, "yaml": false}
	for codec, want := range cases {
		if got := subsetR.SupportsFormat(codec); got != want {
			t.Errorf("subset SupportedFormats: SupportsFormat(%q) = %v; want %v", codec, got, want)
		}
	}
}


// buildStub compiles the stub CLI into a temp directory and returns
// the absolute path. It uses `go build` rather than relying on a
// checked-in binary so the test is reproducible across platforms.
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
