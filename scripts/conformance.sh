#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
out=${1:?caller-owned output directory is required}
mkdir -p "$out"
go run ./cmd/gooo-semantic-dialect-migrator conformance \
  -root "$repo_root" \
  -meta .gooo/migrator.gooo \
  -output-dir "$out"
