#!/bin/sh
# End-to-end smoke test on macOS and Linux (docs/SPEC.md, section 12):
# setup, setup again (nothing may change), check, reset, then everything
# must be back as it was. It uses a temporary HOME and ZDOTDIR, so the real
# rc files and kubeconfig are never touched.
#
# Usage: scripts/smoke-test.sh path/to/kube-onramp
set -eu

bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
export HOME="$work/home" ZDOTDIR="$work/home"
export KUBE_ONRAMP_HOME="$HOME/.kube-onramp" KUBECONFIG="$HOME/.kube/config"
mkdir -p "$HOME/.kube"

fixture="$repo/internal/kubeconfig/testdata/other.canonical.yaml"
cp "$fixture" "$KUBECONFIG"
chmod 644 "$KUBECONFIG" # Machine B had a world-readable kubeconfig

if [ "$(uname -s)" = Darwin ]; then
	export SHELL=/bin/zsh
	rc="$ZDOTDIR/.zshrc"
else
	export SHELL=/bin/bash
	rc="$HOME/.bashrc"
fi
# An rc file that puts Homebrew first, like on Machine B.
printf '%s\n' 'export EDITOR=vi' 'export PATH="/usr/local/bin:$PATH"' >"$rc"
cp "$rc" "$work/rc.orig"

fail() {
	echo "SMOKE TEST FAILED: $*" >&2
	exit 1
}
run() {
	code=0
	out=$("$@" 2>&1) || code=$?
	printf '%s\n' "$out"
}
mtime() { stat -c %Y "$1" 2>/dev/null || stat -f %m "$1"; }
mode() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }
snapshot() {
	for f in "$KUBE_ONRAMP_HOME"/bin/* "$KUBE_ONRAMP_HOME/state.json" "$KUBECONFIG" "$rc"; do
		printf '%s %s %s\n' "$f" "$(mtime "$f")" "$(cksum <"$f")"
	done
}

echo "== 1. setup"
run "$bin" --yes --minimal --skip-login --no-color --team ci-test
[ "$code" = 0 ] || fail "setup exit code $code"
own="$KUBE_ONRAMP_HOME/bin/kube-onramp"
[ -x "$own" ] || fail "kube-onramp was not copied into its bin folder"
grep -q '^# >>> kube-onramp >>>$' "$rc" || fail "the rc block is missing"
[ "$(mode "$KUBECONFIG")" = 600 ] || fail "kubeconfig mode is $(mode "$KUBECONFIG"), want 600"
printf '%s\n' "$out" | grep -q "A new terminal runs kube-onramp's kubectl$" || fail "kube-onramp's kubectl does not win in a new shell"

echo "== 2. setup again: nothing may change"
before=$(snapshot)
sleep 1
run "$bin" --yes --minimal --skip-login --no-color
[ "$code" = 0 ] || fail "second setup exit code $code"
printf '%s\n' "$out" | grep -q 'Everything is already set up' || fail 'the second run did not say "Everything is already set up"'
[ "$before" = "$(snapshot)" ] || fail "the second run changed files"

echo "== 3. check"
run "$own" check --skip-login --no-color
[ "$code" = 0 ] || fail "check exit code $code"

echo "== 4. reset, run by the installed copy"
run "$own" reset --yes --no-color
[ "$code" = 0 ] || fail "reset exit code $code"

echo "== 5. everything is back"
cmp -s "$rc" "$work/rc.orig" || fail "the rc file is not back to the original"
cmp -s "$KUBECONFIG" "$fixture" || fail "the kubeconfig is not back to the original"
[ ! -e "$KUBE_ONRAMP_HOME/bin" ] || fail "the bin folder still exists"
[ ! -e "$KUBE_ONRAMP_HOME/state.json" ] || fail "state.json still exists"
[ -d "$KUBE_ONRAMP_HOME/backups" ] || fail "the backups folder is gone"
echo "SMOKE TEST PASSED"
