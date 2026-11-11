#!/bin/sh
# kube-onramp bootstrap for macOS (and Linux, best effort).
# It downloads kube-onramp, checks its SHA-256 against checksums.txt, puts it
# into ~/.kube-onramp/bin and starts it.
#
# Use (one line):
#   curl -fsSL https://github.com/Sten-Qy-Li/kube-onramp/releases/latest/download/install.sh | sh -s -- --team <team>
#
# All arguments go to kube-onramp. Files fetched by curl get no quarantine
# attribute, so Gatekeeper does not block them. No sudo is used.
set -eu

REPO="Sten-Qy-Li/kube-onramp"
# The release workflow writes the release tag here (docs/DECISIONS.md, D14).
TAG="__KUBE_ONRAMP_TAG__"

KOR_HOME="${KUBE_ONRAMP_HOME:-$HOME/.kube-onramp}"

os_name() {
	if [ "$(uname -s)" = Darwin ]; then
		echo "macOS $(sw_vers -productVersion 2>/dev/null || echo unknown) ($(uname -m))"
	else
		echo "$(uname -s) $(uname -r) ($(uname -m))"
	fi
}

# Print the standard error report, copy it to the clipboard, save it, stop.
fail() {
	code=$1 message=$2 next=$3 detail=${4:-}
	text=$(printf '%s\n' \
		'=== kube-onramp error report (paste this into an AI chatbot) ===' \
		'kube-onramp installs kubectl and kubelogin (int128) and sets up OIDC access to a Kubernetes cluster.' \
		'Trying to: download and start kube-onramp (install.sh)' \
		"Error code: $code" \
		"Message: $message" \
		"Tool: install.sh $TAG | OS: $(os_name) | $(basename "${SHELL:-sh}") | kubectl unknown | kubelogin unknown" \
		'--- details (personal data removed) ---' \
		"$detail" \
		'=== end of report ===')
	text=$(printf '%s\n' "$text" | sed "s|$HOME|~|g")
	if [ -n "${USER:-}" ] && [ "${#USER}" -ge 3 ]; then
		text=$(printf '%s\n' "$text" | sed "s|$USER|<user>|g")
	fi
	printf '\n  %s: %s\n  %s\n\n' "$code" "$message" "$next" >&2
	copied=no
	if command -v pbcopy >/dev/null 2>&1 && printf '%s\n' "$text" | pbcopy 2>/dev/null; then
		copied=yes
		echo "  The report is copied. Paste it into Slack or into an AI chatbot and ask for help." >&2
	fi
	if mkdir -p "$KOR_HOME/logs" 2>/dev/null; then
		file="$KOR_HOME/logs/report-$(date +%Y%m%d-%H%M%S).txt"
		printf '%s\n' "$text" >"$file" && echo "  The report is also saved here: $file" >&2
	elif [ "$copied" = no ]; then
		printf '%s\n' "$text" >&2
	fi
	exit 1
}

# 1. Check the system.
case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail E-BOOT-OS "kube-onramp does not support this system: $(uname -s)." "Use macOS 13 or newer, or Windows." ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail E-BOOT-OS "kube-onramp does not support this processor: $(uname -m)." "Use another computer, or ask a TA for help." ;;
esac
# A shell running under Rosetta on an Apple silicon Mac says x86_64; use the native build.
if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
	arch=arm64
fi
if [ "$os" = linux ] && [ "$arch" != amd64 ]; then
	fail E-BOOT-OS "kube-onramp has no build for Linux on $(uname -m)." "Use macOS or Windows."
fi

# 2. Choose the release.
if [ -n "${KUBE_ONRAMP_VERSION:-}" ]; then
	base="https://github.com/$REPO/releases/download/$KUBE_ONRAMP_VERSION"
else
	case "$TAG" in
	__*) base="https://github.com/$REPO/releases/latest/download" ;;
	*) base="https://github.com/$REPO/releases/download/$TAG" ;;
	esac
fi
asset="kube-onramp_${os}_${arch}"

# 3. Download kube-onramp and checksums.txt.
echo "Downloading kube-onramp..."
mkdir -p "$KOR_HOME/bin" "$KOR_HOME/tmp" || fail E-BOOT-INSTALL "kube-onramp could not make its folder $KOR_HOME." "Check that your home folder is writable, or ask a TA."
tmp="$KOR_HOME/tmp"
curl -fsSL --retry 3 -o "$tmp/checksums.txt" "$base/checksums.txt" ||
	fail E-BOOT-DL "kube-onramp could not be downloaded." "Check that you are online, then paste the install line again." "$base/checksums.txt"
curl -fsSL --retry 3 -o "$tmp/$asset.part" "$base/$asset" ||
	fail E-BOOT-DL "kube-onramp could not be downloaded." "Check that you are online, then paste the install line again." "$base/$asset"

# 4. Check the SHA-256.
want=$(awk -v a="$asset" '$2 == a || $2 == "*" a { print $1 }' "$tmp/checksums.txt")
if command -v shasum >/dev/null 2>&1; then
	got=$(shasum -a 256 "$tmp/$asset.part" | awk '{ print $1 }')
else
	got=$(sha256sum "$tmp/$asset.part" | awk '{ print $1 }')
fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then
	rm -f "$tmp/$asset.part"
	fail E-BOOT-SHA "The downloaded file is not the expected file (the checksum is wrong). It was deleted." \
		"Paste the install line again. If it happens again, send the report to a TA." "want=$want got=$got $base/$asset"
fi

# 5. Put it into place in one step.
chmod 755 "$tmp/$asset.part"
mv -f "$tmp/$asset.part" "$KOR_HOME/bin/kube-onramp" ||
	fail E-BOOT-INSTALL "kube-onramp could not be saved in its folder." "Paste the install line again, or ask a TA."

# 6. Start it. Under "curl | sh" stdin is the download, so the Enter key is
# read from the terminal instead, when there is one.
set +e
if [ -t 1 ] && (exec </dev/tty) 2>/dev/null; then
	"$KOR_HOME/bin/kube-onramp" "$@" </dev/tty
else
	"$KOR_HOME/bin/kube-onramp" "$@"
fi
exit $?
