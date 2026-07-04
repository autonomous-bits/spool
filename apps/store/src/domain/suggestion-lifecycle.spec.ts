/**
 * Tests for suggestion lifecycle domain invariants: state predicates, the
 * pending-only transition guard, and accepted-suggestion → feedback-branch
 * traceability.
 *
 * Story: S05 — Route external feedback through human-reviewed suggestions.
 *
 * Sources of authority:
 *   - docs/specifications/feature-01-core-domain-model/stories/S05-human-reviewed-suggestions.md
 *   - docs/specifications/feature-01-core-domain-model/technical-specification.md
 *     §"Required lifecycle contracts — Suggestion",
 *     §"Protected operation contracts — Accept suggestion, Reject suggestion",
 *     §"Required domain error categories"
 *   - Meridian IDEA-28, IDEA-40
 */

import { describe, expect, it } from 'vitest';
import { branchOwnership, divergencePoint } from './branch-lifecycle.js';
import {
  acceptSuggestion,
  isPendingSuggestion,
  isTerminalSuggestion,
  linkSuggestionToFeedbackBranch,
  rejectSuggestion,
  SuggestionLifecycleError,
  type SuggestionAcceptedDecision,
  type SuggestionRejectedDecision,
  type SuggestionState,
} from './suggestion-lifecycle.js';
import {
  branchId,
  delegatedActor,
  humanActor,
  HumanControlError,
  stakeholderId,
  suggestionId,
  workspaceId,
} from './types/index.js';

// ─── Test fixtures ────────────────────────────────────────────────────────────

const WS = workspaceId('dbb786ac-1d61-41c9-a46a-2c279dd50cc3');
const WS2 = workspaceId('aaaaaaaa-0000-0000-0000-000000000002');
const STAKEHOLDER = stakeholderId('117c5cb8-a140-4bc6-8775-70cc0e1bc784');
const HUMAN = humanActor(STAKEHOLDER);
const DELEGATED = delegatedActor(STAKEHOLDER);
const SUGG_ID = suggestionId('sugg-005');
const TS = '2026-06-29T20:00:00.000Z';

// ─── AC1: pending suggestions are inert ──────────────────────────────────────

describe('isPendingSuggestion / isTerminalSuggestion', () => {
  it('AC1: a pending suggestion is pending and not terminal', () => {
    expect(isPendingSuggestion('pending')).toBe(true);
    expect(isTerminalSuggestion('pending')).toBe(false);
  });

  it('an accepted suggestion is terminal and not pending', () => {
    expect(isPendingSuggestion('accepted')).toBe(false);
    expect(isTerminalSuggestion('accepted')).toBe(true);
  });

  it('a rejected suggestion is terminal and not pending', () => {
    expect(isPendingSuggestion('rejected')).toBe(false);
    expect(isTerminalSuggestion('rejected')).toBe(true);
  });
});

// ─── AC2: accept a suggestion into branch work ───────────────────────────────

describe('acceptSuggestion (pending-only transition guard)', () => {
  it('AC2: accepts a pending suggestion and carries the discipline that should own the feedback branch', () => {
    const decision = acceptSuggestion('pending', HUMAN, WS, SUGG_ID, 'engineering', TS);
    expect(decision.decision).toBe('accepted');
    expect(decision.feedbackBranchDiscipline).toBe('engineering');
  });

  it('throws SuggestionLifecycleError when the suggestion is already accepted', () => {
    expect(() => acceptSuggestion('accepted', HUMAN, WS, SUGG_ID, 'engineering', TS)).toThrow(
      SuggestionLifecycleError,
    );
  });

  it('throws SuggestionLifecycleError when the suggestion is already rejected', () => {
    expect(() => acceptSuggestion('rejected', HUMAN, WS, SUGG_ID, 'engineering', TS)).toThrow(
      SuggestionLifecycleError,
    );
  });

  it('error code is invalid-state-transition for a non-pending suggestion', () => {
    expect(() => acceptSuggestion('accepted', HUMAN, WS, SUGG_ID, 'engineering', TS)).toThrow(
      expect.objectContaining({ code: 'invalid-state-transition' }),
    );
  });

  it('AC5: a delegated actor cannot accept a pending suggestion', () => {
    expect(() => acceptSuggestion('pending', DELEGATED, WS, SUGG_ID, 'engineering', TS)).toThrow(
      HumanControlError,
    );
  });
});

// ─── AC3: reject a suggestion without changing graph state ───────────────────

