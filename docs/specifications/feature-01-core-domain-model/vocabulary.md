<!--
SYNC IMPACT REPORT
==================
Version change: (new) -> 1.0.0
Bump rationale: Initial vocabulary definition for Feature 01: Core Domain Model.

Concepts defined:
  - Workspace, Stakeholder, Idea chunk (Idea), Idea label, Discipline
  - Branch, Relationship (Edge), Suggestion, Feedback item
  - Artifact, Notification, Generated context
  - Lifecycle states: Chunk, Branch, Suggestion, Edge
  - Actor context: human vs. delegated

Follow-up TODOs: none.
-->

# Spool Workspace Vocabulary

This document is the authoritative stakeholder reference for the shared business
vocabulary of Spool. It defines every concept that stakeholders, supervised agents,
and implementation teams use when discussing, reviewing, or building Spool knowledge.

**Principle: everything belongs to a workspace.**  
Every idea, branch, relationship, suggestion, feedback item, artifact, notification,
and generated context belongs to exactly one workspace. One workspace's knowledge is
never treated as belonging to another workspace.

## Sources of authority

| Source | Role |
| --- | --- |
| Meridian workspace `dbb786ac-1d61-41c9-a46a-2c279dd50cc3` | Primary source of truth for all domain detail. If this document and Meridian disagree, Meridian wins and this document must be corrected. |
| `docs/specifications/feature-01-core-domain-model/functional-specification.md` | Business value, stakeholder outcomes, and feature scope. |
| `docs/specifications/feature-01-core-domain-model/technical-specification.md` | Architecture decisions, lifecycle contracts, and implementation constraints. |
| `docs/constitution.md` | Repository engineering rules that govern how vocabulary is implemented in code. |

---

## Core concepts

### Workspace

A workspace is the top-level container for all of a team's knowledge.
Every idea, branch, relationship, suggestion, feedback item, artifact, notification,
and generated context belongs to exactly one workspace.

- Workspaces are isolated from each other. An idea in workspace A is not visible or
  reachable from workspace B.
- Each workspace has its own set of stakeholders, disciplines, and knowledge graph.

**Implementation type:** `WorkspaceId`  
**Technical spec:** "Workspace scoping" decision  

---

### Stakeholder

A stakeholder is a human participant in a workspace — a person who creates, reviews,
approves, or manages ideas.

- Only human stakeholders can perform protected operations: approving an idea,
  accepting or rejecting a suggestion, submitting, verifying, or merging a branch.
- AI agents may act as supervised delegates under a human stakeholder's session but
  cannot perform protected operations.

**Implementation type:** `StakeholderId`, `ActorContext`  
**Technical spec:** "Human accountability" decision  
**Meridian:** IDEA-40, IDEA-42, IDEA-57  

---

### Idea (chunk)

An idea is an atomic, discrete unit of knowledge in a workspace. Ideas are the nodes
in the workspace's knowledge graph. Examples include a product feature, an
architectural decision record (ADR), a design constraint, or an engineering spike.

An idea has two orthogonal state dimensions:

- **Lifecycle stage** (`draft → approved → promoted`): where the idea is on the path
  to becoming approved implementation context.
- **Activity state** (`active`, `superseded`, `inactive`): whether the idea is
  currently the active version, has been superseded by a newer version, or is no
  longer active.

**Implementation type:** `IdeaLabel`, `ChunkType`, `ChunkLifecycleState`,
`ChunkActivityState`  
**Technical spec:** "Required lifecycle contracts — Chunk"  

---

### Idea label

An idea label is the logical identifier for an idea (for example, `IDEA-17`). Labels
identify ideas in relationships and are stable across branch overrides and mainline
promotions. Edges use idea labels as endpoints, not storage identifiers.

**Implementation type:** `IdeaLabel`  
**Technical spec:** "Logical edge endpoints" decision  
**Meridian:** IDEA-36 (ADR), IDEA-37 (ADR)  

---

### Discipline

A discipline is an area of ownership within a workspace. Every branch belongs to
exactly one discipline for its entire lifetime. A branch may modify ideas and
relationships owned by its discipline. It may create cross-disciplinary relationships
to other disciplines' ideas, but it must not modify those target ideas.

