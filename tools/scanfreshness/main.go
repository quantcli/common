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
	// branch= filters out pull_request runs (those are attributed to the head
	// branch), so what comes back is push / schedule / workflow_dispatch on the
	// default branch — the runs that actually represent "main is scanned".
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%d/runs?branch=%s&status=completed&per_page=1",
		url.PathEscape(org), url.PathEscape(repo), workflowID, url.QueryEscape(branch))
	if err := a.get(path, &out); err != nil {
		return nil, err
	}
	if len(out.Runs) == 0 {
		return nil, nil
	}
	return &out.Runs[0], nil
}
