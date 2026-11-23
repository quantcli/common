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
	counts    *OrgCounts
	countsErr error
	workflows map[string][]Workflow
	wfErr     map[string]error
	runs      map[string]*Run
	runErr    map[string]error
}

func (f *fakeAPI) ListRepos(string) ([]Repo, error) { return f.repos, f.reposErr }

// OrgRepoCounts defaults to agreeing exactly with ListRepos, so the coverage
// row passes and each test exercises only the behaviour it is named for. Tests
// about coverage set counts/countsErr explicitly.
func (f *fakeAPI) OrgRepoCounts(string) (OrgCounts, error) {
	if f.countsErr != nil {
		return OrgCounts{}, f.countsErr
	}
	if f.counts != nil {
		return *f.counts, nil
	}
	pub, priv := 0, 0
	for _, r := range f.repos {
		if r.Private {
			priv++
		} else {
			pub++
		}
	}
	return OrgCounts{PublicRepos: pub, TotalPrivate: &priv}, nil
}

// repoRows drops the synthetic coverage row so a test can assert on the
// per-repo verdicts it is actually about.
func repoRows(results []Result) []Result {
	var out []Result
	for _, r := range results {
		if r.Repo != coverageRow {
			out = append(out, r)
		}
	}
	return out
}

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
			all, err := Check(tc.api, tc.cfg, now)
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			results := repoRows(all)
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

// The regression this encodes was observed in production on 2026-11-23.
//
// `/orgs/{org}/repos` is filtered by what the token may see. The scheduled
// run's repo-scoped GITHUB_TOKEN enumerated 6 of quantcli's 7 repos — the
// invisible one being a private repo created three days earlier with no
// security.yml — and the report's closing line read "All non-exempt repos have
// a successful run inside the window". Three consecutive scheduled runs were
// green while a repo in the org was never scanned, and nothing in the output
// distinguished that from genuine org-wide coverage.
//
// A repo the watcher cannot see is exactly the silent-coverage failure the
// watcher exists to remove, so a provable shortfall must go red.
func TestCheckFailsWhenSweepCannotSeeEveryRepo(t *testing.T) {
	// The org reports one private repo; the sweep enumerated none.
	onePrivate := 1
	api := &fakeAPI{
		repos:     []Repo{repo("common")},
		counts:    &OrgCounts{PublicRepos: 1, TotalPrivate: &onePrivate},
		workflows: map[string][]Workflow{"common": securityWorkflow("active")},
		runs:      map[string]*Run{"common": runAt(now.Add(-1*time.Hour), "success")},
	}
	results, err := Check(api, testConfig(), now)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	failures := Failures(results)
	if len(failures) != 1 || failures[0].Reason != ReasonCoverageShort {
		t.Fatalf("a sweep that saw 0 of 1 private repos must fail %s, got %+v",
			ReasonCoverageShort, failures)
	}
	// And the green sentence must not appear when coverage is short.
	if report := Report(testConfig(), results, now); strings.Contains(report, "inside the window — see") {
		t.Errorf("report claimed clean coverage despite a coverage failure:\n%s", report)
	}
}

// A missed public repo is always provable, because public_repos is readable by
// any caller. Pagination bugs and permission oddities both land here.
func TestCheckFailsOnMissedPublicRepo(t *testing.T) {
	api := &fakeAPI{
		repos:     []Repo{repo("common")},
		counts:    &OrgCounts{PublicRepos: 4},
		workflows: map[string][]Workflow{"common": securityWorkflow("active")},
		runs:      map[string]*Run{"common": runAt(now.Add(-1*time.Hour), "success")},
	}
	results, err := Check(api, testConfig(), now)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	failures := Failures(results)
	if len(failures) != 1 || failures[0].Reason != ReasonCoverageShort {
		t.Fatalf("a sweep that saw 1 of 4 public repos must fail %s, got %+v",
			ReasonCoverageShort, failures)
	}
}

