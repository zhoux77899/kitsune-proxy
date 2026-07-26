#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -ne 2 ]]; then
  echo "usage: $0 <version> <output-directory>" >&2
  exit 2
fi

version="$1"
output_directory="$2"
repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ "${output_directory}" != /* ]]; then
  output_directory="${repo_root}/${output_directory}"
fi

mkdir -p "${output_directory}"
staging_directory="$(mktemp -d)"
trap 'rm -rf "${staging_directory}"' EXIT

cd "${repo_root}"
portable_directory="${staging_directory}/portable"
mkdir -p "${portable_directory}"

go build \
  -trimpath \
  -ldflags "-s -w" \
  -o "${portable_directory}/kitsune-proxy" \
  ./cmd/kitsune-proxy
chmod 755 "${portable_directory}/kitsune-proxy"
cp README.md LICENSE "${portable_directory}/"
tar \
  -C "${portable_directory}" \
  -czf "${output_directory}/kitsune-proxy-linux-amd64.tar.gz" \
  .

PACKAGE_VERSION="${version}" \
PACKAGE_BINARY="${portable_directory}/kitsune-proxy" \
  go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.47.0 package \
    --config packaging/linux/nfpm.yaml \
    --packager deb \
    --target "${output_directory}/kitsune-proxy-linux-amd64.deb"

PACKAGE_VERSION="${version}" \
PACKAGE_BINARY="${portable_directory}/kitsune-proxy" \
  go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.47.0 package \
    --config packaging/linux/nfpm.yaml \
    --packager rpm \
    --target "${output_directory}/kitsune-proxy-linux-amd64.rpm"