Recognised disciplines: **product**, **architecture**, **design**, **engineering**.

**Implementation type:** `Discipline`  
**Technical spec:** "Branch ownership" and "Discipline boundary" decisions  
**Meridian:** IDEA-17, IDEA-35, IDEA-40  

---

### Branch

A branch is a unit of proposed change, owned by a single discipline. It diverges from
the mainline at the time it is created and evolves independently until it is submitted
for review, verified, and merged.

**Branch lifecycle:**

| State | Meaning |
| --- | --- |
| `draft` | The branch is being worked on. Graph writes are permitted. |
| `submitted` | The branch has been submitted for verification. Graph writes are locked; only metadata such as verification signals may be appended. Human-only transition. |
| `verified` | A human stakeholder has reviewed advisory signals and declared the branch ready to merge. Human-only transition. |
| `merged` | The branch has been merged into the mainline. Terminal state. Human-only. |

Return-to-draft transitions (from `submitted` or `verified`) are always human-initiated
and never automated. The `merged` state is terminal.

**Implementation type:** `BranchId`, `BranchState`  
**Technical spec:** "Branch ownership", "Branch graph view", "Required lifecycle
contracts — Branch"  
**Meridian:** IDEA-17 (promoted), IDEA-40, IDEA-42, IDEA-43, IDEA-57  

---

### Relationship (edge)

A relationship is a typed, directed connection between two ideas in the workspace
graph. Relationships are defined using idea labels as endpoints — not storage
identifiers — so they survive branch overrides and mainline promotions without
being rewritten.

Recognised relationship types: **refines**, **depends-on**, **supersedes**,
**implements**, **informs**.

**Relationship (edge) states:**

| State | Meaning |
| --- | --- |
| `active` | The relationship is currently in effect. |
| `deactivated` | The relationship has been explicitly deactivated but its history is preserved. |
| `superseded` | The relationship has been replaced by a newer version; the lineage chain is intact. |

Mainline relationships are never destructively deleted. Changing a mainline
relationship creates a new version and marks the previous one as superseded,
preserving an unbroken lineage chain.

**Implementation type:** `RelationshipType`, `EdgeState`  
**Technical spec:** "Logical edge endpoints", "Edge determinism", "Edge lineage"
decisions  
**Meridian:** IDEA-36 (ADR), IDEA-37 (ADR), IDEA-38 (ADR)  

---

### Suggestion

A suggestion is an AI or external feedback item that has been captured in the
workspace's review queue and is awaiting a human decision.

**Suggestion lifecycle:**

| State | Meaning |
| --- | --- |
| `pending` | The suggestion is awaiting human review. |
| `accepted` | A human stakeholder accepted the suggestion, initialising a discipline-scoped feedback branch. Terminal. |
| `rejected` | A human stakeholder rejected the suggestion. Graph state is not modified. Terminal. |

Only human stakeholders can accept or reject suggestions. Supervised delegates
cannot decide on suggestions.

**Implementation type:** `SuggestionId`, `SuggestionState`  
**Technical spec:** "Required lifecycle contracts — Suggestion"; "Protected operation
contracts — Accept suggestion, Reject suggestion"  
**Meridian:** IDEA-28 (promoted)  

---

### Feedback item

A feedback item is a raw submission from an external system or AI agent, made before
it is captured as a suggestion and placed in the human review queue. Once captured,
the feedback item becomes a suggestion.

**Implementation type:** `FeedbackItemId`  
**Meridian:** IDEA-28 (promoted)  
**Functional spec:** "Business value" — feedback is a named vocabulary concept  

---

### Artifact

An artifact is an output produced in the context of workspace activity, such as an
exported document, a generated diagram, or a produced file. Artifacts belong to the
workspace that produced them.

**Implementation type:** `ArtifactId`  
**Functional spec:** "Business value" — artifacts are a named vocabulary concept  

---

### Notification

A notification is a message delivered to a stakeholder about an event in the
workspace. Notifications are scoped to the workspace that generated them. A
stakeholder's delivery preferences determine how and when they receive notifications.

