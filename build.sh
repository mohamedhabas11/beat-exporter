#!/bin/bash
set -euo pipefail

if [[ -z "${GITHUB_WORKSPACE:-}" ]]; then
  GITHUB_WORKSPACE=$(pwd)
  echo "Setting up GITHUB_WORKSPACE to current directory: ${GITHUB_WORKSPACE}"
fi

GITHUB_ACTOR=${GITHUB_ACTOR:-$(whoami)}

GITVERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
GITBRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
GITREVISION=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
TIME=$(date -u +%FT%T%z)

LDFLAGS="-s -w -X github.com/prometheus/common/version.Version=${GITVERSION} \
-X github.com/prometheus/common/version.Revision=${GITREVISION} \
-X github.com/prometheus/common/version.Branch=${GITBRANCH} \
-X github.com/prometheus/common/version.BuildUser=${GITHUB_ACTOR} \
-X github.com/prometheus/common/version.BuildDate=${TIME}"

PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "linux/386"
  "darwin/amd64"
  "darwin/arm64"
)

for PLATFORM in "${PLATFORMS[@]}"; do
  OS=${PLATFORM%/*}
  ARCH=${PLATFORM#*/}
  EXT=""
  [[ "$OS" == "windows" ]] && EXT=".exe"
  echo "Building ${OS}/${ARCH} with version: ${GITVERSION}, revision: ${GITREVISION}, branch: ${GITBRANCH}, buildUser: ${GITHUB_ACTOR}"
  CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} go build -trimpath -ldflags "${LDFLAGS}" -o ".build/${OS}-${ARCH}/beat-exporter${EXT}" .
done

echo "Build complete. Artifacts in .build/"
