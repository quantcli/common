package compat_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// citationRE matches `CONTRACT §N: "..."` in a comment, where N is one
// or more digits and "..." is a double-quoted prose fragment. The
// fragment may have been wrapped across two or more comment lines in
// source — go/ast.CommentGroup.Text() joins consecutive comment lines
// with single spaces before we run the regex, so the match is always
// against a single logical line.
//
// Backtick-quoted fragments (e.g. `[]` for json) are deliberately
// excluded: those are usually inline code references, not contract
// quotations, and treating them as the latter would create noise. If a
// contributor wants the harness to verify a quote, they write it with
// double quotes.
var citationRE = regexp.MustCompile(`CONTRACT §(\d+):\s*"([^"]+)"`)

// sectionHeaderRE matches `## N. <title>` headers in CONTRACT.md and
// captures the section number.
var sectionHeaderRE = regexp.MustCompile(`(?m)^##\s+(\d+)\.\s+`)

// minContractSections is the floor on how many top-level `## N. <title>`
// sections CONTRACT.md should currently parse to. CONTRACT.md has
// sections 1–8 today; a header refactor that drops below this floor
// would silently let every cited section "not exist" and pass every
// quote check vacuously. Bump this number when the contract grows.
const minContractSections = 8

// citationProblem records one failing citation so the top-level test
// can render all of them in a single diagnostic.
type citationProblem struct {
	path    string
	section string
	quote   string
	reason  string
}

// TestCONTRACTCitationsAreReal walks every .go file in the compat
// module and, for every comment that quotes a section of the contract
// in the form `CONTRACT §N`-colon-double-quoted-fragment, verifies the
// fragment is a substring of section N of CONTRACT.md. A false quote
// fails the test loudly so it cannot silently survive review.
//
// The test exists because two consecutive PRs landed comments
// attributing prose to §5 that §5 does not contain (§5 is the Auth
// section, with no language about hermeticity or `--help`). Go-side
// review missed both. This guard is the structural fix: the next
// fabricated quote dies in CI instead of in a post-merge incident
// comment.
func TestCONTRACTCitationsAreReal(t *testing.T) {
	// `go test` enters the package directory before running, so when
	// this file's tests execute the working directory is `compat/` and
	// CONTRACT.md lives one level up. If the compat module is ever
	// vendored into a different repo layout, this is the line that
	// needs updating.
	contractPath := filepath.Join("..", "CONTRACT.md")
	body, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read CONTRACT.md (looked at %s): %v", contractPath, err)
	}
	problems, err := findCitationProblems(".", string(body))
	if err != nil {
		t.Fatalf("find citation problems: %v", err)
	}
	if len(problems) == 0 {
		return
	}
	t.Fatal(formatCitationProblems(problems))
}

