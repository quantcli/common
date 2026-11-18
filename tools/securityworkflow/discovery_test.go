package securityworkflow

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

const (
	// workflowPath is the real file, not a copy. The point of these tests is
	// that the shipped workflow behaves correctly, so they must break when it
	// changes.
	workflowPath = "../../.github/workflows/security.yml"

	// discoveryStep is the step name shared by all three jobs' discovery blocks.
	discoveryStep = "Discover Go modules"

	// notShippedMarker is the line a go.mod carries to declare that the module
	// is org-internal and must not drive the scanning toolchain. Kept in sync
	// with security.yml by TestToolsModuleCarriesTheMarker below.
	notShippedMarker = "// quantcli:not-shipped"
)

// modfile renders a minimal go.mod. notShipped stamps the marker that keeps a
// module out of version-file selection.
func modfile(path, goVersion string, notShipped bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\n", path)
	if notShipped {
		fmt.Fprintf(&b, "%s — org-internal tooling.\n", notShippedMarker)
	}
	fmt.Fprintf(&b, "go %s\n", goVersion)
	return b.String()
}

// discoveryScripts loads the discovery block from every job, skipping the whole
// suite where the script's own dependencies are missing rather than reporting a
// failure the developer cannot act on. The scripts use GNU `find -printf`; CI
// runs them on ubuntu-latest, which is also the only environment whose result
// gates a merge.
func discoveryScripts(t *testing.T) []Script {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	out, err := exec.Command("find", ".", "-maxdepth", "0", "-printf", "%P\n").CombinedOutput()
	if err != nil {
		t.Skipf("find does not support -printf (%v): %s", err, out)
	}
	scripts, err := RunScripts(workflowPath, discoveryStep)
	if err != nil {
		t.Fatalf("load discovery scripts: %v", err)
	}
	return scripts
}

