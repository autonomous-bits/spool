/**
 * Suggestion lifecycle domain module: state predicates, transition guard, and
 * accepted-suggestion → feedback-branch traceability.
 *
 * This module composes the S04-delivered protected operation contracts
 * (`acceptSuggestion`, `rejectSuggestion`) with the `SuggestionState` machine
 * required by this story.
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-01-core-domain-model/stories/S05-human-reviewed-suggestions.md
 * - Technical spec: docs/specifications/feature-01-core-domain-model/technical-specification.md
 *                   §"Required lifecycle contracts — Suggestion",
 *                   §"Protected operation contracts — Accept suggestion, Reject suggestion",
 *                   §"Required domain error categories"
 * - Constitution:   docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian:       IDEA-28, IDEA-40
 *
 * Story: S05 — Route external feedback through human-reviewed suggestions.
 *
 * Note on atomicity: `acceptSuggestion` and `linkSuggestionToFeedbackBranch` are
 * intentionally separate pure domain functions. Persisting an accepted decision
 * together with its feedback branch in a single atomic operation is an
 * application/service-layer (and eventual persistence-layer) responsibility —
 * "Suggestion storage, queue implementation, API request shape" are explicitly
 * out of scope for this story. The domain layer's role is to make it
 * impossible to construct a valid `SuggestionFeedbackBranchLink` except from a
 * `SuggestionAcceptedDecision` and a same-workspace, same-discipline branch.
 */

import type { SuggestionState } from './types/index.js';

export type { SuggestionState };

export {
  SuggestionLifecycleError,
  assertPendingSuggestion,
  acceptSuggestion,
  rejectSuggestion,
  linkSuggestionToFeedbackBranch,
} from './types/index.js';

export type {
  SuggestionLifecycleErrorCode,
  SuggestionAcceptedDecision,
  SuggestionRejectedDecision,
  SuggestionDecision,
  SuggestionFeedbackBranchLink,
  FeedbackBranchOwnership,
} from './types/index.js';

/**
 * Returns true when the suggestion has not yet been decided.
 *
 * AC1: "A stakeholder can see that external feedback is pending human review
 * before it affects approved context." A pending suggestion cannot be linked
 * to a feedback branch and has no bearing on approved or promoted context.
 */
export function isPendingSuggestion(state: SuggestionState): boolean {
  return state === 'pending';
}

/**
 * Returns true when the suggestion has reached a terminal state (accepted or
 * rejected).
 *
 * Technical spec §"Required lifecycle contracts — Suggestion": "Accepted and
 * Rejected are terminal." A terminal suggestion cannot be re-decided.
 */
export function isTerminalSuggestion(state: SuggestionState): boolean {
  return state === 'accepted' || state === 'rejected';
}
