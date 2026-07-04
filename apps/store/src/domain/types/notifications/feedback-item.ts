/**
 * Generic evaluation-feedback domain record: free-text human/agent review
 * commentary associated with the branch it evaluated, distinct from a
 * structured `VerificationSignal` outcome.
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-02-postgres-persistence/stories/S09-feedback-and-verification-notifications-remembered.md
 * - Technical spec: docs/specifications/feature-02-postgres-persistence/technical-specification.md
 *                   §"Feedback notification routing", §"Protected operation contracts"
 * - Constitution:   docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian:       IDEA-67, IDEA-68
 *
 * Story: S09 — Remember feedback and verification notifications without
 * losing the record.
 *
 * AC5: provenance is always derived from the authenticated `ActorContext`
 * (`authoredByStakeholderId: actor.stakeholderId`) — there is no parameter
 * through which a caller can substitute a different, client-claimed
 * stakeholder id. Both human and delegated actors may author feedback
 * (feature-01 "Delegated agents": "AI agents and external systems may submit
 * feedback and act as supervised delegates") — recording feedback is not
 * itself a protected operation, so this function never calls
 * `assertHumanActor`, matching `recordVerificationSignal`'s precedent.
 */

import type { ActorContext } from '../actor/actor-context.js';
import type { ActorKind } from '../actor/actor-kind.js';
import { VocabularyValidationError } from '../errors/vocabulary-validation-error.js';
import type { BranchId } from '../identifiers/branch-id.js';
import type { FeedbackItemId } from '../identifiers/feedback-item-id.js';
import type { StakeholderId } from '../identifiers/stakeholder-id.js';
import type { WorkspaceId } from '../identifiers/workspace-id.js';

/**
 * Minimal validated branch-target shape required to record feedback against
 * a branch. Mirrors `VerificationTargetBranch` in `../verification/verification-signal.js`
 * so a real `BranchOwnership` satisfies it structurally.
 */
export interface FeedbackTargetBranch {
  readonly branchId: BranchId;
  readonly workspaceId: WorkspaceId;
}

/**
 * An evaluation feedback item — free-text review commentary from a
 * stakeholder or delegated agent, associated with the branch it evaluated.
 *
 * AC1: carries `workspaceId`/`branchId` so a stakeholder can see feedback
 * associated with the branch it evaluated.
 *
 * The `__tag` field is a nominal-typing brand (matching this codebase's
 * `string & { readonly __tag: ... }` identifier-branding convention, e.g.
 * `BranchId`/`WorkspaceId`) that only `recordFeedbackItem` below can
 * produce. Without it, a hand-built object literal matching this shape does
 * not structurally satisfy `FeedbackItem` and cannot be passed to
 * `NotificationRepository.submitFeedbackItem` without an explicit,
 * deliberate `as unknown as FeedbackItem` cast — closing a gap found during
 * rubber-duck review of Feature 01/02 against Meridian, where the plain
 * structural interface let a caller construct a spoofed
 * `authoredByStakeholderId` without going through the actor-derived factory.
 */
export type FeedbackItem = {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly feedbackItemId: FeedbackItemId;
  readonly authoredByStakeholderId: StakeholderId;
  readonly authoredByActorKind: ActorKind;
  readonly submittedAt: string;
  readonly content: string;
} & { readonly __tag: 'FeedbackItem' };

/**
 * Records an evaluation feedback item against a branch.
 *
 * Throws `VocabularyValidationError` if `submittedAt` is empty,
 * whitespace-only, or not a valid ISO-8601 date string.
 * Throws `VocabularyValidationError` if `content` is empty or
 * whitespace-only.
 */
export function recordFeedbackItem(
  actor: ActorContext,
  branch: FeedbackTargetBranch,
  feedbackItemId: FeedbackItemId,
  submittedAt: string,
  content: string,
): FeedbackItem {
  const trimmedAt = submittedAt.trim();
  if (
    !trimmedAt ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) ||
    Number.isNaN(Date.parse(trimmedAt))
  ) {
    throw new VocabularyValidationError(
      'FeedbackItem.submittedAt',
      'submittedAt must be a valid ISO-8601 date string',
    );
  }
  const trimmedContent = content.trim();
  if (!trimmedContent) {
    throw new VocabularyValidationError(
      'FeedbackItem.content',
      'content cannot be empty or whitespace',
    );
  }
  return Object.freeze({
    workspaceId: branch.workspaceId,
    branchId: branch.branchId,
    feedbackItemId,
    authoredByStakeholderId: actor.stakeholderId,
    authoredByActorKind: actor.kind,
    submittedAt: trimmedAt,
    content: trimmedContent,
  }) as FeedbackItem;
}
