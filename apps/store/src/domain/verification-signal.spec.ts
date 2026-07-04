/**
 * Tests for verification signal domain invariants.
 *
 * Story: S07 — Treat verification feedback as advisory evidence.
 *
 * Sources of authority:
 *   - docs/specifications/feature-01-core-domain-model/stories/S07-advisory-verification-feedback.md
 *   - docs/specifications/feature-01-core-domain-model/technical-specification.md
 *     §"Verification signals", §"Required lifecycle contracts — Branch",
 *     §"Delegated agents", §"Required domain error categories"
 *   - Meridian IDEA-35, IDEA-43
 */

import { describe, expect, it } from 'vitest';
import {
  branchOwnership,
  divergencePoint,
  mergeBranch,
  returnToDraft,
  submitBranch,
  verifyBranch,
  BranchLifecycleError,
  type BranchOwnership,
} from './branch-lifecycle.js';
import {
  allSignalsPassing,
  hasFailingSignal,
  recordVerificationSignal,
  type VerificationOutcome,
  type VerificationSignal,
} from './verification-signal.js';
import {
  VocabularyValidationError,
  branchId,
  delegatedActor,
  humanActor,
  stakeholderId,
  verificationSignalId,
  workspaceId,
} from './types/index.js';

// ─── Test fixtures ────────────────────────────────────────────────────────────

const WS = workspaceId('dbb786ac-1d61-41c9-a46a-2c279dd50cc3');
const WS2 = workspaceId('aaaaaaaa-0000-0000-0000-000000000002');
const BRANCH = branchId('branch-verify-001');
const STAKEHOLDER = stakeholderId('117c5cb8-a140-4bc6-8775-70cc0e1bc784');
const HUMAN = humanActor(STAKEHOLDER);
const DELEGATED = delegatedActor(stakeholderId('agent-session-001'));
const DIV_POINT = divergencePoint('2026-06-29T19:00:00.000Z');
const TS = '2026-06-29T20:00:00.000Z';

function ownership(ws = WS): BranchOwnership {
  return branchOwnership({
    branchId: BRANCH,
    workspaceId: ws,
    discipline: 'engineering',
    divergedAt: DIV_POINT,
    createdByStakeholderId: STAKEHOLDER,
  });
}

// ─── recordVerificationSignal ─────────────────────────────────────────────────

