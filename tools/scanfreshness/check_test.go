package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// fakeAPI serves canned responses. The failure modes that matter here cannot be
// produced on demand against the live org — we are not going to leave a repo
// untouched for 60 days to watch GitHub switch its cron off — so they are
// reproduced from the shapes the REST API actually returns.
type fakeAPI struct {
	repos     []Repo
	reposErr  error
	workflows map[string][]Workflow
	wfErr     map[string]error
	runs      map[string]*Run
	runErr    map[string]error
}

func (f *fakeAPI) ListRepos(string) ([]Repo, error) { return f.repos, f.reposErr }

func (f *fakeAPI) ListWorkflows(_, repo string) ([]Workflow, error) {
	if err := f.wfErr[repo]; err != nil {
		return nil, err
	}
	return f.workflows[repo], nil
}

func (f *fakeAPI) LatestCompletedRun(_, repo string, _ int64, _ string) (*Run, error) {
	if err := f.runErr[repo]; err != nil {
		return nil, err
	}
	return f.runs[repo], nil
}

func repo(name string) Repo { return Repo{Name: name, DefaultBranch: "main"} }

func securityWorkflow(state string) []Workflow {
	return []Workflow{
		{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
		{ID: 2, Name: "security", Path: ".github/workflows/security.yml", State: state},
	}
}

func runAt(t time.Time, conclusion string) *Run {
	return &Run{
		ID:           99,
		Event:        "schedule",
		Status:       "completed",
		Conclusion:   conclusion,
		CreatedAt:    t.Format(time.RFC3339),
		RunStartedAt: t.Format(time.RFC3339),
		HTMLURL:      "https://github.com/quantcli/x/actions/runs/99",
	}
}

func testConfig() *Config {
	return &Config{Org: "quantcli", Workflow: "security.yml", MaxAgeHours: 48}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		api         *fakeAPI
		wantVerdict Verdict
		wantReason  string
	}{
		{
			name: "fresh successful run passes",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("common")},
				workflows: map[string][]Workflow{"common": securityWorkflow("active")},
				runs:      map[string]*Run{"common": runAt(now.Add(-6*time.Hour), "success")},
			},
			wantVerdict: Pass,
			wantReason:  ReasonFresh,
		},
		{
			// The regression this whole tool exists for: GitHub's 60-day
			// auto-disable leaves the workflow in place and green, and only
			// `state` gives it away.
			name: "cron auto-disabled for inactivity fails",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("crono-export-cli")},
				workflows: map[string][]Workflow{"crono-export-cli": securityWorkflow("disabled_inactivity")},
				runs:      map[string]*Run{"crono-export-cli": runAt(now.Add(-1*time.Hour), "success")},
			},
			wantVerdict: Fail,
			wantReason:  ReasonCronDisabled,
		},
		{
			name: "manually disabled workflow fails",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("withings-export-cli")},
				workflows: map[string][]Workflow{"withings-export-cli": securityWorkflow("disabled_manually")},
				runs:      map[string]*Run{"withings-export-cli": runAt(now.Add(-1*time.Hour), "success")},
			},
			wantVerdict: Fail,
			wantReason:  ReasonCronDisabled,
		},
		{
			name: "run older than the window fails",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("liftoff-export-cli")},
				workflows: map[string][]Workflow{"liftoff-export-cli": securityWorkflow("active")},
				runs:      map[string]*Run{"liftoff-export-cli": runAt(now.Add(-49*time.Hour), "success")},
			},
			wantVerdict: Fail,
			wantReason:  ReasonStale,
		},
		{
			name: "run exactly at the window boundary passes",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("liftoff-export-cli")},
				workflows: map[string][]Workflow{"liftoff-export-cli": securityWorkflow("active")},
				runs:      map[string]*Run{"liftoff-export-cli": runAt(now.Add(-48*time.Hour), "success")},
			},
			wantVerdict: Pass,
			wantReason:  ReasonFresh,
		},
		{
			name: "workflow deleted from the repo fails",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos: []Repo{repo("crono-export-cli")},
				workflows: map[string][]Workflow{"crono-export-cli": {
					{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: "active"},
				}},
			},
			wantVerdict: Fail,
			wantReason:  ReasonWorkflowGone,
		},
		{
			name: "workflow that has never completed a run fails",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("new-export-cli")},
				workflows: map[string][]Workflow{"new-export-cli": securityWorkflow("active")},
				runs:      map[string]*Run{},
			},
			wantVerdict: Fail,
			wantReason:  ReasonNeverRan,
		},
		{
			// A completed-but-failed scan leaves us with no clean verdict,
			// which is operationally the same as not having scanned at all.
			name: "fresh but failing run fails with its own reason code",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("common")},
				workflows: map[string][]Workflow{"common": securityWorkflow("active")},
				runs:      map[string]*Run{"common": runAt(now.Add(-2*time.Hour), "failure")},
			},
			wantVerdict: Fail,
			wantReason:  ReasonScanRed,
		},
		{
			name: "cancelled run fails",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("common")},
				workflows: map[string][]Workflow{"common": securityWorkflow("active")},
				runs:      map[string]*Run{"common": runAt(now.Add(-2*time.Hour), "cancelled")},
			},
			wantVerdict: Fail,
			wantReason:  ReasonScanRed,
		},
		{
			// Fail securely: a repo we cannot read is an unscanned repo.
			name: "unreadable repo fails rather than being skipped",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos: []Repo{repo("secret-thing")},
				wfErr: map[string]error{"secret-thing": errors.New("GET /repos/...: 404 Not Found")},
			},
			wantVerdict: Fail,
			wantReason:  ReasonUnreadable,
		},
		{
			name: "unreadable run list fails",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("common")},
				workflows: map[string][]Workflow{"common": securityWorkflow("active")},
				runErr:    map[string]error{"common": errors.New("GET /repos/...: 403 Forbidden")},
			},
			wantVerdict: Fail,
			wantReason:  ReasonUnreadable,
		},
		{
			name: "unparseable run timestamp fails",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos:     []Repo{repo("common")},
				workflows: map[string][]Workflow{"common": securityWorkflow("active")},
				runs: map[string]*Run{"common": {
					ID: 7, Status: "completed", Conclusion: "success", CreatedAt: "not-a-time",
				}},
			},
			wantVerdict: Fail,
			wantReason:  ReasonUnreadable,
		},
		{
			name: "exempt repo is skipped with its reason",
			cfg: &Config{Org: "quantcli", Workflow: "security.yml", MaxAgeHours: 48,
				Exempt: []Exemption{{Repo: "homebrew-tap", Reason: "no dependency tree of our own"}}},
			api: &fakeAPI{
				repos:     []Repo{repo("homebrew-tap")},
				workflows: map[string][]Workflow{},
			},
			wantVerdict: Skip,
			wantReason:  ReasonExempt,
		},
		{
			name: "archived repo is skipped",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos: []Repo{{Name: "old-cli", Archived: true, DefaultBranch: "main"}},
			},
			wantVerdict: Skip,
			wantReason:  ReasonArchived,
		},
		{
			name: "fork is skipped",
			cfg:  testConfig(),
			api: &fakeAPI{
				repos: []Repo{{Name: "upstream-fork", Fork: true, DefaultBranch: "main"}},
			},
			wantVerdict: Skip,
			wantReason:  ReasonArchived,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			results, err := Check(tc.api, tc.cfg, now)
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1: %+v", len(results), results)
			}
			got := results[0]
			if got.Verdict != tc.wantVerdict || got.Reason != tc.wantReason {
				t.Errorf("got verdict=%s reason=%s, want verdict=%s reason=%s (detail: %s)",
					got.Verdict, got.Reason, tc.wantVerdict, tc.wantReason, got.Detail)
			}
			wantFailures := 0
			if tc.wantVerdict == Fail {
				wantFailures = 1
			}
			if n := len(Failures(results)); n != wantFailures {
				t.Errorf("got %d failures, want %d", n, wantFailures)
			}
		})
	}
}

