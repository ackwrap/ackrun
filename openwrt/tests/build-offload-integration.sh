#!/bin/sh
# Compile on a development host; the guest does not need Go or the repository.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
out=${1:-"$root/dist/openwrt-integration"}
mkdir -p "$out"
out=$(CDPATH= cd -- "$out" && pwd)
cd "$root/backend"
CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH:-amd64} go test -c -o "$out/service.test" ./internal/service
cp "$root/openwrt/tests/run-offload-integration.sh" "$out/"
cp "$root/openwrt/tests/README.md" "$out/"
git -C "$root" rev-parse HEAD > "$out/source-commit.txt"
git -C "$root" diff --binary HEAD -- backend/internal/service openwrt/tests > "$out/source.patch"
(cd "$out" && sha256sum service.test run-offload-integration.sh > SHA256SUMS)
echo "Bundle: $out"