describe('recordVerificationSignal', () => {
  describe('with a human actor', () => {
    it('AC1: returns a VerificationSignal carrying the branch and workspace it evaluated', () => {
      const signal: VerificationSignal = recordVerificationSignal(
        HUMAN,
        ownership(),
        verificationSignalId('signal-001'),
        'passing',
        TS,
        'unit tests passed',
      );
      expect(signal.workspaceId).toBe(WS);
      expect(signal.branchId).toBe(BRANCH);
      expect(signal.outcome).toBe('passing');
      expect(signal.reportedByStakeholderId).toBe(STAKEHOLDER);
      expect(signal.reportedByActorKind).toBe('human');
      expect(signal.reportedAt).toBe(TS);
      expect(signal.summary).toBe('unit tests passed');
    });

    it('returns a frozen (immutable) record', () => {
      const signal = recordVerificationSignal(
        HUMAN,
        ownership(),
        verificationSignalId('signal-002'),
        'passing',
        TS,
        'lint clean',
      );
      expect(Object.isFrozen(signal)).toBe(true);
    });

    it('trims surrounding whitespace from reportedAt and summary', () => {
      const signal = recordVerificationSignal(
        HUMAN,
        ownership(),
        verificationSignalId('signal-003'),
        'passing',
        `  ${TS}  `,
        '  all green  ',
      );
      expect(signal.reportedAt).toBe(TS);
      expect(signal.summary).toBe('all green');
    });

    it('records are scoped to a workspace so the same branch id in different workspaces is distinguishable', () => {
      const inWsA = recordVerificationSignal(
        HUMAN,
        ownership(WS),
        verificationSignalId('signal-004'),
        'passing',
        TS,
        'build passed',
      );
      const inWsB = recordVerificationSignal(
        HUMAN,
        ownership(WS2),
        verificationSignalId('signal-005'),
        'passing',
        TS,
        'build passed',
      );
      expect(inWsA.workspaceId).not.toBe(inWsB.workspaceId);
      expect(inWsA.branchId).toBe(inWsB.branchId);
    });
  });

  describe('with a delegated actor', () => {
    it('AC5: an implementation agent can record a verification signal without being a human actor', () => {
      expect(() =>
        recordVerificationSignal(
          DELEGATED,
          ownership(),
          verificationSignalId('signal-006'),
          'failing',
          TS,
          'integration test suite failed: 3 tests red',
        ),
      ).not.toThrow();
    });

    it('the recorded signal carries reportedByActorKind "delegated", distinguishing it from a human decision', () => {
      const signal = recordVerificationSignal(
        DELEGATED,
        ownership(),
        verificationSignalId('signal-007'),
        'failing',
        TS,
        'ci pipeline failed',
      );
      expect(signal.reportedByActorKind).toBe('delegated');
      expect(signal.reportedByStakeholderId).toBe(DELEGATED.stakeholderId);
    });
  });

  describe('AC2: passing, failing, and mixed outcomes', () => {
    it('accepts every recognised verification outcome', () => {
      const outcomes: VerificationOutcome[] = ['passing', 'failing', 'mixed'];
      for (const outcome of outcomes) {
        const signal = recordVerificationSignal(
          HUMAN,
          ownership(),
          verificationSignalId(`signal-outcome-${outcome}`),
          outcome,
          TS,
          `evaluation resulted in ${outcome}`,
        );
        expect(signal.outcome).toBe(outcome);
      }
    });

    it('multiple signals for the same branch are not deduplicated or collapsed — a stakeholder can review the full history', () => {
      const own = ownership();
      const first = recordVerificationSignal(HUMAN, own, verificationSignalId('signal-h1'), 'passing', TS, 'unit tests passed');
      const second = recordVerificationSignal(
        DELEGATED,
        own,
        verificationSignalId('signal-h2'),
        'failing',
        TS,
        'e2e tests failed',
      );
      const third = recordVerificationSignal(HUMAN, own, verificationSignalId('signal-h3'), 'mixed', TS, 'flaky suite: mixed results');

      const history = [first, second, third];
      expect(history.map((s) => s.outcome)).toEqual(['passing', 'failing', 'mixed']);
      expect(hasFailingSignal(history)).toBe(true);
      expect(allSignalsPassing(history)).toBe(false);
    });

    it('hasFailingSignal is false and allSignalsPassing is true when every signal passes', () => {
      const own = ownership();
      const history = [
        recordVerificationSignal(HUMAN, own, verificationSignalId('signal-p1'), 'passing', TS, 'unit tests passed'),
        recordVerificationSignal(DELEGATED, own, verificationSignalId('signal-p2'), 'passing', TS, 'lint clean'),
      ];
      expect(hasFailingSignal(history)).toBe(false);
      expect(allSignalsPassing(history)).toBe(true);
    });

    it('allSignalsPassing is false for an empty signal history — nothing has been reviewed yet', () => {
      expect(allSignalsPassing([])).toBe(false);
      expect(hasFailingSignal([])).toBe(false);
    });
  });

  describe('validation', () => {
    it('rejects an empty reportedAt', () => {
      expect(() =>
        recordVerificationSignal(HUMAN, ownership(), verificationSignalId('signal-008'), 'passing', '', 'ok'),
      ).toThrow(VocabularyValidationError);
    });

    it('rejects a non-ISO reportedAt', () => {
      expect(() =>
        recordVerificationSignal(HUMAN, ownership(), verificationSignalId('signal-009'), 'passing', 'not-a-date', 'ok'),
      ).toThrow(VocabularyValidationError);
    });

    it('rejects an empty summary', () => {
      expect(() =>
        recordVerificationSignal(HUMAN, ownership(), verificationSignalId('signal-010'), 'passing', TS, ''),
      ).toThrow(VocabularyValidationError);
    });

    it('rejects a whitespace-only summary', () => {
      expect(() =>
        recordVerificationSignal(HUMAN, ownership(), verificationSignalId('signal-011'), 'passing', TS, '   '),
      ).toThrow(VocabularyValidationError);
    });
  });
});

// ─── AC3: feedback alone never transitions branch state ──────────────────────
//
// "A stakeholder can confirm that feedback alone does not verify, unverify,
// merge, reject, reopen, or return a branch to draft."
//
// Proven at the lifecycle boundary (not by absence of an export from this
// module): the real branch-lifecycle transition functions take only
// `(state, actor, ...)` and have no parameter that could carry a recorded
// VerificationSignal, so no volume or outcome of recorded signals can affect
// their result.