// A brand new repo nobody has wired up yet must go red rather than be invisible:
// org-wide discovery is the point of not hardcoding a repo list.
func TestCheckCoversNewReposByDefault(t *testing.T) {
	api := &fakeAPI{
		repos: []Repo{repo("common"), repo("brand-new-export-cli")},
		workflows: map[string][]Workflow{
			"common":               securityWorkflow("active"),
			"brand-new-export-cli": {},
		},
		runs: map[string]*Run{"common": runAt(now.Add(-3*time.Hour), "success")},
	}
	results, err := Check(api, testConfig(), now)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	failures := Failures(results)
	if len(failures) != 1 || failures[0].Repo != "brand-new-export-cli" || failures[0].Reason != ReasonWorkflowGone {
		t.Fatalf("expected brand-new-export-cli to fail as workflow-missing, got %+v", failures)
	}
}

func TestCheckWarnsOnStaleExemption(t *testing.T) {
	cfg := testConfig()
	cfg.Exempt = []Exemption{{Repo: "deleted-repo", Reason: "gone"}}
	api := &fakeAPI{
		repos:     []Repo{repo("common")},
		workflows: map[string][]Workflow{"common": securityWorkflow("active")},
		runs:      map[string]*Run{"common": runAt(now.Add(-1*time.Hour), "success")},
	}
	results, err := Check(api, cfg, now)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if len(Failures(results)) != 0 {
		t.Errorf("a stale exemption is config rot, not an exposure; should not fail: %+v", Failures(results))
	}
	var warned bool
	for _, r := range results {
		if r.Verdict == Warn && r.Reason == ReasonExemptUnknown && r.Repo == "deleted-repo" {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a warn row for the unknown exempt repo, got %+v", results)
	}
}

// An org listing that comes back empty would otherwise render as a clean sweep
// of zero repos — a green check that proves nothing.
func TestCheckRefusesEmptyOrg(t *testing.T) {
	if _, err := Check(&fakeAPI{}, testConfig(), now); err == nil {
		t.Fatal("expected an error for an org that lists zero repos")
	}
}

func TestCheckPropagatesListError(t *testing.T) {
	api := &fakeAPI{reposErr: errors.New("502 Bad Gateway")}
	if _, err := Check(api, testConfig(), now); err == nil {
		t.Fatal("expected an error when the org cannot be enumerated")
	}
}

func TestRunStartedPrefersRunStartedAt(t *testing.T) {
	r := Run{
		ID:           1,
		CreatedAt:    "2026-11-10T00:00:00Z",
		RunStartedAt: "2026-11-12T00:00:00Z",
	}
	got, err := r.Started()
	if err != nil {
		t.Fatalf("Started: %v", err)
	}
	if want := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}

	// Runs predating run_started_at fall back to created_at.
	r.RunStartedAt = ""
	got, err = r.Started()
	if err != nil {
		t.Fatalf("Started: %v", err)
	}
	if want := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("fallback: got %s, want %s", got, want)
	}
}