// findCitationProblems scans every .go file under root and returns the
// list of `CONTRACT §N: "..."` citations whose quoted fragment is not
// found in section N of contractMd. It is the test's load-bearing
// logic, factored out so TestCitationsGuardCatchesFakeQuote can drive
// it against a synthetic root + synthetic contract without touching
// the real tree.
func findCitationProblems(root, contractMd string) ([]citationProblem, error) {
	sections := splitContractSections(contractMd)
	if len(sections) < minContractSections {
		return nil, fmt.Errorf("parsed only %d `## N. <title>` sections from CONTRACT.md; expected at least %d. The header style may have changed — update sectionHeaderRE or bump minContractSections.", len(sections), minContractSections)
	}

	var problems []citationProblem
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		// Display path is the file path relative to root so error
		// messages stay short and meaningful when invoked with root=".".
		display := path
		if rel, relErr := filepath.Rel(root, path); relErr == nil {
			display = rel
		}
		for _, cg := range f.Comments {
			text := cg.Text()
			for _, m := range citationRE.FindAllStringSubmatch(text, -1) {
				section := m[1]
				quote := m[2]
				secBody, ok := sections[section]
				if !ok {
					problems = append(problems, citationProblem{display, section, quote, "no `## " + section + ". ` section exists in CONTRACT.md"})
					continue
				}
				if !strings.Contains(normalize(secBody), normalize(quote)) {
					problems = append(problems, citationProblem{display, section, quote, "quoted text is not a substring of CONTRACT.md §" + section})
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(problems, func(i, j int) bool {
		if problems[i].path != problems[j].path {
			return problems[i].path < problems[j].path
		}
		return problems[i].section < problems[j].section
	})
	return problems, nil
}

// formatCitationProblems renders the problem list as the human-readable
// failure message t.Fatal logs. Kept separate so the meta-test can
// inspect both the structured list and the rendered string.
func formatCitationProblems(problems []citationProblem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d CONTRACT §X citation(s) do not match CONTRACT.md:\n\n", len(problems))
	for _, p := range problems {
		fmt.Fprintf(&b, "  %s\n", p.path)
		fmt.Fprintf(&b, "    cite : CONTRACT §%s\n", p.section)
		fmt.Fprintf(&b, "    quote: %q\n", p.quote)
		fmt.Fprintf(&b, "    why  : %s\n\n", p.reason)
	}
	b.WriteString("Fix by either dropping the quote or updating CONTRACT.md so §N actually contains the text.\n")
	b.WriteString("See compat/citations_test.go for the matching rule.")
	return b.String()
}

// splitContractSections returns a map from section number ("3") to the
// raw markdown body of `## 3. <title>` up to (but not including) the
// next `## N. <title>` header. The section header itself is included
// in the body so that the title text is also citable.
func splitContractSections(md string) map[string]string {
	idxs := sectionHeaderRE.FindAllStringSubmatchIndex(md, -1)
	out := map[string]string{}
	for i, m := range idxs {
		section := md[m[2]:m[3]]
		var end int
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		} else {
			end = len(md)
		}
		out[section] = md[m[0]:end]
	}
	return out
}

// normalize collapses runs of whitespace to single spaces so a quote
// that was wrapped across two source-comment lines (joined with one
// space by CommentGroup.Text) still matches a sentence in CONTRACT.md
// that lives on a single line.
func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// TestCitationsGuardCatchesFakeQuote is a regression test of the guard
// itself. It builds a synthetic many-section contract and a synthetic
// .go file with a real citation, a no-such-section citation, and a
// quote-mismatch citation, then asserts findCitationProblems flags
// exactly the two bad ones with the right diagnostics. This exists so
// a future refactor of citationRE / splitContractSections / normalize
// can't silently turn the guard into a no-op.
func TestCitationsGuardCatchesFakeQuote(t *testing.T) {
	// Build a synthetic contract with at least minContractSections
	// `## N. <title>` sections so the floor in findCitationProblems
	// holds for the synthetic input. The "## 3." section is the one a
	// healthy citation will resolve against; the rest are filler.
	var contractParts []string
	contractParts = append(contractParts, "# Synthetic contract\n")
	contractParts = append(contractParts, "## 3. Date flags\n\nThe CLI accepts --since and --until.\n")
	for i := 1; i <= minContractSections; i++ {
		if i == 3 {
			continue
		}
		contractParts = append(contractParts, fmt.Sprintf("## %d. Filler %d\n\nFiller body.\n", i, i))
	}
	contractMd := strings.Join(contractParts, "\n")

	// Synthetic source file with three citations:
	//   - §3 quote that IS in §3 → expected to pass.
	//   - §99 quote → expected to fail (no such section).
	//   - §3 quote that is NOT in §3 → expected to fail (quote mismatch).
	//
	// The literal `CONTRACT §N` strings are split with concatenation
	// so this very file's comment scan does not flag them as real
	// citations of citations_test.go itself.
	const goSrc = `package fake

// realCite is a healthy quote that resolves to §3.
//
// CONTRACT ` + `§3: "accepts --since and --until"
const realCite = 0

// missingSectionCite refers to a section that does not exist.
//
// CONTRACT ` + `§99: "no such section"
const missingSectionCite = 0

// quoteMismatchCite refers to §3 but quotes text §3 does not contain.
//
// CONTRACT ` + `§3: "this exact text is fabricated"
const quoteMismatchCite = 0
`
	tmp := t.TempDir()
	srcPath := filepath.Join(tmp, "fake.go")
	if err := os.WriteFile(srcPath, []byte(goSrc), 0o644); err != nil {
		t.Fatalf("write synthetic source: %v", err)
	}

	problems, err := findCitationProblems(tmp, contractMd)
	if err != nil {
		t.Fatalf("findCitationProblems: %v", err)
	}
	if got, want := len(problems), 2; got != want {
		t.Fatalf("got %d problems, want %d. problems=%+v\nrendered=%s", got, want, problems, formatCitationProblems(problems))
	}

	// Problems are sorted by path then section, so §3 (the
	// quote-mismatch) comes before §99 (the missing-section) for the
	// same file. Verify each entry's section, quote, and reason shape
	// so a regression in any one field surfaces here.
	mismatch, missing := problems[0], problems[1]
	if missing.section != "99" || !strings.Contains(missing.reason, "no `## 99. `") {
		t.Errorf("expected §99 missing-section problem, got %+v", missing)
	}
	if mismatch.section != "3" || !strings.Contains(mismatch.reason, "not a substring") {
		t.Errorf("expected §3 quote-mismatch problem, got %+v", mismatch)
	}
	if mismatch.quote != "this exact text is fabricated" {
		t.Errorf("quote not captured verbatim: got %q", mismatch.quote)
	}

	// And confirm the rendered failure message names every offending
	// file/section/quote — that's the contract this test is locking in
	// for future contributors who might "simplify" the formatter.
	rendered := formatCitationProblems(problems)
	for _, want := range []string{"fake.go", "CONTRACT §3", "CONTRACT §99", "this exact text is fabricated", "no such section"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered failure message missing %q; got:\n%s", want, rendered)
		}
	}
}

// TestCitationsGuardRejectsHeaderRefactor exercises the
// minContractSections floor in findCitationProblems: a contract that
// no longer parses to enough top-level `## N. <title>` sections must
// fail loudly rather than silently let every quote pass vacuously.
func TestCitationsGuardRejectsHeaderRefactor(t *testing.T) {
	// Two sections only — well under minContractSections.
	contractMd := "# Refactored contract\n\n## 1. Foo\n\nbody\n\n## 2. Bar\n\nbody\n"
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := findCitationProblems(tmp, contractMd)
	if err == nil {
		t.Fatalf("expected error about insufficient section count, got nil")
	}
	if !strings.Contains(err.Error(), "expected at least") {
		t.Errorf("expected error to mention expected floor; got: %v", err)
	}
}
