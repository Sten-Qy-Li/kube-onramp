# kube-onramp

One command that gets your laptop ready for Kubernetes work on a shared cluster. The first supported cluster is the University of Tartu DevOps course cluster (fall 2026).

> **Status: not released yet.** v0.1.0 is planned for 15 November 2026. The commands below work after that.

## What it does

- It installs `kubectl` and `kubelogin` (the int128 version). It uses its own copies in its own folder.
- It adds the course cluster to your Kubernetes settings file (the "kubeconfig"). It makes a backup first.
- It logs you in with your University of Tartu account in the browser.
- It checks that you can see your team's namespace.

You can run it again at any time. If everything is already fine, it changes nothing.

## Start

Replace `<team>` with your team name, for example `best-team`.

**Windows** (open PowerShell and paste this one line):

```powershell
[Net.ServicePointManager]::SecurityProtocol = 'Tls12'; & ([scriptblock]::Create((irm https://github.com/Sten-Qy-Li/kube-onramp/releases/latest/download/install.ps1))) -Team <team>
```

**macOS** (open Terminal and paste this one line):

```sh
curl -fsSL https://github.com/Sten-Qy-Li/kube-onramp/releases/latest/download/install.sh | sh -s -- --team <team>
```

Then follow the text on the screen. You only need to press Enter once and log in once.

**Other way (Windows):** download `kube-onramp_windows_amd64.exe` from the [Releases page](https://github.com/Sten-Qy-Li/kube-onramp/releases) and double-click it. Windows may show a blue "Windows protected your PC" window. Click **More info**, then **Run anyway**.

## Before a presentation

Open a terminal and run:

```
kube-onramp check
```

It checks your login and your access. If your login is old, the browser opens so you can log in again. It does not install anything.

## What it changes on your computer

| | Windows | macOS |
|---|---|---|
| Its own folder | `%LOCALAPPDATA%\kube-onramp\` | `~/.kube-onramp/` |
| Programs | `kubectl`, `kubectl-oidc_login`, `kube-onramp` in `bin\` of its folder | the same, in `bin/` |
| PATH (where the terminal looks for programs) | adds its `bin` folder to the start of your user PATH | adds a marked block to `~/.zshrc` (or `~/.bash_profile`) |
| Kubernetes settings | adds the cluster `ut-devops`, the user `ut-devops-oidc` and the contexts `ut-devops-staging` and `ut-devops-prod` to `~/.kube/config` | the same, and sets the file to mode 600 |
| Backups | your old `~/.kube/config` is copied to `backups\` in its folder | the same |

It does **not** change or remove other programs, for example a `kubectl` from Docker Desktop, winget or Homebrew. It does not need admin rights.

## How to undo it

```
kube-onramp reset
```

This removes only what kube-onramp added: its folder (except backups), its PATH entry, and its entries in your Kubernetes settings. It puts your old current context back.

## Problems

When something goes wrong, kube-onramp shows an error code, copies a short report to your clipboard, and saves it in its `logs` folder. Personal data (your user name, tokens, emails) is removed from the report. Paste it into Slack or into an AI chatbot and ask for help.

| Code | What it means |
|---|---|
| `E-PRE-*` | Your computer is not ready (OS, disk space, network, login ports). Nothing was changed. |
| `E-BOOT-*` | The install script could not download or start kube-onramp. |
| `E-DL`, `E-SHA`, `E-EXTRACT` | A download failed or was not the expected file. |
| `E-PATH` | PATH could not be updated. |
| `E-KCFG` | The Kubernetes settings file could not be read or written. |
| `E-LOGIN-TIMEOUT`, `E-LOGIN-REJECTED` | The browser login did not finish, or the cluster did not accept it. |
| `E-CLUSTER-UNREACHABLE` | The cluster cannot be reached from your network. |
| `E-ACCESS-FORBIDDEN` | You are logged in, but you cannot see your team's namespace. Check your team name first. |
| `W-SHADOW` | Another `kubectl` comes first on your PATH (often Docker Desktop's). |

More details for each code will be added with v0.1.0.

## Supply chain

kube-onramp does not install "whatever is newest" at install time. Every tool has a pinned version and a pinned SHA-256 checksum in [`catalog/tools.yaml`](catalog/tools.yaml). If a download does not match, nothing is installed.

- Each checksum was checked against the upstream checksum file and by hashing the downloaded file.
- The release binaries of kube-onramp have a `checksums.txt`. The install scripts check it.
- CI pins every GitHub Action to a commit SHA, and runs `govulncheck` and a Trivy scan. Trivy runs as a checksum-pinned binary, because the `trivy-action` tags were hijacked in March 2026 ([GHSA-69fq-xp46-6x23](https://github.com/aquasecurity/trivy/security/advisories/GHSA-69fq-xp46-6x23)).

## For developers

- [`docs/SPEC.md`](docs/SPEC.md): what the tool does.
- [`docs/DECISIONS.md`](docs/DECISIONS.md): why it works this way.
- [`CLAUDE.md`](CLAUDE.md): conventions, and how to run it without touching your own setup.

```sh
go test ./...
golangci-lint run
goreleaser release --snapshot --clean
```

Adding a tool or a cluster means editing `catalog/tools.yaml` or `profiles/*.yaml`.

## Notes

Student project. Not affiliated with or endorsed by the University of Tartu.

Built with [Claude Code](https://claude.com/claude-code).

Copyright 2026 The kube-onramp Authors. Licensed under the [Apache License 2.0](LICENSE).
