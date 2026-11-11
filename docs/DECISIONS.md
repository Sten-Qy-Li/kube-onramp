# Design decisions

Short entries: what was decided and why. Newest at the bottom. When a decision changes, add a new entry that replaces the old one (don't rewrite history).

## D1. Go 1.27.4 toolchain, `go 1.27.0` language version (2026-11-11)
Go 1.27.4 is the latest stable release (go.dev/dl). The Go wiki says Go 1.27 and later need macOS 13 Ventura, so Go 1.27 still runs on Machine B (macOS 13.7.8). `go.mod` has `go 1.27.0` and `toolchain go1.27.4`. Check the wiki before moving to a newer Go: a later release may drop macOS 13.

## D2. Pins: kubectl v1.36.6, kubelogin (int128) v1.36.4 (2026-11-11)
The cluster answered anonymous `GET /version` with `v1.36.6`, and `dl.k8s.io/release/stable-1.36.txt` is `v1.36.6`. kubelogin v1.36.4 is the latest non-prerelease on GitHub. Every SHA-256 in the catalog was checked twice: against the upstream `.sha256` file, and by downloading the file and hashing it.

## D3. Minimum OS in the catalog comes from the Go version inside each binary
`go version <file>` reads the build info without running the file. kubectl v1.36.6 is built with go1.26.10 and kubelogin v1.36.4 with go1.26.6. Per the Go wiki, Go 1.25–1.26 need Windows 10, macOS 12 and Linux kernel 3.2, so those are the minimums. kube-onramp itself (Go 1.27) needs macOS 13.

## D4. Identity check for int128 kubelogin uses `--help`
int128's root help starts with "Log in to the OpenID Connect provider." Azure's kubelogin (v0.2.22, `pkg/cmd/root.go`) uses "login to azure active directory and populate kubeconfig with AAD tokens". The check requires the first text and rejects "azure active directory". The version comes from the `version` subcommand (int128 has no `--version` flag): `kubelogin version v1.36.4 (go1.26.6 windows_amd64)`. Builds without a version string (seen with the winget copy on Machine C: `kubelogin version  (go…)`) count as "right product, version unknown".

## D5. The CA comes from the course page and matches a working kubeconfig
`certificate-authority-data` on https://devops.cs.ut.ee/phase3/week9/ matches a working course kubeconfig byte for byte. It is a self-signed `CN=kubernetes` CA, valid until 2035-09-07. Only the cluster fields of the local kubeconfig were read.

## D6. Data files live at the repo root and are embedded by a root package
`go:embed` can't reach parent directories, so `assets.go` (package `kubeonramp`, at the module root) embeds `profiles/` and `catalog/`. `internal/profile` and `internal/catalog` parse those bytes. This keeps the data easy to find for contributors ("add a tool = edit `catalog/tools.yaml`").

## D7. YAML is parsed with `sigs.k8s.io/yaml`, strictly
It converts YAML to JSON and uses `json` tags, so the same structs work for both. It comes in with client-go anyway. Strict mode turns unknown or misspelled keys into errors, so a typo in the catalog fails the tests instead of being ignored.

## D8. Catalog stores the file name without `.exe`
`binary: kubectl-oidc_login`, and the code adds `.exe` on Windows. One field instead of one per platform, and it can't get out of sync.

## D9. Step numbering
Section 8 has 8 parts, but the login step must be "Step 6 of 8". Section 8.4 is split into step 4 (download tools) and step 5 (PATH and kubeconfig); 8.7 and 8.8 share step 8. See the table in SPEC section 8.

## D10. "No file writes" on a second run excludes the log and the lock
The spec also requires a log for every run. All other files (bin, state.json, kubeconfig, rc files, PATH) must keep their bytes and modification times.

## D11. Kubeconfig "untouched" means the same content
`clientcmd.Write` re-serializes, so comments and key order are lost (kubectl does the same with `kubectl config set-…`). The timestamped backup keeps the original bytes.

## D12. Network preflight follows redirects
GitHub release downloads redirect to `release-assets.githubusercontent.com` (seen 2026-11-11; it was `objects.githubusercontent.com` before 2025). `dl.k8s.io` answered directly with 200. Following each catalog URL's redirects tests the real hosts and survives host changes.

## D13. `install.ps1` is pure ASCII
GitHub serves release assets as `application/octet-stream`. PowerShell 5.1's `irm` may decode such a body with a legacy code page, so any non-ASCII character could break the script.

## D14. Release candidates: the tag is baked into the install scripts
`releases/latest/` skips pre-releases. The release workflow writes the tag into the scripts before GoReleaser uploads them, so `releases/download/v0.1.0-rc.1/install.ps1` installs rc.1. Raw binary names have no version (`kube-onramp_windows_amd64.exe`), so the URL is easy to build.

## D15. Windows 11 detection uses the build number
On Machine C (build 26300) the registry `ProductName` says "Windows 10 Education". Build 22000 or higher is reported as "Windows 11". The edition comes from `ProductName`, the feature version from `DisplayVersion`.

## D16. `reset` on Windows deletes its own `.exe` after exiting
Windows can't delete a running executable, and delete-on-reboot (`MoveFileEx`) needs admin rights. A short detached helper process waits for the tool to exit, then deletes the bin dir.

## D17. CI supply chain: SHA-pinned actions, Trivy as a pinned binary, 7-day cooldown
- Actions are pinned to full commit SHAs, with the tag in a comment.
- Trivy runs as the release tarball with a pinned SHA-256, not via `aquasecurity/trivy-action` or `setup-trivy`. On 2026-03-19 an attacker force-pushed 76 of 77 `trivy-action` tags and all `setup-trivy` tags to credential-stealing code, and published a malicious Trivy v0.69.4 (GHSA-69fq-xp46-6x23).
- Tool versions used in CI must be at least 7 days old when pinned. That incident's malicious release was live for about 3 hours. So: GoReleaser v2.20.0 (not v2.20.1, released the same day), Trivy v0.76.1 (not v0.76.2, 6 days old).

## D18. Intel macOS CI runs on `macos-15-intel`
GitHub's runner list (2026-11-11) has `macos-15-intel` and `macos-26-intel` for Intel; there is no macOS 13 runner. `macos-15-intel` is the closest to Machine B. Machine B (a teammate) is the only real macOS 13 test.

## D19. Windows PATH tests use a throwaway registry key
`KUBE_ONRAMP_HOME` doesn't isolate `HKCU\Environment`. The hidden variable `KUBE_ONRAMP_TEST_ENV_KEY` points PATH reads and writes at `HKCU\Software\<key>` instead, so local runs never change the developer's real PATH. CI runners are throwaway machines and use the real key.

## D20. Known machine fact: Docker Desktop kubectl on the system PATH
On Machine C, Docker Desktop (admin install) puts kubectl v1.34.1 in the system PATH. Windows puts the system PATH before the user PATH, so a no-admin tool can't win there: expect `W-SHADOW`, with "not acceptable" because 1.34 is two minors behind the 1.36 server. Machine A's per-user Docker install is on the user PATH, where kube-onramp's entry comes first.

## D21. curl on Windows needs `--ssl-no-revoke` for the cluster
Windows curl (schannel) fails with "revocation status is unknown" against the private cluster CA, which has no CRL or OCSP endpoint. Go's TLS stack doesn't check revocation, so the tool isn't affected. Only relevant for manual checks; bootstrap scripts never contact the cluster.

## D22. License
Apache-2.0. The copyright holder is written as "The kube-onramp Authors" (in `NOTICE` and the README), so no personal name appears in the repo.

## D23. kubelogin's version comes from kube-onramp's install record (replaces part of D4)
D4 said that only some builds print an empty version. That was wrong: the official v1.36.4 release binaries print `kubelogin version  (go1.26.6 …)`, because they are built with `-ldflags "-X main.version="`. Their Go build info has `vcs.revision=4d0d2336…`, which is the `v1.36.4` tag (checked with the GitHub API); the winget copy on Machine C has the same revision. So: a copy that prints no version counts as kube-onramp's pinned version only if its SHA-256 matches the hash recorded in `state.json` when kube-onramp installed it. Otherwise it is downloaded again (once). The download itself was already checked against the pinned SHA-256, which is stronger than a version string.

## D24. The run lock is an OS file lock
`LockFileEx` on Windows, `flock` on macOS/Linux, on `<home>/lock`. When a process dies, the OS releases its lock, so a stale lock can't block later runs. The PID inside the file is only for people.

## D25. A profile context that is already current stays current
Setup switches the current context to staging only if the current context is not one of the profile's contexts. A student who chose prod keeps prod, and a second run changes nothing.

## D26. Symlinked rc files and kubeconfigs are written through
Atomic writes replace the file at the path. If `~/.zshrc` or the kubeconfig is a symlink (dotfile managers), kube-onramp writes to the symlink's target, so the link stays a link.

## D27. The clipboard is used only when a person is watching
The error report is copied to the clipboard only when stdout is a terminal. Scripts, CI and test runs don't overwrite anyone's clipboard. The report file is always saved.

## D28. Extra codes and one extra flag
- `E-INSTALL`: a verified file could not be put into the bin folder, or the new copy doesn't start (often antivirus).
- `E-LOGIN-FAILED`: a login failure that is not a timeout, a rejection, busy ports or the network.
- `--relogin`: forgets kube-onramp's own token cache first. Used when the browser logged in with the wrong account ("You are logged in as …" shows someone else).

## D29. Network preflight only tests hosts the run needs
Download hosts are tested only for tools that look like they need a download (no own copy, or `state.json` has another version). A run with nothing to install doesn't need GitHub. The OIDC issuer and the API server are tested unless `--skip-login` is given (CI can't rely on reaching the cluster).

