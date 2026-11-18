// Operational tooling for quantcli/common. Kept in its own module so that the
// contract-facing `compat` module stays exactly what it claims to be, and so a
// tool dependency can never leak into anything an export-cli imports.
//
// Hard rule: this module is standard-library-only. Anything that would need a
// third-party dependency belongs in CI as a pinned action, not here — see
// SECURITY.md "Supply-chain policy".
module github.com/quantcli/common/tools

// quantcli:not-shipped — do not remove this marker; security.yml greps for it.
// It excludes this module from scanner toolchain selection only (QUA-99).
// Without it, raising this module's `go` directive would make the scanner
// analyse shipped code under a newer Go than we release.
go 1.21
