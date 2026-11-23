# Acceptance test results

The acceptance tests from [SPEC.md](SPEC.md), section 13: v0.1.0 (all ran on 2026-11-11) and v0.2 (below). The machines are described in SPEC section 2; team names are left out on purpose.

"Isolated" means a temporary `KUBE_ONRAMP_HOME`, a temporary `KUBECONFIG` and a throwaway registry key instead of the real user PATH (see `CLAUDE.md`).

| # | Test | Where | Version | Result |
|---|---|---|---|---|
| 1 | Clean Windows: one-liner, login, `kubectl get pods` in a new window | Windows Sandbox (Windows 11 24H2) | rc.1, rc.2 | Pass |
| 2 | Machine A: kube-onramp's copies win, old entries kept, backup, login | Windows 10 Pro 22H2; per-user Docker Desktop kubectl 1.36.1; winget kubelogin; course kubeconfig | rc.3 | Pass |
| 3 | Machine B: copies win in a new zsh, kubeconfig mode 600, `config.broken` untouched | macOS 13.7.8, Intel; Homebrew kubectl 1.35.10 and kubelogin 1.36.4; kubeconfig mode 644 | rc.3 | Pass |
| 4 | Run twice: "Everything is already set up", no file changes | Machine C (isolated) and CI smoke tests | rc.1+ | Pass |
| 5 | Interrupted run: killed during a download, re-run completes | Machine C (isolated) | rc.1 | Pass, after fix D37 |
| 6 | Double launch: second instance exits with code 3 | Machine C (isolated) | rc.1 | Pass |
| 7 | Broken kubeconfig (UTF-16): backed up, replaced, user told where | Machine C (isolated) | rc.1 | Pass |
| 8 | Wrong team name: `E-ACCESS-FORBIDDEN` with the namespace | Machine C (isolated) | rc.1 | Pass |
| 9 | Login timeout: closed browser gives `E-LOGIN-TIMEOUT`, report on the clipboard and redacted | Windows Sandbox | rc.3 | Pass, after fix D39 |
| 10 | github.com blocked: fails in preflight, nothing changed | Machine C (isolated; dead proxy for github.com only) | rc.1 | Pass |
| 11 | Reset removes only kube-onramp's changes; setup works again | Machine C (isolated) and CI smoke tests | rc.1+ | Pass |
| 12 | Port fallback: 8000 busy uses 18000; both busy gives `E-PRE-PORTS` | Machine C (isolated) | rc.1 | Pass |

Also tested: a real (not isolated) setup and `check` on Machine C, which has Docker Desktop's kubectl 1.34.1 on the system PATH. Result as expected: `W-SHADOW` for kubectl, everything else `[OK]`, the course's `etais-user@kubernetes` entries kept.

## Bugs found by these tests
- D23: kubelogin v1.36.4 release builds print no version.
- D36: vulnerable `x/net` and `x/text` versions pulled in by client-go (found by CI).
- D37: an interrupted first run had not saved the team name yet.
- D38: right after the one-liner, `kube-onramp` was not found in the same PowerShell window.
- D39: a closed browser was reported as `E-CLUSTER-UNREACHABLE`.

## v0.2 acceptance tests (v0.2.0-rc.1)

