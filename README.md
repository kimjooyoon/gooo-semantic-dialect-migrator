# gooo-semantic-dialect-migrator

`gooo-semantic-dialect-migrator` migrates an existing Gooo v1 program to a
declared v2 dialect while carrying explicit semantic evidence. The
authoritative protocol is [`.gooo/migrator.gooo`](.gooo/migrator.gooo): it
declares the dialect versions, typed operations, preservation predicates,
reversible/lossy boundary, UNKNOWN tuple, fixed denominator, and decision
precedence. Go only parses, lowers, executes, verifies, and renders the
declarations.

The migration chain is:

```text
.gooo meta + v1 program + typed operation
  → migrated v2 .gooo
  → canonical semantic IR + generated Go binding
  → source/target execution
  → exact preservation vector
  → REFUTED > UNKNOWN > CLOSED
```

Compatibility is never inferred from text similarity. A `CLOSED` receipt
requires explicit origin mappings, inverse evidence where the meta operation
requires it, exact terminal reason/effect replay, and every declared hard
predicate. Missing origin/inverse evidence and a declared lossy capability
boundary are `UNKNOWN`. A changed terminal reason or effect trace is
`REFUTED`, even when another field is also uncertain.

The fixed conformance corpus has eight cases:

| ordinal | case | expected |
|---:|---|---|
| 1 | no-op-canonical | CLOSED |
| 2 | symbol-rename-origin | CLOSED |
| 3 | one-to-many-split | CLOSED |
| 4 | comment-only | CLOSED |
| 5 | missing-inverse-origin | UNKNOWN |
| 6 | lossy-capability | UNKNOWN |
| 7 | semantic-incompatibility | REFUTED |
| 8 | replay-closed | CLOSED |

## Use

All output paths are caller-owned and must be outside the input repository.
The input `.gooo` program is never overwritten, including for `UNKNOWN` and
`REFUTED` cases.

```text
go run ./cmd/gooo-semantic-dialect-migrator migrate \
  -root . \
  -meta .gooo/migrator.gooo \
  -case fixtures/cases/symbol-rename-origin.json \
  -output-dir /tmp/gooo-semantic-dialect-migrator-case

go run ./cmd/gooo-semantic-dialect-migrator conformance \
  -root . \
  -meta .gooo/migrator.gooo \
  -output-dir /tmp/gooo-semantic-dialect-migrator-conformance
```

The conformance output contains one report and generated artifact set per
case, a fixed-case vector, `conformance-index.json`, and `ci-summary.md`.
Inventory excludes only the repository-root `README.md`. Metrics contain
exact integer counts for Go/Gooo physical lines, descendant directories,
regular files, generated files/bytes, per-stage `wall_ms` and
`peak_rss_kib`, and test total/selected/executed/reused/failed/unknown. No
score, weighted average, estimate, or percentage is emitted.

CI additionally records independent runner-observed integer fields:
`compile_wall_ms`, `compile_peak_rss_kib`, `build_wall_ms`,
`build_peak_rss_kib`, `test_wall_ms`, `test_peak_rss_kib`,
`conformance_wall_ms`, `conformance_peak_rss_kib`, `integration_wall_ms`,
and `integration_peak_rss_kib`. The five local authority fields
`local_test_executions`, `local_build_executions`, `local_vet_executions`,
`local_conformance_executions`, and `local_integration_executions` are
exactly zero. Missing, null, string, negative, or extra runner fields fail
closed. The CI runner obtains command wall time and maximum RSS from
`/usr/bin/time` and annotates the already-generated conformance artifact.
The annotation verifies that the conformance replay digest is unchanged.

The CI measurement wrapper is [`scripts/ci-metrics.sh`](scripts/ci-metrics.sh).
It measures the compile artifact generation, Go build, Go test, conformance,
and integration commands independently with `/usr/bin/time`, then validates
the final artifact schema before upload.

Local test/build/vet/conformance execution is intentionally outside this
repository's release protocol. GitHub Actions is the validation authority.
The workflows do not commit, push, merge, release, or mutate an input source.
