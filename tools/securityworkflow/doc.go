// Package securityworkflow exercises the shell logic embedded in
// .github/workflows/security.yml against synthetic repository trees.
//
// Why this exists: security.yml decides, in bash, which discovered go.mod
// drives setup-go for the two jobs that ANALYSE our code. That choice is a
// detection control — govulncheck reports stdlib advisories relative to the
// toolchain it analyses with, so a wrong pick silently clears vulnerabilities
// that are still present in what we release. The failure presents as a green
// scan, which is the one failure mode no amount of watching the Actions tab
// catches.
//
// Two properties of that logic cannot be checked by reading it:
//
//   - It must not be capturable by a module we do not ship. quantcli/common
//     holds both the product (compat/) and org-internal tooling (tools/), and
//     tools/ is explicitly licensed to move its `go` directive ahead of the
//     product pin — ci.yml builds it with `go-version: stable`. Before QUA-99
//     the rule was "highest `go` directive wins" over every discovered module,
//     so the correct module won only because 1.26.11 happens to exceed 1.21.
//     A routine dev-tooling bump would have inverted it.
//
//   - The three copies of the discovery step inside the file must agree. The
//     block is duplicated per job because GitHub Actions has no way to share a
//     step body, so the tests below run every copy through every case.
//
// The scripts are lifted out of the real workflow file rather than restated
// here, so a refactor that reintroduces the old ordering rule fails these
// tests instead of passing them. security.yml is copied verbatim into each
// *-export-cli, so covering the copy in `common` covers all four repos.
package securityworkflow
