#!/usr/bin/env bash
# Mirror the latest relay-vX.Y.Z GitHub Release client binaries into
# /opt/9router/manage/relay/download (served at manage.lampt.works/relay/download/),
# verifying every file against the release's SHA256SUMS.txt.
set -euo pipefail
dst=/opt/9router/manage/relay/download
repo=lampt24/9router
tag=$(curl -fsS "https://api.github.com/repos/$repo/releases?per_page=30" | python3 -I -c '
import json, sys
r = [x for x in json.load(sys.stdin) if x["tag_name"].startswith("relay-v") and not x["draft"] and not x["prerelease"]]
print(r[0]["tag_name"] if r else "")')
[[ -n "$tag" ]] || exit 0
[[ "$(cat "$dst/.version" 2>/dev/null)" == "$tag" ]] && exit 0
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
base="https://github.com/$repo/releases/download/$tag"
curl -fsSL -o "$tmp/SHA256SUMS.txt" "$base/SHA256SUMS.txt"
files=(9r-relay-windows-amd64.exe 9r-relay-windows-arm64.exe)
for f in "${files[@]}"; do curl -fsSL -o "$tmp/$f" "$base/$f"; done
(cd "$tmp" && grep -E " (9r-relay-windows-(amd64|arm64)\.exe)$" SHA256SUMS.txt | sha256sum -c --quiet -)
install -m 644 "$tmp"/SHA256SUMS.txt "${files[@]/#/$tmp/}" "$dst/"
echo "$tag" > "$dst/.version"
echo "relay client $tag mirrored to $dst"
