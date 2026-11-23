package workflowtriggers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The `on:` block security.yml carried until QUA-115, verbatim. This is the
// regression case: the checker exists to turn this shape red.
const preFixSecurityOn = `name: security

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  # Daily re-scan.
  schedule:
    - cron: '17 6 * * *'
  workflow_dispatch:

permissions: {}
`

func TestCheckSource(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string // substrings, one per expected finding, in order
	}{
		{
			name: "the bug this checker exists for",
			src:  preFixSecurityOn,
			want: []string{"pull_request`: `branches` restricts"},
		},
		{
			name: "fixed shape passes",
			src: `on:
  push:
    branches: [main]
  pull_request:
  schedule:
    - cron: '17 6 * * *'
`,
		},
		{
			name: "push may be pinned to main",
			src: `on:
  push:
    branches: [main, release/*]
    paths-ignore: ['**.md']
`,
		},
		{
			name: "paths filter is not a base-branch filter",
			src: `on:
  pull_request:
    paths:
      - '.github/workflows/scan-freshness.yml'
      - 'tools/**'
`,
		},
		{
			name: "types filter is not a base-branch filter",
			src: `on:
  pull_request_target:
    types: [opened, reopened, ready_for_review]
`,
		},
		{
			name: "branches-ignore counts too",
			src: `on:
  pull_request:
    branches-ignore: [experimental]
`,
			want: []string{"`branches-ignore` restricts"},
		},
		{
			name: "privileged trigger is covered",
			src: `on:
  pull_request_target:
    types: [opened]
    branches: [main]
`,
			want: []string{"pull_request_target`: `branches` restricts"},
		},
		{
			name: "both triggers reported independently",
			src: `on:
  pull_request:
    branches: [main]
  pull_request_target:
    branches: [main]
`,
			want: []string{"pull_request`: `branches`", "pull_request_target`: `branches`"},
		},
		{
			name: "flow sequence cannot carry a filter",
			src:  "on: [push, pull_request]\n",
		},
		{
			name: "bare scalar cannot carry a filter",
			src:  "on: pull_request\n",
		},
		{
			name: "quoted on key is still the on key",
			src: `'on':
  pull_request:
    branches: [main]
`,
			want: []string{"`branches` restricts"},
		},
		{
			name: "yaml 1.1 spelling of the on key",
			src: `true:
  pull_request:
    branches: [main]
`,
			want: []string{"`branches` restricts"},
		},
		{
			name: "no on key at all",
			src: `name: reusable
jobs: {}
`,
		},
		{
			name: "trailing comment on the trigger key does not hide the filter",
			src: `on:
  pull_request: # scanned on every PR
    branches: [main]
`,
			want: []string{"`branches` restricts"},
		},
		{
			name: "comment lines inside the block are not keys",
			src: `on:
  # pull_request:
  #   branches: [main]
  push:
    branches: [main]
`,
		},
		{
			name: "a branches key under jobs is out of scope",
			src: `on:
  pull_request:
jobs:
  build:
    steps:
      - run: echo branches: [main]
`,
		},
		{
			name: "marker above the trigger opts out",
			src: `on:
  # quantcli:base-branch-filter-ok — relays only matter for main-targeted PRs
  pull_request:
    branches: [main]
`,
		},
		{
			name: "marker inside the trigger body opts out",
			src: `on:
  pull_request:
    # quantcli:base-branch-filter-ok — see ADR-7
    branches: [main]
`,
		},
		{
			name: "marker without a reason is not an exemption",
			src: `on:
  # quantcli:base-branch-filter-ok
  pull_request:
    branches: [main]
`,
			want: []string{"`branches` restricts"},
		},
		{
			name: "marker on one trigger does not cover the next",
			src: `on:
  # quantcli:base-branch-filter-ok — deliberate, see ADR-7
  pull_request_target:
    branches: [main]
  pull_request:
    branches: [main]
`,
			want: []string{"pull_request`: `branches`"},
		},
		{
			name: "inline mapping is refused rather than guessed at",
			src: `on:
  pull_request: {branches: [main]}
`,
			want: []string{"rewrite as a block mapping"},
		},
		{
			name: "on with neither value nor body",
			src: `on:
jobs: {}
`,
			want: []string{"no value and no block body"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckSource("workflow.yml", tc.src)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d finding(s), want %d\ngot: %v", len(got), len(tc.want), got)
			}
			for i, want := range tc.want {
				if !strings.Contains(got[i].String(), want) {
					t.Errorf("finding %d = %q, want it to contain %q", i, got[i], want)
				}
			}
		})
	}
}

func TestFindingPointsAtTheFilterLine(t *testing.T) {
	got := CheckSource("security.yml", preFixSecurityOn)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(got), got)
	}
	// Line 7 of preFixSecurityOn is `    branches: [main]` under pull_request.
	// The filter line, not the trigger line: that is the line to delete.
	if got[0].Line != 7 {
		t.Errorf("finding line = %d, want 7 (the `branches:` line)", got[0].Line)
	}
	if got[0].File != "security.yml" {
		t.Errorf("finding file = %q, want %q", got[0].File, "security.yml")
	}
}

// TestRepoWorkflowsPass is the live half of the regression check: it runs the
// rule over this repository's real workflow files. It fails against the
// pre-QUA-115 tree and passes after it, and it is what stops the filter from
// being reintroduced here — including in a workflow added later, or one copied
// in from an export-cli.
func TestRepoWorkflowsPass(t *testing.T) {
	dir := filepath.Join("..", "..", ".github", "workflows")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("workflow dir not found from the tools module: %v", err)
	}
	findings, err := Check(dir)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
	if len(findings) > 0 {
		t.Log("A pull_request trigger pinned to a base branch means PRs stacked on " +
			"another branch run nothing and report nothing. Drop the filter, or opt " +
			"out with a `# " + MarkerPrefix + " <reason>` comment.")
	}
}

func TestCheckRejectsAnEmptyDir(t *testing.T) {
	if _, err := Check(t.TempDir()); err == nil {
		t.Fatal("want an error for a directory with no workflow files, got nil")
	}
}

func TestCheckRejectsAMissingDir(t *testing.T) {
	if _, err := Check(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want an error for a missing directory, got nil")
	}
}

func TestCheckReadsBothExtensionsAndSorts(t *testing.T) {
	dir := t.TempDir()
	bad := "on:\n  pull_request:\n    branches: [main]\n"
	for _, name := range []string{"b.yaml", "a.yml", "notes.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2 (one per workflow file, none for notes.md): %v", len(findings), findings)
	}
	if filepath.Base(findings[0].File) != "a.yml" || filepath.Base(findings[1].File) != "b.yaml" {
		t.Errorf("findings not sorted by file: %v", findings)
	}
}
