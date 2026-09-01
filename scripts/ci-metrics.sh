#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
out=${1:?caller-owned output directory is required}
measure_dir="$out/runner-measurements"
compile_out="$out/compile"
conformance_out="$out/conformance"
integration_out="$out/integration"
mkdir -p "$measure_dir" "$compile_out" "$conformance_out" "$integration_out"

[[ -x /usr/bin/time ]] || { echo '/usr/bin/time is required' >&2; exit 1; }

measure() {
  name=$1
  shift
  /usr/bin/time -f '%e %M' -o "$measure_dir/$name.time" "$@"
}

parse_measurement() {
  name=$1
  read -r elapsed peak extra < "$measure_dir/$name.time"
  [[ -n "${elapsed:-}" && -n "${peak:-}" && -z "${extra:-}" ]] || { echo "invalid $name measurement" >&2; exit 1; }
  wall_ms=$(awk -v seconds="$elapsed" 'BEGIN { if (seconds !~ /^[0-9]+([.][0-9]+)?$/) exit 1; printf "%.0f", seconds * 1000 }') || { echo "invalid $name elapsed value" >&2; exit 1; }
  [[ "$wall_ms" =~ ^[0-9]+$ && "$peak" =~ ^[0-9]+$ ]] || { echo "non-integer $name measurement" >&2; exit 1; }
  printf '%s %s\n' "$wall_ms" "$peak"
}

measure compile go run ./cmd/gooo-semantic-dialect-migrator migrate \
  -root "$repo_root" -meta .gooo/migrator.gooo \
  -case fixtures/cases/symbol-rename-origin.json -output-dir "$compile_out" > "$measure_dir/compile.stdout"
measure build go build ./... > "$measure_dir/build.stdout"
measure test go test ./... -count=1 > "$measure_dir/test.stdout"
measure conformance scripts/conformance.sh "$conformance_out" > "$measure_dir/conformance.stdout"
measure integration scripts/integration.sh "$integration_out" > "$measure_dir/integration.stdout"

read -r compile_wall_ms compile_peak_rss_kib < <(parse_measurement compile)
read -r build_wall_ms build_peak_rss_kib < <(parse_measurement build)
read -r test_wall_ms test_peak_rss_kib < <(parse_measurement test)
read -r conformance_wall_ms conformance_peak_rss_kib < <(parse_measurement conformance)
read -r integration_wall_ms integration_peak_rss_kib < <(parse_measurement integration)

observations="$measure_dir/runner-observations.json"
printf '{\n  "compile_wall_ms": %s,\n  "compile_peak_rss_kib": %s,\n  "build_wall_ms": %s,\n  "build_peak_rss_kib": %s,\n  "test_wall_ms": %s,\n  "test_peak_rss_kib": %s,\n  "conformance_wall_ms": %s,\n  "conformance_peak_rss_kib": %s,\n  "integration_wall_ms": %s,\n  "integration_peak_rss_kib": %s,\n  "local_test_executions": 0,\n  "local_build_executions": 0,\n  "local_vet_executions": 0,\n  "local_conformance_executions": 0,\n  "local_integration_executions": 0\n}\n' \
  "$compile_wall_ms" "$compile_peak_rss_kib" "$build_wall_ms" "$build_peak_rss_kib" \
  "$test_wall_ms" "$test_peak_rss_kib" "$conformance_wall_ms" "$conformance_peak_rss_kib" \
  "$integration_wall_ms" "$integration_peak_rss_kib" > "$observations"

go run ./cmd/gooo-semantic-dialect-migrator annotate-metrics \
  -index "$conformance_out/conformance-index.json" \
  -observations "$observations" > "$measure_dir/annotation.stdout"
go run ./cmd/gooo-semantic-dialect-migrator validate-metrics \
  -metrics "$conformance_out/metrics.json" > "$measure_dir/validation.stdout"