describe('rejectSuggestion (pending-only transition guard)', () => {
  it('AC3: rejects a pending suggestion and carries no branch/discipline-affecting field', () => {
    const decision = rejectSuggestion('pending', HUMAN, WS, SUGG_ID, TS);
    expect(decision.decision).toBe('rejected');
    expect('feedbackBranchDiscipline' in decision).toBe(false);
  });

  it('throws SuggestionLifecycleError when the suggestion is already accepted', () => {
    expect(() => rejectSuggestion('accepted', HUMAN, WS, SUGG_ID, TS)).toThrow(
      SuggestionLifecycleError,
    );
  });

  it('throws SuggestionLifecycleError when the suggestion is already rejected', () => {
    expect(() => rejectSuggestion('rejected', HUMAN, WS, SUGG_ID, TS)).toThrow(
      SuggestionLifecycleError,
    );
  });

  it('error code is invalid-state-transition for a non-pending suggestion', () => {
    expect(() => rejectSuggestion('rejected', HUMAN, WS, SUGG_ID, TS)).toThrow(
      expect.objectContaining({ code: 'invalid-state-transition' }),
    );
  });

  it('AC5: a delegated actor cannot reject a pending suggestion', () => {
    expect(() => rejectSuggestion('pending', DELEGATED, WS, SUGG_ID, TS)).toThrow(
      HumanControlError,
    );
  });
});

// ─── AC4: trace accepted branch work back to its suggestion ──────────────────

describe('linkSuggestionToFeedbackBranch', () => {
  const ownership = branchOwnership({
    branchId: branchId('branch-005'),
    workspaceId: WS,
    discipline: 'engineering',
    divergedAt: divergencePoint(TS),
    createdByStakeholderId: STAKEHOLDER,
  });

  it('AC4: links the accepted suggestion to the branch that was created for it', () => {
    const decision: SuggestionAcceptedDecision = acceptSuggestion(
      'pending',
      HUMAN,
      WS,
      SUGG_ID,
      'engineering',
      TS,
    );
    const link = linkSuggestionToFeedbackBranch(decision, ownership);
    expect(link.suggestionId).toBe(SUGG_ID);
    expect(link.branchId).toBe(ownership.branchId);
    expect(link.workspaceId).toBe(WS);
    expect(link.discipline).toBe('engineering');
  });

  it('returns a frozen (immutable) record', () => {
    const decision = acceptSuggestion('pending', HUMAN, WS, SUGG_ID, 'engineering', TS);
    const link = linkSuggestionToFeedbackBranch(decision, ownership);
    expect(Object.isFrozen(link)).toBe(true);
  });

  it('throws tenant-boundary-violation when the branch workspace does not match the decision workspace', () => {
    const decision = acceptSuggestion('pending', HUMAN, WS2, SUGG_ID, 'engineering', TS);
    expect(() => linkSuggestionToFeedbackBranch(decision, ownership)).toThrow(
      expect.objectContaining({ code: 'tenant-boundary-violation' }),
    );
  });

  it('throws discipline-boundary-violation when the branch discipline does not match the decision discipline', () => {
    const decision = acceptSuggestion('pending', HUMAN, WS, SUGG_ID, 'product', TS);
    expect(() => linkSuggestionToFeedbackBranch(decision, ownership)).toThrow(
      expect.objectContaining({ code: 'discipline-boundary-violation' }),
    );
  });

  it('cannot be constructed from a rejected decision (compile-time enforced; AC3)', () => {
    const rejected: SuggestionRejectedDecision = rejectSuggestion(
      'pending',
      HUMAN,
      WS,
      SUGG_ID,
      TS,
    );
    expect(() => {
      // @ts-expect-error — linkSuggestionToFeedbackBranch requires a SuggestionAcceptedDecision,
      // not a SuggestionRejectedDecision. This line only compiles if the type guard is removed;
      // it also fails at runtime (defense in depth) via the explicit `decision.decision` check.
      linkSuggestionToFeedbackBranch(rejected, ownership);
    }).toThrow(expect.objectContaining({ code: 'invalid-state-transition' }));
  });
});

// ─── Sanity: SuggestionState covers exactly pending/accepted/rejected ────────

describe('SuggestionState', () => {
  it('has exactly three states', () => {
    const states: SuggestionState[] = ['pending', 'accepted', 'rejected'];
    for (const state of states) {
      expect(['pending', 'accepted', 'rejected']).toContain(state);
    }
  });
});
