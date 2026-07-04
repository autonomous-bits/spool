/**
 * Verification signal domain module: advisory-only feedback records
 * associated with a branch.
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-01-core-domain-model/stories/S07-advisory-verification-feedback.md
 * - Technical spec: docs/specifications/feature-01-core-domain-model/technical-specification.md
 *                   §"Verification signals", §"Required lifecycle contracts — Branch",
 *                   §"Delegated agents", §"Required domain error categories"
 * - Constitution:   docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian:       IDEA-35, IDEA-43
 *
 * Story: S07 — Treat verification feedback as advisory evidence.
 *
 * Note on the advisory-only invariant (AC3): this module intentionally has no
 * function that accepts or returns a `BranchState`. The branch lifecycle
 * transitions (`submitBranch`, `verifyBranch`, `mergeBranch`, `returnToDraft`)
 * live in `./branch-lifecycle.js` and take only `(state, actor, ...)` — there
 * is no parameter through which a recorded `VerificationSignal` could
 * influence a transition. See `branch-lifecycle.spec.ts` and
 * `verification-signal.spec.ts` for tests proving the two modules are
 * structurally and behaviourally independent.
 */

import type { VerificationOutcome } from './types/index.js';

export type { VerificationOutcome };

export { recordVerificationSignal } from './types/index.js';

export type { VerificationSignal, VerificationTargetBranch } from './types/index.js';

/**
 * Returns true when at least one recorded signal for a branch is `'failing'`
 * or `'mixed'` — i.e. not a clean pass. Named for the common case of
 * flagging feedback that needs a stakeholder's attention; it intentionally
 * treats `'mixed'` the same as `'failing'` here rather than silently
 * collapsing the two, since a stakeholder reviewing this helper's result
 * should still inspect the underlying signal history for the precise
 * `'failing'` vs `'mixed'` distinction (AC2) before deciding what happens
 * next.
 *
 * AC2: "A stakeholder can review passing, failing, or mixed feedback before
 * deciding what happens next." This is a pure read-side helper over already
 * recorded signals; it does not transition, and cannot transition, any
 * branch state.
 */
export function hasFailingSignal(signals: readonly { readonly outcome: VerificationOutcome }[]): boolean {
  return signals.some((signal) => signal.outcome === 'failing' || signal.outcome === 'mixed');
}

/**
 * Returns true when every recorded signal for a branch has a passing outcome.
 *
 * An empty signal history is not "all passing" — there is nothing to review
 * yet, so this returns `false` for an empty array.
 */
export function allSignalsPassing(signals: readonly { readonly outcome: VerificationOutcome }[]): boolean {
  return signals.length > 0 && signals.every((signal) => signal.outcome === 'passing');
}
