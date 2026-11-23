package parity_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/quantcli/common/compat"
	"github.com/quantcli/common/compat/parity"
)

// TestRunCrossCodec_AgainstConformingStub is the library's own gate for
// the happy path. The stub's "parity" dataset is conforming by
// construction: every CSV column names a field its JSON record emits,
// deliberately mixing a full flattened path (`sleep_deep_sec`), a bare
// nested leaf (`hr_average`) and an array-element leaf (`name`). If
// this starts failing, the bundle rejects a projection CONTRACT §4
// permits.
func TestRunCrossCodec_AgainstConformingStub(t *testing.T) {
	parity.RunCrossCodec(t, compat.Runner{
		Binary: buildStub(t),
		Env:    []string{"STUBCLI_DATASET=parity", "TZ=UTC"},
	})
}

// TestRunCrossCodec_AgainstConformingStubSubcommand runs the same
// check through the cobra-mode dispatch, so a regression in
// compat.Runner's subcommand prefixing surfaces here too. This is the
// shape every real exporter uses — crono, liftoff and withings all
// hang their contract surface off subcommands.
func TestRunCrossCodec_AgainstConformingStubSubcommand(t *testing.T) {
	parity.RunCrossCodec(t, compat.Runner{
		Binary:      buildStub(t),
		Env:         []string{"STUBCLI_MODE=cobra", "STUBCLI_DATASET=parity", "TZ=UTC"},
		Subcommands: []string{"biometrics"},
	})
}

// TestRunCrossCodec_CamelCaseConventionPasses pins down the second of
// the two case conventions §4 permits. The "camel" dataset is the
// liftoff shape — camelCase JSON keys
// (`sessionDurationSeconds`, `exerciseTypes`, `inputOne`) projected to
// a snake_case CSV header — so not one column matches a JSON key
// byte-for-byte. A bundle that compared names literally would fail
// every liftoff subcommand; this test is what stops that regression.
func TestRunCrossCodec_CamelCaseConventionPasses(t *testing.T) {
	parity.RunCrossCodec(t, compat.Runner{
		Binary: buildStub(t),
		Env:    []string{"STUBCLI_DATASET=camel", "TZ=UTC"},
	})
}

// TestRunCrossCodec_FailsOnDriftingStub is the adversarial half, and
// the test that makes this bundle worth anything. A bundle that only
// ever ran against a conforming stub would pass with its assertion
// deleted.
//
// The stub's "drift" dataset reproduces withings-export-cli#42 in
// miniature: the same JSON record, but a CSV header carrying
// `duration_min` (a unit-converted column with no JSON field) and
// `total_sleep_min` (a computed column with no JSON equivalent). The
// bundle must fail, and must name both offending columns.
//
// Run in a child process: t.Errorf on a real testing.T cannot be
// caught in-process, mirroring compat/formats' floor test.
func TestRunCrossCodec_FailsOnDriftingStub(t *testing.T) {
	if os.Getenv("COMPAT_PARITY_DRIFT_CHILD") == "1" {
		parity.RunCrossCodec(t, compat.Runner{
			Binary: buildStub(t),
			Env:    []string{"STUBCLI_DATASET=drift", "TZ=UTC"},
		})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCrossCodec_FailsOnDriftingStub$")
	cmd.Env = append(os.Environ(), "COMPAT_PARITY_DRIFT_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("drifting stub passed the parity bundle; the assertion is not load-bearing:\n%s", output)
	}
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("could not run child test: %v", err)
	}
	for _, want := range []string{"duration_min", "total_sleep_min", "cross-codec parity"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("failure diagnostic does not mention %q:\n%s", want, output)
		}
	}
	// The conforming columns must NOT be reported as invented.
	for _, unwanted := range []string{"invented: date,", "invented: date "} {
		if strings.Contains(string(output), unwanted) {
			t.Errorf("conforming column `date` reported as invented:\n%s", output)
		}
	}
}

