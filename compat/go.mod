module github.com/quantcli/common/compat

// The `go` directive is this module's supported-toolchain floor, and govulncheck
// resolves stdlib advisories against it. Keeping it at the oldest Go release line
// that still receives upstream security fixes (1.26, as of 2026-11-13; latest is
// 1.27.4) means a stdlib finding here is a real, fixable signal rather than a
// standing report that our floor is end-of-life. The previous `go 1.21` floor was
// vestigial — nothing builds compat with it; all three export-clis pin 1.26.10 —
// and it reported two reachable stdlib vulnerabilities that no 1.21 patch will fix
// (GO-2025-3956, GO-2025-3750; see QUA-83).
//
// The patch is pinned rather than left as a bare `go 1.26`, matching the
// export-clis. security.yml feeds this directive to setup-go, and a bare minor
// version is a range: run 43490674235 resolved it to 1.26.10 in one job and
// 1.26.11 in another, because setup-go prefers a patch already cached on the
// runner image. govulncheck reports stdlib advisories relative to the toolchain
// it analyses with, so a range would make a stdlib finding appear or vanish
// depending on which patch the runner happened to have — 1.26.11 is itself the
// fix for GO-2026-6859. Pin the patch, and bump it deliberately.
go 1.26.11
