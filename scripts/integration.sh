#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
out=${1:?caller-owned output directory is required}
mkdir -p "$out"
go run ./cmd/gooo-semantic-dialect-migrator migrate \
  -root "$repo_root" \
  -meta .gooo/migrator.gooo \
  -case fixtures/cases/symbol-rename-origin.json \
  -output-dir "$out/symbol-rename-origin" >/dev/null

test -s "$out/symbol-rename-origin/migrated.gooo"
test -s "$out/symbol-rename-origin/source.semantic-ir.json"
test -s "$out/symbol-rename-origin/target.semantic-ir.json"
test -s "$out/symbol-rename-origin/source.gooo.go"
test -s "$out/symbol-rename-origin/target.gooo.go"
grep -q 'dialect=v2' "$out/symbol-rename-origin/migrated.gooo"
grep -q '"decision": "CLOSED"' "$out/symbol-rename-origin/case-report.json"
