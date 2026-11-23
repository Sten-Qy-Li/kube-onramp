# Open items

State on 2026-11-23: **v0.1.0 is released** (2026-11-11); all 12 v0.1 acceptance tests pass ([TESTING.md](TESTING.md)). **M2 has started:** SPEC and DECISIONS (D40–D48), the catalog, the code, the unit and smoke tests and the README are done. CI is green with the extras on Windows and macOS, and **`v0.2.0-rc.1` is out** (pre-release). Next: acceptance tests 13–20 with the teammates.

## Check around Phase 4 / Checkpoint 4 (mid-December)
- [ ] **Prod namespace name.** The Week 14 page says `<team-name>-prod`, and ETAIS adds `ns-`, so `ns-<team>-prod` (`namespaces.prod` in `profiles/ut-devops.yaml`) should be right (D46). Confirm it with a real login. If it is wrong, fix the template in the next release.
  - On 2026-11-23 the maintainer's team had no prod namespace yet: `auth can-i get pods` said `no`, and ETAIS listed only the staging namespace. The Week 13 page doesn't mention it, and the Week 14 page ("Each team has a dedicated production namespace") doesn't say who creates it. So the team creates `<team>-prod` in ETAIS the same way as staging (Week 9), then confirms the name with a real login.
- [ ] **Before Checkpoint 4:** every team member runs `kube-onramp check`.

## M2 / v0.2.0 (started 2026-11-23; scope in SPEC §13)
- [x] **Catalog:** kubectl v1.37.2 (D41) and the six extras (D42, D43), all 28 files checked twice on 2026-11-23. `main` installs the extras: don't tag an rc before CI (with the extras smoke step) is green.
- [x] **Version rules:** the catalog field `accept`, used by `W-SHADOW` and `W-SKEW` (D44).
- [x] **Extras never block:** preflight skips an extra with a warning, `check` inspects the extras, and the new-terminal list comes from the catalog (D45, D48).
- [x] **Prod:** the profile field `prodRequired`, the code `E-PROD-FORBIDDEN`, and the Phase 4 access checks (D46, D48).
- [x] **Texts:** summary lines for the extras, the next steps, the error report header, and the README.
- [x] **Unit and smoke tests** (SPEC §12). The smoke step for the extras runs only in CI (D48). Watch its first CI run: it is the first time the extras actually run.
- [ ] **Acceptance tests 13–20** (SPEC §13), after an rc exists. Most need Machines A and B or a real login; tests 16 and 17 need a prepared machine or a team without prod access. Test 17 still works after the prod namespace exists: pass `--prod-namespace` with a namespace that doesn't exist.
- [x] **rc.1:** `v0.2.0-rc.1` was tagged on 2026-11-23 from `7c97e1d` (CI green, including the extras smoke step). It is a pre-release; `releases/latest` is still v0.1.0.
- [ ] **Release:** after acceptance tests 13–20 pass, the final `v0.2.0` tag (needs the maintainer's OK). Then record the results in TESTING.md.

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
