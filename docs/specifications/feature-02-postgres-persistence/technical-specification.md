# Feature 02: Persistent Knowledge Store Technical Specification

## Purpose

This file is a technical fidelity index for Feature 02. It records only the implementation
decisions that agents must not infer independently.

Business intent lives in
`docs/specifications/feature-02-postgres-persistence/functional-specification.md`. Product and
architecture detail lives in Meridian workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`. If this file
and Meridian disagree, Meridian wins and this file must be corrected before implementation
continues.

## Authority chain

Implementation decisions must be resolved in this order:

1. Meridian workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`, starting from `IDEA-31`, `IDEA-32`,
   `IDEA-33`, `IDEA-38`, `IDEA-41`, `IDEA-46`, `IDEA-47`, `IDEA-49`, `IDEA-62`, `IDEA-63`,
   `IDEA-65`, `IDEA-67`, `IDEA-68`, and `IDEA-69`.
2. `docs/specifications/feature-02-postgres-persistence/functional-specification.md`.
3. `docs/constitution.md`.
4. `apps/store/AGENTS.md` and `apps/mcp/AGENTS.md`.
5. This technical specification.

## Implementation boundary decisions

| Decision | Required implementation constraint | Source |
| --- | --- | --- |
| Store owns persistence | `apps/store` owns the PostgreSQL-backed persistence adapter for chunks, edges, branches, suggestions, chunk-artifact associations, delivery subscriptions, and notifications. Persistence code must map to and preserve Feature 01 domain invariants; it must not redefine lifecycle or authorization rules. | Constitution III, IV; store AGENTS |
| Delta-based branch storage | Branch-specific chunk and edge changes (additions, overrides, deactivations) must be stored as branch-scoped delta records, not as full copies of the mainline graph. A branch's resolved view is computed by combining branch-scoped deltas with mainline records at read time. | `IDEA-32`, `IDEA-33` |
| Divergence tracking | Every branch must persist a divergence marker captured at branch creation. Conflict checks and rebase/catch-up flows must use this marker to identify mainline changes made after divergence; it must be updated only after local overrides are confirmed to integrate the conflicting mainline change. | `IDEA-41` |
| Conflict detection scope | Pre-merge conflict detection must inspect chunk content changes, edge relationship changes (type or status), and chunk-artifact association changes made independently on both the branch and mainline since divergence. | `IDEA-46` |
| Atomic merge | Branch merge must execute as a single atomic persistence transaction covering all persisted graph mutations involved in the merge: chunk updates, edge updates, chunk-artifact association updates, and branch-status updates. Any failure must roll back the entire operation; partial merges (including partially merged artifact associations) must not be observable. | `IDEA-46`, `IDEA-47`, `IDEA-62`, `IDEA-69`, feature-01 promotion path |
| Edge lineage persistence | Mainline edges must never be physically or logically deleted. Deactivating an edge without a type change must supersede it with a new edge version, preserving an unbroken lineage chain: the current version becomes `superseded` and a new version — same identity, state `deactivated` — is appended (a lineage's first version can only be `active`; `deactivated` can only appear as a lineage's *last* version). Replacing an edge's relationship type must create a new active edge for the new type and close the old type's generation the same way — appending a `deactivated` version tagged with a precise successor pointer — rather than flipping its current row's state in place. At most one active edge of a given relationship type may exist between two chunks within a resolved branch view. (Clarified during feature-02 story S03 implementation review, which found the persistence layer initially flipped a deactivating edge's current row in place instead of appending.) | `IDEA-38`, feature-01 edge determinism |
| Logical edge identity in persistence | Persisted edge records must resolve endpoints through logical idea labels, not storage-row identifiers, so branch overrides and mainline promotions do not require endpoint rewrites. | Feature-01 logical edge endpoints |
| Suggestion persistence | Suggestions must be persisted with a `pending` initial status pending human stakeholder review. A suggestion accepted into a branch must retain a durable link from that branch back to its originating suggestion. | `IDEA-49`, feature-01 accept-suggestion contract |
| Chunk-artifact association lifecycle | Chunk-to-artifact associations must be versioned per branch (active, superseded, deactivated) using the same delta-based model as chunks and edges, so branch review does not mutate mainline associations. | `IDEA-62` |
| Downstream delivery split | Real-time push delivery of merged, visibility-resolved graph updates must run as an asynchronous background process triggered by branch merge events; it must not block the merge transaction. On-demand pull access must be served synchronously through existing query access rather than through the push path. | `IDEA-63` |
| Delivery subscription persistence | Downstream push consumers and their discipline filters must be persisted as durable subscription records scoped to a workspace, independent of any single delivery attempt. | `IDEA-65` |
| Feedback notification routing | Evaluation feedback and verification signals must be routed and persisted as notification records immediately upon ingestion, independent of whether the stakeholder is online. At minimum, the author of the evaluated branch must be notified; other relevant stakeholders may also be notified. | `IDEA-67`, `IDEA-68` |
| Notification acknowledgement is non-destructive | Acknowledging or reading a notification must not delete or mutate the underlying feedback or verification signal record it references. | Functional spec AC 5 |
| Pre-merge history reconstruction | The pre-merge state of any merged branch must remain reconstructable after merge by tracing chunk, edge, and chunk-artifact records back to their originating branch, even though mainline promotion clears branch-scoped write isolation on the promoted records. | `IDEA-69` |
| Tenant isolation | Every persisted record scoped to workspace-owned data must carry an unambiguous workspace association, and all read paths must filter by it. Cross-workspace joins or lookups must not be possible through the persistence layer. | Functional spec AC 6; feature-01 workspace scoping |

