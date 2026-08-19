# Spool Implementation Gap Report

**Date:** 2026-08-19  
**Scope:** Current `cmd/` and `internal/` implementation compared with the research in
this directory. `idea-graph-vcs-research-v2.md` is treated as the authoritative target
because it explicitly supersedes `idea-graph-vcs-research.md`.

## Executive assessment

Spool has a sound, tested local proof-of-concept for content-addressed graph snapshots:
canonical CBOR/BLAKE3 object IDs, branching, staging, commits, snapshot resolution,
diff, entity history, containment lookup, bounded impact analysis, durable replacement,
and process locking are implemented.

It does **not** yet implement the v2 product architecture. It now has a typed property-graph
foundation with authored schema policy and validation plus deterministic three-way graph merge
previews and clean applies, but lacks a real immutable object-store and projection lifecycle,
conflict-resolution completion, and the typed agent/MCP retrieval plane. The project should
therefore be described as a local graph-VCS foundation, not as an agent retrieval system.

## Evidence-based coverage

| Area | Status | Implementation evidence | Assessment |
| --- | --- | --- | --- |
| Content addressing | Partial | `repository.store` uses canonical CBOR and type-and-length-prefixed BLAKE3 IDs. | Correct foundation; objects are retained in one persisted JSON document rather than an object database. |
| Snapshots and branches | Partial | `graphSnapshot` has node, edge, inbound, outbound, and schema roots; nodes have labels and tagged recursive properties; edges have types and tagged recursive properties; commits, branch refs, staging, and explicit resolution exist. | Core semantics exist, but roots are sorted full lists rather than chunked Prolly trees and there are no independent refs/reflogs. |
| Schema policy and validation | Partial | Canonical TOML schemas define node-label and edge-type rules, required typed properties, natural-key uniqueness, cardinality, and acyclic/self-loop invariants. `spl schema migrate` stages a schema and graph mutations atomically; `spl validate` reports immutable-snapshot violations. | Staging, commit, durable-open, and merge previews validate schemas. Historical snapshots retain their schema roots and remain readable. No public `fsck` or schema-aware query filtering exists. |
| Durable local updates | Partial | `.spl/repository.json`, lock file, temp-file sync/replace, rollback, and merge transaction recovery are implemented. | The atomic-replace discipline is good, but the monolithic state file prevents independent immutable-object durability, incremental storage, and garbage collection. |
| Diff, history, containment, impact | Partial | `Diff`, `History`, `BranchesContaining`, and `Impact` have CLI/tool-adapter exposure and preserve labels, typed properties, and edge types. | Useful initial operations, but they lack schema-aware filtering, natural-key containment, and consistent snapshot-scoped v2 request semantics. |
| Merge lifecycle | Partial | `spl merge preview` computes a deterministic merge base and three-way node/edge/property-key result; it reports ordered structural and schema conflicts plus validation failures. `spl merge apply` recomputes the exact preview ID and materializes a two-parent commit with the combined graph. Leases, conflict transaction state, recovery, abort/finalize primitives, and schema validation of clean/resolved snapshots remain available in the repository layer. | Clean merges now combine source and target graph changes. Public conflict inspection is the preview response; there is no public durable conflict-resolution, finalize, or abort flow, no semantic conflict classifier, and simultaneous schema-root edits deliberately conflict. |
| Query budgets | Partial | `QueryBudget` normalizes row, byte, depth, visited, and timeout limits; diff enforces row/byte limits and impact enforces depth/visited. | Timeout is only a normalized value, not an execution deadline. History, containment, and resolve do not enforce equivalent result/response budgets or partial-result metadata. |
| CLI contract | Partial | `spl init`, add, status, commit, branch, switch, schema migrate, validate, resolve, diff, history, branches-containing, `merge preview`, and `merge apply` emit JSON. | No merge conflict-resolution/finalize/abort commands, `log`, query, search, filter, traversal, path, context, fsck, GC, or remote commands. |

## Prioritized gaps

