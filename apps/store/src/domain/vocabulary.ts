/**
 * Spool workspace vocabulary.
 *
 * This module defines the shared business vocabulary for Spool's core domain.
 * Every concept is workspace-scoped: every idea, branch, relationship,
 * suggestion, feedback item, artifact, notification, and generated context
 * belongs to exactly one workspace.
 *
 * Sources of authority:
 * - Functional spec:  docs/specifications/feature-01-core-domain-model/functional-specification.md
 * - Technical spec:   docs/specifications/feature-01-core-domain-model/technical-specification.md
 * - Constitution:     docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian:         workspace dbb786ac-1d61-41c9-a46a-2c279dd50cc3
 *
 * Story: S01 — Use a shared workspace vocabulary.
 */

// ─── Vocabulary validation error ─────────────────────────────────────────────

/**
 * Thrown when a vocabulary value object cannot be constructed from invalid
 * input (e.g. an empty or whitespace-only identifier string).
 */
export class VocabularyValidationError extends Error {
  override readonly name = 'VocabularyValidationError';

  constructor(
    readonly concept: string,
    readonly reason: string,
  ) {
    super(`${concept}: ${reason}`);
  }
}

// ─── Branded identifiers ─────────────────────────────────────────────────────
//
// Each identifier is a distinct named type so callers cannot accidentally
// use one in place of another (Constitution IV — primitive obsession is
// discouraged). The __tag discriminant exists only at the type level and is
// never present on runtime values.

/**
 * Uniquely identifies a workspace (tenant).
 *
 * Every idea, branch, relationship, suggestion, feedback item, artifact,
 * notification, and generated context belongs to exactly one workspace.
 *
 * Technical spec: "Workspace scoping" implementation boundary decision.
 */
export type WorkspaceId = string & { readonly __tag: 'WorkspaceId' };

/**
 * Uniquely identifies a stakeholder (human team member or participant).
 *
 * Every protected operation must be attributable to a human stakeholder.
 *
 * Technical spec: "Human accountability" decision.
 * Meridian: IDEA-40, IDEA-42, IDEA-57
 */
export type StakeholderId = string & { readonly __tag: 'StakeholderId' };

/**
 * Uniquely identifies a branch within a workspace.
 *
 * A branch is owned by a single discipline and records a divergence point
 * from the mainline at the time it is created. Branch boundaries persist
 * through merge, preserving provenance and merge lineage.
 *
 * Meridian: IDEA-17 (promoted)
 * Technical spec: "Branch ownership" decision.
 */
export type BranchId = string & { readonly __tag: 'BranchId' };

/**
 * The logical label for an idea chunk (e.g. "IDEA-17").
 *
 * Graph edges reference chunks by their logical label, not by storage UUID.
 * This ensures that relationships survive branch overrides and mainline
 * promotions without rewriting foreign keys.
 *
 * Meridian: IDEA-36 (ADR), IDEA-37 (ADR)
 * Technical spec: "Logical edge endpoints" decision.
 */
export type IdeaLabel = string & { readonly __tag: 'IdeaLabel' };

/**
 * Uniquely identifies a suggestion.
 *
 * A suggestion is an AI or external feedback item that has been captured in
 * the workspace's review queue and is awaiting a human decision (accept or
 * reject). Accepting a suggestion initialises a discipline-scoped feedback
 * branch.
 *
 * Meridian: IDEA-28 (promoted)
 * Technical spec: "Delegated agents" decision; "Suggestion" lifecycle.
 */
export type SuggestionId = string & { readonly __tag: 'SuggestionId' };

/**
 * Uniquely identifies a feedback item submitted by an external system or
 * AI agent before it is captured as a suggestion.
 *
 * Feedback items are the raw submissions from external sources. Once
 * captured into the workspace's review queue they become suggestions and
 * are subject to human review before influencing the graph.
 *
 * Meridian: IDEA-28 (promoted)
 * Functional spec: "Business value" — feedback is a first-class vocabulary concept.
 */
export type FeedbackItemId = string & { readonly __tag: 'FeedbackItemId' };

/**
 * Uniquely identifies an artifact produced in a workspace.
 *
 * Artifacts are outputs associated with workspace activity (for example,
 * exported documents or generated files). They belong to the workspace that
 * produced them.
 *
 * Functional spec: "Business value" — artifacts are a named workspace concept.
 */
export type ArtifactId = string & { readonly __tag: 'ArtifactId' };

/**
 * Uniquely identifies a notification delivered to a stakeholder.
 *
 * Notifications are scoped to the workspace that generated them. A
 * stakeholder's delivery preferences determine how and when they receive
 * workspace notifications.
 *
 * Functional spec: "Business value" — notifications and delivery preferences
 * are named vocabulary concepts.
 */
export type NotificationId = string & { readonly __tag: 'NotificationId' };

