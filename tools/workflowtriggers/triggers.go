// Package workflowtriggers enforces one rule over this repo's workflow files:
// a `pull_request` (or `pull_request_target`) trigger must not be restricted by
// base branch.
//
// Why it is worth a checker rather than a review habit: the failure mode is an
// absent check, not a red one. With
//
//	on:
//	  pull_request:
//	    branches: [main]
//
// a pull request whose base is another PR's branch runs nothing at all — no
// govulncheck, no osv-scanner, no licence allowlist, no lint, no tests — and the
// PR page shows no sign that anything is missing. Nothing appears in any run
// history either, because no run ever started. A dependency bump merged through
// such a chain in QUA-113. Every other control in this repo announces its own
// absence (security.yml fails rather than skips when module discovery finds
// nothing; scan-freshness.yml goes red when a cron is auto-disabled); this one
// could not, so the check lives here instead.
//
// `push: branches: [main]` is deliberately untouched: push-triggered runs answer
// "did the merged state scan clean", which is a question about main.
//
// A filter that is genuinely wanted opts in with a marker comment carrying a
// reason — see MarkerPrefix. The default is the safe one, and the dangerous one
// has to be written down.
//
// The scanner is hand-rolled because the tools module is standard-library-only
// (see tools/go.mod). It understands the `on:` forms this org actually writes:
// a block mapping, a flow sequence (`on: [push, pull_request]`), and a bare
// scalar (`on: pull_request`). An `on:` block it cannot read is reported as a
// finding rather than passed over, on the same principle as the rest of the
// file — a control that cannot tell you whether it checked anything is not a
// control.
package workflowtriggers

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MarkerPrefix opts one trigger out of the rule. It must be followed by a
// reason on the same line, and must appear either in the comment block directly
// above the trigger key or inside the trigger's own mapping:
//
//	# quantcli:base-branch-filter-ok — relays reviews on main-targeted PRs only
//	pull_request:
//	  branches: [main]
const MarkerPrefix = "quantcli:base-branch-filter-ok"

// Triggers under the rule. `pull_request_target` is included because it is the
// same event shape with more privilege: a silently skipped privileged workflow
// (auto-assign.yml, code-owner-approval.yml) means a PR merges without the gate
// it looks like it passed.
var checkedTriggers = []string{"pull_request", "pull_request_target"}

// Base-branch filter keys. `paths`/`paths-ignore` are not here on purpose:
// they scope a workflow to the files it is about, which is orthogonal to the
// base of the branch (scan-freshness.yml relies on exactly that).
var filterKeys = []string{"branches", "branches-ignore"}

// Finding is one violation. Line is 1-indexed, so the string form is clickable
// in a CI log.
type Finding struct {
	File    string // path as given to the checker
	Trigger string // "pull_request" or "pull_request_target"
	Problem string
	Line    int
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: `%s`: %s", f.File, f.Line, f.Trigger, f.Problem)
}

