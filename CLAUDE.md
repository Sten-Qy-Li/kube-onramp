# CLAUDE.md

Context for AI coding sessions on `kube-onramp`. Read `docs/SPEC.md` (what to build) and `docs/DECISIONS.md` (why it is built this way) before changing behavior.

**Start here:** `docs/OPEN-ITEMS.md` lists what is left to do (v0.1.0 is released). Keep it up to date: tick off finished items and add new ones.

## Hard rules

### Privacy (the repo is public)
Never commit:
- personal names, emails, OS usernames or home-directory paths;
- team names or real namespace names (use `<team>`, `best-team`, `ci-test` in docs, tests and fixtures);
- tokens, or kubeconfig content other than the public cluster info.

Allowed: the repo owner's GitHub handle, but only in the Go module path (and import paths), the README one-liners and the GoReleaser config; the cluster URL, OIDC issuer, client ID and cluster CA (they are on a public course page).
The maintainer's real team name is used only at runtime (command line, temp folders), never in files that get committed.
Commits use the repo-local git identity (GitHub handle + GitHub noreply email). Don't change it. Commit messages end with the `Co-Authored-By: Claude …` trailer.

### Ask before
- final release tags (`v0.1.0`, `v0.2.0`, ...), changing GitHub repo settings, force-pushing, deleting anything (branches, tags, releases, files outside the scratch area);
- installing anything on the maintainer's machine;
- running `kube-onramp` in a way that changes the real PATH, shell rc files or `~/.kube/config`.

Standing OK (no need to ask): push to `main`, and push `-rc` pre-release tags once CI is green.

### Don't guess facts
Versions, download URLs, release asset names, checksums and CLI flags must be checked upstream before they are pinned. Record what was checked and when in `docs/DECISIONS.md`. Don't execute downloaded third-party binaries to learn their output; read the upstream source, or use `go version <file>` (reads build info without running it).

## Keep the docs current
When a decision changes behavior, update `docs/SPEC.md` and add a short entry to `docs/DECISIONS.md` (what and why).

## Language for users
All text shown to users (messages, errors, prompts, README sections for students) is plain English at CEFR level B1: short sentences, one idea per line, explain technical words. Labels are ASCII: `[OK]`, `[FIXED]`, `[WARN]`, `[FAIL]`. No stack traces or raw Go errors on screen.

## Build and test

```sh
go build ./...
go test ./...
go vet ./...
golangci-lint run
govulncheck ./...
goreleaser release --snapshot --clean   # local release dry run, output in dist/
```

Cross-compile check: `CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build ./cmd/kube-onramp` (also windows/amd64, darwin/arm64, linux/amd64).

### Running the tool locally without touching the real setup
Always isolate:

PowerShell:
```powershell
$env:KUBE_ONRAMP_HOME = "$env:TEMP\kor-home"
$env:KUBECONFIG = "$env:TEMP\kor-home\kubeconfig"
$env:KUBE_ONRAMP_TEST_ENV_KEY = 'kube-onramp-test'   # PATH edits go to HKCU\Software\kube-onramp-test
go run ./cmd/kube-onramp --yes --skip-login --team ci-test
```

macOS/Linux:
```sh
export HOME="$(mktemp -d)" ZDOTDIR="$HOME" KUBE_ONRAMP_HOME="$HOME/.kube-onramp" KUBECONFIG="$HOME/.kube/config"
go run ./cmd/kube-onramp --yes --skip-login --team ci-test
```

Tests must do the same through `t.TempDir()` and `t.Setenv`; a test must never read or write the developer's real home, PATH or kubeconfig.

### End-to-end smoke tests (same as CI)
```powershell
go build -o $env:TEMP\kor\kube-onramp.exe ./cmd/kube-onramp
./scripts/smoke-test.ps1 -Binary $env:TEMP\kor\kube-onramp.exe   # uses a throwaway registry key unless CI=true
```
```sh
go build -o /tmp/kor/kube-onramp ./cmd/kube-onramp && sh scripts/smoke-test.sh /tmp/kor/kube-onramp
```
They download the pinned kubectl and kubelogin (about 70 MB). The real browser login needs a person: run with a temporary `KUBE_ONRAMP_HOME` and `KUBECONFIG`, without `--skip-login`, and ask the maintainer to log in.

### Release
`scripts/bake-tag.sh <tag>` writes the tag into `build/scripts/install.*`; the release workflow does this before GoReleaser. `-rc` tags become GitHub pre-releases.

## Conventions
- Pure Go, `CGO_ENABLED=0`. No UPX.
- Pure logic (plan, PATH string editing, rc blocks, kubeconfig merge, redaction) lives in functions without I/O and gets table-driven tests. OS calls stay in thin wrappers.
- Adding a tool or a cluster should mean editing `catalog/tools.yaml` or `profiles/*.yaml`, not code.
- GitHub Actions are pinned to full commit SHAs. New CI tool versions must be at least 7 days old (D17).
