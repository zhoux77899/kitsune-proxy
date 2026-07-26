#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -ne 3 ]]; then
  echo "usage: $0 <version> <architecture> <output-directory>" >&2
  exit 2
fi

version="$1"
architecture="$2"
output_directory="$3"
case "${architecture}" in
  amd64 | arm64) ;;
  *)
    echo "unsupported macOS architecture: ${architecture}" >&2
    exit 2
    ;;
esac

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ "${output_directory}" != /* ]]; then
  output_directory="${repo_root}/${output_directory}"
fi
mkdir -p "${output_directory}"

actual_architecture="$(go env GOARCH)"
if [[ "${actual_architecture}" != "${architecture}" ]]; then
  echo "runner architecture is ${actual_architecture}, expected ${architecture}" >&2
  exit 1
fi

staging_directory="$(mktemp -d)"
trap 'rm -rf "${staging_directory}"' EXIT
bundle="${staging_directory}/Kitsune Proxy.app"
dmg_root="${staging_directory}/dmg"
mkdir -p "${bundle}/Contents/MacOS" "${bundle}/Contents/Resources" "${dmg_root}"

cd "${repo_root}"
go build \
  -trimpath \
  -ldflags "-s -w" \
  -o "${bundle}/Contents/MacOS/kitsune-proxy" \
  ./cmd/kitsune-proxy
cp packaging/macos/Info.plist "${bundle}/Contents/Info.plist"
cp assets/generated/kitsune.icns "${bundle}/Contents/Resources/kitsune.icns"
chmod 755 "${bundle}/Contents/MacOS/kitsune-proxy"
plutil -replace CFBundleShortVersionString -string "${version}" "${bundle}/Contents/Info.plist"
plutil -lint "${bundle}/Contents/Info.plist"

ditto \
  -c \
  -k \
  --sequesterRsrc \
  --keepParent \
  "${bundle}" \
  "${output_directory}/kitsune-proxy-macos-${architecture}.zip"

ditto "${bundle}" "${dmg_root}/Kitsune Proxy.app"
ln -s /Applications "${dmg_root}/Applications"
hdiutil create \
  -quiet \
  -ov \
  -format UDZO \
  -volname "Kitsune Proxy" \
  -srcfolder "${dmg_root}" \
  "${output_directory}/kitsune-proxy-macos-${architecture}.dmg"
hdiutil verify -quiet "${output_directory}/kitsune-proxy-macos-${architecture}.dmg"