// TestRunCrossCodec_SkipsOnEmptyResultSet pins down the
// no-vacuous-pass rule. The default stub dataset is the empty result
// (`[]` for json, header-only for csv), which has no JSON keys to
// compare against. §8 says a subtest that skips attests nothing, so
// the bundle must skip with a named reason rather than report a pass
// an exporter could earn by exporting nothing.
func TestRunCrossCodec_SkipsOnEmptyResultSet(t *testing.T) {
	bin := buildStub(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCrossCodec_EmptyResultChild$", "-test.v")
	cmd.Env = append(os.Environ(), "COMPAT_PARITY_EMPTY_CHILD=1", "COMPAT_PARITY_STUB="+bin)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("empty-result run should pass (as a skip), got %v:\n%s", err, output)
	}
	if !strings.Contains(string(output), "empty result set") {
		t.Errorf("expected a named skip reason mentioning the empty result set:\n%s", output)
	}
	if !strings.Contains(string(output), "SKIP") {
		t.Errorf("expected the subtest to SKIP rather than PASS:\n%s", output)
	}
}

// TestRunCrossCodec_EmptyResultChild is the child body for
// TestRunCrossCodec_SkipsOnEmptyResultSet. It is a no-op unless
// invoked with COMPAT_PARITY_EMPTY_CHILD=1 so a normal `go test` run
// does not execute it twice.
func TestRunCrossCodec_EmptyResultChild(t *testing.T) {
	if os.Getenv("COMPAT_PARITY_EMPTY_CHILD") != "1" {
		t.Skip("child-process body; driven by TestRunCrossCodec_SkipsOnEmptyResultSet")
	}
	parity.RunCrossCodec(t, compat.Runner{
		Binary: os.Getenv("COMPAT_PARITY_STUB"),
		Env:    []string{"TZ=UTC"},
	})
}

// TestRunCrossCodec_FailsOnSparseKeySet is the adversarial half of the
// record-shape subtest. The "sparse" dataset is the miniature of
// withings `sleep`'s device-dependent `data{}`: two records, the
// second omitting `deep_sec` rather than reporting it as null.
//
// Its CSV header is conforming, so this proves the two subtests are
// independent — the failure comes from the key set, not from a column
// the json codec lacks.
func TestRunCrossCodec_FailsOnSparseKeySet(t *testing.T) {
	if os.Getenv("COMPAT_PARITY_SPARSE_CHILD") == "1" {
		parity.RunCrossCodec(t, compat.Runner{
			Binary: buildStub(t),
			Env:    []string{"STUBCLI_DATASET=sparse", "TZ=UTC"},
		})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCrossCodec_FailsOnSparseKeySet$", "-test.v")
	cmd.Env = append(os.Environ(), "COMPAT_PARITY_SPARSE_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("sparse key set passed the parity bundle; the record-shape assertion is not load-bearing:\n%s", output)
	}
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("could not run child test: %v", err)
	}
	for _, want := range []string{"deep_sec", "record shape", "record 1"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("failure diagnostic does not mention %q:\n%s", want, output)
		}
	}
	// The cross-codec subtest must still pass: the sparse dataset's
	// header names only fields the json codec emits. If this fails, the
	// two subtests are entangled and a key-set defect would be reported
	// as an invented column.
	if !strings.Contains(string(output), "PASS: TestRunCrossCodec_FailsOnSparseKeySet/CSVColumnsExistInJSON") {
		t.Errorf("cross-codec subtest did not pass on the sparse dataset; the subtests are not independent:\n%s", output)
	}
}

