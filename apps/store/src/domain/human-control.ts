/**
 * Human-control domain invariants and protected operation contracts.
 *
 * This module implements the protected operation contracts for the three
 * remaining human-only operations not covered by branch-lifecycle.ts:
 *
 *   - Chunk approval        (approveChunk)
 *   - Suggestion acceptance (acceptSuggestion)
 *   - Suggestion rejection  (rejectSuggestion)
 *
 * It also exports the reusable `assertHumanActor` guard and `HumanControlError`
 * so adapters can map the `'unauthorized-actor'` category without inspecting
 * free-form message text.
 *
 * **IMPORTANT — provenance, not authentication proof:**
 * `ActorContext.kind` is descriptive provenance metadata. It is NOT proof of
 * human authentication. Protected operations must be authenticated through
 * human-scoped session credentials at the application boundary. The domain
 * guard here is defense-in-depth only.
 *
 * Technical spec §"Human accountability": every approval, suggestion decision,
 * verification decision, and merge decision must be attributable to a human
 * stakeholder ID.
 * Technical spec §"Delegated agents": delegated sessions cannot approve chunks,
 * accept or reject suggestions, submit branches, verify branches, or merge
 * branches.
 * Technical spec §"Protected operation contracts".
 * Technical spec §"Required domain error categories" #3: unauthorized actor.
 * Meridian IDEA-28, IDEA-40, IDEA-42, IDEA-57.
 *
 * Story: S04 — Preserve human control over accountable decisions.
 */

import type {
  ActorContext,
  Discipline,
  HumanActorContext,
  IdeaLabel,
  StakeholderId,
  SuggestionId,
  WorkspaceId,
} from './vocabulary.js';
import { VocabularyValidationError, isHumanActor } from './vocabulary.js';

export type {
  ActorContext,
  Discipline,
  HumanActorContext,
  IdeaLabel,
  StakeholderId,
  SuggestionId,
  WorkspaceId,
};

// ─── Domain error ─────────────────────────────────────────────────────────────

/**
 * Thrown when a delegated actor attempts a human-only protected operation
 * (chunk approval, suggestion acceptance, or suggestion rejection).
 *
 * The `code` property is stable and machine-readable. Adapters must map
 * domain failures by code, never by inspecting free-form message text.
 *
 * Technical spec §"Required domain error categories" #3: unauthorized actor.
 */
export class HumanControlError extends Error {
  override readonly name = 'HumanControlError';
  readonly code = 'unauthorized-actor' as const;

  constructor(readonly reason: string) {
    super(reason);
  }
}

// ─── Guard ────────────────────────────────────────────────────────────────────

/**
 * Asserts that the given actor is a direct human stakeholder.
 *
 * Throws `HumanControlError` with code `'unauthorized-actor'` if the actor
 * is a supervised delegate.
 *
 * **This is a defense-in-depth invariant.** Real authentication must happen at
 * the application boundary through human-scoped session credentials, not by
 * inspecting `actor.kind` alone.
 *
 * Technical spec §"Protected operation contracts":
 * "Human authentication for protected operations must come from human-scoped
 * session credentials. Self-reported delegation … must never be accepted as
 * proof of direct human authentication."
 * Meridian IDEA-40, IDEA-57.
 */
export function assertHumanActor(
  actor: ActorContext,
  operation: string,
): asserts actor is HumanActorContext {
  if (!isHumanActor(actor)) {
    throw new HumanControlError(
      `only a direct human stakeholder may ${operation}; delegated actors cannot perform this operation`,
    );
  }
}

// ─── Chunk approval ───────────────────────────────────────────────────────────

/**
 * Immutable accountability record for a chunk approval.
 *
 * Carries the workspace, chunk's idea label, the human stakeholder who
 * approved it, and the approval timestamp, so a stakeholder can always tell
 * which human is accountable for which workspace's decision.
 *
 * Technical spec §"Human accountability": "Every … approval … must be
 * attributable to a human stakeholder ID."
 * Technical spec §"Workspace scoping": every operation is workspace-scoped.
 * Technical spec §"Protected operation contracts": "Approve chunk — Delegated
 * agents cannot be the approving actor."
 * S04 AC1.
 */
export interface ChunkApprovalRecord {
  readonly workspaceId: WorkspaceId;
  readonly chunkLabel: IdeaLabel;
  readonly approvedByStakeholderId: StakeholderId;
  readonly approvedAt: string;
}

/**
 * Creates a frozen ChunkApprovalRecord for a human-approved chunk.
 *
 * Throws `HumanControlError('unauthorized-actor')` if the actor is a
 * supervised delegate.
 * Throws `VocabularyValidationError` if `approvedAt` is not a valid
 * ISO-8601 date string.
 *
 * Technical spec §"Protected operation contracts": "Approve chunk".
 * S04 AC1, AC3.
 */
export function approveChunk(
  actor: ActorContext,
  workspaceId: WorkspaceId,
  chunkLabel: IdeaLabel,
  approvedAt: string,
): ChunkApprovalRecord {
  assertHumanActor(actor, 'approve a chunk');
  const trimmedAt = approvedAt.trim();
  if (
    !trimmedAt ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) ||
    isNaN(Date.parse(trimmedAt))
  ) {
    throw new VocabularyValidationError(
      'ChunkApprovalRecord.approvedAt',
      'approvedAt must be a valid ISO-8601 date string',
    );
  }
  return Object.freeze({
    workspaceId,
    chunkLabel,
    approvedByStakeholderId: actor.stakeholderId,
    approvedAt: trimmedAt,
  });
}

