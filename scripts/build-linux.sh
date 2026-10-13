#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
arch="${1:-amd64}"
case "$arch" in amd64|arm64) ;; *) echo 'Use amd64 or arm64' >&2; exit 1 ;; esac
if [[ $# -gt 2 || (${2:-} != '' && ${2:-} != --skip-web-build) ]]; then
  echo 'Usage: build-linux.sh [amd64|arm64] [--skip-web-build]' >&2
  exit 1
fi
cd "$root"
version=$(sed -n 's/^version=\([0-9][0-9.]*\)$/\1/p' packaging/fnos/netlite/manifest)
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid manifest version' >&2; exit 1; }
caddy="$root/dist/caddy-linux-$arch"
[[ -f "$caddy" ]] || { echo "Build Caddy first: bash scripts/build-caddy.sh linux $arch" >&2; exit 1; }
# Check the input's ELF machine before shipping it in an architecture-specific archive.
machine=$(od -An -t u1 -j 18 -N 2 "$caddy" | xargs)
magic=$(od -An -t u1 -N 4 "$caddy" | xargs)
expected=62; [[ "$arch" == arm64 ]] && expected=183
[[ "$magic" == '127 69 76 70' && "$machine" == "$expected 0" ]] || { echo 'Caddy ELF architecture mismatch' >&2; exit 1; }
if [[ ${2:-} == --skip-web-build ]]; then
  [[ -f web/dist/index.html ]] || { echo 'Build the frontend before using --skip-web-build' >&2; exit 1; }
else
  npm --prefix web ci
  npm --prefix web run build
fi
# Create a clean staging directory so previous builds cannot leak files into a package.
stage=$(mktemp -d "$root/dist/linux-$arch-XXXXXX")
cleanup() {
  # Only remove the fresh directory created above, within this repository's dist.
  [[ "$stage" == "$root/dist/linux-$arch-"* ]] && rm -rf -- "$stage"
}
trap cleanup EXIT
bundle="$stage/bundle"
mkdir -p "$bundle/bin" "$bundle/web" "$bundle/deploy" "$bundle/licenses"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags '-s -w' -o "$bundle/bin/netlite" ./cmd/netlite
cp "$caddy" "$bundle/bin/caddy"
cp -R web/dist/. "$bundle/web/"
cp deploy/netlite.service "$bundle/deploy/"
cp docs/BUILD.md "$bundle/README.md"
cp THIRD_PARTY_NOTICES.md docs/licenses/acmez-LICENSE.txt "$bundle/licenses/"
printf 'version=%s\nplatform=linux\narch=%s\ncommit=%s\n' "$version" "$arch" "$(git rev-parse HEAD)" > "$bundle/BUILD-INFO"
find "$bundle" -type d -exec chmod 755 {} +
find "$bundle" -type f -exec chmod 644 {} +
output="$root/dist/netlite-$version-linux-$arch.tar.gz"
# Explicit archive modes also preserve executable bits when building with Git Bash
# on a Windows filesystem that cannot represent Unix executable permissions.
tar --owner=0 --group=0 --numeric-owner --mode='u=rwX,go=rX' \
  --exclude='./bin/netlite' --exclude='./bin/caddy' -cf "$stage/package.tar" -C "$bundle" .
tar --owner=0 --group=0 --numeric-owner --mode=755 \
  -rf "$stage/package.tar" -C "$bundle" ./bin/netlite ./bin/caddy
gzip -n -c "$stage/package.tar" > "$output"
tar -tzf "$output" ./bin/netlite ./bin/caddy ./web/index.html ./deploy/netlite.service \
  ./licenses/THIRD_PARTY_NOTICES.md ./licenses/acmez-LICENSE.txt ./BUILD-INFO >/dev/null
echo "Linux: $output"
