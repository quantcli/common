// Operational tooling for quantcli/common. Kept in its own module so that the
// contract-facing `compat` module stays exactly what it claims to be, and so a
// tool dependency can never leak into anything an export-cli imports.
//
// Hard rule: this module is standard-library-only. Anything that would need a
// third-party dependency belongs in CI as a pinned action, not here — see
// SECURITY.md "Supply-chain policy".
module github.com/quantcli/common/tools

// quantcli:not-shipped — this module is org-internal and is in no release, so
// the `go` directive below is free to move ahead of the product pin in
// compat/go.mod; ci.yml builds this module with `go-version: stable`.
//
// Do not remove this marker. security.yml greps for it to keep this module out
// of `go-version-file` selection, because the toolchain setup-go resolves there
// is the one govulncheck analyses compat/ with, and govulncheck reports stdlib
// advisories relative to that toolchain. Unmarked, raising the directive below
// above compat's would scan shipped code under a newer Go than we release and
// silently clear stdlib vulns that are still in the release — a green scan over
// a vulnerable library (QUA-99). This module is scanned either way; the marker
// only decides which module picks the toolchain.
go 1.21
