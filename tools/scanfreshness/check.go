package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Config is the on-disk policy (.github/scan-freshness.json).
type Config struct {
	// Org is the GitHub organisation whose repos are swept.
	Org string `json:"org"`
	// Workflow is the workflow file name every non-exempt repo must run,
	// matched against the workflow's `path` as `.github/workflows/<Workflow>`.
	Workflow string `json:"workflow"`
	// MaxAgeHours is the freshness window: the newest completed run of Workflow
	// on the default branch must be younger than this.
	MaxAgeHours float64 `json:"maxAgeHours"`
	// Exempt lists repos that are not required to run Workflow. Every entry
	// carries a reason so the exemption is reviewable rather than folklore.
	Exempt []Exemption `json:"exempt"`
}

// Exemption excuses one repo from the freshness requirement.
type Exemption struct {
	Repo   string `json:"repo"`
	Reason string `json:"reason"`
}

// LoadConfig reads and validates the policy file.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Org == "" {
		return nil, fmt.Errorf("parse %s: org is required", path)
	}
	if cfg.Workflow == "" {
		return nil, fmt.Errorf("parse %s: workflow is required", path)
	}
	if cfg.MaxAgeHours <= 0 {
		return nil, fmt.Errorf("parse %s: maxAgeHours must be > 0", path)
	}
	for _, e := range cfg.Exempt {
		if e.Repo == "" || e.Reason == "" {
			return nil, fmt.Errorf("parse %s: every exempt entry needs both repo and reason", path)
		}
	}
	return &cfg, nil
}

// Repo is the subset of the repository API we act on.
type Repo struct {
	Name          string `json:"name"`
	Archived      bool   `json:"archived"`
	Disabled      bool   `json:"disabled"`
	Fork          bool   `json:"fork"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
}

// Workflow is the subset of the Actions workflow API we act on. State is the
// field that exposes GitHub's 60-day auto-disable: a workflow switched off for
// inactivity reports `disabled_inactivity` while still existing in the repo.
type Workflow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// Run is the subset of the Actions run API we act on.
type Run struct {
	ID           int64  `json:"id"`
	Event        string `json:"event"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	CreatedAt    string `json:"created_at"`
	RunStartedAt string `json:"run_started_at"`
	HTMLURL      string `json:"html_url"`
}

