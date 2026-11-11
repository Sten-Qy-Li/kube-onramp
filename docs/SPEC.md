# kube-onramp specification

This is the specification for `kube-onramp`. It started as the build brief and is kept current. When a decision changes behavior, update this file and record the reason in [DECISIONS.md](DECISIONS.md).

## 1. Background and goal

The University of Tartu course "DevOps: Automating Software Delivery and Operations" (fall 2026) gives every student OIDC access to a shared Kubernetes cluster. Each team gets namespaces in that cluster.

Students must install the following by hand, and many fail somewhere along the way:
- `kubectl`;
- the int128 `kubelogin` plugin (`kubectl oidc-login`);
- a kubeconfig with an exec-based OIDC login.

Every team member must show working cluster access at two checkpoint presentations: Checkpoint 3 on Nov 20 and Checkpoint 4 in week 16 (mid-December).

`kube-onramp` is a single-binary tool that gets a laptop **ready for Kubernetes work on this cluster**. It works by reconciling: it inspects the machine, works out what is missing or broken, fixes only that, logs the user in, and proves that access works.

Things it does not ask the user to do:
- open a terminal and type commands (other than one paste);
- edit files;
- understand PATH.

Design it as a **generic engine** with the UT course as the first **profile** (section 6). It should be a credible, reusable open-source tool, not a one-off script.

**Out of scope:** the tool must never generate course deliverables. That means no Helm charts, Kubernetes manifests, HPA/KEDA configs, CI YAML or Grafana dashboards. It only sets up laptops.

---

## 2. Users and reference machines

**Users:** impatient, non-expert students. They skip instructions and run things twice. All text shown to users must be in simple English at CEFR level B1 (section 11).

The machines below were captured from real teammates; personal details have been removed. They are the main test targets:

| | Machine A (Windows) | Machine B (macOS) |
|---|---|---|
| OS | Windows 10 Pro 22H2 (10.0.19045), x64 | macOS 13.7.8 Ventura (22H730), **Intel** (x86_64), no Rosetta |
| Shell | PowerShell 5.1, ExecutionPolicy CurrentUser = RemoteSigned | zsh (`/bin/zsh`) |
| Package managers | winget | Homebrew at `/usr/local` |
| kubectl found | Docker Desktop's (per-user install): `%LOCALAPPDATA%\Programs\DockerDesktop\resources\bin\kubectl.exe` | `/usr/local/bin/kubectl` (Homebrew or Docker) |
| kubelogin found | int128 via winget: `kubectl-oidc_login.exe` and `kubelogin.exe` under `%LOCALAPPDATA%\Microsoft\WinGet\Packages\int128.kubelogin_…` | `/usr/local/bin/kubectl-oidc_login` (int128 via Homebrew) |
| `~/.kube` contents | `config` (about 2.2 KB, probably the course config), `cache/`, plus an unrelated `deployment.yaml` | `config` (about 2.1 KB, **mode 644**), `cache/`, `config.broken` (24 bytes, from an earlier failed attempt) |
| Other | WSL distros `docker-desktop` and `Ubuntu` | |

Machine C is my own Windows machine, the one you run on. It already has a working setup, so it's the test bed for "already set up" and "repair".

What this means for the design:
- Both teammates already have partial or working installs. **Repair and coexistence matter more than clean installs.**
- Their existing copies come from Docker Desktop, winget and Homebrew. The tool must **leave them untouched** and still make its own copies win (section 8).
- Machine B's kubeconfig is world-readable, and Helm warns about that. The tool sets mode 600 on macOS.
- macOS 13 is the oldest supported macOS. Machine B can't upgrade further, so do not build with a Go version that requires macOS 14+.

---

## 3. Scope

### Core (v0.1)
| Tool | Purpose |
|---|---|
| `kubectl` | Pinned to the cluster's minor version. The server reported v1.36.6 on 2026-11-11 (anonymous `GET /version`), so the pin is kubectl v1.36.6 |
| `kubelogin` (int128) | Installed as `kubectl-oidc_login[.exe]`. Latest stable 1.x (v1.36.4 on 2026-11-11) |

### Extras (v0.2, installed by default, skipped with `--minimal`)
| Tool | Why students need it |
|---|---|
| Helm **4** (latest 4.x) | Phase 4 templating. The course uses `--rollback-on-failure`, which is a Helm 4 flag. Helm 3 stops getting security patches on Nov 11, 2026 |
| k9s | Watching pods, HPA scaling and chaos-killed pods live during demos |
| Trivy | Reproducing the CI image or filesystem scan locally |
| kubeconform | Reproducing the CI manifest validation locally |
| `oha` (verify it suits; otherwise propose an alternative) | Generating HTTP load to demo autoscaling |

