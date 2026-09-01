#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
meta="$repo_root/.gooo/migrator.gooo"

test "$(grep -c '^operation ' "$meta")" -eq 7
test "$(grep -c '^case ' "$meta")" -eq 8
test "$(grep -c '^predicate ' "$meta")" -eq 7
grep -q '^authority metacode$' "$meta"
grep -q '^denominator id=semantic-dialect-migrator-v1 cases=8 unit=fixed-case$' "$meta"
grep -q '^precedence REFUTED>UNKNOWN>CLOSED$' "$meta"
grep -q '^unknown_fields stage,step,reason,unknown_class,next_operation,blocked_by$' "$meta"
grep -q '^source_policy input_repository_writes=0 outputs=caller_owned_only overwrite_source=never$' "$meta"
grep -q 'README.md' "$repo_root/README.md"

if grep -nE 'git (commit|merge|push|reset|checkout)|gh (pr merge|release delete)' "$repo_root/scripts"/*.sh "$repo_root/cmd"/*/*.go; then
  echo 'automatic source mutation or destructive repository integration is forbidden' >&2
  exit 1
fi

echo 'semantic_audit=CLOSED'