// TestRunCrossCodec_KeySetSubtestActuallyRuns guards the subtest
// against quietly becoming a no-op. Every skip guard in
// jsonKeySetIsUniform is reachable — an exporter with one record, no
// csv writer, or SkipDataPath hits one — so a regression that made the
// subtest skip unconditionally would leave every other test in this
// file green. This asserts it reports PASS, not SKIP, against the
// conforming two-record stub.
func TestRunCrossCodec_KeySetSubtestActuallyRuns(t *testing.T) {
	if os.Getenv("COMPAT_PARITY_RUNS_CHILD") == "1" {
		parity.RunCrossCodec(t, compat.Runner{
			Binary: buildStub(t),
			Env:    []string{"STUBCLI_DATASET=parity", "TZ=UTC"},
		})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCrossCodec_KeySetSubtestActuallyRuns$", "-test.v")
	cmd.Env = append(os.Environ(), "COMPAT_PARITY_RUNS_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("conforming stub should pass, got %v:\n%s", err, output)
	}
	if !strings.Contains(string(output), "PASS: TestRunCrossCodec_KeySetSubtestActuallyRuns/JSONKeySetIsUniform") {
		t.Errorf("JSONKeySetIsUniform did not run against the conforming stub (skipped?):\n%s", output)
	}
}

// TestRunCrossCodec_FailsOnNullCollection is the adversarial half of
// the empty-collection subtest. The "nullcollection" dataset reports
// an empty `tags` collection as `null` instead of `[]` — liftoff's
// `exerciseData` defect, which is what a nil Go slice marshals to.
//
// Both other subtests must still pass: the key set is identical across
// the two records and the CSV header is the conforming one, so the
// only thing that moved is the value under one key. That is what makes
// this subtest worth having — neither existing assertion can see this
// defect, because neither looks at a value's type.
func TestRunCrossCodec_FailsOnNullCollection(t *testing.T) {
	if os.Getenv("COMPAT_PARITY_NULLCOLL_CHILD") == "1" {
		parity.RunCrossCodec(t, compat.Runner{
			Binary: buildStub(t),
			Env:    []string{"STUBCLI_DATASET=nullcollection", "TZ=UTC"},
		})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCrossCodec_FailsOnNullCollection$", "-test.v")
	cmd.Env = append(os.Environ(), "COMPAT_PARITY_NULLCOLL_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("a null empty collection passed the parity bundle; the assertion is not load-bearing:\n%s", output)
	}
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("could not run child test: %v", err)
	}
	for _, want := range []string{"tags", "empty collection is `[]`, never `null`", "nil slice"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("failure diagnostic does not mention %q:\n%s", want, output)
		}
	}
	// `duration_sec` is null in the second record and is never an array,
	// so it is a legitimately absent scalar — exactly what §4 requires.
	// Reporting it would make the subtest fail every conforming exporter.
	if strings.Contains(string(output), "duration_sec: array") {
		t.Errorf("a null scalar was reported as a null collection; §4 requires an absent scalar to be null:\n%s", output)
	}
	for _, mustPass := range []string{
		"PASS: TestRunCrossCodec_FailsOnNullCollection/CSVColumnsExistInJSON",
		"PASS: TestRunCrossCodec_FailsOnNullCollection/JSONKeySetIsUniform",
	} {
		if !strings.Contains(string(output), mustPass) {
			t.Errorf("%s did not pass; the subtests are entangled:\n%s", mustPass, output)
		}
	}
}