describe('AC3: verification signals cannot automate a branch lifecycle transition', () => {
  it('recording failing signals does not prevent or influence verifyBranch — only human-initiated calls transition state', () => {
    const own = ownership();
    // Record three failing signals — if signals could influence the
    // lifecycle, one might expect verification to be blocked or altered.
    recordVerificationSignal(DELEGATED, own, verificationSignalId('sig-a'), 'failing', TS, 'suite failed');
    recordVerificationSignal(DELEGATED, own, verificationSignalId('sig-b'), 'failing', TS, 'suite failed again');
    recordVerificationSignal(DELEGATED, own, verificationSignalId('sig-c'), 'mixed', TS, 'partially failing');

    // The transition function accepts no signal/outcome argument at all —
    // its result depends only on `state` and `actor`.
    expect(verifyBranch('submitted', HUMAN)).toBe('verified');
  });

  it('recording passing signals does not itself merge, verify, or submit a branch — no transition function is invoked by recording a signal', () => {
    const own = ownership();
    recordVerificationSignal(DELEGATED, own, verificationSignalId('sig-d'), 'passing', TS, 'all green');
    recordVerificationSignal(DELEGATED, own, verificationSignalId('sig-e'), 'passing', TS, 'all green again');

    // Recording signals is a pure, independent operation — branch state is
    // whatever it was before recording; nothing here calls submitBranch,
    // verifyBranch, mergeBranch, or returnToDraft.
    expect(submitBranch('draft', HUMAN, 'engineering', 'engineering')).toBe('submitted');
  });

  it('a delegated actor can record any number of signals but still cannot verify, merge, submit, or return a branch to draft', () => {
    const own = ownership();
    recordVerificationSignal(DELEGATED, own, verificationSignalId('sig-f'), 'passing', TS, 'all green');

    expect(() => verifyBranch('submitted', DELEGATED)).toThrow(BranchLifecycleError);
    expect(() => mergeBranch('verified', DELEGATED)).toThrow(BranchLifecycleError);
    expect(() => submitBranch('draft', DELEGATED, 'engineering', 'engineering')).toThrow(
      BranchLifecycleError,
    );
    expect(() => returnToDraft('submitted', DELEGATED)).toThrow(BranchLifecycleError);
  });

  it('a human actor must still explicitly call verifyBranch/mergeBranch/returnToDraft — recording a signal alone never performs the transition', () => {
    const own = ownership();
    // Even a passing-only history does not implicitly verify the branch:
    // the branch state passed to verifyBranch is caller-supplied, not derived
    // from recorded signals.
    recordVerificationSignal(HUMAN, own, verificationSignalId('sig-g'), 'passing', TS, 'all green');
    recordVerificationSignal(HUMAN, own, verificationSignalId('sig-h'), 'passing', TS, 'still green');

    // The branch is still 'submitted' until a human explicitly verifies it —
    // recording signals did not advance the state on its own.
    const stateAfterSignals: 'submitted' = 'submitted';
    expect(verifyBranch(stateAfterSignals, HUMAN)).toBe('verified');
  });
});

// ─── AC4: manual decision after reviewing feedback ────────────────────────────
//
// "A stakeholder can manually decide whether a branch is verified or needs
// more work after reviewing feedback." Branch state storage and the
// feedback-routing decision workflow are out of scope for this story; the
// domain guarantee this story delivers is that the decision function
// (`verifyBranch`) is entirely separate from, and not driven by, the
// verification-signal record.

describe('AC4: reviewing feedback and deciding are separate operations', () => {
  it('a stakeholder reviews the recorded signal history, then separately calls verifyBranch to decide it is verified', () => {
    const own = ownership();
    const history = [
      recordVerificationSignal(DELEGATED, own, verificationSignalId('sig-i'), 'failing', TS, 'needs more work'),
    ];

    // Review step: a stakeholder can inspect outcomes.
    expect(hasFailingSignal(history)).toBe(true);

    // Decide step: despite failing feedback, only an explicit human call
    // changes branch state — the decision is manual, not automatic.
    expect(verifyBranch('submitted', HUMAN)).toBe('verified');
  });

  it('a stakeholder reviews failing feedback, then separately calls returnToDraft to decide it needs more work', () => {
    const own = ownership();
    const history = [
      recordVerificationSignal(DELEGATED, own, verificationSignalId('sig-j'), 'failing', TS, 'regression detected'),
      recordVerificationSignal(DELEGATED, own, verificationSignalId('sig-k'), 'mixed', TS, 'partial coverage failure'),
    ];

    // Review step: a stakeholder can inspect outcomes and see they are not
    // clean passes.
    expect(hasFailingSignal(history)).toBe(true);
    expect(allSignalsPassing(history)).toBe(false);

    // Decide step: the "needs more work" outcome (IDEA-43's other named
    // transition, "unverify"/return-to-draft) is likewise only reachable
    // through an explicit human call — recording the failing/mixed history
    // above did not return the branch to draft on its own.
    expect(returnToDraft('submitted', HUMAN)).toBe('draft');
  });

  it('a stakeholder reviewing an all-passing history may still decide the branch needs more work — the record never forces the decision', () => {
    const own = ownership();
    const history = [
      recordVerificationSignal(HUMAN, own, verificationSignalId('sig-l'), 'passing', TS, 'all green'),
    ];

    expect(allSignalsPassing(history)).toBe(true);

    // Even with an all-passing signal history, the human stakeholder's
    // manual decision governs — nothing in this module compels verifyBranch
    // to be called, and a human may instead choose returnToDraft.
    expect(returnToDraft('submitted', HUMAN)).toBe('draft');
  });
});