The tests from SPEC section 13 for v0.2. Tests 14 and 17 ran on 2026-11-23 on Machine C (isolated, with the maintainer's real login). The extras' own folders went into the temporary folder too (`K9S_LOGS_DIR`, `TRIVY_CACHE_DIR`, `HELM_*_HOME`). Tests 15, 16, 18 and 20 ran the same day in Windows Sandbox (Windows 11 24H2, clean). There, the rc.1 exe was downloaded from the release and checked against `checksums.txt`, then run with `--yes --skip-login --team ci-test`.

| # | Test | Where | Result |
|---|---|---|---|
| 13 | Extras on Machines A and B, `helm version` shows 4.x | Machines A and B | Waiting for the teammates |
| 14 | `helm list --kube-context ut-devops-staging` with kube-onramp's login | Machine C (isolated, real login) | Pass (prod works too) |
| 15 | `get.helm.sh` blocked (hosts file): run finishes, `W-EXTRA-HELM`, core results as in a normal first run, exit code 0. A later run installs Helm | Windows Sandbox | Pass |
| 16 | Helm v3.22.1 on the system PATH: `W-SHADOW` for helm with "That copy is too old or not the right program" | Windows Sandbox | Pass |
| 17 | No prod access (`--prod-namespace` with a namespace that doesn't exist): everything installed, then `E-PROD-FORBIDDEN` with a report, exit code 1. `check` exits with 1 and doesn't say "Everything works". The report has no email or home path | Machine C (isolated, real login) | Pass |
| 18 | `kubeconform.exe` deleted, then `check`: `[WARN]`, "Run kube-onramp to fix this.", exit code 0 | Windows Sandbox | Pass |
| 19 | Run twice with extras: "Everything is already set up" | CI smoke tests: pass. Windows Sandbox: **fails** with rc.1 because of oha (below), passes with the fix (D50). Machines A and B: waiting for the teammates | Pass in the Sandbox with the fix |
| 20 | `reset --yes`: bin folder with all extras gone, user PATH back, `state.json` gone. Folders the tools made stay (`%LOCALAPPDATA%\k9s`, `~\.kube`) | Windows Sandbox | Pass |

Also seen with the real login:
- The prod namespace `ns-<team>-prod` gives `[OK]`, so the profile template is right (D49).
- Every Phase 4 rights check (secrets, HPA, KEDA, PDB) says "You can" in both namespaces. `check` exits with 0.
- Each extra printed its own version in the log (kubelogin prints none, D23). Of the extras, only k9s made a folder (its empty log folder, D42).

## Bugs found by the v0.2 tests
- **oha does not start on a clean Windows** (D49). It needs the Visual C++ runtime (`vcruntime140.dll`), which Windows doesn't include. In the Sandbox, every run shows `W-EXTRA-OHA`, and the second run never says "Everything is already set up". Machine C and GitHub's Windows runner have the runtime, so CI didn't see it.
- **A new copy that fails its test run stays in the bin folder** (`installTool` places it first, then runs it). So the new-terminal check says `[OK]` for oha, and each later run tries to repair it again.
- **`W-EXTRA-OHA` says "Run kube-onramp again later"**, but trying again doesn't help in this case.

All three are fixed in D50. Checked on 2026-11-23 in a clean Windows Sandbox (no `vcruntime140.dll`) with a local build of `549d7af`:
- The first setup skips oha before the download. `W-EXTRA-OHA` names the Microsoft Visual C++ Redistributable, gives the link, and says that admin rights are needed. It doesn't say "try again later". The other five extras install, and `[OK] Network` is shown. Exit code 0.
- A second setup says "Everything is already set up" and changes nothing (bin folder, `state.json`, kubeconfig, user PATH).
- `check` explains oha, doesn't say "Run kube-onramp to fix this.", says "Everything works" and exits with 0.
- Then the Sandbox installed the runtime from the link (`vc_redist.x64.exe` 14.52.37029.0, valid Microsoft signature). The next setup shows `[FIXED] oha 1.17.0` with no warning, and the run after that says "Everything is already set up". `reset` removes the bin folder.
- A unit test checks that a new copy that doesn't start never reaches the bin folder, neither as a new file nor over an older copy. It fails on the old code.

## Upgrade from v0.1.0 to v0.2.0-rc.2 (Windows Sandbox, 2026-11-23)
The teammates and Machine C have v0.1 installed, and all other tests started with nothing installed. This test copies what `install.ps1` does: it puts the downloaded exe into `bin\kube-onramp.exe` and runs it from there. Both exes were checked against their release's `checksums.txt`. The Sandbox started with the course kubeconfig (`etais-user@kubernetes`, as on Machine A), without the Visual C++ runtime, and every run used `--team ci-test --yes --skip-login`.
- v0.1.0 installs kubectl 1.36.6. Then rc.2 plans "Update kubectl to 1.37.2" and the five extras, and skips oha with `W-EXTRA-OHA`. kubelogin stays `[OK]`, exit code 0, no `E-` code.
- After the upgrade: `bin\kubectl.exe` is v1.37.2, and `bin\kube-onramp.exe` is the rc.2 file (`version` prints rc.2). The state file keeps the team and adds only the new tools. The bin folder is first on the user PATH, and only once. The course entries in the kubeconfig are kept, and the current context is `ut-devops-staging`.
- The install line again: "Everything is already set up", and nothing changed. `check`: exit code 0, "Everything works".
- With the runtime installed: oha `[FIXED]`, no warnings. The next run says "Everything is already set up", with no warnings.
- `reset` after the upgrade: the bin folder is gone, the user PATH is as before v0.1.0, the current context is back to `etais-user@kubernetes` (recorded by v0.1.0), and only kube-onramp's entries are removed.
- Not covered: the macOS upgrade. Its parts that differ from Windows (the rc block, file modes) are unchanged since v0.1.

## Automated tests (every push)
- Unit tests on Windows, Intel macOS, Apple silicon macOS and Linux.
- End-to-end smoke tests on `windows-latest` and `macos-15-intel`: setup, setup again (no changes), check, reset, everything back byte for byte.
- golangci-lint, shellcheck, govulncheck, a Trivy scan, a PowerShell 5.1 parse check of `install.ps1`, and a GoReleaser dry run.

## Not covered
- `darwin/arm64`: unit tests in CI only; no smoke test or real machine.
- `linux/amd64`: best effort, unit tests in CI only.
- macOS 13 is only covered by Machine B; GitHub has no macOS 13 runner.
