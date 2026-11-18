package securityworkflow

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Script is one `run:` block lifted out of a workflow file, with enough
// context to name it in a failure message.
type Script struct {
	Job  string // enclosing job key, e.g. "govulncheck"
	Step string // the step's `name:`, e.g. "Discover Go modules"
	Body string // the script text, dedented to column zero
}

func (s Script) String() string { return fmt.Sprintf("job %q / step %q", s.Job, s.Step) }

// RunScripts returns every `run: |` block in the workflow at path that belongs
// to a step named stepName, in file order.
//
// This is a hand-rolled scanner rather than a YAML parse because the tools
// module is standard-library-only (see tools/go.mod) and the shape it has to
// handle is fixed: a literal block scalar under a `run:` key inside a step
// list. It understands only what security.yml uses — `- ` step items, `key:
// value` scalars, and `run: |` blocks — and ignores everything else, so a step
// that nests a `name:` or `run:` key under `with:` would confuse it. No
// workflow in this org does that.
func RunScripts(path, stepName string) ([]Script, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow: %w", err)
	}

	const (
		stNormal     = iota
		stAwaitBlock // saw `run: |`; the first non-empty line fixes the indent
		stInBlock
	)

	var (
		found       []Script
		owner       []int // found[i] came from step index owner[i]
		names       = map[int]string{}
		stepIdx     = -1
		state       = stNormal
		body        []string
		blockIndent int
		job         string
		inJobs      bool
	)

	emit := func() {
		found = append(found, Script{Job: job, Body: strings.Join(body, "\n")})
		owner = append(owner, stepIdx)
		body = nil
		state = stNormal
	}

	for _, line := range strings.Split(string(raw), "\n") {
		if state == stAwaitBlock || state == stInBlock {
			if strings.TrimSpace(line) == "" {
				body = append(body, "")
				continue
			}
			if state == stAwaitBlock {
				blockIndent = indentOf(line)
				state = stInBlock
			}
			if indentOf(line) >= blockIndent {
				body = append(body, line[blockIndent:])
				continue
			}
			emit() // dedent ends the block; fall through and reread this line
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		switch indent := indentOf(line); {
		case indent == 0:
			// Only `jobs:` introduces job keys; `on:` also has indent-2 keys.
			inJobs = trimmed == "jobs:"
			continue
		case inJobs && indent == 2 && isBareKey(trimmed):
			job = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, "- "); ok {
			stepIdx++
			trimmed = strings.TrimSpace(rest)
		}
		key, value, ok := splitScalar(trimmed)
		if !ok {
			continue
		}
		switch key {
		case "name":
			names[stepIdx] = unquote(value)
		case "run":
			if strings.HasPrefix(value, "|") {
				state = stAwaitBlock
			}
		}
	}
	if state == stAwaitBlock || state == stInBlock {
		emit()
	}

	var out []Script
	for i, s := range found {
		if names[owner[i]] != stepName {
			continue
		}
		s.Step = stepName
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no `run:` block found for a step named %q", path, stepName)
	}
	return out, nil
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// isBareKey reports whether s is a mapping key with no inline value, e.g.
// "govulncheck:" but not "permissions: {}".
func isBareKey(s string) bool {
	k, v, ok := splitScalar(s)
	return ok && v == "" && k != ""
}

// splitScalar splits "key: value" on the first colon. It rejects anything
// whose key side contains whitespace, which is how list items and prose lines
// get filtered out.
func splitScalar(s string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(s, ":")
	if !ok || key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	return key, strings.TrimSpace(value), true
}

func unquote(s string) string {
	for _, q := range []string{`"`, `'`} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// Fixture is a synthetic repository tree: repo-relative path to file contents.
type Fixture map[string]string

// Result is what one script run produced.
type Result struct {
	// Outputs is the parsed $GITHUB_OUTPUT, including heredoc-delimited values.
	Outputs map[string]string
	// Log is stdout and stderr combined, which is where ::error:: and
	// ::warning:: annotations land.
	Log string
	// ExitCode is 0 on success. A non-zero code is a red job, which for the
	// discovery step is the intended outcome when there is nothing to scan.
	ExitCode int
}

// Run materialises fixture under workdir and executes script against it the
// way the Actions runner does: `shell: bash` runs `bash -e <file>` with the
// repository as the working directory, and $GITHUB_OUTPUT points at a file
// outside the tree (so it is never itself discovered as repository content).
//
// workdir must be an empty directory owned by the caller, e.g. t.TempDir().
func Run(workdir, script string, fixture Fixture) (Result, error) {
	repo := filepath.Join(workdir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return Result{}, err
	}
	for name, content := range fixture {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return Result{}, err
		}
	}

	scriptPath := filepath.Join(workdir, "step.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		return Result{}, err
	}
	outPath := filepath.Join(workdir, "github_output")
	if err := os.WriteFile(outPath, nil, 0o600); err != nil {
		return Result{}, err
	}

	cmd := exec.Command("bash", "-e", scriptPath)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GITHUB_OUTPUT="+outPath)
	combined, err := cmd.CombinedOutput()

	res := Result{Log: string(combined)}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return res, fmt.Errorf("run script: %w", err)
		}
		res.ExitCode = exit.ExitCode()
	}

	res.Outputs, err = parseOutputs(outPath)
	if err != nil {
		return res, err
	}
	return res, nil
}

// parseOutputs reads the two $GITHUB_OUTPUT forms the workflow writes:
// `key=value` and a heredoc block `key<<DELIM` … `DELIM`.
func parseOutputs(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read GITHUB_OUTPUT: %w", err)
	}
	out := map[string]string{}
	lines := strings.Split(string(raw), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}
		if key, delim, ok := strings.Cut(line, "<<"); ok && !strings.Contains(key, "=") {
			var vals []string
			for i++; i < len(lines) && lines[i] != delim; i++ {
				vals = append(vals, lines[i])
			}
			out[key] = strings.Join(vals, "\n")
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			out[key] = value
		}
	}
	return out, nil
}
