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

// TestRunContract_NonHermeticDataPath_SkipsDataPath pins down the
// Runner.DataPathHermetic affordance: an exporter whose data path
// requires non-hermetic state (auth token, live API, env credentials)
// must pass the bundle even though every --format invocation would
// fail at the data-emission step.
//
// The proof is adversarial. We build the stub with
// STUBCLI_NEEDS_TOKEN=1 so it actively rejects every successful
// --format value with "not logged in" on stderr and exit 2 — the
// withings/crono/liftoff shape today. Then we set
// DataPathHermetic: compat.BoolPtr(false) on the Runner so the
// bundle skips JSONIsArray, CSVHasHeader, and DefaultIsMarkdown.
// The whole suite must pass: parse-level subtests still run because
// they exercise the format-validation path (which fires before the
// "not logged in" branch in the stub), and the data-path subtests
// skip cleanly.
//
// If anyone deletes the IsDataPathHermetic skip guard in jsonIsArray,
// csvHasHeader, or defaultIsMarkdown, this test fails because the
// stub rejects every --format call with "not logged in" and the
// data-path subtest runs against it.
func TestRunContract_NonHermeticDataPath_SkipsDataPath(t *testing.T) {
	bin := buildStub(t)
	r := compat.Runner{
		Binary:           bin,
		Env:              []string{"STUBCLI_NEEDS_TOKEN=1"},
		DataPathHermetic: compat.BoolPtr(false),
	}
	formats.RunContract(t, r)
}

// TestRunContract_NonHermeticDataPath_BeatsSupportedFormats pins
// down the documented composition of DataPathHermetic and
// SupportedFormats: when a CLI's data path is non-hermetic, the
// data-path subtests skip on the hermeticity check FIRST so the
// integrator does not have to lie about the codec surface to opt
// out. Without that ordering, a non-hermetic CLI that supports all
// three §4 codecs would still hit the stub's "not logged in" path
// because SupportsFormat returns true for every codec.
//
// The stub is built with STUBCLI_NEEDS_TOKEN=1 (rejects every
// --format value at data emission) and the Runner declares
// DataPathHermetic: false alongside SupportedFormats covering all
// three §4 codecs. The bundle must pass.
func TestRunContract_NonHermeticDataPath_BeatsSupportedFormats(t *testing.T) {
	bin := buildStub(t)
	r := compat.Runner{
		Binary:           bin,
		Env:              []string{"STUBCLI_NEEDS_TOKEN=1"},
		DataPathHermetic: compat.BoolPtr(false),
		SupportedFormats: []string{"markdown", "json", "csv"},
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