// TestVersionFileSelection is the regression check for QUA-99. Every case runs
// against every job's copy of the discovery block, so a copy that drifts fails
// here rather than in production.
func TestVersionFileSelection(t *testing.T) {
	tests := []struct {
		name string
		// fixture is the synthetic repository tree.
		fixture Fixture
		// wantVersionFile is the go.mod that must drive setup-go. Empty means
		// the step is expected to fail before choosing one.
		wantVersionFile string
		// wantModules is the full scan set, which is deliberately wider than
		// the selection set: non-shipped modules are still scanned.
		wantModules []string
		// wantExitCode is 0 unless the step is expected to go red.
		wantExitCode int
		// wantLog, if set, must appear in every job's log.
		wantLog string
		// wantSelectorLog, if set, must appear in the log of the jobs that
		// select a version file. The osv-scanner job shares the discovery half
		// of the block but not the selection half, so it never emits these.
		wantSelectorLog string
	}{
		{
			// The bug. Before the fix the rule ranged over every discovered
			// module, so the non-shipped module's higher directive won and
			// govulncheck analysed the shipped module under a toolchain newer
			// than we release — silently clearing stdlib advisories.
			name: "non-shipped module declaring a higher directive cannot capture the pin",
			fixture: Fixture{
				"compat/go.mod": modfile("github.com/quantcli/common/compat", "1.26.11", false),
				"tools/go.mod":  modfile("github.com/quantcli/common/tools", "1.30.0", true),
			},
			wantVersionFile: "compat/go.mod",
			wantModules:     []string{"compat/go.mod", "tools/go.mod"},
		},
		{
			// quantcli/common as it stands today. This passed before the fix
			// too, but only because 1.26.11 > 1.21 — which is what made the
			// case above reachable by a routine dev-tooling bump.
			name: "quantcli/common today: shipped module already has the higher directive",
			fixture: Fixture{
				"compat/go.mod": modfile("github.com/quantcli/common/compat", "1.26.11", false),
				"tools/go.mod":  modfile("github.com/quantcli/common/tools", "1.21", true),
			},
			wantVersionFile: "compat/go.mod",
			wantModules:     []string{"compat/go.mod", "tools/go.mod"},
		},
		{
			// The export-cli shape: one module at the repository root, nothing
			// marked, so the selection rule is a no-op. This is why the fix is
			// safe to propagate verbatim to repos that have no tools/.
			name: "single root module is selected unchanged",
			fixture: Fixture{
				"go.mod": modfile("github.com/quantcli/crono-export-cli", "1.26.11", false),
			},
			wantVersionFile: "go.mod",
			wantModules:     []string{"go.mod"},
		},
		{
			// The original rule still has to hold among shipped modules: a
			// toolchain older than the highest directive could not build them
			// all.
			name: "among shipped modules the highest directive still wins",
			fixture: Fixture{
				"a/go.mod": modfile("example.com/a", "1.25.0", false),
				"b/go.mod": modfile("example.com/b", "1.26.11", false),
				"c/go.mod": modfile("example.com/c", "1.9", false),
			},
			wantVersionFile: "b/go.mod",
			wantModules:     []string{"a/go.mod", "b/go.mod", "c/go.mod"},
		},
		{
			// A repo with no shipped module has no product pin to protect, so
			// falling back to the plain highest-directive rule over the full set
			// is correct — but it is also how a mis-marked repo looks, hence the
			// warning.
			name: "all modules marked not-shipped falls back to the full set with a warning",
			fixture: Fixture{
				"tools/go.mod": modfile("example.com/tools", "1.21", true),
				"ops/go.mod":   modfile("example.com/ops", "1.22", true),
			},
			wantVersionFile: "ops/go.mod",
			wantModules:     []string{"ops/go.mod", "tools/go.mod"},
			wantSelectorLog: "::warning::Every discovered Go module is marked",
		},
		{
			// The marker is a declaration, not a keyword: prose that merely
			// mentions it must not exclude a shipped module. Without the
			// line anchor this go.mod would silently stop driving the pin.
			name: "marker mentioned in prose does not exclude a shipped module",
			fixture: Fixture{
				"compat/go.mod": "module example.com/compat\n\n" +
					"// This module is shipped. Do not add quantcli:not-shipped here;\n" +
					"// see .github/workflows/security.yml \"Module discovery\".\n" +
					"go 1.26.11\n",
				"tools/go.mod": modfile("example.com/tools", "1.30.0", true),
			},
			wantVersionFile: "compat/go.mod",
			wantModules:     []string{"compat/go.mod", "tools/go.mod"},
		},
		{
			// QUA-83's exclusions, re-pinned: a vendored or fixture go.mod is
			// not ours, so it must neither be scanned nor drive the pin.
			name: "vendor and testdata modules are excluded entirely",
			fixture: Fixture{
				"go.mod":                          modfile("example.com/cli", "1.26.11", false),
				"vendor/example.com/dep/go.mod":   modfile("example.com/dep", "1.31.0", false),
				"internal/testdata/broken/go.mod": modfile("example.com/fixture", "1.32.0", false),
			},
			wantVersionFile: "go.mod",
			wantModules:     []string{"go.mod"},
		},
		{
			// QUA-83's hard failure. Every quantcli repo is Go, so finding
			// nothing means discovery is broken, not that there is nothing to
			// scan — and a skip here is what reported green for months.
			name:         "no module anywhere fails the job",
			fixture:      Fixture{"README.md": "not a Go repo\n"},
			wantExitCode: 1,
			wantLog:      "module discovery is broken",
		},
	}

	scripts := discoveryScripts(t)
	for _, tc := range tests {
		for _, script := range scripts {
			t.Run(tc.name+"/"+script.Job, func(t *testing.T) {
				res, err := Run(t.TempDir(), script.Body, tc.fixture)
				if err != nil {
					t.Fatalf("%s: %v\nlog:\n%s", script, err, res.Log)
				}
				if res.ExitCode != tc.wantExitCode {
					t.Fatalf("%s: exit code = %d, want %d\nlog:\n%s",
						script, res.ExitCode, tc.wantExitCode, res.Log)
				}
				if tc.wantLog != "" && !strings.Contains(res.Log, tc.wantLog) {
					t.Errorf("%s: log does not mention %q\nlog:\n%s", script, tc.wantLog, res.Log)
				}
				if tc.wantExitCode != 0 {
					return
				}

				// The osv-scanner job walks the tree itself, so its copy of
				// the block emits neither output. Everything it does share is
				// asserted above.
				if got, ok := res.Outputs["go-version-file"]; ok {
					if got != tc.wantVersionFile {
						t.Errorf("%s: go-version-file = %q, want %q\nlog:\n%s",
							script, got, tc.wantVersionFile, res.Log)
					}
					if tc.wantSelectorLog != "" && !strings.Contains(res.Log, tc.wantSelectorLog) {
						t.Errorf("%s: log does not mention %q\nlog:\n%s",
							script, tc.wantSelectorLog, res.Log)
					}
				}
				if got, ok := res.Outputs["modules"]; ok {
					want := strings.Join(tc.wantModules, "\n")
					if got != want {
						t.Errorf("%s: modules = %q, want %q\nlog:\n%s",
							script, got, want, res.Log)
					}
				}
			})
		}
	}
}

