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
	contractPath := filepath.Join("..", "CONTRACT.md")
	body, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read CONTRACT.md (looked at %s): %v", contractPath, err)
	}
	sections := splitContractSections(string(body))
	if len(sections) == 0 {
		t.Fatalf("no `## N. <title>` headers found in %s; the regex assumes the contract uses that header style", contractPath)
	}

	type problem struct {
		path    string
		section string
		quote   string
		reason  string
	}
	var problems []problem

	walkErr := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
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
		for _, cg := range f.Comments {
			text := cg.Text()
			for _, m := range citationRE.FindAllStringSubmatch(text, -1) {
				section := m[1]
				quote := m[2]
				secBody, ok := sections[section]
				if !ok {
					problems = append(problems, problem{path, section, quote, "no `## " + section + ". ` section exists in CONTRACT.md"})
					continue
				}
				if !strings.Contains(normalize(secBody), normalize(quote)) {
					problems = append(problems, problem{path, section, quote, "quoted text is not a substring of CONTRACT.md §" + section})
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk compat tree: %v", walkErr)
	}

	if len(problems) == 0 {
		return
	}

	sort.Slice(problems, func(i, j int) bool {
		if problems[i].path != problems[j].path {
			return problems[i].path < problems[j].path
		}
		return problems[i].section < problems[j].section
	})

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
	t.Fatal(b.String())
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
