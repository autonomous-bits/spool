---
name: spool-technical-specs
description: >
  Minimal technical specification guidance for Spool. Use when creating or
  revising feature technical specs in docs/specifications so implementation
  agents get non-negotiable technical constraints without duplicating Meridian.
metadata:
  version: "1.0"
  compatibility: "Spool technical/specification workflow with Meridian MCP"
---

# Spool Technical Specs

Use this skill when creating or revising Spool technical specifications.

Technical specs in this repository should be intentionally small. They should act as a technical
fidelity index: capture only the implementation decisions agents must not infer independently, and
refer back to the functional spec and Meridian for product and architecture detail.

## Core rule

Meridian is the authoritative source of product and architecture detail.

If a technical spec and Meridian disagree, Meridian wins. The technical spec should make that
authority relationship explicit and must be corrected before implementation proceeds.

## Relationship to functional specs

The functional spec owns business intent, stakeholder value, user-facing outcomes, acceptance
criteria, and Meridian references.

The technical spec owns implementation-critical disambiguators, such as:

- workspace or application ownership boundaries
- domain model and value-object requirements
- lifecycle contracts that must not be guessed
- protected operations and authorization invariants
- graph, branch, lineage, and provenance invariants
- adapter boundaries for store, MCP, API, or other surfaces
- domain error categories needed for consistent adapters
- testing expectations for the contracts it records

Do not restate functional value or detailed Meridian content unless the restatement is needed to
prevent an implementation error.

## Required structure

Use this structure by default:

1. `# Feature NN: Name Technical Specification`
2. `## Purpose`
3. `## Authority chain`
4. `## Implementation boundary decisions`
5. `## Required lifecycle contracts` when lifecycle behavior exists
6. `## Protected operation contracts` when human-control, auth, or write gates exist
7. `## Required domain error categories`
8. `## Fidelity traceability matrix`
9. `## Testing expectations`
10. `## Non-goals`

Omit sections that do not apply, but do not omit a section if its absence would force an
implementation agent to infer a non-negotiable decision.

## Authority chain

List sources in this order:

1. Meridian workspace and starting chunks.
2. The feature functional specification.
3. `docs/constitution.md`.
4. Relevant `AGENTS.md` files.
5. The technical specification.

Include the Meridian workspace ID and the labels of the starting chunks. The functional spec should
hold UUIDs and snapshot-date detail; repeat those only when needed for traceability.

## Implementation boundary decisions

Prefer a compact table:

```markdown
| Decision | Required implementation constraint | Source |
| --- | --- | --- |
| Store owns domain rules | `apps/store` owns entities, value objects, lifecycle transitions, invariant checks, domain errors, and persistence-facing ports. | Constitution III, IV; store AGENTS |
```

Each row should answer: "What wrong implementation decision would an agent make if this row did not
exist?"

## Protected operation contracts

For human-control or security-sensitive behavior, be explicit about:

- which operations require direct human authentication
- which operations delegated agents cannot perform
- whether discipline membership is required
- which operations are write-locked
- which signals or feedback are advisory only
- which client-provided claims must not be trusted

If Meridian says direct human authentication is required, include the anti-spoofing invariant:
self-reported delegation, impersonation, actor, or client headers must not be accepted as proof of
direct human authentication.

## Fidelity traceability matrix

Include a small table mapping each functional acceptance criterion to the technical rule that
preserves it and the Meridian source.

Example:

```markdown
| Functional acceptance criterion | Technical fidelity rule | Meridian source |
| --- | --- | --- |
| Stakeholders can distinguish advisory feedback from human approval. | Verification signals never automate transitions; agents cannot approve, submit, verify, or merge. | `IDEA-40`, `IDEA-43`, `IDEA-57` |
```

## Technical-spec content

Technical specs may include:

- authority chain and conflict rules
- implementation boundary decisions
- domain entities, value objects, and invariants at a conceptual level
- lifecycle states and allowed transitions
- protected operation contracts
- domain error categories
- persistence-facing contracts that do not choose schema details
- adapter implications that do not define API or MCP schemas
- test expectations for behavior contracts
- explicit non-goals

Technical specs must not include:

- code
- database DDL, migrations, indexes, or query strategy
- concrete REST routes, DTOs, guards, request/response shapes, or transport details
- concrete MCP tool schemas or handler designs
- authentication provider integration details
- performance targets or benchmark design
- generated document rendering formats
- broad architecture essays duplicated from Meridian

## Fidelity checklist

Before considering a technical spec complete:

- [ ] The spec is short enough that Meridian remains the detailed source of truth.
- [ ] The spec points to the functional spec for business intent.
- [ ] The authority chain includes Meridian, the functional spec, the constitution, and relevant `AGENTS.md` files.
- [ ] Every technical rule prevents a concrete implementation mistake.
- [ ] Human-control and delegation rules are explicit when present in Meridian.
- [ ] Lifecycle and merge/promotion behavior cannot be misread.
- [ ] Graph, lineage, branch, and provenance invariants are preserved when relevant.
- [ ] Non-goals exclude code, DDL, REST/MCP schemas, auth-provider details, and performance targets.
- [ ] A rubber-duck review has checked fidelity against Meridian after compression.

## Review and iteration loop

After drafting or revising a technical spec:

1. Re-read the spec against the technical-spec checklist.
2. Compare each implementation boundary decision, lifecycle contract, protected operation, and
   invariant against the functional spec and the current Meridian neighbourhoods used for the work.
3. Run a rubber-duck review focused on lost Meridian fidelity, overreach, missing non-negotiable
   invariants, and implementation ambiguity introduced by compression.
4. Apply every substantive correction from the review while keeping the spec minimal.
5. Repeat the review-and-correction loop until the reviewer finds no blocking or substantive
   non-blocking fidelity issues.
6. Only then report the spec as complete.