## D30. `check`: a different default context or file mode is a warning
Wrong current context and a world-readable kubeconfig are shown as `[WARN]`. Missing or broken tools, PATH or kubeconfig entries are `[FAIL]` and give exit code 1 with "Run kube-onramp to fix this."

## D31. The rc block prepends to PATH without an "already there?" check
macOS `path_helper` (in `/etc/zprofile`, login shells) moves inherited PATH entries behind `/usr/local/bin`. A guard like `case ":$PATH:" in *bin*` would then keep Homebrew first in nested login shells. A duplicate entry is harmless; losing to Homebrew is not.

## D32. Login timeouts
kube-onramp waits up to 5 minutes for `kubectl auth whoami`. kubelogin has its own 180 s authentication timeout (default, v1.36.4), which usually ends first; both become `E-LOGIN-TIMEOUT`. kubelogin prints the login URL only when it can't open a browser ("Please visit the following URL in your browser manually: …"); kube-onramp shows it as "Open this link in your browser".

## D33. Windows minimum is build 19045 (Windows 10 22H2)
Lower builds get `E-PRE-OS`. 22H2 is the last Windows 10 release, so every supported Windows 10 machine has it.

## D34. Smoke tests compare bytes
The CI smoke tests start from `internal/kubeconfig/testdata/other.canonical.yaml` (kept in clientcmd's exact output form by a unit test) and an rc file, and require identical bytes after setup + reset. On the Windows runner they test the real `HKCU\Environment`; locally the script uses the throwaway key (D19).

## D35. gosec exclusions
kube-onramp's job is to read and write the user's files and run their tools. gosec rules G103, G115, G204, G301, G302, G304 and G306 flag exactly that, so they are excluded in `.golangci.yml`, with a reason per rule. Secrets (kubeconfig, state, reports, logs) are still written with mode 600.

## D36. Dependencies are upgraded when CI finds a vulnerability
client-go v0.37.2 pulls in `golang.org/x/net` v0.57.0 and `golang.org/x/text` v0.40.0. govulncheck (reachable calls in HTTP/2) and Trivy (HIGH) flagged them on 2026-11-11. They are raised to x/net v0.62.0 (2026-10-27) and x/text v0.43.0 (2026-10-09), the versions that fix the issues and are at least 7 days old. CI fails on any reachable vulnerability (govulncheck) or HIGH/CRITICAL finding (Trivy), so this repeats whenever new advisories appear.

## D37. The team name is saved right after the Enter press
Found in acceptance test 5 (2026-11-11): a run killed during the downloads had not saved the team yet, so a re-run without `--team` stopped with "needs your team name". state.json is now saved after the confirmation (it is only written when its content changes, so a run with nothing to do still writes nothing).