/**
 * Uniquely identifies a generated context package.
 *
 * Generated context is a projection from approved or promoted idea chunks
 * and their active, resolved, label-based relationships. It is not the
 * source of truth — approved chunks and edges are.
 *
 * Technical spec: "Generated context" decision.
 * Meridian: IDEA-36, IDEA-37, IDEA-38
 */
export type GeneratedContextId = string & { readonly __tag: 'GeneratedContextId' };

// ─── Domain enumerations ─────────────────────────────────────────────────────

/**
 * The discipline (area of ownership) that a branch or chunk belongs to.
 *
 * A branch belongs to exactly one discipline for its lifetime. A branch may
 * only modify chunks and edges owned by its discipline, or create
 * cross-disciplinary edges to other disciplines' chunks (without modifying
 * those target chunks).
 *
 * Meridian: IDEA-17, IDEA-35, IDEA-40
 * Technical spec: "Branch ownership" and "Discipline boundary" decisions.
 */
export type Discipline = 'product' | 'architecture' | 'design' | 'engineering';

/**
 * Classifies the kind of an idea chunk.
 *
 * Technical spec: "Rich domain model" required concept.
 * Values observed in Meridian workspace dbb786ac-1d61-41c9-a46a-2c279dd50cc3.
 */
export type ChunkType = 'feature' | 'capability' | 'constraint' | 'adr' | 'spike';

/**
 * Whether a chunk's context is a permanent decision record or a transient
 * working note.
 *
 * Technical spec: "Rich domain model" required concept.
 */
export type ContextKind = 'permanent' | 'transient';

/**
 * The semantic type of a relationship (graph edge) between two idea chunks.
 *
 * Edges are defined between idea labels, not storage row UUIDs. This means
 * the same relationship type remains meaningful after branch overrides and
 * mainline promotions.
 *
 * Meridian: IDEA-36 (ADR), IDEA-37 (ADR), IDEA-38 (ADR)
 * Technical spec: "Logical edge endpoints", "Edge determinism", "Edge
 * lineage" decisions.
 */
export type RelationshipType =
  | 'refines'
  | 'depends-on'
  | 'supersedes'
  | 'implements'
  | 'informs';

// ─── Lifecycle states ─────────────────────────────────────────────────────────

/**
 * The progression stage of an idea chunk on the path to mainline.
 *
 * draft → approved → promoted
 *
 * Technical spec: "Required lifecycle contracts" — Chunk.
 */
export type ChunkLifecycleState = 'draft' | 'approved' | 'promoted';

/**
 * The activity state of an idea chunk, independent of its lifecycle stage.
 *
 * An approved or promoted chunk may become superseded or inactive without
 * losing history. Approval state and activity state are tracked separately.
 *
 * Technical spec: "Required lifecycle contracts" — Chunk (activity state is
 * separate from lifecycle stage).
 */
export type ChunkActivityState = 'active' | 'superseded' | 'inactive';

/**
 * The lifecycle state of a branch.
 *
 * draft → submitted → verified → merged (terminal)
 *
 * Transitions from submitted or verified back to draft are human-initiated
 * only and are never automated. The merged state is terminal.
 *
 * Technical spec: "Required lifecycle contracts" — Branch.
 * Meridian: IDEA-40 (human-only submission gate), IDEA-42 (human-only
 * merge), IDEA-43 (manual verified transition), IDEA-57 (autonomous agents
 * forbidden from merging).
 */
export type BranchState = 'draft' | 'submitted' | 'verified' | 'merged';

/**
 * The lifecycle state of a suggestion.
 *
 * pending → accepted (terminal) or rejected (terminal)
 *
 * Accepting a suggestion initialises a discipline-scoped feedback branch.
 * Rejecting a suggestion must not modify graph state.
 *
 * Technical spec: "Required lifecycle contracts" — Suggestion.
 * Meridian: IDEA-28 (accepted suggestion initialises a feedback branch).
 */
export type SuggestionState = 'pending' | 'accepted' | 'rejected';

/**
 * The state of a graph edge (relationship) between two idea chunks.
 *
 * Mainline edges are never destructively deleted. A relationship change
 * creates a new edge version and marks the previous one as superseded,
 * preserving an unbroken lineage chain.
 *
 * Technical spec: "Required lifecycle contracts" — Edge.
 * Meridian: IDEA-38 (immutable mainline edges, supersession for changes).
 */
export type EdgeState = 'active' | 'deactivated' | 'superseded';

// ─── Actor context ─────────────────────────────────────────────────────────

/**
 * Whether an actor is a direct human stakeholder or a supervised delegate.
 *
 * Meridian: IDEA-40, IDEA-57
 */
export type ActorKind = 'human' | 'delegated';

