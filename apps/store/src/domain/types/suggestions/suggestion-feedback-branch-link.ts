import { SuggestionLifecycleError } from '../errors/suggestion-lifecycle-error.js';
import type { BranchId } from '../identifiers/branch-id.js';
import type { SuggestionId } from '../identifiers/suggestion-id.js';
import type { WorkspaceId } from '../identifiers/workspace-id.js';
import type { Discipline } from '../vocabulary/discipline.js';
import type { SuggestionAcceptedDecision } from './suggestion-accepted-decision.js';

/**
 * Traceability record connecting an accepted suggestion to the discipline-scoped
 * feedback branch it initialized.
 *
 * Technical spec §"Required lifecycle contracts — Suggestion": "Accepted suggestions
 * must remain linked to the feedback branch they initialize."
 *
 * AC4: A stakeholder can trace accepted branch work back to the suggestion that
 * started it.
 */
export interface SuggestionFeedbackBranchLink {
  readonly suggestionId: SuggestionId;
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly discipline: Discipline;
}

/**
 * Minimal shape of branch ownership needed to validate a feedback-branch link,
 * without creating a dependency on the full `branch-lifecycle.ts` module.
 *
 * This intentionally mirrors the fields of `BranchOwnership`
 * (`apps/store/src/domain/branch-lifecycle.ts`) so callers can pass a real
 * `BranchOwnership` instance directly.
 */
export interface FeedbackBranchOwnership {
  readonly branchId: BranchId;
  readonly workspaceId: WorkspaceId;
  readonly discipline: Discipline;
}

/**
 * Links an accepted suggestion decision to the feedback branch it initialized.
 *
 * Validates that the branch belongs to the same workspace as the decision
 * (technical spec §"Workspace scoping") and was created in the discipline the
 * decision designated (technical spec §"Accept suggestion" — "creates a linked
 * feedback branch scoped to one discipline").
 *
 * Throws `SuggestionLifecycleError` with code `invalid-state-transition` if
 * `decision.decision` is not `'accepted'` (defense in depth — the type system
 * already restricts this parameter to `SuggestionAcceptedDecision`).
 * Throws `SuggestionLifecycleError` with code `tenant-boundary-violation` if the
 * branch's workspace does not match the decision's workspace.
 * Throws `SuggestionLifecycleError` with code `discipline-boundary-violation` if
 * the branch's discipline does not match `decision.feedbackBranchDiscipline`.
 */
export function linkSuggestionToFeedbackBranch(
  decision: SuggestionAcceptedDecision,
  branchOwnership: FeedbackBranchOwnership,
): SuggestionFeedbackBranchLink {
  if (decision.decision !== 'accepted') {
    throw new SuggestionLifecycleError(
      'invalid-state-transition',
      'only an accepted suggestion decision may be linked to a feedback branch',
    );
  }
  if (branchOwnership.workspaceId !== decision.workspaceId) {
    throw new SuggestionLifecycleError(
      'tenant-boundary-violation',
      `feedback branch workspace '${branchOwnership.workspaceId}' does not match suggestion workspace '${decision.workspaceId}'`,
    );
  }
  if (branchOwnership.discipline !== decision.feedbackBranchDiscipline) {
    throw new SuggestionLifecycleError(
      'discipline-boundary-violation',
      `feedback branch discipline '${branchOwnership.discipline}' does not match accepted suggestion discipline '${decision.feedbackBranchDiscipline}'`,
    );
  }
  return Object.freeze({
    suggestionId: decision.suggestionId,
    workspaceId: decision.workspaceId,
    branchId: branchOwnership.branchId,
    discipline: branchOwnership.discipline,
  });
}
