# Spool Implementation Gap Report

**Date:** 2026-08-20
**Scope:** Current `cmd/` and `internal/` implementation compared with the research in
this directory. `idea-graph-vcs-research-v2.md` is treated as the authoritative target
because it explicitly supersedes `idea-graph-vcs-research.md`.

## Executive assessment

Spool has a sound, tested local proof-of-concept for content-addressed graph snapshots:
canonical CBOR/BLAKE3 object IDs, branching, staging, commits, snapshot resolution,
diff, entity history, containment lookup, bounded impact analysis, durable replacement,
and process locking are implemented.

It does **not** yet implement the v2 product architecture. It now has a typed property-graph
foundation with authored schema policy and validation, a real immutable loose-and-pack object
store with chunked graph roots, explicit maintenance, and `fsck`, deterministic three-way graph
merge previews, clean applies, a durable public conflict-resolution lifecycle, and a rebuildable
SQLite/FTS5 branch-head projection. It still lacks the typed agent/MCP retrieval plane. The
project should therefore be described as a local graph-VCS foundation, not as an agent retrieval
system.

## Evidence-based coverage

| Area | Status | Implementation evidence | Assessment |
| --- | --- | --- | --- |
| Content addressing | Partial | Typed canonical CBOR objects use type-and-length-prefixed BLAKE3 IDs. New objects are loose envelopes; verified zstd pack/index generations are published through an atomic manifest and preserve the same identities. | The immutable-object boundary, compaction, and pack-aware reads are real and crash-safe. Delta compression, automatic maintenance thresholds, and remote pack transfer remain absent. |
| Snapshots and branches | Partial | `graphSnapshot` has counted node, edge, inbound, outbound, and schema roots; deterministic bounded Prolly leaf/internal trees back every graph index. Commits, per-branch refs, `HEAD`, staging, reflogs, and explicit resolution exist. | Core canonical VCS semantics and explicit pack-backed maintenance now exist locally; remotes and ref compaction remain deferred. |
| Schema policy and validation | Partial | Canonical TOML schemas define node-label and edge-type rules, required typed properties, natural-key uniqueness, cardinality, and acyclic/self-loop invariants. `spl schema migrate` stages a schema and graph mutations atomically; `spl validate` reports immutable-snapshot violations; `spl fsck` validates reachable object, tree, graph, schema, staging, and merge state. | Staging, commit, durable-open, and merge previews validate schemas. Historical snapshots retain their schema roots and remain readable. Schema-aware query filtering is still absent. |
| SQLite projection | Partial | A private WAL-mode `.spl/graph.db` records version, branch-head commit/node-root watermark, and build state; it is rebuilt from canonical objects on open or catch-up. It holds current nodes, edges, labels, opted-in scalar property indexes, commit ancestry/change records, and FTS5 rows for titles, labels, and top-level string properties. | Projection failure never invalidates canonical state, and corrupt/stale databases rebuild. It remains branch-head-only and has no public typed query/search surface or incremental GraphDiff update. |
| Durable local updates | Partial | `config.toml`, `HEAD`, independent refs/staging/reflogs, loose objects, pack manifests, lock file, synced replacement, rollback, and merge transaction recovery are implemented. `spl gc` retains refs, permanent reflogs, and resolved merge snapshots; it packs reachable objects and grace-prunes unreachable loose objects. | Object creation precedes mutable ref updates. Pack publication is verified before loose or superseded-pack cleanup, and reflog retention inventory makes GC fail closed on missing historical roots. |
| Diff, history, containment, impact | Partial | `Diff`, `History`, `BranchesContaining`, and `Impact` have CLI/tool-adapter exposure and preserve labels, typed properties, and edge types. Snapshot-scoped reads require a branch, pin reachable commits once, and return repository/snapshot/projection provenance; diff records both pinned endpoints. | Useful initial operations, but they lack schema-aware filtering and natural-key containment. |
| Merge lifecycle | Partial | `spl merge preview` returns ordered structural, schema, and schema-derived semantic conflicts with stable IDs and affected paths. A conflicted exact `spl merge apply` persists an owner-gated target lease and preview; `merge conflicts`, `resolve`, `finalize`, and `abort` expose the durable lifecycle. Resolution requires complete source/target selections and supports validated mutation overrides; finalization rechecks the exact binding and schema before creating a two-parent commit. | The local lifecycle is complete for current graph semantics. Simultaneous schema-root edits still conflict deliberately, and conflict meaning is limited to structural overlap and declared-schema validation rather than inferred domain semantics. |
| Query budgets | Partial | `QueryBudget` accepts only caller limits that narrow configured maxima. Query deadlines combine caller and budget deadlines, propagate through snapshot selection and repository scans, produce deterministic timed-out prefixes where available, and expose completion metadata. Diff, history, containment, impact, and resolve cap complete public JSON envelopes and account for exact response bytes. | Budgets are implemented for current typed reads; the planned typed search, filtering, traversal, and path surfaces do not yet exist. |
| CLI contract | Partial | `spl init`, add, status, commit, branch, switch, schema migrate, validate, resolve, diff, history, branches-containing, `fsck`, `gc`, and `merge preview`, `apply`, `conflicts`, `resolve`, `finalize`, and `abort` emit JSON. | No `log`, query, search, filter, traversal, path, context, or remote commands. |