/**
 * Describes who performed or initiated an action.
 *
 * **This is descriptive provenance metadata, not proof of human
 * authentication.** Protected operations (approve chunk, accept/reject
 * suggestion, submit branch, verify branch, merge branch) must verify
 * human authentication through session credentials — never by inspecting
 * this field alone.
 *
 * Technical spec: "Human accountability" decision; "Delegated agents"
 * decision; "Protected operation contracts".
 * Meridian: IDEA-40, IDEA-42, IDEA-57
 */
export interface ActorContext {
  readonly kind: ActorKind;
  readonly stakeholderId: StakeholderId;
}

/** An actor verified as a direct human stakeholder. */
export type HumanActorContext = ActorContext & { readonly kind: 'human' };

/** An actor acting as a supervised delegate under a human session token. */
export type DelegatedActorContext = ActorContext & { readonly kind: 'delegated' };

// ─── Workspace scoping ──────────────────────────────────────────────────────

/**
 * A value that explicitly belongs to a single workspace.
 *
 * Every concept in Spool — ideas, branches, relationships, suggestions,
 * feedback items, artifacts, notifications, and generated context — is
 * workspace-scoped. This container makes that membership explicit and
 * visible, enabling workspace isolation checks at application boundaries.
 *
 * Workspace isolation (the guarantee that one workspace's knowledge is never
 * treated as belonging to another) is enforced by application-layer logic
 * that compares workspaceId values at every boundary crossing. This type
 * provides the vocabulary for that check.
 *
 * Technical spec: "Workspace scoping" decision.
 */
export interface WorkspaceScoped<T> {
  readonly workspaceId: WorkspaceId;
  readonly value: T;
}

// ─── Constructor functions ───────────────────────────────────────────────────

function trimAndValidate(concept: string, value: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new VocabularyValidationError(
      concept,
      'identifier cannot be empty or whitespace',
    );
  }
  return trimmed;
}

/** Creates a WorkspaceId from a non-empty string. Leading/trailing whitespace is trimmed. */
export function workspaceId(value: string): WorkspaceId {
  return trimAndValidate('WorkspaceId', value) as WorkspaceId;
}

/** Creates a StakeholderId from a non-empty string. Leading/trailing whitespace is trimmed. */
export function stakeholderId(value: string): StakeholderId {
  return trimAndValidate('StakeholderId', value) as StakeholderId;
}

/** Creates a BranchId from a non-empty string. Leading/trailing whitespace is trimmed. */
export function branchId(value: string): BranchId {
  return trimAndValidate('BranchId', value) as BranchId;
}

/** Creates an IdeaLabel from a non-empty string. Leading/trailing whitespace is trimmed. */
export function ideaLabel(value: string): IdeaLabel {
  return trimAndValidate('IdeaLabel', value) as IdeaLabel;
}

/** Creates a SuggestionId from a non-empty string. Leading/trailing whitespace is trimmed. */
export function suggestionId(value: string): SuggestionId {
  return trimAndValidate('SuggestionId', value) as SuggestionId;
}

/** Creates a FeedbackItemId from a non-empty string. Leading/trailing whitespace is trimmed. */
export function feedbackItemId(value: string): FeedbackItemId {
  return trimAndValidate('FeedbackItemId', value) as FeedbackItemId;
}

/** Creates an ArtifactId from a non-empty string. Leading/trailing whitespace is trimmed. */
export function artifactId(value: string): ArtifactId {
  return trimAndValidate('ArtifactId', value) as ArtifactId;
}

/** Creates a NotificationId from a non-empty string. Leading/trailing whitespace is trimmed. */
export function notificationId(value: string): NotificationId {
  return trimAndValidate('NotificationId', value) as NotificationId;
}

/** Creates a GeneratedContextId from a non-empty string. Leading/trailing whitespace is trimmed. */
export function generatedContextId(value: string): GeneratedContextId {
  return trimAndValidate('GeneratedContextId', value) as GeneratedContextId;
}

/** Wraps a value with an explicit workspace membership. */
export function inWorkspace<T>(id: WorkspaceId, value: T): WorkspaceScoped<T> {
  return { workspaceId: id, value };
}

/** Creates a HumanActorContext. */
export function humanActor(id: StakeholderId): HumanActorContext {
  return { kind: 'human', stakeholderId: id };
}

/** Creates a DelegatedActorContext. */
export function delegatedActor(id: StakeholderId): DelegatedActorContext {
  return { kind: 'delegated', stakeholderId: id };
}

/** Returns true if the actor is a direct human stakeholder. */
export function isHumanActor(actor: ActorContext): actor is HumanActorContext {
  return actor.kind === 'human';
}

/** Returns true if the actor is a supervised delegate. */
export function isDelegatedActor(
  actor: ActorContext,
): actor is DelegatedActorContext {
  return actor.kind === 'delegated';
}
