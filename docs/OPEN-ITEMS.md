# Open items

State on 2026-11-23: **v0.1.0 is released** (2026-11-11); all 12 v0.1 acceptance tests pass ([TESTING.md](TESTING.md)). **M2 has started:** SPEC and DECISIONS (D40–D47) are updated. The catalog, code and tests are next.

## Check around Phase 4 / Checkpoint 4 (mid-December)
- [ ] **Prod namespace name.** The Week 14 page says `<team-name>-prod`, and ETAIS adds `ns-`, so `ns-<team>-prod` (`namespaces.prod` in `profiles/ut-devops.yaml`) should be right (D46). Confirm it with a real login. If it is wrong, fix the template in the next release.
- [ ] **Before Checkpoint 4:** every team member runs `kube-onramp check`.

## M2 / v0.2.0 (started 2026-11-23; scope in SPEC §13)
- [ ] **Catalog:** kubectl v1.37.2 (the server is v1.37.2 since 2026-11-23, D41) and the six extras (D42, D43). Check every URL and SHA-256 twice (D2; for oha, GitHub's digest replaces the checksum file) and record the values in D42.
- [ ] **Version rules:** the catalog field `accept`, used by inspect, `W-SHADOW` and `W-SKEW` (D44).
- [ ] **Extras never block:** preflight skips an extra with a warning, `check` inspects the extras, and the new-terminal list comes from the catalog (D45). The code path for extras (`installExtras`, `W-EXTRA-*`, `--minimal`) exists but has never run with real entries.
- [ ] **Prod:** the profile field `prodRequired`, the code `E-PROD-FORBIDDEN`, and the Phase 4 access checks (D46).
- [ ] **Texts:** summary lines for the extras, the next steps (`helm version`, `kubectl kustomize --help`), and the error report header (SPEC §8.8, §10).
- [ ] **Tests and docs:** the unit and smoke tests in SPEC §12, acceptance tests 13–20 (SPEC §13), and the README (tools, sizes, the new download host `get.helm.sh`).
- [ ] **Release:** `v0.2.0-rc.N` for the teammates (Machines A and B); the final `v0.2.0` tag needs the maintainer's OK.

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