// Without org read scope the private count is absent. That is not provably a
// gap, so it must not go red every run — a row that always fails gets muted,
// and then the real failure is invisible too. But it must not be reported as
// full coverage either: the sweep says what it could and could not see.
func TestCheckDisclosesUnverifiablePrivateScope(t *testing.T) {
	api := &fakeAPI{
		repos:     []Repo{repo("common")},
		counts:    &OrgCounts{PublicRepos: 1, TotalPrivate: nil},
		workflows: map[string][]Workflow{"common": securityWorkflow("active")},
		runs:      map[string]*Run{"common": runAt(now.Add(-1*time.Hour), "success")},
	}
	results, err := Check(api, testConfig(), now)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if n := len(Failures(results)); n != 0 {
		t.Errorf("an unreadable private count is unknown, not a proven gap; want 0 failures, got %d", n)
	}
	var disclosed bool
	for _, r := range results {
		if r.Repo == coverageRow && r.Verdict == Warn && r.Reason == ReasonCoverageUnver {
			disclosed = true
		}
	}
	if !disclosed {
		t.Fatalf("expected a %s warn row disclosing the scope, got %+v", ReasonCoverageUnver, results)
	}
	// The closing sentence must point at the scope row rather than assert
	// coverage it cannot demonstrate.
	report := Report(testConfig(), results, now)
	if strings.Contains(report, "All non-exempt repos have") {
		t.Errorf("report still makes the unqualified org-wide claim:\n%s", report)
	}
	if !strings.Contains(report, coverageRow) {
		t.Errorf("report does not surface the coverage row:\n%s", report)
	}
}

// If the org endpoint itself cannot be read, the sweep has no way to check its
// own reach, so it fails rather than guessing.
func TestCheckFailsWhenOrgTallyUnreadable(t *testing.T) {
	api := &fakeAPI{
		repos:     []Repo{repo("common")},
		countsErr: errors.New("403 Forbidden"),
		workflows: map[string][]Workflow{"common": securityWorkflow("active")},
		runs:      map[string]*Run{"common": runAt(now.Add(-1*time.Hour), "success")},
	}
	results, err := Check(api, testConfig(), now)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	failures := Failures(results)
	if len(failures) != 1 || failures[0].Reason != ReasonCoverageShort {
		t.Fatalf("want %s when the org tally is unreadable, got %+v", ReasonCoverageShort, failures)
	}
	// The failure detail is published into a public issue; it carries the
	// status line only, never the credential that failed.
	for _, forbidden := range []string{"Bearer", "Authorization"} {
		if strings.Contains(failures[0].Detail, forbidden) {
			t.Errorf("coverage failure detail leaked %q: %s", forbidden, failures[0].Detail)
		}
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

// Regression for the stale-read bug found in production on 2026-11-18.
//
// The checker used to ask the API for `status=completed` and take the first
// row. Measured against withings-export-cli that day, a scheduled run which
// self-reported `"status": "completed", "conclusion": "success"` was omitted
// from the response whenever `status=completed` was passed, while the
// unfiltered query returned it first. The checker therefore read a last-scan
// time ~22h older than the truth, which over-reports age and would have fired
// a false `stale` alert once the real gap crossed the window.
//
// The fix filters on status client-side, so these cases pin the selection
// rules against an unfiltered page exactly as the API returns it.
func TestSelectLatestRun(t *testing.T) {
	completed := func(id int64, event, status string) Run {
		return Run{ID: id, Event: event, Status: status, Conclusion: "success"}
	}

	tests := []struct {
		name   string
		runs   []Run
		wantID int64 // expected run ID, 0 means expect nil
	}{
		{
			name:   "newest completed branch run wins",
			runs:   []Run{completed(3, "schedule", "completed"), completed(2, "push", "completed")},
			wantID: 3,
		},
		{
			// The shape that caused the bug: an unfiltered page whose newest
			// entry is the fresh scheduled run. The old code never saw it.
			name: "fresh scheduled run is not skipped",
			runs: []Run{
				completed(44337479064, "schedule", "completed"),
				completed(44144674797, "workflow_dispatch", "completed"),
			},
			wantID: 44337479064,
		},
		{
			// Without the query filter, in-flight runs now appear in the page.
			name: "in-progress and queued runs are skipped",
			runs: []Run{
				completed(5, "schedule", "in_progress"),
				completed(4, "schedule", "queued"),
				completed(3, "push", "completed"),
			},
			wantID: 3,
		},
		{
			name: "pull request runs are still skipped",
			runs: []Run{
				completed(9, "pull_request", "completed"),
				completed(8, "pull_request_target", "completed"),
				completed(7, "schedule", "completed"),
			},
			wantID: 7,
		},
		{
			name:   "nothing qualifying yields nil",
			runs:   []Run{completed(2, "pull_request", "completed"), completed(1, "schedule", "in_progress")},
			wantID: 0,
		},
		{name: "empty page yields nil", runs: nil, wantID: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := selectLatestRun(tc.runs)
			if tc.wantID == 0 {
				if got != nil {
					t.Fatalf("got run %d, want nil", got.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("got nil, want run %d", tc.wantID)
			}
			if got.ID != tc.wantID {
				t.Errorf("got run %d, want %d", got.ID, tc.wantID)
			}
		})
	}
}