// Started reports when the run began, preferring run_started_at and falling
// back to created_at (older runs predate run_started_at).
func (r Run) Started() (time.Time, error) {
	for _, s := range []string{r.RunStartedAt, r.CreatedAt} {
		if s == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, fmt.Errorf("run %d: parse timestamp %q: %w", r.ID, s, err)
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("run %d: no timestamp", r.ID)
}

// API is the GitHub surface the check needs. It is an interface so the checker
// can be exercised against canned responses, including the failure modes we
// cannot reproduce on demand against the live org (a cron auto-disabled for
// inactivity, a deleted workflow, a repo that has never completed a run).
type API interface {
	ListRepos(org string) ([]Repo, error)
	ListWorkflows(org, repo string) ([]Workflow, error)
	// LatestCompletedRun returns the newest completed run of workflowID on
	// branch, or nil if the workflow has never completed one there.
	LatestCompletedRun(org, repo string, workflowID int64, branch string) (*Run, error)
}

// Verdict classifies one repo. Only Fail is actionable; Skip and Warn are
// recorded so the report shows the whole sweep rather than just the bad news.
type Verdict string

const (
	Pass Verdict = "pass"
	Fail Verdict = "fail"
	Skip Verdict = "skip"
	Warn Verdict = "warn"
)

// Reason codes. These are stable strings: they end up in the alert issue and in
// SECURITY.md, so operators can map an alert to a runbook entry without reading
// the source.
const (
	ReasonFresh         = "fresh"
	ReasonExempt        = "exempt"
	ReasonArchived      = "archived"
	ReasonWorkflowGone  = "workflow-missing"
	ReasonCronDisabled  = "workflow-disabled"
	ReasonNeverRan      = "no-completed-run"
	ReasonStale         = "stale"
	ReasonScanRed       = "scan-red"
	ReasonUnreadable    = "unreadable"
	ReasonExemptUnknown = "exempt-entry-unknown"
)

// Result is one row of the sweep.
type Result struct {
	Repo    string
	Verdict Verdict
	Reason  string
	Detail  string
	URL     string
}

// Check sweeps the org and returns one Result per repo considered, sorted by
// repo name. The error return is reserved for failures that make the sweep
// itself meaningless (we could not enumerate the org); a per-repo API failure
// is a Fail row, not an abort — an unreadable repo is treated as an unscanned
// repo rather than silently dropped.
func Check(api API, cfg *Config, now time.Time) ([]Result, error) {
	repos, err := api.ListRepos(cfg.Org)
	if err != nil {
		return nil, fmt.Errorf("list repos for %s: %w", cfg.Org, err)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("list repos for %s: org reported zero repos, refusing to report a clean sweep", cfg.Org)
	}

	exempt := map[string]string{}
	for _, e := range cfg.Exempt {
		exempt[e.Repo] = e.Reason
	}
	seen := map[string]bool{}

	var results []Result
	for _, repo := range repos {
		seen[repo.Name] = true
		if reason, ok := exempt[repo.Name]; ok {
			results = append(results, Result{repo.Name, Skip, ReasonExempt, reason, ""})
			continue
		}
		// Archived and fork repos cannot usefully run Actions (archived repos
		// are read-only; forks are not ours to gate). Report, don't fail.
		if repo.Archived || repo.Disabled || repo.Fork {
			results = append(results, Result{repo.Name, Skip, ReasonArchived, "archived, disabled, or a fork", ""})
			continue
		}
		results = append(results, checkRepo(api, cfg, now, repo))
	}

	// An exemption for a repo that no longer exists is config rot, not an
	// exposure: warn so it gets cleaned up, but do not go red over it.
	for _, e := range cfg.Exempt {
		if !seen[e.Repo] {
			results = append(results, Result{e.Repo, Warn, ReasonExemptUnknown,
				"listed as exempt but not present in the org", ""})
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Repo < results[j].Repo })
	return results, nil
}

func checkRepo(api API, cfg *Config, now time.Time, repo Repo) Result {
	wantPath := ".github/workflows/" + cfg.Workflow

	workflows, err := api.ListWorkflows(cfg.Org, repo.Name)
	if err != nil {
		return Result{repo.Name, Fail, ReasonUnreadable,
			fmt.Sprintf("could not list workflows: %v", err), ""}
	}

	var wf *Workflow
	for i := range workflows {
		if workflows[i].Path == wantPath {
			wf = &workflows[i]
			break
		}
	}
	if wf == nil {
		return Result{repo.Name, Fail, ReasonWorkflowGone,
			fmt.Sprintf("no %s in this repo", wantPath), ""}
	}
	// "active" is the only state that actually runs. `disabled_inactivity` is
	// GitHub's 60-day auto-disable for scheduled workflows in public repos --
	// the failure mode this whole check exists for.
	if wf.State != "active" {
		return Result{repo.Name, Fail, ReasonCronDisabled,
			fmt.Sprintf("%s state is %q, not \"active\"", cfg.Workflow, wf.State),
			workflowURL(cfg.Org, repo.Name, cfg.Workflow)}
	}

	branch := repo.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	run, err := api.LatestCompletedRun(cfg.Org, repo.Name, wf.ID, branch)
	if err != nil {
		return Result{repo.Name, Fail, ReasonUnreadable,
			fmt.Sprintf("could not list runs: %v", err), ""}
	}
	if run == nil {
		return Result{repo.Name, Fail, ReasonNeverRan,
			fmt.Sprintf("%s has never completed a run on %s", cfg.Workflow, branch),
			workflowURL(cfg.Org, repo.Name, cfg.Workflow)}
	}

	started, err := run.Started()
	if err != nil {
		return Result{repo.Name, Fail, ReasonUnreadable, err.Error(), run.HTMLURL}
	}
	age := now.Sub(started)
	window := time.Duration(cfg.MaxAgeHours * float64(time.Hour))
	if age > window {
		return Result{repo.Name, Fail, ReasonStale,
			fmt.Sprintf("last completed run on %s was %s ago (window %s)",
				branch, roundHours(age), roundHours(window)), run.HTMLURL}
	}
	// A completed-but-not-successful scan means we have no clean verdict for
	// this repo, which is operationally the same position as not having
	// scanned it. Distinct reason code so the alert says which one it is.
	if run.Conclusion != "success" {
		return Result{repo.Name, Fail, ReasonScanRed,
			fmt.Sprintf("last completed run on %s concluded %q %s ago",
				branch, run.Conclusion, roundHours(age)), run.HTMLURL}
	}

	return Result{repo.Name, Pass, ReasonFresh,
		fmt.Sprintf("last successful run on %s was %s ago", branch, roundHours(age)), run.HTMLURL}
}

func workflowURL(org, repo, workflow string) string {
	return fmt.Sprintf("https://github.com/%s/%s/actions/workflows/%s", org, repo, workflow)
}

func roundHours(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%.1fh", d.Hours())
	}
}

// Failures returns only the actionable rows.
func Failures(results []Result) []Result {
	var out []Result
	for _, r := range results {
		if r.Verdict == Fail {
			out = append(out, r)
		}
	}
	return out
}

// Report renders the sweep as markdown. It is written both to the Actions step
// summary and into the alert issue body, so it must stay free of anything that
// is not safe to publish: repo names, reason codes, ages, and run URLs only.
func Report(cfg *Config, results []Result, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Security scan freshness — `%s`\n\n", cfg.Org)
	fmt.Fprintf(&b, "Workflow `%s`, freshness window %s, checked at %s.\n\n",
		cfg.Workflow, roundHours(time.Duration(cfg.MaxAgeHours*float64(time.Hour))),
		now.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "| Repo | Verdict | Reason | Detail |\n|---|---|---|---|\n")
	for _, r := range results {
		detail := r.Detail
		if r.URL != "" {
			detail = fmt.Sprintf("%s ([run](%s))", detail, r.URL)
		}
		fmt.Fprintf(&b, "| `%s` | %s | `%s` | %s |\n", r.Repo, verdictLabel(r.Verdict), r.Reason, detail)
	}
	failures := Failures(results)
	b.WriteString("\n")
	if len(failures) == 0 {
		fmt.Fprintf(&b, "All non-exempt repos have a successful `%s` run inside the window.\n", cfg.Workflow)
		return b.String()
	}
	fmt.Fprintf(&b, "**%d repo(s) are not currently covered by a fresh scan.** "+
		"See [SECURITY.md](https://github.com/%s/common/blob/main/SECURITY.md#scan-freshness-monitoring) "+
		"for what each reason code means and how to clear it.\n", len(failures), cfg.Org)
	return b.String()
}

func verdictLabel(v Verdict) string {
	switch v {
	case Pass:
		return "PASS"
	case Fail:
		return "**FAIL**"
	case Warn:
		return "WARN"
	default:
		return "skip"
	}
}
