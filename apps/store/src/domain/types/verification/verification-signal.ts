import type { ActorContext } from '../actor/actor-context.js';
import type { ActorKind } from '../actor/actor-kind.js';
import { VocabularyValidationError } from '../errors/vocabulary-validation-error.js';
import type { BranchId } from '../identifiers/branch-id.js';
import type { StakeholderId } from '../identifiers/stakeholder-id.js';
import type { VerificationSignalId } from '../identifiers/verification-signal-id.js';
import type { WorkspaceId } from '../identifiers/workspace-id.js';
import type { VerificationOutcome } from '../lifecycle/verification-outcome.js';

/**
 * Minimal validated branch-target shape required to record a verification
 * signal against a branch. A real `BranchOwnership`
 * (`apps/store/src/domain/branch-lifecycle.ts`) satisfies this structurally,
 * so callers can pass one directly — mirroring `FeedbackBranchOwnership` in
 * `../suggestions/suggestion-feedback-branch-link.ts`.
 */
export interface VerificationTargetBranch {
  readonly branchId: BranchId;
  readonly workspaceId: WorkspaceId;
}

/**
 * An advisory-only record of verification feedback (from an agent, tool, or
 * reviewer) associated with the branch it evaluated.
 *
 * Technical spec §"Verification signals": "Verification feedback is advisory
 * history only. Signals must never automatically verify, unverify, merge,
 * reject, reopen, or otherwise transition a branch." Meridian IDEA-43.
 *
 * This type intentionally has no relationship to `BranchState` — it carries
 * no field that could drive a lifecycle transition, and no function in this
 * module returns or mutates a `BranchState`. The branch lifecycle transitions
 * (`submitBranch`, `verifyBranch`, `mergeBranch`, `returnToDraft` in
 * `../../branch-lifecycle.ts`) take only `(state, actor, ...)` and have no
 * parameter through which a `VerificationSignal` could be threaded — proving
 * AC3 ("feedback alone does not verify, unverify, merge, reject, reopen, or
 * return a branch to draft") at the lifecycle boundary, not merely by the
 * absence of an export from this module.
 *
 * AC1: carries `workspaceId`/`branchId` so a stakeholder can see feedback
 * associated with the branch it evaluated.
 * AC4: distinguishable from `ChunkApprovalRecord` / `SuggestionDecision` —
 * it has no `decision` discriminant and cannot itself be an approval.
 * AC5: `reportedByActorKind` may be `'delegated'` — an implementation agent
 * can contribute a signal without being treated as the human decision maker
 * (technical spec §"Delegated agents").
 */
export interface VerificationSignal {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly signalId: VerificationSignalId;
  readonly outcome: VerificationOutcome;
  readonly reportedByStakeholderId: StakeholderId;
  readonly reportedByActorKind: ActorKind;
  readonly reportedAt: string;
  readonly summary: string;
}

/**
 * Records a verification signal against a branch.
 *
 * Both human and delegated actors may record a signal — technical spec
 * §"Delegated agents": "AI agents and external systems may submit feedback
 * and act as supervised delegates." Unlike protected operations
 * (`approveChunk`, `acceptSuggestion`, `verifyBranch`, ...), this function
 * never calls `assertHumanActor`: recording advisory feedback is not itself a
 * protected decision.
 *
 * Multiple signals may be recorded for the same branch; this function does
 * not deduplicate or collapse outcomes into a single "current" value, so a
 * stakeholder can review the full passing/failing/mixed history (AC2)
 * before deciding what happens next.
 *
 * Throws `VocabularyValidationError` if `reportedAt` is empty, whitespace-only,
 * or not a valid ISO-8601 date string.
 * Throws `VocabularyValidationError` if `summary` is empty or whitespace-only
 * — an outcome alone is not meaningful advisory evidence without a summary
 * of what was evaluated.
 */
export function recordVerificationSignal(
  actor: ActorContext,
  branch: VerificationTargetBranch,
  signalId: VerificationSignalId,
  outcome: VerificationOutcome,
  reportedAt: string,
  summary: string,
): VerificationSignal {
  const trimmedAt = reportedAt.trim();
  if (
    !trimmedAt ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) ||
    Number.isNaN(Date.parse(trimmedAt))
  ) {
    throw new VocabularyValidationError(
      'VerificationSignal.reportedAt',
      'reportedAt must be a valid ISO-8601 date string',
    );
  }
  const trimmedSummary = summary.trim();
  if (!trimmedSummary) {
    throw new VocabularyValidationError(
      'VerificationSignal.summary',
      'summary cannot be empty or whitespace',
    );
  }
  return Object.freeze({
    workspaceId: branch.workspaceId,
    branchId: branch.branchId,
    signalId,
    outcome,
    reportedByStakeholderId: actor.stakeholderId,
    reportedByActorKind: actor.kind,
    reportedAt: trimmedAt,
    summary: trimmedSummary,
  });
}
