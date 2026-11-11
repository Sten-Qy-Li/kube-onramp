# Open items

State on 2026-11-11: **v0.1.0 is released**; all 12 acceptance tests pass ([TESTING.md](TESTING.md)). M2 has not started.

## Check around Phase 4 / Checkpoint 4 (mid-December)
- [ ] **Prod namespace name.** Phase 4 (from Nov 23) creates the prod namespace. Confirm it really is `ns-<team>-prod` (`namespaces.prod` in `profiles/ut-devops.yaml`). If not, fix the template and release v0.1.x. Until it exists, `W-PROD-NOT-YET` is expected.
- [ ] **Cluster upgrade.** The kubectl pin (v1.36.6) follows the server's minor version. If `kube-onramp check` shows `W-SKEW`, or the server version changes, bump kubectl in `catalog/tools.yaml`: check the URL and SHA-256 upstream first (D2), then release.
- [ ] **Before Checkpoint 4:** every team member runs `kube-onramp check`.

## M2 / v0.2.0 (not started)
- [ ] Extra tools: Helm 4 (latest 4.x, also check "must be 4.x" per SPEC 8.2), k9s, Trivy, kubeconform, a load generator (check that `oha` suits; otherwise propose another). Each needs a catalog entry with verified pins. For Trivy, pin a version released after the March 2026 incident, with its checksum (D17).
  - The code path for extras (`installExtras`, `W-EXTRA-*`, `--minimal`) exists but has never run with real entries.
- [ ] `go run ./tools/update-catalog`: the maintainer command that finds new upstream versions and checksums (SPEC 6.2).

## Nice to have
- [ ] SBOM in the release (GoReleaser + syft), optional per SPEC 5.
- [ ] Smoke test on Apple silicon (`macos-latest`); today only unit tests run there.
- [ ] Automated dependency updates (Dependabot or similar) with the 7-day cooldown rule (D17); check the options upstream first.
- [ ] Delete the old `v0.1.0-rc.*` pre-releases on GitHub (ask the maintainer first).

## Maintenance notes
- CI fails when a new vulnerability is published for a dependency (govulncheck, Trivy). The fix is usually a version bump (D36).
- A `W-SHADOW` caused by an all-users Docker Desktop install can't be fixed without admin rights (D20). Updating Docker Desktop is the advice.

## How to pick this up again
Open Claude Code in this folder and say, for example: "Read docs/OPEN-ITEMS.md. Phase 4 started, let's check the prod namespace" or "Let's start M2."
`CLAUDE.md` gives the rules. SPEC.md says what to build, DECISIONS.md says why, and TESTING.md lists what was tested.
