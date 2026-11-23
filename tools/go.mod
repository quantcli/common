// Operational tooling for quantcli/common. Kept in its own module so that the
// contract-facing `compat` module stays exactly what it claims to be, and so a
// tool dependency can never leak into anything an export-cli imports.
//
// Hard rule: this module is standard-library-only. Anything that would need a
// third-party dependency belongs in CI as a pinned action, not here — see
// SECURITY.md "Supply-chain policy".
module github.com/quantcli/common/tools

go 1.21