Extras must never block the core. If an extra fails, show a warning and carry on.

### Never installed
- Lens (GUI app, account, auto-updates);
- Docker Desktop;
- ArgoCD and Flux CLIs;
- standalone Kustomize (it's built into kubectl).

Never uninstall or modify software that the tool didn't install.

---

## 4. Platforms and build

| Target | Status |
|---|---|
| `windows/amd64` | Must work. Windows 10 22H2 and later, PowerShell 5.1 |
| `darwin/amd64` | Must work. macOS 13 and later (Machine B) |
| `darwin/arm64` | Build and ship it. CI-tested only, as no team hardware is available |
| `linux/amd64` | Build it for CI and best effort. Not a user target |

Build rules:
- **Go version:** the latest stable release that still supports macOS 13. Per the Go wiki (https://go.dev/wiki/MinimumRequirements), Go 1.27 and later require macOS 13 Ventura or newer. A later Go release will drop macOS 13, so check the wiki before upgrading Go. Pin it in `go.mod` with a `toolchain` line. Pure Go, `CGO_ENABLED=0`. Current pin: `toolchain go1.27.4` (verified 2026-11-11).
- **No UPX or binary packing.** It increases antivirus false positives.
- **Catalog check:** each catalog entry records the minimum OS it supports. Preflight refuses to install a tool whose recorded minimum is above the machine's OS version.

---

## 5. Distribution

- **Public GitHub repo** named `kube-onramp`, released with **GoReleaser** from **GitHub Actions** when a `v*` tag is pushed. Each release publishes:
  - raw binaries for each platform, plus archives;
  - `checksums.txt`;
  - `install.ps1` and `install.sh` as release assets.

  An SBOM is a nice-to-have.

- **The one-liners** are the main way to start the tool. Teammates paste exactly one line.

  Windows (PowerShell 5.1 compatible):
  ```powershell
  [Net.ServicePointManager]::SecurityProtocol = 'Tls12'; & ([scriptblock]::Create((irm https://github.com/<owner>/kube-onramp/releases/latest/download/install.ps1))) -Team <team>
  ```
  `install.ps1` declares PowerShell-style parameters (`-Team`, `-Minimal`, `-Yes`, …) and maps them to the binary's `--team`, `--minimal`, `--yes`, because PowerShell doesn't parse `--flag` tokens reliably.
  macOS:
  ```sh
  curl -fsSL https://github.com/<owner>/kube-onramp/releases/latest/download/install.sh | sh -s -- --team <team>
  ```
  Adjust the argument-passing syntax if needed. The requirement is that a pre-filled team name can be passed. Test both lines for real.

- **Bootstrap script requirements:**
  - **What they do:** detect the OS and architecture, download the matching binary and `checksums.txt`, verify SHA-256, put the binary in the tool's bin dir (section 8.4), and run it with all arguments forwarded.
  - **If a bootstrap step fails:** print the standard error report (section 10) with an `E-BOOT-*` code.
  - **`install.ps1` (PowerShell 5.1):**
    - force TLS 1.2;
    - use `Invoke-WebRequest -UseBasicParsing`;
    - set `$ProgressPreference = 'SilentlyContinue'`, since the progress bar makes downloads very slow on 5.1;
    - never call `exit`, because `iex` runs in the user's own session. Return instead;
    - no admin rights, and no execution-policy changes.
  - **`install.sh`:**
    - POSIX `sh` with `set -eu`;
    - `shasum -a 256` on macOS (`sha256sum` on Linux);
    - no `sudo`;
    - run the binary with `</dev/tty` so it can read the Enter key press, because stdin is the pipe under `curl | sh`.
  - **Why the one-liners:** files fetched by `curl` get no macOS quarantine attribute, so Gatekeeper doesn't block them. On Windows, the one-liner also avoids SmartScreen.
- **Fallback:** direct download of the `.exe` from the release page is fine. One SmartScreen prompt is acceptable. Document it in the README.
- **Release candidates:** each release bakes its own tag into `install.ps1` and `install.sh`. So `https://github.com/<owner>/kube-onramp/releases/download/<tag>/install.ps1` installs exactly `<tag>`, and `releases/latest/` (which skips pre-releases) always means the latest final release.
- **`install.ps1` is pure ASCII.** PowerShell 5.1 can decode a downloaded script with the wrong code page, so non-ASCII characters are not allowed in it.
- **Raw binary names** have no version, so the scripts can build the URL from the tag: `kube-onramp_<os>_<arch>[.exe]`.

---

## 6. Data model: profile and catalog

Both are embedded data files (`go:embed`, YAML or JSON). Adding a tool or a cluster should mean adding data, not code.

### 6.1 Profile (`profiles/ut-devops.yaml`)

| Field | Value |
|---|---|
| Display name | University of Tartu DevOps course cluster |
| API server | `https://kubernetes.devops.cs.ut.ee:6443` |
| CA data | `certificate-authority-data` from the course kubeconfig. Get it from https://devops.cs.ut.ee/phase3/week9/ and cross-check against my working `%USERPROFILE%\.kube\config` (read only; never modify it) |
| OIDC issuer | `https://keycloak.hpc.ut.ee/realms/ETAIS` |
| OIDC client ID | `kubernetes.devops.cs.ut.ee` |
| Extra kubelogin args | `--oidc-pkce-method=auto` |
| Exec `apiVersion` | `client.authentication.k8s.io/v1beta1`, as in the course config |
| Kubeconfig names | cluster `ut-devops`, user `ut-devops-oidc`, contexts `ut-devops-staging` and `ut-devops-prod` |
| Namespace templates | `ns-{team}-staging`, `ns-{team}-prod`. The prod namespace appears in Phase 4; confirm its exact name then. Both can be overridden with flags |
| Team name rule | As the course defines it: lowercase, spaces and underscores become hyphens. `Best_Team` → `best-team` → `ns-best-team-staging`. Also accept a full namespace typed by mistake (`ns-best-team-staging` → `best-team`). Validate as a DNS-1123 label |
| Access checks (staging) | Required: `get pods`. Informational: `list deployments.apps`, `create deployments.apps`, `create secrets`, `list services` |
| Login wording | "your University of Tartu account" |
| Help links | course site https://courses.cs.ut.ee/2026/DevOps/fall, ETAIS https://minu.etais.ee, Grafana https://monitoring.kubernetes.devops.cs.ut.ee/ |

### 6.2 Catalog (`catalog/tools.yaml`)

Each entry has:
- an `id`;
- a `role` (`core` or `extra`);
- a pinned `version`;
- for each platform: download URL, archive type, path of the binary inside the archive, **pinned SHA-256**, download size in bytes (for the size estimate in the plan), and minimum OS version;
- the installed file name without extension (for example `kubectl-oidc_login`); `.exe` is added on Windows;
- a version command and a regex to parse its output;
- an **identity check**, so the tool can tell int128 `kubelogin` from Microsoft's unrelated Azure `kubelogin`.

Also add a maintainer command, for example `go run ./tools/update-catalog`. It looks up the latest upstream versions, fetches upstream checksum files, and rewrites the catalog for review. Pinned hashes (rather than trusting whatever upstream serves at install time) are a supply-chain measure worth calling out in the README.

---

## 7. Commands, flags, exit codes

```
kube-onramp [setup]   full reconcile (default command)
kube-onramp check     inspect + log in if needed + verify; never installs
kube-onramp reset     remove exactly what kube-onramp added
kube-onramp version
```

| Flag | Meaning |
|---|---|
| `--team <name>` | Team name, normalized per the profile rule. Remembered in the state file after the first run |
| `--staging-namespace`, `--prod-namespace` | Override the templates |
| `--minimal` | Core only, no extras |
| `--yes` | Skip the single Enter-press confirmation (for CI) |
| `--dry-run` | Show the plan, then exit |
| `--verbose` | More detail on screen. The log file is always verbose |
| `--profile <id>` | Default `ut-devops` |
| `--no-color` | Also honor the `NO_COLOR` environment variable |
| `--skip-login` | Hidden, for tests and CI: skip the browser login and cluster checks |
| `--relogin` | Forget kube-onramp's own token cache and log in again (for a wrong browser account) |

Environment variable `KUBE_ONRAMP_HOME` overrides the tool's home directory (for tests).

Exit codes:
- `0`: success, possibly with warnings;
- `1`: a core step failed;
- `2`: usage error;
- `3`: another run is in progress.

If stdin isn't a terminal and `--yes` isn't given, refuse with a clear message instead of hanging.

---

## 8. The setup flow

The tool reconciles: **inspect, compute the difference, act only on the difference, verify**.

- Running it twice changes nothing the second time. In that case the plan is empty and the tool prints "Everything is already set up", with no file writes and no changed modification times. (The run's own log file and the lock file are the exceptions: every run writes a log.)
- Running it after a crash or a closed window continues where it stopped.

Show progress as `Step N of 8: <what>`. The 8 steps are:

| Step | Text on screen | Sections |
|---|---|---|
| 1 | Checking your computer | 8.1 |
| 2 | Looking at what is installed | 8.2 |
| 3 | Making a plan | 8.3 |
| 4 | Installing tools | 8.4 (downloads) |
| 5 | Setting up PATH and your Kubernetes settings | 8.4 (PATH, kubeconfig) |
| 6 | Checking your login | 8.5 |
| 7 | Checking your access to the cluster | 8.6 |
| 8 | Extra tools and final check | 8.7, 8.8 |

### 8.1 Preflight (read-only; any failure stops the run before anything changes)

1. **OS, version and architecture.** Check them against the support matrix. If the tool is running inside WSL, tell the user to run it on Windows itself, and say why.
   - **Windows name:** don't trust the registry `ProductName` (on Windows 11 it can still say "Windows 10 …"). Use the build number: 22000 or higher is "Windows 11", lower is "Windows 10". Keep the edition from `ProductName` (for example "Pro", "Education") and the feature version from `DisplayVersion` (for example "22H2"). Use this name in the summary and in the error report's OS line.
2. **Lock file.** If another run holds it, print "kube-onramp is already running in another window" and exit with code 3. Recover stale locks, for example when the recorded PID no longer exists.
3. **Free disk space** in the tool home: at least 1 GB.
4. **Network.** Reach every host the plan needs:
   - the catalog's download hosts, including GitHub's redirect targets. Follow the redirects of each catalog URL instead of hard-coding the target hosts (GitHub moved release assets to `release-assets.githubusercontent.com` in 2025);
   - the OIDC issuer: `GET <issuer>/.well-known/openid-configuration` must return 200;
   - the API server: `GET /version` using the profile CA. Any HTTP response counts as reachable. Use the result for the version-skew check.

   Honor system proxy environment variables. Never disable TLS verification.
5. **Login ports.** The browser callback needs `127.0.0.1:8000` or `127.0.0.1:18000`. Check that at least one of them is free.
6. **`KUBECONFIG` environment variable.** If it's set, report it, and follow client-go's rules about which file gets written.

### 8.2 Inspect (read-only)

For each catalog tool, run five checks:
1. **Present:** every copy on PATH, plus the tool's own copy.
2. **Right product:** the identity check.
3. **Acceptable version:**
   - kubectl must be within ±1 minor of the server;
   - Helm must be 4.x;
   - a tool that prints no version (kubelogin v1.36.4 release builds) counts as the pinned version only if it is kube-onramp's own copy with the SHA-256 recorded at install time (D23).
4. **Actually runs:** the version command succeeds.
5. **Wins on PATH in a new terminal.** Compute this for a new shell, not the current process:
   - **Windows:** build PATH as the system PATH plus the user PATH from the registry, with variables expanded, then resolve.
   - **macOS:** run `zsh -l -i -c 'command -v kubectl'` (or the user's `$SHELL`) with an output marker, so that noisy rc files don't break the parsing.

Also inspect:
- **PATH:** whether the tool's bin dir is present and comes first.
- **Kubeconfig:** whether it can be parsed (load it with client-go `clientcmd`, the same rules kubectl uses), whether the profile entries exist and match, what the current context is, and the file mode on macOS.
- **The state file**, if one exists.

### 8.3 Plan and confirm

Turn the differences into a short list of actions in B1 English, for example:
- "Install kubectl 1.xx"
- "Add the course cluster to your Kubernetes settings (a backup is made first)"
- "Log in"

Also show:
- the namespaces the team name maps to;
- a rough download size.

Then ask for **exactly one Enter press** ("Press Enter to start, or close this window to stop."). Skip it with `--yes`. If the plan is empty, skip straight to login and verification.

If no team name is known, ask for it once, before the plan. Show an example and validate the input.

### 8.4 Apply core

**Locations.** Install the tool's own pinned copies; never reuse other copies.

| | Windows | macOS |
|---|---|---|
| Tool home | `%LOCALAPPDATA%\kube-onramp\` | `~/.kube-onramp/` |
| Binaries (including `kube-onramp` itself) | `<home>\bin` | `<home>/bin` |
| Other contents | `state.json`, `logs\`, `backups\`, `cache\` | the same |

**Downloads:**
1. Download to a temporary file in the tool home.
2. Verify the SHA-256 against the catalog.
3. Extract, and set the executable bit on macOS.
4. Rename into place, so the file appears in one step.

Interrupting can never leave a half-written file in `bin`. Show a simple percentage while downloading.

**PATH on Windows:**
- Edit `HKCU\Environment\Path` directly. Keep the registry value type (`REG_EXPAND_SZ` or `REG_SZ`) and every existing entry, and **never truncate** (no `setx`).
- Prepend the bin dir, and remove duplicates of the tool's own entry only.
- Broadcast `WM_SETTINGCHANGE` ("Environment") so newly opened terminals see the change. Update the current process too.

**PATH on macOS:**
- Add a marked block to the shell's rc file: `~/.zshrc` for zsh, `~/.bash_profile` for bash. The markers are `# >>> kube-onramp >>>` and `# <<< kube-onramp <<<`.
- The block prepends the bin dir. It must end up ahead of Homebrew's `/usr/local/bin` (or `/opt/homebrew/bin`).
- Editing must be idempotent.

**Kubeconfig:**
1. Back up the whole file to `backups/` with a timestamp.
2. Load and merge with client-go `clientcmd`. Add or update **only** the profile's cluster, user and contexts, and leave everything else untouched. That includes the course's original `etais-user@kubernetes` entries and Machine B's `config.broken`. "Untouched" means the content stays the same: `clientcmd.Write` re-serializes the file, so YAML comments and formatting are not kept (kubectl behaves the same way). The backup keeps the exact original bytes.
3. Set the current context to `ut-devops-staging`, and record the previous one for `reset`. If the current context is already `ut-devops-prod`, keep it (D25).
4. Serialize with `clientcmd.Write`, then write atomically (temporary file, then rename). Use UTF-8 without BOM, and mode 600 on macOS.
5. If the file can't be parsed (for example UTF-16, broken YAML or binary garbage), keep the backup, write a fresh file, and tell the user where the backup is.

**Exec entry for the user.** Point it straight at the tool's own plugin binary by **absolute path**, so that login works even if PATH is wrong:
- `command`: `<home>/bin/kubectl-oidc_login[.exe]`
- `args`:
  - `get-token`
  - `--oidc-issuer-url=…`
  - `--oidc-client-id=…`
  - `--oidc-pkce-method=auto`
  - `--token-cache-dir=<home>/cache/oidc-login`

  The separate cache dir isolates the login from stale caches left by teammates' earlier attempts. It also lets `reset` clean up.
- `interactiveMode: IfAvailable`, `provideClusterInfo: false`.

### 8.5 Log in (the only human step)

1. Before anything opens, print: "Step 6 of 8: Checking your login. If a browser window opens, log in with your University of Tartu account, then come back to this window."
2. Run the tool's own `kubectl auth whoami --context ut-devops-staging` with a 5-minute timeout. If the cached token is still valid, this finishes instantly with no browser.
3. If the browser can't open, kubelogin prints a URL. Capture it and show it prominently: "Open this link in your browser: …".
4. Show "You are logged in as <username>" and ask the user to check it's them. This catches a wrong browser account.
5. Classify failures:
   - **Timeout:** `E-LOGIN-TIMEOUT`.
   - **401 or token rejected:** clear the tool's own token cache and retry once, then `E-LOGIN-REJECTED`.
   - **Both ports busy:** `E-PRE-PORTS`. This should already have been caught in preflight.
   - **Network:** `E-CLUSTER-UNREACHABLE`.

### 8.6 Verify access

Run `kubectl auth can-i` against the staging namespace, using the profile's checks.
- `get pods` must pass. If it's forbidden, raise `E-ACCESS-FORBIDDEN`. Show the exact namespace used, and explain that a wrong team name and "no access yet" look the same. The next steps are: check the name, then ask the person who created the namespace in ETAIS or a TA.
- For prod: a forbidden or missing namespace is `W-PROD-NOT-YET` (a yellow warning, not red) until Phase 4.

Also warn with `W-SKEW` if kubectl and the server are more than one minor version apart.

### 8.7 Apply extras (v0.2; skipped by `--minimal`)

Same download, verify and install steps as the core, one tool at a time. Each failure becomes `W-EXTRA-<tool>` with its own short report saved to `logs/`. The final result can still be green for cluster access.

### 8.8 Verify in a fresh shell, then summarize

1. **Fresh-shell check.** Resolve `kubectl`, `kubectl-oidc_login`, `kube-onramp` (and the extras) the way a new terminal would (section 8.2, check 5).
   - If the tool's own copies win, report `[OK]`.
   - If another copy wins on Windows (for example one on the system PATH, which a no-admin tool can't outrank), report `W-SHADOW`. Say whether that copy is acceptable, and if it isn't, say what to update (usually Docker Desktop).
2. **Summary.** One line per item, with labels `[OK]`, `[FIXED]`, `[WARN]` or `[FAIL]`. Then the next steps:
   - "Open a NEW terminal window."
   - "Try: kubectl get pods"
   - how to switch between staging and prod;
   - "Before a presentation, run: kube-onramp check"
   - the help links.
3. **Keep the window open on Windows.** If the console belongs only to this process (double-clicked from Explorer, for example `GetConsoleProcessList` returns 1), end with "Press Enter to close". Do the same after a fatal error.

---

## 9. `check` and `reset`

- **`check`:** runs preflight (light), inspect, login, verify-access and summary.
  - It never installs or changes anything except renewing the login. If the login has expired, it **opens the browser automatically**, after the same notice as in 8.5.
  - If something is broken, it says "Run kube-onramp to fix this." and exits with code 1.
  - This is the command teammates run 10 minutes before each presentation.
- **`reset`:** driven by `state.json`.
  - It removes: the tool's bin dir and cache; its PATH entry or rc block; and **only its own kubeconfig entries**. It doesn't restore an old backup, because that would lose later changes.
  - It restores the previous current context, keeps the `backups/` folder, asks for one Enter press (or accepts `--yes`), and prints what it removed.
  - On Windows a running `.exe` can't delete itself. `reset` starts a short detached helper that deletes `kube-onramp.exe` and the bin dir after the tool has exited.
  - It leaves other software alone.
  - Running `setup` again afterwards must work.

---

## 10. Errors, logs and the error report

### Full log
Every run writes a verbose log to `logs/<timestamp>.log`. Keep the last 10.

### Error report
On any `E-*` error, print a short report and **copy it to the clipboard automatically**:
- Windows: use the Win32 clipboard API, not `clip.exe`, which can mangle non-ASCII text;
- macOS: `pbcopy`.

Also save it as `logs/report-<timestamp>.txt` and print that path. Then say: "The report is copied. Paste it into Slack or into an AI chatbot and ask for help."

Report format (keep it short; the user is going to paste it to someone):
```
=== kube-onramp error report (paste this into an AI chatbot) ===
kube-onramp installs kubectl and kubelogin (int128) and sets up OIDC access to a Kubernetes cluster.
Trying to: log in to the cluster through the browser (step 6 of 8)
Error code: E-LOGIN-TIMEOUT
Tool: kube-onramp v0.1.0 | OS: Windows 10 Pro 22H2 x64 (build 19045) | PowerShell 5.1 | kubectl v1.xx.x | kubelogin v1.xx.x
--- last 30 log lines (personal data removed) ---
...
=== end of report ===
```

**Redaction**, applied to both the report and the full log:
- home directory → `~`;
- OS username → `<user>`;
- the cluster username shown by `kubectl auth whoami` → `<cluster-user>` (it's shown on screen to the user, but never kept in logs or reports);
- anything that looks like a JWT (`eyJ…`) → `<token>`;
- emails → `<email>`;
- values of `id-token`, `refresh-token`, `access_token`, `client-secret` and `password` → `<redacted>`.

Unit-test the redaction.

### Error codes
Each code has a fixed B1 message and next steps. Define these, and add more as needed:

| Code | Meaning |
|---|---|
| `E-PRE-OS`, `E-PRE-WSL`, `E-PRE-DISK`, `E-PRE-NET`, `E-PRE-PORTS` | Preflight failures (`E-PRE-NET` names the host it couldn't reach) |
| `E-BOOT-*` | Bootstrap script failures |
| `E-DL`, `E-SHA`, `E-EXTRACT` | Download, checksum mismatch, archive problems |
| `E-INSTALL` | A verified file couldn't be put into the bin folder, or the new copy doesn't start |
| `E-PATH` | Couldn't update PATH |
| `E-KCFG` | Couldn't read or write the kubeconfig |
| `E-LOGIN-TIMEOUT`, `E-LOGIN-REJECTED`, `E-LOGIN-FAILED` | Login failures (`E-LOGIN-FAILED`: anything that is not a timeout, rejection, port or network problem) |
| `E-CLUSTER-UNREACHABLE` | API server not reachable |
| `E-ACCESS-FORBIDDEN` | No access to the staging namespace |
| `E-INTERNAL` | A recovered panic. The stack trace goes only in the log file, never on screen |
| `W-EXTRA-<tool>`, `W-SHADOW`, `W-SKEW`, `W-PROD-NOT-YET` | Warnings |

Example message:
> **E-ACCESS-FORBIDDEN**: You are logged in, but you cannot see the namespace `ns-<team>-staging`.
> 1. Check your team name. Run again with: `kube-onramp --team <your-team>`
> 2. If the name is right, ask the teammate who created the namespace in ETAIS, or a TA, to give you access.

---

## 11. UX and language rules

- **Plain B1 English.** Short sentences, one idea per line. Avoid jargon. When a technical word is needed (PATH, kubeconfig), add a few plain words around it.
- **Say what happens before it happens**, especially before the browser opens.
- **No raw stack traces or Go errors on screen.** Map them to codes and plain messages, and put details in the log.
- **Plain ASCII labels:** `[OK]`, `[FIXED]`, `[WARN]`, `[FAIL]`. These display correctly in old Windows 10 consoles.
- **Color is optional.** On Windows, enable VT processing; fall back to no color.
- **Typing:** the only input is one Enter press, plus the team name once if it's missing.
- **Finish fast** when nothing needs doing.

---

## 12. Engineering

### Suggested layout (adjust if you have a better one; record why in `docs/DECISIONS.md`)
```
cmd/kube-onramp/        main
internal/platform/      OS/arch/version detection, WSL detection, console handling
internal/catalog/       catalog loading and validation
internal/profile/       profile loading, team-name normalization
internal/inspect/       the five checks, PATH resolution for a new shell
internal/plan/          diff → actions (pure, table-tested)
internal/install/       download, verify, extract, atomic install
internal/pathenv/       Windows registry PATH and macOS rc-block editing (pure core + thin OS layer)
internal/kubeconfig/    backup, merge, atomic write
internal/login/         whoami, timeout, URL capture, failure classification
internal/access/        can-i checks
internal/report/        log, redaction, error report, clipboard
internal/state/         state.json
internal/ui/            output, step counter, labels, prompts
assets.go               package kubeonramp: go:embed of profiles/ and catalog/
profiles/  catalog/  scripts/install.ps1  scripts/install.sh  tools/update-catalog/
docs/SPEC.md  docs/DECISIONS.md  CLAUDE.md  README.md  LICENSE
```

### Tests
**Unit tests (table-driven):**
- team-name normalization;
- PATH string editing, covering `REG_EXPAND_SZ`, duplicates and very long PATHs;
- rc-block insert, update and remove;
- kubeconfig merge with golden files: an empty file, a course-style file, a file with other clusters, a broken UTF-16 file;
- plan computation from inspect results;
- redaction;
- catalog validation (every platform has a URL, a SHA-256 and a minimum OS).

**Test isolation:** `KUBE_ONRAMP_HOME`, a temporary `KUBECONFIG`, and on macOS a temporary `HOME` and `ZDOTDIR`, so tests never touch the developer's real setup. On Windows, the hidden variable `KUBE_ONRAMP_TEST_ENV_KEY` points PATH reads and writes at a throwaway key under `HKCU\Software\` instead of `HKCU\Environment`.

### CI (GitHub Actions)
**Checks:** `go vet`, `golangci-lint`, `go test ./...`, and `govulncheck`. Also run a Trivy filesystem scan of the repo, which ties in with the course's supply-chain topic.

**CI supply chain:** third-party actions are pinned to full commit SHAs. Trivy runs as a release binary with a pinned SHA-256, not through `trivy-action` (see DECISIONS.md). Tool versions used in CI are at least 7 days old when pinned.

**Smoke test** on `windows-latest` and on an Intel macOS runner. As of 2026-11-11 the Intel runners are `macos-15-intel` and `macos-26-intel`; there is no macOS 13 runner, so Machine B is the only macOS 13 test:
1. `setup --yes --minimal --skip-login --team ci-test`
2. Run it again and assert the plan was empty.
3. `check --skip-login`
4. `reset --yes`
5. Assert the PATH, rc files and kubeconfig are back to their original state.

### Release
GoReleaser on `v*` tags. Checksums are required; an SBOM is optional.

### `CLAUDE.md`
Record: the privacy rule, the "ask before" rules, how to run tests in isolation, the B1 language rule, and "update docs/SPEC.md and docs/DECISIONS.md when decisions change".

### README
Include:
- what the tool is;
- the one-liners;
- what it changes on your machine;
- how to undo it (`reset`);
- troubleshooting by error code;
- the supply-chain approach (pinned checksums);
- a disclaimer: "Student project. Not affiliated with or endorsed by the University of Tartu."

Also: "Built with Claude Code".

License: Apache-2.0, copyright "The kube-onramp Authors".

---

## 13. Milestones and acceptance tests

| Milestone | Target | Contents |
|---|---|---|
| M0: scaffold | Wed Nov 11 – Thu Nov 12 | Repo, CI, `docs/SPEC.md`, `CLAUDE.md`, profile (with verified CA data), catalog entries for kubectl and kubelogin with verified pins, Go toolchain pinned |
| M1: **v0.1.0** | **Sun Nov 15** | Core flow on Windows and macOS, `check`, `reset`, error reports, bootstrap scripts, GoReleaser release. Teammates test it on Nov 13–15, before Checkpoint 3 on Fri Nov 20 |
| M2: v0.2.0 | about Sun Nov 22 (Phase 4 starts Nov 23) | Extras (Helm 4, k9s, Trivy, kubeconform, load generator), `--minimal`, prod context checks, `update-catalog` |

### Acceptance tests for v0.1 (each must pass)
1. **Clean Windows (Sandbox):** after the one-liner and login, `kubectl get pods -n ns-<team>-staging` works in a **new** PowerShell window.
2. **Machine A setup** (Docker Desktop kubectl, winget kubelogin, course kubeconfig):
   - the tool's own copies win in a new terminal;
   - the old entries and files are untouched;
   - a backup exists;
   - login works.
3. **Machine B setup** (Homebrew copies in `/usr/local/bin`, kubeconfig mode 644, `config.broken`):
   - the tool's own copies win in a new zsh;
   - the kubeconfig is mode 600;
   - `config.broken` is untouched.
4. **Run twice:** the second run says "Everything is already set up", exits with code 0, and changes nothing.
5. **Interrupted run:** close the window during a download, then re-run. It completes.
6. **Double launch:** the second instance exits with code 3 and the "already running" message.
7. **Broken kubeconfig:** a UTF-16 or invalid YAML file is backed up and replaced, and the user is told where the backup is.
8. **Wrong team name:** gives `E-ACCESS-FORBIDDEN` and shows the namespace used.
9. **Login timeout:** closing the browser gives `E-LOGIN-TIMEOUT`, and the report is on the clipboard and redacted.
10. **Blocked network:** with github.com blocked, the run fails in preflight before any change.
11. **Reset:** `reset` removes only the tool's own changes, and `setup` afterwards works again.
12. **Port fallback:** with port 8000 busy, login falls back to 18000. With both busy, the user gets a clear message.

---

## 14. Resolved questions (2026-11-11)

1. **Repo owner:** the maintainer's personal GitHub account (no organization). Its handle may appear only in the Go module path, the README one-liners and the GoReleaser config. Commits use the account's noreply email.
2. **Dev tools:** Go, GoReleaser and golangci-lint are installed with winget; govulncheck with `go install`. CI pins its own versions.
3. **Clean-machine test:** Windows Sandbox (Windows 11). A `.wsb` file maps a folder with test builds into the Sandbox read-only. Machine A covers Windows 10.
4. **License:** Apache-2.0, copyright "The kube-onramp Authors".
5. **Testers:** a Windows teammate runs acceptance test 2 (Machine A); a Mac teammate runs test 3 (Machine B). Both start from `v0.1.0-rc.1`.
6. **Verified profile data:** the CA data on the course page matches a working course kubeconfig (SHA-256 fingerprint `20:B2:2B:5E:…:4D:90:5E`, `CN=kubernetes`, valid until 2035-09-07). The server answers anonymous `GET /version`.
