#!/bin/sh
# Writes build/scripts/install.{ps1,sh} with the release tag baked in, so a
# release's install scripts install exactly that release (docs/DECISIONS.md, D14).
# Usage: scripts/bake-tag.sh v0.1.0-rc.1
set -eu
tag=${1:?usage: bake-tag.sh <tag>}
case "$tag" in
*[!A-Za-z0-9._-]*) echo "bad tag: $tag" >&2; exit 2 ;;
esac
mkdir -p build/scripts
for f in install.ps1 install.sh; do
	sed "s/__KUBE_ONRAMP_TAG__/$tag/g" "scripts/$f" >"build/scripts/$f"
	grep -q "$tag" "build/scripts/$f" || { echo "tag not written into $f" >&2; exit 1; }
done
chmod 755 build/scripts/install.sh
echo "baked $tag into build/scripts"
