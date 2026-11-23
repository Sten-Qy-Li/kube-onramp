# Open items

State on 2026-11-23: **v0.1.0 is released** (2026-11-11); all 12 v0.1 acceptance tests pass ([TESTING.md](TESTING.md)). **M2 has started:** SPEC and DECISIONS (D40–D50), the catalog, the code, the unit and smoke tests and the README are done. CI is green with the extras on Windows and macOS, and **`v0.2.0-rc.1` is out** (pre-release). Acceptance tests 14–18 and 20 pass, and **`v0.2.0-rc.2`** (the oha fix, D50) is out. Next: tests 13 and 19 with the teammates, then the final tag.

## Check around Phase 4 / Checkpoint 4 (mid-December)
- [x] **Prod namespace name.** Confirmed with a real login on 2026-11-23: `ns-<team>-prod` is right (D49).
  - The team creates `<team>-prod` itself in ETAIS, the same way as staging (Week 9). The maintainer's team had none until 2026-11-23. The Week 13 page doesn't mention it, and the Week 14 page ("Each team has a dedicated production namespace") doesn't say who creates it.
- [ ] **Before Checkpoint 4:** every team member runs `kube-onramp check`.

## M2 / v0.2.0 (started 2026-11-23; scope in SPEC §13)
- [x] **Catalog:** kubectl v1.37.2 (D41) and the six extras (D42, D43), all 28 files checked twice on 2026-11-23. `main` installs the extras: don't tag an rc before CI (with the extras smoke step) is green.
- [x] **Version rules:** the catalog field `accept`, used by `W-SHADOW` and `W-SKEW` (D44).
- [x] **Extras never block:** preflight skips an extra with a warning, `check` inspects the extras, and the new-terminal list comes from the catalog (D45, D48).
- [x] **Prod:** the profile field `prodRequired`, the code `E-PROD-FORBIDDEN`, and the Phase 4 access checks (D46, D48).
- [x] **Texts:** summary lines for the extras, the next steps, the error report header, and the README.
- [x] **Unit and smoke tests** (SPEC §12). The smoke step for the extras runs only in CI (D48). Watch its first CI run: it is the first time the extras actually run.
- [ ] **Acceptance tests 13–20** (SPEC §13). On 2026-11-23 with rc.1: 14, 15, 16, 17, 18 and 20 pass, and the upgrade from v0.1.0 to rc.2 passes in Windows Sandbox ([TESTING.md](TESTING.md)). Still open: 13 and 19 on Machines A and B (the teammates). Test 19 fails on a clean Windows because of oha (next item).
- [x] **oha on Windows without the Visual C++ runtime** (D49). Fixed in D50: the catalog field `needs`, the preflight and `check` skip it with clear advice, and new copies are tried before they go into `bin`. Verified in a clean Windows Sandbox ([TESTING.md](TESTING.md)).
- [x] **rc.2:** `v0.2.0-rc.2` was tagged on 2026-11-23 from `549d7af` (CI green). It contains the D50 fix. It is a pre-release; `releases/latest` is still v0.1.0. Teammates use rc.2 for tests 13 and 19.
- [x] **rc.1:** `v0.2.0-rc.1` was tagged on 2026-11-23 from `7c97e1d` (CI green, including the extras smoke step). It is a pre-release; `releases/latest` is still v0.1.0.
- [ ] **Release:** after acceptance tests 13–20 pass, the final `v0.2.0` tag (needs the maintainer's OK). Then record the results in TESTING.md.

## After v0.2.0
- [ ] **Project tools: Node 24, and a check of Git and the GitLab SSH login.** The maintainer's rule (2026-11-23): steps that a tool can do should be done by kube-onramp. This revisits D47. Node is the only real candidate. The rest needs a person's accounts (the SSH key in GitLab), permissions (GitLab role, Grafana), an admin installer (Docker Desktop, Git on macOS), or work inside the team's repository (`npm ci`). For v0.2 these steps stay manual, because the next teammate run is also the acceptance test, with no time for a fix. Open questions:
  - Node is a folder (node, npm, npx), not one file. It needs a new install type, shims or a second PATH entry, and changes to `reset`.
  - On Windows, a Node from the official installer is on the system PATH and would still win (the same limit as Docker's kubectl, D20).
  - Check Node's minimum macOS version upstream against Machine B (macOS 13.7.8).
- [ ] **A PowerShell profile line for `W-SHADOW` on Windows** (the same rule; candidate, not decided). When a copy on the system PATH wins (D20), kube-onramp could offer to add one line to the PowerShell profile that puts its bin folder first. It needs no admin rights, but it only works in PowerShell (not cmd or Git Bash), and `reset` must remove it. Facts found on 2026-11-23 on Machine C:
  - Its Docker Desktop (4.69.0, kubectl 1.34.1) came from the Microsoft Store. The Store page says Docker manages the updates, but Docker Desktop's update page points back to the Store, so it can't be updated in place.
  - Docker's release notes: the newest version is 4.100.1. Since 4.96.3 it includes kubectl v1.36.1, which is acceptable for a 1.37 cluster.
  - Docker's install page: the installer's default is now a per-user install (no admin rights), which puts Docker on the user PATH, after kube-onramp's entry. The `W-SHADOW` advice could mention this.

## Nice to have
- [ ] `go run ./tools/update-catalog`: the maintainer command that finds new upstream versions and checksums (SPEC 6.2). Moved out of M2 (D47).
- [ ] SBOM in the release (GoReleaser + syft), optional per SPEC 5.
- [ ] Smoke test on Apple silicon (`macos-latest`); today only unit tests run there.
- [ ] Automated dependency updates (Dependabot or similar) with the 7-day cooldown rule (D17); check the options upstream first.
- [ ] Delete the old `v0.1.0-rc.*` pre-releases on GitHub (ask the maintainer first).

## Maintenance notes
- CI fails when a new vulnerability is published for a dependency (govulncheck, Trivy). The fix is usually a version bump (D36).
- A `W-SHADOW` caused by an all-users Docker Desktop install can't be fixed without admin rights (D20). Updating Docker Desktop is the advice.

## How to pick this up again
Open Claude Code in this folder and say, for example: "Read docs/OPEN-ITEMS.md. Let's continue M2 with the catalog."
`CLAUDE.md` gives the rules. SPEC.md says what to build, DECISIONS.md says why, and TESTING.md lists what was tested.