// TestDiscoveryBlockShape pins the structure the table test relies on. Without
// it, a block accidentally deleted or renamed would make TestVersionFileSelection
// silently cover fewer jobs while still passing.
func TestDiscoveryBlockShape(t *testing.T) {
	scripts := discoveryScripts(t)

	const wantJobs = 3 // govulncheck, osv-scanner, license-policy
	if len(scripts) != wantJobs {
		var jobs []string
		for _, s := range scripts {
			jobs = append(jobs, s.Job)
		}
		t.Fatalf("found %d %q steps in %s (jobs: %v), want %d — every job that "+
			"scans must discover modules",
			len(scripts), discoveryStep, workflowPath, jobs, wantJobs)
	}

	// Two jobs analyse our code and therefore select a version file; one
	// (osv-scanner) only reads lockfiles and uses `go-version: stable`.
	fixture := Fixture{
		"compat/go.mod": modfile("example.com/compat", "1.26.11", false),
		"tools/go.mod":  modfile("example.com/tools", "1.30.0", true),
	}
	var selectors []Script
	for _, script := range scripts {
		res, err := Run(t.TempDir(), script.Body, fixture)
		if err != nil {
			t.Fatalf("%s: %v", script, err)
		}
		if _, ok := res.Outputs["go-version-file"]; ok {
			selectors = append(selectors, script)
		}
	}
	if want := 2; len(selectors) != want {
		t.Fatalf("%d discovery blocks emit go-version-file, want %d", len(selectors), want)
	}

	// Actions has no way to share a step body, so the selection logic is
	// physically duplicated. Byte-identity is the cheapest way to keep the two
	// copies reviewable: a diff between them should be empty, so a one-sided
	// edit is a test failure rather than a subtle behaviour split.
	for _, script := range selectors[1:] {
		if script.Body != selectors[0].Body {
			t.Errorf("%s differs from %s; the discovery blocks in the two "+
				"pinning jobs must stay byte-identical", script, selectors[0])
		}
	}
}

// TestToolsModuleCarriesTheMarker keeps the fix from being inert. The workflow
// change only protects the pin if the non-shipped module in this repo actually
// declares itself, and the marker has to match the pattern security.yml greps
// for: its own comment line in go.mod.
func TestToolsModuleCarriesTheMarker(t *testing.T) {
	// Same anchoring as the workflow's grep: start of line, optional leading
	// whitespace, `//`, the marker, then a word boundary.
	marker := regexp.MustCompile(`(?m)^[ \t]*//[ \t]*` + regexp.QuoteMeta(strings.TrimPrefix(notShippedMarker, "// ")) + `([ \t]|$)`)

	for _, tc := range []struct {
		path string
		want bool
		why  string
	}{
		{"../go.mod", true, "tools is org-internal and builds with `go-version: stable` in ci.yml, so it must never drive the scanning toolchain"},
		{"../../compat/go.mod", false, "compat is the shipped product; marking it would hand the pin to tooling"},
	} {
		raw, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("read %s: %v", tc.path, err)
		}
		if got := marker.Match(raw); got != tc.want {
			t.Errorf("%s: carries %q = %v, want %v — %s",
				tc.path, notShippedMarker, got, tc.want, tc.why)
		}
	}
}