| Priority | Gap | Research baseline | Current shortfall and consequence | Recommended next increment |
| --- | --- | --- | --- | --- |
| P0 | Merge conflict-resolution completion | v2 §10.4 and Milestone 0 acceptance | Deterministic three-way node/edge/property-key previews and clean preview-bound applies are implemented. Structural and schema conflicts are returned by preview, but no public durable conflict inspection/resolution, finalize, or abort flow exists; semantic conflicts and affected paths are not classified. | Persist and expose conflicted previews, bind resolve/finalize/abort to their preview IDs, add semantic conflict classification and affected paths, and preserve exact-preview checks through finalization. |
| P1 | Canonical object-store layout and lifecycle | v2 §§4, 11, and Milestone 0 | All objects, projections, commits, refs, and staged state are serialized into `.spl/repository.json`. Roots contain complete sorted IDs, not structural tree nodes. No loose objects, packfiles, refs, reflogs, fsck, reachability GC, or object-level recovery exists. | Separate immutable objects from mutable refs/staging; introduce chunked content-addressed tree nodes before implementing pack/GC. Add fsck before remote synchronization. |
| P1 | Versioned SQLite projection | v2 §§5-7 and Milestone 1 | There is no SQLite dependency, `graph.db`, schema version, projection watermark, FTS5, typed property index, historical snapshot cache, or rebuild/catch-up process. In-memory maps named `projections` are canonical-state mirrors, not rebuildable query projections. | Specify and implement the v2 SQLite projection contract, watermark, build states, and rebuild-from-canonical-object test path. Do not expose raw physical tables as an API. |
| P1 | Strict snapshot request semantics | v2 §7 | `resolve` requires a branch and validates explicit commit reachability, but diff/history/impact accept a branch **or** unrestricted commit. Responses are inconsistent: only resolve returns snapshot/projection metadata. | Use one mandatory `(repository, branch, commit?)` selector and response envelope for every read tool. Pin once, require reachability unless detached access is explicit, and expose projection watermark/state. |
| P1 | Query execution safety and observability | v2 §§7, 8.4, and 13 | No SQL sandbox is needed yet because SQL is absent, but timeout/cancellation is not propagated into repository scans, and most read operations lack response caps, truncation states, elapsed/visited metrics, or deterministic pagination. | Establish a common query executor with context deadline enforcement, row/byte/visited limits, partial-result metadata, and deterministic pagination before adding expensive retrieval. |
| P1 | Search and agent context assembly | v2 §9; agent-query research | There is no FTS5 lexical search, semantic/vector projection, ranking, snippets, provenance assembly, or one-call evidence context. | Add FTS5 plus metadata filters and bounded graph expansion as the default local retrieval path. Defer embeddings until lexical quality and packaging benchmarks justify them. |
| P2 | Traversal and path capability | v2 §§8-10 | Impact provides an outgoing, all-edge BFS over a hypothetical delta. It has no direction/edge-type filters, arbitrary start nodes, stop labels, induced subgraph, shortest path, or explicit truncation. | Generalize traversal into typed `traverse` and `path` tools, retaining the existing deterministic BFS as a building block. |
| P2 | Version-control reasoning completeness | v2 §10 | Diff and history now retain labels, edge types, and properties; history reports title/label/property changes. Diff still has only ID/title filters, and natural-key containment is declared but returns no matches. | Complete schema-aware diff/history/containment after schema policy lands; add merge preview/conflict inspection as public operations. |
| P2 | Benchmarks and acceptance evidence | v2 §14; agent-query research | Unit tests pass, but there are no benchmarks, scale fixtures, latency percentiles, projection-lag measurements, or retrieval-quality evaluation. | Add benchmark fixtures and measurements for the v2 matrix before selecting vector or graph-native dependencies. |

## Research interpretation and scope decisions

The JSON-LD research does not require replacing Spool's storage with JSON-LD. It recommends
CBOR/BLAKE3 plus derived SQLite for the general IdeaGraph-scale target, with JSON-LD kept as
an interchange format.

Semantic/vector retrieval, graph-native projections, and a remote hub are intentionally not
immediate implementation gaps. The v2 research makes them conditional on measured workloads;
they should not be selected before the SQLite/FTS retrieval baseline and benchmark suite exist.

## Suggested delivery sequence

1. **Foundation completion:** finish public conflicted-merge resolution, finalize, and abort
   operations; preserve exact-preview binding and extend conflict reporting with semantic
   classification and affected paths.
2. **Durable local VCS:** split immutable objects and mutable state; add fsck, recovery, and
   object reachability/collection design.
3. **Retrieval baseline:** build versioned SQLite/FTS projections with strict watermarks,
   snapshot selectors, query limits, and rebuild tests.
4. **Agent surface:** expose typed MCP retrieval and merge preview/conflicts; add task-level
   context assembly.
5. **Evidence-led extensions:** benchmark the baseline, then decide whether vector retrieval,
   graph-native projections, or remotes are warranted.

## Validation performed

`go test ./...` passes for the current repository, including schema parsing and validation,
schema migration/staging/persistence, resolver, merge, and CLI packages. This establishes that
the remaining gaps are missing scope relative to the research, not failing behavior in the
implemented local-core tests.
