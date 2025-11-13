#!/usr/bin/env bash
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required but not found in PATH" >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

if [[ ! -f go.mod ]]; then
  echo "go.mod not found in ${ROOT_DIR}" >&2
  exit 1
fi

if [[ ! -f .github/workflows/golang-ci-lint.yml ]]; then
  echo ".github/workflows/golang-ci-lint.yml not found in ${ROOT_DIR}" >&2
  exit 1
fi

GO_VERSION_LINE="$(grep -E '^go [0-9]+\.[0-9]+' go.mod | head -n1 || true)"
if [[ -z "${GO_VERSION_LINE}" ]]; then
  echo "Unable to determine Go version from go.mod" >&2
  exit 1
fi
GO_VERSION="$(awk '{print $2}' <<<"${GO_VERSION_LINE}")"

GCI_VERSION_LINE="$(grep -E 'GCI_VERSION=' .github/workflows/golang-ci-lint.yml | head -n1 || true)"
if [[ -z "${GCI_VERSION_LINE}" ]]; then
  echo "Unable to determine golangci-lint version from workflow file" >&2
  exit 1
fi
GCI_VERSION="$(awk -F= '{print $2}' <<<"${GCI_VERSION_LINE}" | tr -d ' \"')"

echo " 🚚 Running golangci-lint v${GCI_VERSION} in Docker..."
docker run --rm --name "Golanci-lint_${GCI_VERSION}" -e GOFLAGS=-mod=readonly \
  -v "${ROOT_DIR}:/app" \
  -w /app \
  "golangci/golangci-lint:v${GCI_VERSION}" \
  golangci-lint run
echo " ✅ golangci-lint succeeded"

echo " 🚚 Running go test ./... using golang:${GO_VERSION} Docker image..."
docker run --rm --name "Go_Tests_${GO_VERSION}" -e GOFLAGS=-mod=readonly \
  -v "${ROOT_DIR}:/app" \
  -w /app \
  "golang:${GO_VERSION}" \
  bash -c "go test ./..."
echo " ✅ Test succeeded"