// Check walks dir for workflow files and returns every finding, sorted by file
// then line. A directory with no workflow files is an error: the only reason to
// point this checker somewhere is to check something, so "nothing found" is a
// wrong path rather than a clean result.
func Check(dir string) ([]Finding, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read workflow dir: %w", err)
	}

	var (
		findings []Finding
		files    int
	)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		files++
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		findings = append(findings, CheckSource(path, string(raw))...)
	}
	if files == 0 {
		return nil, fmt.Errorf("%s: no workflow files found", dir)
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

// CheckSource applies the rule to one workflow file's contents. name is used
// only in findings.
func CheckSource(name, src string) []Finding {
	lines := strings.Split(src, "\n")

	start, inline, ok := findOnKey(lines)
	if !ok {
		// No `on:` at all. A reusable workflow (`workflow_call`) or a malformed
		// file; either way there is no pull_request trigger to restrict.
		return nil
	}
	if inline != "" {
		// `on: pull_request` or `on: [push, pull_request]`. Neither form can
		// carry a branches filter, so both are compliant by construction.
		return nil
	}

	block := blockAfter(lines, start)
	baseIndent := -1
	for _, ln := range block {
		if !isBlank(lines[ln]) {
			baseIndent = indentOf(lines[ln])
			break
		}
	}
	if baseIndent < 0 {
		return []Finding{{name, "on", "`on:` has no value and no block body", start + 1}}
	}

	var findings []Finding
	for i, ln := range block {
		line := lines[ln]
		if isBlank(line) || indentOf(line) != baseIndent {
			continue
		}
		key, value, isKey := splitKey(strings.TrimSpace(line))
		if !isKey || !isChecked(key) {
			continue
		}
		if value != "" {
			// e.g. `pull_request: {types: [opened]}`. Flow mappings are not a
			// shape this org writes, and reading one properly needs a YAML
			// parser, so refuse to vouch for it rather than guess.
			if strings.ContainsAny(value, "{[") {
				findings = append(findings, Finding{name, key,
					"inline mapping or sequence — rewrite as a block mapping so the base-branch filter rule can be checked", ln + 1})
			}
			continue
		}

		sub := childLines(lines, block[i+1:], baseIndent)
		if hasMarker(lines, ln, sub) {
			continue
		}
		for _, sln := range sub {
			sk, _, sok := splitKey(strings.TrimSpace(lines[sln]))
			if !sok || !isFilterKey(sk) {
				continue
			}
			findings = append(findings, Finding{name, key,
				fmt.Sprintf("`%s` restricts this trigger by base branch, so a PR stacked on another branch runs nothing and reports nothing; drop it, or opt out with a `# %s <reason>` comment", sk, MarkerPrefix),
				sln + 1})
		}
	}
	return findings
}

// findOnKey locates the top-level `on` key and returns its line index and any
// value written on the same line.
func findOnKey(lines []string) (idx int, inline string, ok bool) {
	for i, line := range lines {
		if isBlank(line) || indentOf(line) != 0 {
			continue
		}
		key, value, isKey := splitKey(strings.TrimSpace(line))
		// YAML 1.1 reads a bare `on` as the boolean true, which is why some
		// linters insist on quoting it; accept every spelling in use.
		switch unquote(key) {
		case "on", "true":
			return i, value, isKey
		}
	}
	return 0, "", false
}

// blockAfter returns the indices of the lines belonging to the block that
// starts after line start: everything indented past column zero, up to the next
// top-level key. Blank and comment lines inside the block are kept, because the
// marker lives in a comment.
func blockAfter(lines []string, start int) []int {
	var out []int
	for i := start + 1; i < len(lines); i++ {
		if isBlank(lines[i]) {
			out = append(out, i)
			continue
		}
		if indentOf(lines[i]) == 0 {
			break
		}
		out = append(out, i)
	}
	// Trailing blanks belong to whatever comes next, not to this block.
	for len(out) > 0 && isBlank(lines[out[len(out)-1]]) {
		out = out[:len(out)-1]
	}
	return out
}

// childLines returns the indices in rest that are indented deeper than
// parentIndent, stopping at the first line that is not.
func childLines(lines []string, rest []int, parentIndent int) []int {
	var out []int
	for _, ln := range rest {
		if isBlank(lines[ln]) {
			out = append(out, ln)
			continue
		}
		if indentOf(lines[ln]) <= parentIndent {
			break
		}
		out = append(out, ln)
	}
	return out
}

// hasMarker reports whether the trigger at line key is opted out: the marker
// with a reason, either in the comment block immediately above it or anywhere
// in its own body.
func hasMarker(lines []string, key int, sub []int) bool {
	for i := key - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(t, "#") {
			break
		}
		if markerWithReason(t) {
			return true
		}
	}
	for _, ln := range sub {
		if markerWithReason(strings.TrimSpace(lines[ln])) {
			return true
		}
	}
	return false
}

// markerWithReason requires text after the marker. A bare marker is not an
// exemption: the point of the opt-out is the written reason.
func markerWithReason(comment string) bool {
	_, rest, found := strings.Cut(comment, MarkerPrefix)
	if !found {
		return false
	}
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(rest), "—-:")) != ""
}

func isChecked(key string) bool { return contains(checkedTriggers, key) }

func isFilterKey(key string) bool { return contains(filterKeys, key) }

func contains(set []string, s string) bool {
	for _, v := range set {
		if v == s {
			return true
		}
	}
	return false
}

// isBlank reports whether a line carries no YAML structure: empty, whitespace,
// or a comment.
func isBlank(line string) bool {
	t := strings.TrimSpace(line)
	return t == "" || strings.HasPrefix(t, "#")
}

func indentOf(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }

// splitKey splits `key: value`, dropping any trailing comment from the value.
// A key side containing whitespace is not a key, which is how list items and
// prose get filtered out.
func splitKey(s string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(s, ":")
	if !ok || key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	value = strings.TrimSpace(value)
	// Strip a trailing comment so `pull_request: # note` reads as the block key
	// it is rather than as a key with an inline value. Quoted hashes are left
	// alone; nothing this checker reads needs more than that.
	if strings.HasPrefix(value, "#") {
		return key, "", true
	}
	if i := strings.Index(value, " #"); i >= 0 && !strings.Contains(value[:i], `"`) && !strings.Contains(value[:i], "'") {
		value = strings.TrimSpace(value[:i])
	}
	return key, value, true
}

func unquote(s string) string {
	for _, q := range []string{`"`, `'`} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			return s[1 : len(s)-1]
		}
	}
	return s
}