func TestReportMarksFailuresAndStaysPublishable(t *testing.T) {
	api := &fakeAPI{
		repos: []Repo{repo("common"), repo("crono-export-cli")},
		workflows: map[string][]Workflow{
			"common":           securityWorkflow("active"),
			"crono-export-cli": securityWorkflow("disabled_inactivity"),
		},
		runs: map[string]*Run{
			"common":           runAt(now.Add(-2*time.Hour), "success"),
			"crono-export-cli": runAt(now.Add(-70*24*time.Hour), "success"),
		},
	}
	results, err := Check(api, testConfig(), now)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	report := Report(testConfig(), results, now)
	for _, want := range []string{"crono-export-cli", ReasonCronDisabled, "**FAIL**", "1 repo(s)"} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	// The report is posted into a public issue; it must not grow fields that
	// are not safe to publish.
	for _, forbidden := range []string{"Authorization", "Bearer", "token"} {
		if strings.Contains(report, forbidden) {
			t.Errorf("report contains %q, which must never be published:\n%s", forbidden, report)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	good := write("good.json", `{
		"org": "quantcli",
		"workflow": "security.yml",
		"maxAgeHours": 48,
		"exempt": [{"repo": "homebrew-tap", "reason": "no module of our own"}]
	}`)
	cfg, err := LoadConfig(good)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Org != "quantcli" || cfg.MaxAgeHours != 48 || len(cfg.Exempt) != 1 {
		t.Errorf("unexpected config: %+v", cfg)
	}

	bad := map[string]string{
		"missing org":         `{"workflow": "security.yml", "maxAgeHours": 48}`,
		"missing workflow":    `{"org": "quantcli", "maxAgeHours": 48}`,
		"zero window":         `{"org": "quantcli", "workflow": "security.yml", "maxAgeHours": 0}`,
		"negative window":     `{"org": "quantcli", "workflow": "security.yml", "maxAgeHours": -1}`,
		"exemption no reason": `{"org": "q", "workflow": "s.yml", "maxAgeHours": 1, "exempt": [{"repo": "x"}]}`,
		"unknown field typo":  `{"org": "q", "workflow": "s.yml", "maxAgeHrs": 48}`,
		"not json":            `nope`,
	}
	for name, body := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(write(strings.ReplaceAll(name, " ", "_")+".json", body)); err == nil {
				t.Errorf("expected %s to be rejected", name)
			}
		})
	}

	if _, err := LoadConfig(filepath.Join(dir, "absent.json")); err == nil {
		t.Error("expected a missing config file to be an error")
	}
}

// The shipped policy file must parse and must actually cover the repos we care
// about: a typo here disables the check in production, not in a test.
func TestShippedPolicyFileIsValid(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", ".github", "scan-freshness.json"))
	if err != nil {
		t.Fatalf("shipped policy file: %v", err)
	}
	if cfg.Org != "quantcli" {
		t.Errorf("org = %q, want quantcli", cfg.Org)
	}
	if cfg.Workflow != "security.yml" {
		t.Errorf("workflow = %q, want security.yml", cfg.Workflow)
	}
	// Wide enough to survive one missed daily run, tight enough that two
	// consecutive misses are loud.
	if cfg.MaxAgeHours < 24 || cfg.MaxAgeHours > 72 {
		t.Errorf("maxAgeHours = %v, want between 24 and 72", cfg.MaxAgeHours)
	}
	for _, e := range cfg.Exempt {
		if strings.HasSuffix(e.Repo, "-export-cli") || e.Repo == "common" {
			t.Errorf("%s is a code repo and must not be exempt from scanning", e.Repo)
		}
	}
}
