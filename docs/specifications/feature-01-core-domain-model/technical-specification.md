# Feature 01: Core Domain Model Technical Specification

## Purpose

This file is a technical fidelity index for Feature 01. It records only the implementation decisions
that agents must not infer independently.

Business intent lives in
`docs/specifications/feature-01-core-domain-model/functional-specification.md`. Product and
architecture detail lives in Meridian workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`. If this file
and Meridian disagree, Meridian wins and this file must be corrected before implementation continues.

## Authority chain

Implementation decisions must be resolved in this order:

1. Meridian workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`, starting from `IDEA-17`, `IDEA-28`,
   `IDEA-35`, `IDEA-36`, `IDEA-37`, `IDEA-38`, `IDEA-40`, `IDEA-42`, `IDEA-43`, and `IDEA-57`.
2. `docs/specifications/feature-01-core-domain-model/functional-specification.md`.
3. `docs/constitution.md`.
4. `apps/store/AGENTS.md` and `apps/mcp/AGENTS.md`.
5. This technical specification.

## Implementation boundary decisions

| Decision | Required implementation constraint | Source |
| --- | --- | --- |
| Store owns domain rules | `apps/store` must own entities, value objects, lifecycle transitions, invariant checks, domain errors, and persistence-facing ports for the core model. The canonical rule logic itself (e.g. `domain/branch-lifecycle.ts`'s guards) lives only in the domain layer and controllers/DTOs must never invent their own copy of it — but persistence adapters (e.g. `BranchGraphRepository`, `ArtifactAssociationRepository`) are permitted, and in fact required, to *call* those same domain guards inside their own transactions as a defense-in-depth check before a write, so a branch's write-lock/discipline-boundary state can never be raced between check and write. Adapters must not define new business rules of their own. | Constitution III, IV; store AGENTS |
| MCP is adapter-only | `apps/mcp` must call the store boundary through the harness or store API and must not duplicate branch, graph, lifecycle, authorization, or approval rules. | MCP AGENTS; `IDEA-35` |
| Rich domain model | Domain-significant values must be represented as named value objects or equivalent rich types, not free-form strings. Required concepts: workspace ID, stakeholder ID, branch ID, idea label, discipline, chunk type, context kind, relationship type, and actor context. | Constitution IV |
| Workspace scoping | Every aggregate and graph operation is tenant/workspace scoped. Cross-workspace chunks, branches, edges, suggestions, and generated context must not connect or resolve together. | Functional spec AC; Constitution IV |
| Branch ownership | A branch has exactly one discipline for its lifetime and records a divergence point when created. Branch work is isolated from mainline writes. Merged branches retain their boundary, provenance, history, and merge lineage. | `IDEA-17`, `IDEA-40` |
| Branch graph view | A branch resolves mainline references from its divergence point without cloning the mainline graph. Later mainline changes do not silently mutate the branch view; they surface through conflict checks or later explicit catch-up behavior. | `IDEA-17` |
| Discipline boundary | A branch may modify chunks and edges owned by its discipline. It may create cross-disciplinary edges to other disciplines' chunks only when it does not modify those target chunks. | `IDEA-35` |
| Logical edge endpoints | Edges identify chunks by logical idea labels, not storage-row UUIDs. Label-based relationships must survive branch overrides and mainline promotions without endpoint rewrites. | `IDEA-36`, `IDEA-37` |
| Edge determinism | After branch overrides and deactivations are applied, a resolved graph view may contain at most one active edge for the same source label, target label, and relationship type. | `IDEA-38` |
| Edge lineage | Mainline edges are immutable. Mainline relationship changes must supersede prior edge versions and preserve lineage; promoted edge history must not be deleted. Deactivation is a supersession too: `deactivateEdge` appends a new `deactivated` version rather than mutating the current version's state in place (clarified during feature-02 story S03; see feature-02 technical spec §"Edge lineage persistence"). | `IDEA-38` |
| Human accountability | Every graph modification, approval, suggestion decision, verification decision, and merge decision must be attributable to a human stakeholder ID. | `IDEA-28`, `IDEA-40`, `IDEA-42`, `IDEA-57` |
| Delegated agents | AI agents and external systems may submit feedback and act as supervised delegates, but delegated sessions cannot approve chunks, accept or reject suggestions, submit branches, verify branches, or merge branches. | `IDEA-28`, `IDEA-40`, `IDEA-42`, `IDEA-57` |
| Verification signals | Verification feedback is advisory history only. Signals must never automatically verify, unverify, merge, reject, reopen, or otherwise transition a branch. | `IDEA-43` |
| Promotion path | Chunk and edge promotion to mainline occurs only inside an atomic, direct-human-authenticated merge of a verified branch. There is no standalone promotion path that bypasses merge. | `IDEA-42`, `IDEA-57` |
| Generated context | Generated context packages are projections from approved or promoted chunks and active resolved relationships. Generated documents are not the source of truth. | Functional spec AC |

## Required lifecycle contracts

Implement the lifecycle states below. Do not add alternate state names unless Meridian is updated
first.

| Model | States and transitions |
| --- | --- |
| Chunk | Draft to Approved to Promoted. Approved or Promoted chunks may become Superseded or inactive according to later persistence/merge behavior. Approval state and activity state are separate. |
| Branch | Draft to Submitted; Submitted to Verified or human-initiated return to Draft; Verified to Merged or human-initiated return to Draft. Return-to-Draft transitions are never automated. Merged is terminal. Submitted, Verified, and Merged branches are graph-write locked, though verification signals, status logs, and audit metadata may still be appended. |
| Suggestion | Pending to Accepted or Rejected. Accepted and Rejected are terminal. Accepted suggestions must remain linked to the feedback branch they initialize. |
| Edge | Active, Deactivated, or Superseded. Mainline edges cannot be destructively deleted; changes create lineage-preserving supersession records. A single version can only be `Active`; both `Deactivated` and `Superseded` are always produced by appending a new version onto a prior `Active` one, never by mutating a stored version's state in place. |

## Protected operation contracts

These operation contracts are intentionally narrower than API or MCP design. Later routes, tools, or
transport schemas must map to these behaviors instead of redefining them.

| Operation | Non-negotiable contract |
| --- | --- |
| Approve chunk | Requires a direct human-authenticated stakeholder. Delegated agents cannot be the approving actor. |
| Accept suggestion | Requires a direct human-authenticated stakeholder and creates a linked feedback branch scoped to one discipline. Delegated agents cannot decide. |
| Reject suggestion | Requires a direct human-authenticated stakeholder and must not modify graph state. Delegated agents cannot decide. |
| Submit branch | Requires a direct human-authenticated stakeholder from the branch discipline and locks graph writes. |
| Verify branch | Requires a direct human-authenticated stakeholder in the workspace after advisory signals have been reviewed. This feature does not require branch-discipline membership for verification unless a later governance specification adds it. |
| Merge branch | Requires a Verified branch, a direct human-authenticated stakeholder in the workspace, conflict checks against the divergence point, and all-or-nothing application. This feature does not require branch-discipline membership for merge unless a later governance specification adds it. |

Human authentication for protected operations must come from human-scoped session credentials.
Self-reported delegation, impersonation, actor, or client headers must never be accepted as proof of
direct human authentication.

Active draft chunks in a branch block merge with an invalid state transition. Merge must not silently
discard draft graph work or partially promote only approved items from the same branch.

## Required domain error categories

Adapters must map domain failures explicitly and must not inspect free-form message text or return
success-shaped fallbacks.

Required categories:

1. Not found.
2. Invalid state transition.
3. Unauthorized actor.
4. Write locked.
5. Discipline boundary violation.
6. Branch isolation violation.
7. Duplicate active relationship.
8. Lineage violation.
9. Tenant boundary violation.

## Fidelity traceability matrix

| Functional acceptance criterion | Technical fidelity rule | Meridian source |
| --- | --- | --- |
| Stakeholders can explain key concepts using Meridian-backed definitions. | Keep this file minimal and defer detailed vocabulary to Meridian and the functional spec. | All referenced chunks |
| Stakeholders can tell ownership and review discipline. | Branches are single-discipline; stakeholder actions carry human provenance; discipline boundaries are enforced. | `IDEA-17`, `IDEA-35`, `IDEA-40` |
| Stakeholders can tell whether context is draft, approved, promoted, superseded, or inactive. | Implement chunk approval/activity states and edge active/deactivated/superseded states without destructive history loss. | `IDEA-38` |
| Stakeholders can distinguish advisory feedback from human approval. | Verification signals never automate transitions; agents cannot approve, accept/reject suggestions, submit, verify, or merge. | `IDEA-28`, `IDEA-40`, `IDEA-43`, `IDEA-57` |
| Agents receive traceable approved implementation context. | Context packages project approved or promoted chunks and active label-based relationships with provenance. | `IDEA-36`, `IDEA-37`, `IDEA-38` |

## Testing expectations

Implementation must use TDD and Vitest. Tests must prove the contracts in this file at the smallest
appropriate level: value-object validation, lifecycle transitions, human-only protected operations,
branch and discipline boundaries, label-based relationship resolution, edge uniqueness, lineage
preservation, suggestion review, advisory verification signals, tenant isolation, and generated
context provenance.

Before implementation is considered complete, repository checks must pass: `pnpm build`,
`pnpm typecheck`, and `pnpm test`.

## Non-goals

This feature does not define database DDL, migrations, REST routes, DTOs, MCP tool schemas,
authentication-provider integration, notification delivery, generated document rendering, caching, or
performance targets. Those later specifications must depend on this model instead of redefining it.
