#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
target_os="${1:-linux}"
target_arch="${2:-$(go env GOARCH)}"
case "$target_os/$target_arch" in linux/amd64|linux/arm64|windows/amd64|windows/arm64) ;; *) echo 'Use linux/windows and amd64/arm64' >&2; exit 1 ;; esac
mkdir -p "$root/dist" "$root/tools"
stage="$(mktemp -d "$root/tools/caddy-build-XXXXXX")"
trap 'rm -rf "$stage"' EXIT
cd "$stage"
go mod init netlite-caddy-build
go get github.com/caddyserver/caddy/v2/cmd/caddy@v2.11.7
output="$root/dist/caddy-$target_os-$target_arch"
if [[ "$target_os" == windows ]]; then output+='.exe'; fi
CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags '-s -w' -o "$output" github.com/caddyserver/caddy/v2/cmd/caddy
echo "Standard Caddy: $output"