// ─── Suggestion decisions ─────────────────────────────────────────────────────

/**
 * Immutable accountability record for an accepted suggestion.
 *
 * Carries the workspace, suggestion ID, the deciding human stakeholder,
 * timestamp, and the discipline for the linked feedback branch, so a
 * stakeholder can always tell which human made which workspace's acceptance
 * decision.
 *
 * Acceptance creates a linked feedback branch scoped to
 * `feedbackBranchDiscipline`. The record captures the human decision: which
 * suggestion was accepted, by whom, when, and for which discipline's feedback
 * branch.
 *
 * The `feedbackBranchDiscipline` identifies which discipline's feedback branch
 * should be initialised from this suggestion. Actual branch creation is the
 * responsibility of the application service layer.
 *
 * Technical spec §"Protected operation contracts": "Accept suggestion —
 * Requires a direct human-authenticated stakeholder and creates a linked
 * feedback branch scoped to one discipline. Delegated agents cannot decide."
 * Technical spec §"Workspace scoping": every operation is workspace-scoped.
 * Meridian IDEA-28.
 * S04 AC1, AC4.
 */
export interface SuggestionAcceptedDecision {
  readonly decision: 'accepted';
  readonly workspaceId: WorkspaceId;
  readonly suggestionId: SuggestionId;
  readonly decidedByStakeholderId: StakeholderId;
  readonly decidedAt: string;
  readonly feedbackBranchDiscipline: Discipline;
}

/**
 * Immutable accountability record for a rejected suggestion.
 *
 * Carries the workspace, suggestion ID, the deciding human stakeholder, and
 * timestamp. Rejection must not modify graph state.
 *
 * Technical spec §"Protected operation contracts": "Reject suggestion —
 * Requires a direct human-authenticated stakeholder and must not modify graph
 * state. Delegated agents cannot decide."
 * Technical spec §"Workspace scoping": every operation is workspace-scoped.
 * S04 AC1, AC4.
 */
export interface SuggestionRejectedDecision {
  readonly decision: 'rejected';
  readonly workspaceId: WorkspaceId;
  readonly suggestionId: SuggestionId;
  readonly decidedByStakeholderId: StakeholderId;
  readonly decidedAt: string;
}

/**
 * A suggestion decision: either accepted (with a linked feedback branch
 * discipline) or rejected (with no graph state change).
 *
 * The `decision` discriminant allows adapters and callers to branch without
 * inspecting message text.
 *
 * S04 AC4.
 */
export type SuggestionDecision =
  | SuggestionAcceptedDecision
  | SuggestionRejectedDecision;

/**
 * Records a human stakeholder's decision to accept a suggestion.
 *
 * Throws `HumanControlError('unauthorized-actor')` if the actor is a
 * supervised delegate.
 * Throws `VocabularyValidationError` if `decidedAt` is not a valid ISO-8601
 * date string.
 *
 * Technical spec §"Protected operation contracts": "Accept suggestion".
 * Meridian IDEA-28.
 * S04 AC1, AC2, AC3, AC4.
 */
export function acceptSuggestion(
  actor: ActorContext,
  workspaceId: WorkspaceId,
  suggestionId: SuggestionId,
  feedbackBranchDiscipline: Discipline,
  decidedAt: string,
): SuggestionAcceptedDecision {
  assertHumanActor(actor, 'accept a suggestion');
  const trimmedAt = decidedAt.trim();
  if (
    !trimmedAt ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) ||
    isNaN(Date.parse(trimmedAt))
  ) {
    throw new VocabularyValidationError(
      'SuggestionAcceptedDecision.decidedAt',
      'decidedAt must be a valid ISO-8601 date string',
    );
  }
  return Object.freeze({
    decision: 'accepted',
    workspaceId,
    suggestionId,
    decidedByStakeholderId: actor.stakeholderId,
    decidedAt: trimmedAt,
    feedbackBranchDiscipline,
  });
}

/**
 * Records a human stakeholder's decision to reject a suggestion.
 *
 * Throws `HumanControlError('unauthorized-actor')` if the actor is a
 * supervised delegate.
 * Throws `VocabularyValidationError` if `decidedAt` is not a valid ISO-8601
 * date string.
 *
 * Rejection does not modify graph state. This function records only the
 * decision; no graph modification is performed or implied.
 *
 * Technical spec §"Protected operation contracts": "Reject suggestion".
 * S04 AC1, AC2, AC3, AC4.
 */
export function rejectSuggestion(
  actor: ActorContext,
  workspaceId: WorkspaceId,
  suggestionId: SuggestionId,
  decidedAt: string,
): SuggestionRejectedDecision {
  assertHumanActor(actor, 'reject a suggestion');
  const trimmedAt = decidedAt.trim();
  if (
    !trimmedAt ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) ||
    isNaN(Date.parse(trimmedAt))
  ) {
    throw new VocabularyValidationError(
      'SuggestionRejectedDecision.decidedAt',
      'decidedAt must be a valid ISO-8601 date string',
    );
  }
  return Object.freeze({
    decision: 'rejected',
    workspaceId,
    suggestionId,
    decidedByStakeholderId: actor.stakeholderId,
    decidedAt: trimmedAt,
  });
}