## Required lifecycle contracts

This feature persists the lifecycle states and transitions defined in
`docs/specifications/feature-01-core-domain-model/technical-specification.md` (chunk, branch,
suggestion, edge). It does not introduce new lifecycle states. Persistence must be able to
represent every state and transition in that table without lossy collapsing (for example, edge
`superseded` must remain distinguishable from `deactivated`).

## Protected operation contracts

Persistence must support, and must not weaken, the protected operation contracts defined in
`docs/specifications/feature-01-core-domain-model/technical-specification.md` (approve chunk,
accept/reject suggestion, submit/verify/merge branch). Specifically:

- Merge persistence must enforce all-or-nothing application (see Atomic merge, above).
- Persisted suggestion and notification records must retain enough provenance to attribute
  accept/reject decisions and verification signals to a human stakeholder; the persistence layer
  must not accept a client-supplied actor claim as a substitute for authenticated provenance.
- Verification signal and feedback persistence must remain advisory: writing a signal or
  notification record must never itself trigger a lifecycle transition.

## Required domain error categories

Persistence adapters must surface failures using the domain error categories defined in
`docs/specifications/feature-01-core-domain-model/technical-specification.md` (not found, invalid
state transition, unauthorized actor, write locked, discipline boundary violation, branch isolation
violation, duplicate active relationship, lineage violation, tenant boundary violation). This
feature adds no new error categories; conflict-detection failures during merge must map to
existing categories (for example, lineage violation or branch isolation violation) rather than
introducing an ad hoc conflict error type.

## Fidelity traceability matrix

| Functional acceptance criterion | Technical fidelity rule | Meridian source |
| --- | --- | --- |
| A stakeholder can identify current approved context in a workspace. | Mainline and branch views are computed from delta records combined with mainline at read time, keeping approved context distinguishable from branch-scoped drafts. | `IDEA-32`, `IDEA-33` |
| A stakeholder can trace an approved idea or relationship back to its review history. | Edge lineage is preserved through supersession chains; merged branch state remains reconstructable via origin tracking. | `IDEA-38`, `IDEA-69` |
| A stakeholder can see pending, accepted, and rejected suggestions. | Suggestions persist with pending status and a durable link from any resulting branch back to the originating suggestion. | `IDEA-49` |
| A stakeholder can see verification feedback attached to the branch it evaluated. | Feedback and verification signals are persisted and routed to stakeholders immediately upon ingestion without mutating branch state. | `IDEA-67`, `IDEA-68` |
| A stakeholder can acknowledge review notifications without losing the historical feedback record. | Acknowledgement is non-destructive to the underlying feedback/signal record. | `IDEA-68` |
| A workspace owner can confirm that one workspace's knowledge does not appear in another workspace. | Every workspace-owned record carries an unambiguous workspace association; cross-workspace reads are not possible. | Feature-01 workspace scoping |
| An implementation agent can request current approved context without treating draft or superseded work as approved. | Divergence tracking and conflict detection ensure branch-scoped and mainline records stay distinguishable through merge; delivery pull queries read resolved, visibility-correct state. | `IDEA-41`, `IDEA-46`, `IDEA-63` |

## Testing expectations

Implementation must use TDD and Vitest. Tests must prove the contracts in this file at the
persistence-adapter level: delta-based branch resolution against mainline, divergence-marker
conflict detection, atomic all-or-nothing merge (including forced-failure rollback), edge lineage
preservation across supersession, suggestion-to-branch linkage, chunk-artifact association
versioning, notification persistence and non-destructive acknowledgement, delivery subscription
persistence, and tenant/workspace isolation across all of the above. Integration tests must run
against the containerized Postgres runtime described in `apps/store/AGENTS.md`, not an in-memory
substitute, for any test asserting transactional or resolved-view consistency behavior.

Before implementation is considered complete, repository checks must pass: `pnpm build`,
`pnpm typecheck`, and `pnpm test`.

## Non-goals

This feature does not define concrete database DDL, migrations, indexes, or query implementations;
REST routes, DTOs, or MCP tool schemas; authentication-provider integration; generated document
rendering formats; or performance/benchmark targets. Those details belong to implementation and,
where architecturally significant, to Meridian ADRs rather than this file.