**Implementation type:** `NotificationId`  
**Functional spec:** "Business value" — notifications and delivery preferences are
named vocabulary concepts  

---

### Generated context

A generated context package is a projection derived from approved or promoted ideas
and their active, resolved, label-based relationships. It provides implementation
agents with traceable, workspace-scoped knowledge.

Generated context is **not** the source of truth. The approved ideas and active
relationships in the knowledge graph are the source of truth; the generated context is
a read-only view derived from them.

**Implementation type:** `GeneratedContextId`, `ContextKind`  
**Technical spec:** "Generated context" decision  
**Meridian:** IDEA-36, IDEA-37, IDEA-38  

---

### Actor context

Actor context describes who performed or initiated an action. It distinguishes between
a direct human stakeholder and a supervised AI delegate acting under a human session
token.

| Kind | Meaning |
| --- | --- |
| `human` | A direct human stakeholder performing an action with their own credentials. |
| `delegated` | An AI agent or automated system acting as a supervised delegate under a human session token. |

> **Important:** Actor context is descriptive provenance metadata. It is not proof
> of direct human authentication. Protected operations must verify human authentication
> through session credentials — never by inspecting the actor kind field alone.

**Implementation type:** `ActorContext`, `ActorKind`, `HumanActorContext`,
`DelegatedActorContext`  
**Technical spec:** "Human accountability" decision; "Delegated agents" decision;
"Protected operation contracts"  
**Meridian:** IDEA-40, IDEA-42, IDEA-57  

---

## Traceability table

This table maps every named concept to its definition source in each authority.

| Concept | Business definition | Functional spec reference | Technical spec reference | Meridian reference |
| --- | --- | --- | --- | --- |
| Workspace | Top-level tenant container for all workspace knowledge | "Business value", AC1–5 | "Workspace scoping" decision | Workspace `dbb786ac-...` root |
| Stakeholder | Human participant with credentials | "Business value" | "Human accountability", "Delegated agents" | IDEA-40, IDEA-42, IDEA-57 |
| Idea (chunk) | Atomic unit of knowledge; node in the graph | "Business value", outcome 1–2 | "Rich domain model"; "Required lifecycle contracts — Chunk" | IDEA-17 |
| Idea label | Logical identifier for an idea; edge endpoint | Outcome 5 | "Logical edge endpoints", "Edge determinism" | IDEA-36, IDEA-37 |
| Discipline | Area of ownership for a branch | Outcome 3 | "Branch ownership", "Discipline boundary" | IDEA-17, IDEA-35, IDEA-40 |
| Branch | Single-discipline unit of proposed change | Outcome 2–3 | "Branch ownership", "Branch graph view", "Required lifecycle contracts — Branch" | IDEA-17, IDEA-40, IDEA-42, IDEA-43, IDEA-57 |
| Relationship (edge) | Typed directed link between two ideas | Outcome 5 | "Logical edge endpoints", "Edge lineage", "Required lifecycle contracts — Edge" | IDEA-36, IDEA-37, IDEA-38 |
| Suggestion | Captured AI/external feedback awaiting human review | "Business value", AC4 | "Delegated agents", "Protected operation contracts", "Required lifecycle contracts — Suggestion" | IDEA-28 |
| Feedback item | Raw external/AI submission before capture as suggestion | "Business value" | — | IDEA-28 |
| Artifact | Workspace-scoped output produced from knowledge activity | "Business value" | — | — |
| Notification | Stakeholder delivery event scoped to a workspace | "Business value" | — | — |
| Generated context | Read-only projection from approved ideas and active relationships | Outcome 6, AC5 | "Generated context" decision | IDEA-36, IDEA-37, IDEA-38 |
| Actor context | Provenance of an action (human vs. supervised delegate) | AC4 | "Human accountability", "Delegated agents", "Protected operation contracts" | IDEA-40, IDEA-42, IDEA-57 |

---

**Version:** 1.0.0  
**Story:** S01 — Use a shared workspace vocabulary  
**Ratified:** 2026-06-29  
