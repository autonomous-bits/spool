import type { SuggestionState } from '../lifecycle/suggestion-state.js';

/**
 * All machine-readable error codes produced by suggestion lifecycle invariants.
 *
 * Technical spec §"Required domain error categories":
 * - `invalid-state-transition`      — decision attempted on a non-pending suggestion
 * - `discipline-boundary-violation` — feedback branch discipline does not match the decision
 * - `tenant-boundary-violation`     — feedback branch workspace does not match the decision
 */
export type SuggestionLifecycleErrorCode =
  'invalid-state-transition' | 'discipline-boundary-violation' | 'tenant-boundary-violation';

/**
 * Thrown when a suggestion lifecycle invariant is violated.
 *
 * The `code` property is stable and machine-readable. Adapters must map domain
 * failures by code, never by inspecting free-form message text.
 *
 * Technical spec §"Required domain error categories".
 */
export class SuggestionLifecycleError extends Error {
  override readonly name = 'SuggestionLifecycleError';

  constructor(
    readonly code: SuggestionLifecycleErrorCode,
    readonly reason: string,
  ) {
    super(reason);
  }
}

/**
 * Asserts that a suggestion is still `pending` before a decision (accept/reject) is applied.
 *
 * Technical spec §"Required lifecycle contracts — Suggestion": "Pending to Accepted or
 * Rejected. Accepted and Rejected are terminal." A suggestion that has already been decided
 * cannot be re-decided.
 *
 * Throws `SuggestionLifecycleError` with code `invalid-state-transition` if `state` is not
 * `pending`.
 */
export function assertPendingSuggestion(state: SuggestionState, operation: string): void {
  if (state !== 'pending') {
    throw new SuggestionLifecycleError(
      'invalid-state-transition',
      `cannot ${operation} a suggestion that is already '${state}'; only pending suggestions may be decided`,
    );
  }
}