// TestRunCrossCodec_NullCollectionSubtestActuallyRuns guards the new
// subtest against quietly becoming a no-op, the same way
// TestRunCrossCodec_KeySetSubtestActuallyRuns does for the key set.
//
// This one matters more than most. The subtest is opportunistic — it
// finds nothing unless the fixture carries both an empty and a
// populated collection — so a regression that made it skip or no-op
// would be invisible in every other test here. The conforming "parity"
// dataset does carry both (`tags` is populated in record 0 and `[]` in
// record 1), so a PASS here is a real exercise of the comparison and
// not a vacuous one.
func TestRunCrossCodec_NullCollectionSubtestActuallyRuns(t *testing.T) {
	if os.Getenv("COMPAT_PARITY_NULLCOLL_RUNS_CHILD") == "1" {
		parity.RunCrossCodec(t, compat.Runner{
			Binary: buildStub(t),
			Env:    []string{"STUBCLI_DATASET=parity", "TZ=UTC"},
		})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCrossCodec_NullCollectionSubtestActuallyRuns$", "-test.v")
	cmd.Env = append(os.Environ(), "COMPAT_PARITY_NULLCOLL_RUNS_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("conforming stub should pass, got %v:\n%s", err, output)
	}
	if !strings.Contains(string(output),
		"PASS: TestRunCrossCodec_NullCollectionSubtestActuallyRuns/JSONCollectionsAreNeverNull") {
		t.Errorf("JSONCollectionsAreNeverNull did not run against the conforming stub (skipped?):\n%s", output)
	}
}

// TestRunCrossCodec_SkipDataPathGatesSubtest pins down that
// Runner.SkipDataPath keeps the bundle adoptable by an exporter whose
// data path needs credentials CI does not have — the same escape hatch
// compat/formats offers.
//
// The proof is adversarial: the stub is pointed at the drifting
// dataset, which the bundle would otherwise fail. With SkipDataPath
// the suite is green. Delete the guard in csvColumnsExistInJSON and
// this test fails.
func TestRunCrossCodec_SkipDataPathGatesSubtest(t *testing.T) {
	parity.RunCrossCodec(t, compat.Runner{
		Binary:       buildStub(t),
		Env:          []string{"STUBCLI_DATASET=drift", "TZ=UTC"},
		SkipDataPath: true,
	})
}

// TestRunCrossCodec_PartialCodecSkips pins down that an exporter
// without a csv writer can wire the bundle in without failing: parity
// needs two codecs, and a markdown+json exporter has only one of
// them. The stub actively rejects --format csv here, so a missing skip
// guard surfaces as a hard failure rather than a silent pass.
func TestRunCrossCodec_PartialCodecSkips(t *testing.T) {
	parity.RunCrossCodec(t, compat.Runner{
		Binary:           buildStub(t),
		Env:              []string{"STUBCLI_FORMATS=markdown,json", "STUBCLI_DATASET=parity", "TZ=UTC"},
		SupportedFormats: []string{compat.FormatMarkdown, compat.FormatJSON},
	})
}

// TestRunCrossCodec_BelowFloorFails mirrors compat/formats: a
// SupportedFormats declaration missing markdown or json is
// non-conforming per §4 and must be rejected loudly, not skipped.
func TestRunCrossCodec_BelowFloorFails(t *testing.T) {
	if os.Getenv("COMPAT_PARITY_FLOOR_CHILD") == "1" {
		parity.RunCrossCodec(t, compat.Runner{
			Binary:           "unused",
			SupportedFormats: []string{"csv"},
			SkipDataPath:     true,
		})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCrossCodec_BelowFloorFails$")
	cmd.Env = append(os.Environ(), "COMPAT_PARITY_FLOOR_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("below-floor declaration passed: %s", output)
	}
	if !strings.Contains(string(output), "declares less than the §4 floor {markdown, json}") {
		t.Fatalf("missing floor diagnostic: %s", output)
	}
}

// TestRunCrossCodec_EmptyBinaryFails keeps the zero-value Runner from
// silently no-opping.
func TestRunCrossCodec_EmptyBinaryFails(t *testing.T) {
	if os.Getenv("COMPAT_PARITY_BIN_CHILD") == "1" {
		parity.RunCrossCodec(t, compat.Runner{})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCrossCodec_EmptyBinaryFails$")
	cmd.Env = append(os.Environ(), "COMPAT_PARITY_BIN_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("empty Runner.Binary passed: %s", output)
	}
	if !strings.Contains(string(output), "Runner.Binary is empty") {
		t.Fatalf("missing empty-binary diagnostic: %s", output)
	}
}

// buildStub compiles the stub CLI into a temp directory and returns the
// absolute path, mirroring compat/formats' helper.
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
