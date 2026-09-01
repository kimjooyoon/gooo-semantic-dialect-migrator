# Protocol v1

## Authority

`.gooo/migrator.gooo` is the only semantic authority. It fixes the two
dialects, seven typed operations, seven preservation predicates, eight case
ordinals, the decision precedence, and the six-field UNKNOWN tuple. The JSON
contract is an audit mirror; changing it does not change execution authority.

## Migration evidence

Each case supplies a source path and an ordered operation list. Operations
carry their type, origin pairs, inverse evidence, and any lossy boundary.
`SPLIT_NODE` carries its target parts so the one-to-many mapping is explicit.
The migrator renders a new v2 source in the caller-owned output directory and
then parses that rendered source again before lowering it. This makes the
artifact round trip part of the evidence rather than trusting an in-memory
candidate.

The execution model follows the unique outgoing edge from the declared entry,
collects node effects in order, and unions declared capabilities. A terminal
reason/effect difference is a semantic contradiction. The evaluator records
each predicate's declaration bit, observed bit, before digest, after digest,
and detail in fixed meta-source order.

## Decision and UNKNOWN

`REFUTED` takes precedence over `UNKNOWN`, which takes precedence over
`CLOSED`. UNKNOWN records always contain exactly the protocol fields:
`stage`, `step`, `reason`, `unknown_class`, `next_operation`, and
`blocked_by`. Missing inverse/origin evidence and lossy capability migration
remain UNKNOWN until a future operation supplies the missing proof.

## Authority boundary

The CLI refuses an output directory inside the input root. It does not write
to source files, git state, remotes, pull requests, tags, or releases. CI
uses temporary caller-owned output and publishes it as an artifact for audit.

## Runner metrics

The CI artifact schema has independent integer pairs for `compile`, `build`,
`test`, `conformance`, and `integration`: each pair is `wall_ms` and
`peak_rss_kib`. The CI wrapper obtains these values from `/usr/bin/time`; it
does not reuse the internal Go stage clock for command metrics. Five authority
fields record local execution counts for test, build, vet, conformance, and
integration, and the meta source fixes each to zero.

`annotate-metrics` accepts exactly the declared runner fields. Missing, null,
string, fractional, negative, or extra fields are rejected. It rewrites only
the caller-owned conformance index and metrics files and verifies that the
case-level replay digest is identical before and after annotation.
