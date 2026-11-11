# Acceptance test results

The acceptance tests from [SPEC.md](SPEC.md), section 13, for v0.1.0. All ran on 2026-11-11. The machines are described in SPEC section 2; team names are left out on purpose.

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

## Automated tests (every push)
- Unit tests on Windows, Intel macOS, Apple silicon macOS and Linux.
- End-to-end smoke tests on `windows-latest` and `macos-15-intel`: setup, setup again (no changes), check, reset, everything back byte for byte.
- golangci-lint, shellcheck, govulncheck, a Trivy scan, a PowerShell 5.1 parse check of `install.ps1`, and a GoReleaser dry run.

## Not covered
- `darwin/arm64`: unit tests in CI only; no smoke test or real machine.
- `linux/amd64`: best effort, unit tests in CI only.
- macOS 13 is only covered by Machine B; GitHub has no macOS 13 runner.
