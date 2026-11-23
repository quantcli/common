// Command scanfreshness verifies that every quantcli repo has recently
// completed a successful security scan.
//
// Why this exists: GitHub disables `schedule:` triggers in a public repository
// after 60 days with no repository activity, and announces it by email rather
// than by failing anything. The daily `security.yml` re-scan is therefore a
// detection control that switches itself off in exactly the case it was built
// for — a quiet repo — while the Actions tab keeps showing a green history,
// because the last run that ever happened was green.
//
// This command turns that silence into a red build. It is deliberately a
// detector rather than a workaround (a keepalive commit would reset the timer
// but hide runner outages, deleted workflows, and failing scans), and it sweeps
// the whole org rather than a hardcoded list, so a new repo is covered the day
// it is created unless somebody writes down a reason for it not to be.
//
// Exit codes:
//
//	0  every non-exempt repo has a successful scan inside the window
//	1  at least one repo is stale, disabled, unreadable, or red
//	2  the check itself could not run (bad config, org not enumerable)
//
// Usage:
//
//	scanfreshness [-config .github/scan-freshness.json] [-max-age-hours N] [-summary out.md]
//
// Reads GITHUB_TOKEN (or SCAN_FRESHNESS_TOKEN, which wins if both are set) for
// authentication. The token is only ever used as an Authorization header; it is
// never printed, including in error paths.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const apiBase = "https://api.github.com"

func main() {
	configPath := flag.String("config", ".github/scan-freshness.json", "path to the freshness policy file")
	maxAgeHours := flag.Float64("max-age-hours", 0, "override the policy's freshness window (hours); 0 uses the policy value")
	summaryPath := flag.String("summary", "", "also write the markdown report to this file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanfreshness: %v\n", err)
		os.Exit(2)
	}
	if *maxAgeHours > 0 {
		// Used by the negative test: pointing the live check at a window of
		// effectively zero must turn every row red.
		cfg.MaxAgeHours = *maxAgeHours
	}

	api := &httpAPI{
		client: &http.Client{Timeout: 30 * time.Second},
		token:  firstNonEmpty(os.Getenv("SCAN_FRESHNESS_TOKEN"), os.Getenv("GITHUB_TOKEN")),
	}

	now := time.Now().UTC()
	results, err := Check(api, cfg, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanfreshness: %v\n", err)
		os.Exit(2)
	}

	report := Report(cfg, results, now)
	fmt.Print(report)
	if *summaryPath != "" {
		if err := os.WriteFile(*summaryPath, []byte(report), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "scanfreshness: write summary: %v\n", err)
			os.Exit(2)
		}
	}

	if failures := Failures(results); len(failures) > 0 {
		for _, f := range failures {
			// ::error:: annotations surface on the Actions run itself, so the
			// failure is readable without opening the step log.
			fmt.Fprintf(os.Stderr, "::error::%s: %s (%s)\n", f.Repo, f.Reason, f.Detail)
		}
		os.Exit(1)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// httpAPI is the live GitHub REST implementation of API.
type httpAPI struct {
	client *http.Client
	token  string
}

func (a *httpAPI) get(path string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, apiBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "quantcli-scanfreshness")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		// net/http errors embed the URL, never the headers, so this cannot
		// leak the token.
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s: %s", path, resp.Status, snippet(body))
	}
	return json.Unmarshal(body, out)
}

func snippet(body []byte) string {
	const max = 200
	var msg struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &msg); err == nil && msg.Message != "" {
		body = []byte(msg.Message)
	}
	if len(body) > max {
		return string(body[:max]) + "…"
	}
	return string(body)
}

func (a *httpAPI) ListRepos(org string) ([]Repo, error) {
	var all []Repo
	// Guard rail rather than a real limit: 10 pages is 1000 repos, far past
	// anything this org will hold, and it stops a pagination bug from looping.
	for page := 1; page <= 10; page++ {
		var batch []Repo
		path := fmt.Sprintf("/orgs/%s/repos?per_page=100&page=%d", url.PathEscape(org), page)
		if err := a.get(path, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			return all, nil
		}
	}
	return nil, errors.New("org repo listing did not terminate within 10 pages")
}

// OrgRepoCounts reads the org's own repository tally. `public_repos` is
// returned to any caller; `total_private_repos` only to one with organisation
// read scope, and is absent otherwise — which CoverageScope treats as
// "unknown" rather than zero.
func (a *httpAPI) OrgRepoCounts(org string) (OrgCounts, error) {
	var out OrgCounts
	path := fmt.Sprintf("/orgs/%s", url.PathEscape(org))
	if err := a.get(path, &out); err != nil {
		return OrgCounts{}, err
	}
	return out, nil
}

func (a *httpAPI) ListWorkflows(org, repo string) ([]Workflow, error) {
	var out struct {
		Workflows []Workflow `json:"workflows"`
	}
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows?per_page=100",
		url.PathEscape(org), url.PathEscape(repo))
	if err := a.get(path, &out); err != nil {
		return nil, err
	}
	return out.Workflows, nil
}

func (a *httpAPI) LatestCompletedRun(org, repo string, workflowID int64, branch string) (*Run, error) {
	var out struct {
		Runs []Run `json:"workflow_runs"`
	}
	// branch= matches a run's head branch, which keeps out pull requests from
	// branches inside the repo but not one opened from a fork's own `main`:
	// that run is attributed to head branch "main" too. Left in, a green fork
	// PR would read as "main was scanned" and mask a dead cron, and a red one
	// would read as scan-red. So fetch a page and take the newest run that is
	// not a pull_request event — push / schedule / workflow_dispatch are the
	// runs that actually represent "the default branch is scanned".
	//
	// Deliberately NO `status=completed` here, even though that is exactly what
	// we want and the API offers it. Measured 2026-11-18 against
	// withings-export-cli: a scheduled run that self-reports `"status":
	// "completed", "conclusion": "success"` was omitted from the response when
	// `status=completed` (or `status=success`) was passed, while the same query
	// without it returned the run first. The filter is served from an index
	// that lags the run record, so it silently hides recent runs and we read a
	// stale last-scan time — which over-reports age and fires a false `stale`
	// alert. Filter on the status field client-side instead; it is accurate in
	// the run body. See selectLatestRun.
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%d/runs?branch=%s&per_page=20",
		url.PathEscape(org), url.PathEscape(repo), workflowID, url.QueryEscape(branch))
	if err := a.get(path, &out); err != nil {
		return nil, err
	}
	return selectLatestRun(out.Runs), nil
}

// selectLatestRun picks the newest run that actually represents "the default
// branch has been scanned": completed, and not triggered by a pull request.
// The API returns runs newest-first, so the first match wins.
//
// Both filters are applied here rather than as query parameters on purpose —
// see the note in LatestCompletedRun about `status=completed` dropping recent
// runs. Queued and in-progress runs are skipped rather than treated as the
// answer: a scan that has not finished has not produced a verdict, and
// accepting it would report a repo as freshly scanned on the strength of a job
// that might still fail.
func selectLatestRun(runs []Run) *Run {
	for i := range runs {
		if runs[i].Status != "completed" {
			continue
		}
		if isPullRequestEvent(runs[i].Event) {
			continue
		}
		return &runs[i]
	}
	return nil
}

// isPullRequestEvent reports whether a run was triggered by a pull request
// rather than by activity on the branch itself.
func isPullRequestEvent(event string) bool {
	return event == "pull_request" || event == "pull_request_target"
}