## Prioritized gaps

| Priority | Gap | Research baseline | Current shortfall and consequence | Recommended next increment |
| --- | --- | --- | --- | --- |
| P1 | Search and agent context assembly | v2 §9; agent-query research | The private projection now supplies FTS5 candidates and schema-driven scalar indexes, but no public typed search/filter API, ranking response, snippets, provenance assembly, graph expansion, or one-call evidence context exists. | Add typed lexical search and metadata filters over the projection, then add bounded graph expansion and context assembly. Defer embeddings until lexical quality and packaging benchmarks justify them. |
| P2 | Traversal and path capability | v2 §§8-10 | Impact provides an outgoing, all-edge BFS over a hypothetical delta. It has no direction/edge-type filters, arbitrary start nodes, stop labels, induced subgraph, shortest path, or explicit truncation. | Generalize traversal into typed `traverse` and `path` tools, retaining the existing deterministic BFS as a building block. |
| P2 | Version-control reasoning completeness | v2 §10 | Diff and history now retain labels, edge types, and properties; history reports title/label/property changes. Diff still has only ID/title filters, and natural-key containment is declared but returns no matches. | Complete schema-aware diff/history/containment after schema policy lands. |
| P2 | Benchmarks and acceptance evidence | v2 §14; agent-query research | Unit tests pass, but there are no benchmarks, scale fixtures, latency percentiles, projection-lag measurements, or retrieval-quality evaluation. | Add benchmark fixtures and measurements for the v2 matrix before selecting vector or graph-native dependencies. |

## Research interpretation and scope decisions

The JSON-LD research does not require replacing Spool's storage with JSON-LD. It recommends
CBOR/BLAKE3 plus derived SQLite for the general IdeaGraph-scale target, with JSON-LD kept as
an interchange format.

Semantic/vector retrieval, graph-native projections, and a remote hub are intentionally not
immediate implementation gaps. The v2 research makes them conditional on measured workloads;
they should not be selected before the SQLite/FTS retrieval baseline and benchmark suite exist.

## Suggested delivery sequence

1. **Agent surface:** expose typed lexical search/filter, merge preview/conflicts, and task-level
   context assembly; add historical projection caching only after the selector contract is stable.
2. **Storage maintenance evidence:** measure loose-object growth, pack size, compaction cost,
   and GC reclaim behavior before introducing delta compression, automatic maintenance, or
   remote pack transfer.
3. **Evidence-led extensions:** benchmark the baseline, then decide whether vector retrieval,
   graph-native projections, or remotes are warranted.

## Validation performed

`make check` and `make fuzz` pass for the current repository, including schema parsing and
validation, schema migration/staging/persistence, projection build/rebuild/recovery, resolver,
merge, pack/GC, `fsck`, and CLI packages. This establishes that the remaining gaps are missing
scope relative to the research, not failing behavior in the implemented local-core tests.
