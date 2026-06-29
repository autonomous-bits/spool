/**
 * Tests for branch lifecycle domain types, invariants, and transition functions.
 *
 * Story: S03 — Keep branch work owned by one discipline.
 * Story: S04 — Preserve human control over accountable decisions.
 *   (BranchSubmittedRecord and BranchVerifiedRecord sections)
 *
 * Sources of authority:
 *   - docs/specifications/feature-01-core-domain-model/stories/S03-discipline-owned-branches.md
 *   - docs/specifications/feature-01-core-domain-model/stories/S04-human-control-of-decisions.md
 *   - docs/specifications/feature-01-core-domain-model/technical-specification.md
 *     §"Branch ownership", §"Branch graph view", §"Required lifecycle contracts — Branch",
 *     §"Protected operation contracts", §"Required domain error categories"
 *   - Meridian IDEA-17, IDEA-29, IDEA-35, IDEA-40, IDEA-41, IDEA-43
 */

import { describe, expect, it } from 'vitest';
import {
  BranchLifecycleError,
  assertDisciplineBoundaryForWrite,
  assertGraphWriteAllowed,
  assertSubmitDiscipline,
  assertWorkspaceMatch,
  branchOwnership,
  branchSubmittedRecord,
  branchVerifiedRecord,
  divergencePoint,
  isDraftBranch,
  isMergedBranch,
  isWriteLocked,
  mergeBranch,
  mergeLineage,
  returnToDraft,
  submitBranch,
  verifyBranch,
  withBranchProvenance,
  type BranchGraphProvenance,
  type BranchOwnership,
  type BranchSubmittedRecord,
  type BranchVerifiedRecord,
  type MergeLineage,
} from './branch-lifecycle.js';
import {
  VocabularyValidationError,
  branchId,
  delegatedActor,
  humanActor,
  stakeholderId,
  workspaceId,
  type BranchState,
} from './vocabulary.js';

// ─── Test fixtures ────────────────────────────────────────────────────────────

const WS = workspaceId('dbb786ac-1d61-41c9-a46a-2c279dd50cc3');
const WS2 = workspaceId('aaaaaaaa-0000-0000-0000-000000000001');
const BRANCH = branchId('branch-001');
const STAKEHOLDER = stakeholderId('117c5cb8-a140-4bc6-8775-70cc0e1bc784');
const HUMAN = humanActor(STAKEHOLDER);
const DELEGATED = delegatedActor(STAKEHOLDER);
const DIV_POINT = divergencePoint('2026-06-29T19:00:00.000Z');

function ownership(discipline: 'product' | 'architecture' | 'design' | 'engineering' = 'engineering'): BranchOwnership {
  return branchOwnership({
    branchId: BRANCH,
    workspaceId: WS,
    discipline,
    divergedAt: DIV_POINT,
    createdByStakeholderId: STAKEHOLDER,
  });
}

// ─── BranchLifecycleError ─────────────────────────────────────────────────────

describe('BranchLifecycleError', () => {
  it('is an Error with a machine-readable code and reason message', () => {
    const err = new BranchLifecycleError('write-locked', 'branch is locked');
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe('BranchLifecycleError');
    expect(err.code).toBe('write-locked');
    expect(err.message).toBe('branch is locked');
  });

  it('supports all required domain error codes', () => {
    // Technical spec §"Required domain error categories" — adapters must map
    // domain failures by code, not by inspecting message text.
    const codes: BranchLifecycleError['code'][] = [
      'write-locked',
      'unauthorized-actor',
      'discipline-boundary-violation',
      'invalid-state-transition',
      'branch-isolation-violation',
    ];
    for (const code of codes) {
      const err = new BranchLifecycleError(code, 'test');
      expect(err.code).toBe(code);
    }
  });
});

// ─── divergencePoint ─────────────────────────────────────────────────────────

describe('divergencePoint', () => {
  it('creates a DivergencePoint from a valid ISO-8601 timestamp', () => {
    const dp = divergencePoint('2026-06-29T19:00:00.000Z');
    expect(dp).toBe('2026-06-29T19:00:00.000Z');
  });

  it('trims surrounding whitespace', () => {
    const dp = divergencePoint('  2026-06-29T19:00:00.000Z  ');
    expect(dp).toBe('2026-06-29T19:00:00.000Z');
  });

  it('rejects an empty string', () => {
    expect(() => divergencePoint('')).toThrow();
  });

  it('rejects a whitespace-only string', () => {
    expect(() => divergencePoint('   ')).toThrow();
  });

  it('rejects a string without YYYY-MM-DD prefix (numeric-only)', () => {
    expect(() => divergencePoint('12345')).toThrow();
  });

  it('rejects a non-date string', () => {
    expect(() => divergencePoint('not-a-date')).toThrow();
  });


});

// ─── branchOwnership ─────────────────────────────────────────────────────────

describe('branchOwnership', () => {
  it('creates an ownership record with the expected fields', () => {
    const own = ownership('product');
    expect(own.branchId).toBe(BRANCH);
    expect(own.workspaceId).toBe(WS);
    expect(own.discipline).toBe('product');
    expect(own.divergedAt).toBe(DIV_POINT);
    expect(own.createdByStakeholderId).toBe(STAKEHOLDER);
  });

  it('returns a frozen (immutable) object', () => {
    expect(Object.isFrozen(ownership())).toBe(true);
  });

  it('records all four disciplines', () => {
    for (const d of ['product', 'architecture', 'design', 'engineering'] as const) {
      expect(branchOwnership({
        branchId: BRANCH,
        workspaceId: WS,
        discipline: d,
        divergedAt: DIV_POINT,
        createdByStakeholderId: STAKEHOLDER,
      }).discipline).toBe(d);
    }
  });
});

// ─── AC1: A stakeholder can tell which discipline owns a branch for its lifetime ──

describe('AC1 — single-discipline ownership is visible on BranchOwnership', () => {
  it('BranchOwnership carries the discipline for its lifetime', () => {
    const own = ownership('architecture');
    expect(own.discipline).toBe('architecture');
  });

  it('the discipline field is immutable — cannot be overwritten', () => {
    const own = ownership('design');
    expect(() => {
      // @ts-expect-error — intentional runtime mutation attempt
      (own as Record<string, unknown>)['discipline'] = 'engineering';
    }).toThrow();
    expect(own.discipline).toBe('design');
  });

  it('divergedAt records when the branch diverged from mainline', () => {
    // Meridian IDEA-41: divergence point = diverged_at timestamp
    const ts = '2026-01-15T10:00:00.000Z';
    const own = branchOwnership({
      branchId: BRANCH,
      workspaceId: WS,
      discipline: 'product',
      divergedAt: divergencePoint(ts),
      createdByStakeholderId: STAKEHOLDER,
    });
    expect(own.divergedAt).toBe(ts);
  });
});

// ─── AC2: A stakeholder can tell when branch work is editable or locked ───────

describe('AC2 — editable vs locked states', () => {
  it('isDraftBranch returns true for draft', () => {
    expect(isDraftBranch('draft')).toBe(true);
  });

  it('isDraftBranch returns false for submitted', () => {
    expect(isDraftBranch('submitted')).toBe(false);
  });

  it('isDraftBranch returns false for verified', () => {
    expect(isDraftBranch('verified')).toBe(false);
  });

  it('isDraftBranch returns false for merged', () => {
    expect(isDraftBranch('merged')).toBe(false);
  });

  it('isWriteLocked returns false for draft', () => {
    expect(isWriteLocked('draft')).toBe(false);
  });

  it('isWriteLocked returns true for submitted (in review)', () => {
    expect(isWriteLocked('submitted')).toBe(true);
  });

  it('isWriteLocked returns true for verified', () => {
    expect(isWriteLocked('verified')).toBe(true);
  });

  it('isWriteLocked returns true for merged (terminal)', () => {
    expect(isWriteLocked('merged')).toBe(true);
  });

  it('isMergedBranch returns true only for merged', () => {
    const states: BranchState[] = ['draft', 'submitted', 'verified', 'merged'];
    const results = states.map(isMergedBranch);
    expect(results).toEqual([false, false, false, true]);
  });

  it('draft and write-locked are mutually exclusive', () => {
    const allStates: BranchState[] = ['draft', 'submitted', 'verified', 'merged'];
    for (const state of allStates) {
      expect(isDraftBranch(state) && isWriteLocked(state)).toBe(false);
    }
  });
});

// ─── AC3: A stakeholder from the branch discipline can submit branch work ─────

describe('AC3 — submit requires human actor from branch discipline', () => {
  it('assertSubmitDiscipline passes when disciplines match', () => {
    expect(() => assertSubmitDiscipline('engineering', 'engineering')).not.toThrow();
  });

  it('assertSubmitDiscipline throws discipline-boundary-violation when disciplines differ', () => {
    expect(() => assertSubmitDiscipline('product', 'engineering'))
      .toThrow(BranchLifecycleError);

    try {
      assertSubmitDiscipline('product', 'engineering');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('discipline-boundary-violation');
    }
  });

  it('submitBranch transitions draft to submitted for matching discipline', () => {
    const next = submitBranch('draft', HUMAN, 'engineering', 'engineering');
    expect(next).toBe('submitted');
  });

  it('submitBranch throws invalid-state-transition when branch is not draft', () => {
    for (const state of ['submitted', 'verified', 'merged'] as BranchState[]) {
      expect(() => submitBranch(state, HUMAN, 'engineering', 'engineering'))
        .toThrow(BranchLifecycleError);

      try {
        submitBranch(state, HUMAN, 'engineering', 'engineering');
      } catch (err) {
        expect((err as BranchLifecycleError).code).toBe('invalid-state-transition');
      }
    }
  });

  it('submitBranch throws discipline-boundary-violation for wrong discipline', () => {
    expect(() => submitBranch('draft', HUMAN, 'product', 'engineering'))
      .toThrow(BranchLifecycleError);

    try {
      submitBranch('draft', HUMAN, 'product', 'engineering');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('discipline-boundary-violation');
    }
  });

  it('submitBranch throws unauthorized-actor when a delegated actor attempts submission', () => {
    try {
      submitBranch('draft', DELEGATED, 'engineering', 'engineering');
      expect.fail('should have thrown');
    } catch (err) {
      expect(err).toBeInstanceOf(BranchLifecycleError);
      expect((err as BranchLifecycleError).code).toBe('unauthorized-actor');
    }
  });

  // AC3 — HumanActorContext type accepted; delegated actors rejected at runtime
  // with 'unauthorized-actor'. Real session authentication is the application
  // boundary's responsibility (technical spec §"Protected operation contracts").
});

// ─── AC4: submitted/verified/merged changes are write-locked ─────────────────

describe('AC4 — graph writes blocked on write-locked branches', () => {
  it('assertGraphWriteAllowed does not throw for draft', () => {
    expect(() => assertGraphWriteAllowed('draft')).not.toThrow();
  });

  it('assertGraphWriteAllowed throws write-locked for submitted', () => {
    expect(() => assertGraphWriteAllowed('submitted')).toThrow(BranchLifecycleError);
    try {
      assertGraphWriteAllowed('submitted');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('write-locked');
    }
  });

  it('assertGraphWriteAllowed throws write-locked for verified', () => {
    expect(() => assertGraphWriteAllowed('verified')).toThrow(BranchLifecycleError);
    try {
      assertGraphWriteAllowed('verified');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('write-locked');
    }
  });

  it('assertGraphWriteAllowed throws write-locked for merged', () => {
    expect(() => assertGraphWriteAllowed('merged')).toThrow(BranchLifecycleError);
    try {
      assertGraphWriteAllowed('merged');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('write-locked');
    }
  });

  it('verifyBranch transitions submitted to verified (human-only)', () => {
    expect(verifyBranch('submitted', HUMAN)).toBe('verified');
  });

  it('verifyBranch throws unauthorized-actor when a delegated actor attempts verification', () => {
    try {
      verifyBranch('submitted', DELEGATED);
      expect.fail('should have thrown');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('unauthorized-actor');
    }
  });

  it('verifyBranch throws invalid-state-transition if not submitted', () => {
    for (const state of ['draft', 'verified', 'merged'] as BranchState[]) {
      try {
        verifyBranch(state, HUMAN);
        expect.fail('should have thrown');
      } catch (err) {
        expect((err as BranchLifecycleError).code).toBe('invalid-state-transition');
      }
    }
  });

  it('mergeBranch transitions verified to merged (terminal, human-only)', () => {
    expect(mergeBranch('verified', HUMAN)).toBe('merged');
  });

  it('mergeBranch throws unauthorized-actor when a delegated actor attempts merge', () => {
    try {
      mergeBranch('verified', DELEGATED);
      expect.fail('should have thrown');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('unauthorized-actor');
    }
  });

  it('mergeBranch throws invalid-state-transition if not verified', () => {
    for (const state of ['draft', 'submitted', 'merged'] as BranchState[]) {
      try {
        mergeBranch(state, HUMAN);
        expect.fail('should have thrown');
      } catch (err) {
        expect((err as BranchLifecycleError).code).toBe('invalid-state-transition');
      }
    }
  });

  it('returnToDraft transitions submitted to draft', () => {
    expect(returnToDraft('submitted', HUMAN)).toBe('draft');
  });

  it('returnToDraft throws unauthorized-actor when a delegated actor attempts return-to-draft', () => {
    try {
      returnToDraft('submitted', DELEGATED);
      expect.fail('should have thrown');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('unauthorized-actor');
    }
  });

  it('returnToDraft transitions verified to draft', () => {
    expect(returnToDraft('verified', HUMAN)).toBe('draft');
  });

  it('returnToDraft throws invalid-state-transition for merged (terminal)', () => {
    try {
      returnToDraft('merged', HUMAN);
      expect.fail('should have thrown');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('invalid-state-transition');
    }
  });

  it('returnToDraft throws invalid-state-transition if already draft', () => {
    try {
      returnToDraft('draft', HUMAN);
      expect.fail('should have thrown');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('invalid-state-transition');
    }
  });
});

// ─── AC5: A stakeholder can trace merged work back to the branch ──────────────

describe('AC5 — merge lineage and graph provenance enable traceability', () => {
  it('mergeLineage captures branch identity and merge provenance', () => {
    const own = ownership('product');
    const lineage: MergeLineage = mergeLineage(own, '2026-06-30T12:00:00.000Z', HUMAN);

    expect(lineage.branchId).toBe(BRANCH);
    expect(lineage.workspaceId).toBe(WS);
    expect(lineage.discipline).toBe('product');
    expect(lineage.divergedAt).toBe(DIV_POINT);
    expect(lineage.mergedAt).toBe('2026-06-30T12:00:00.000Z');
    expect(lineage.mergedByStakeholderId).toBe(STAKEHOLDER);
  });

  it('mergeLineage is frozen (immutable)', () => {
    const lineage = mergeLineage(ownership(), '2026-06-30T12:00:00.000Z', HUMAN);
    expect(Object.isFrozen(lineage)).toBe(true);
  });

  it('mergeLineage retains the full divergence point from the original branch', () => {
    // divergedAt enables reconstruction of the mainline state at branch creation
    const ts = '2026-06-01T08:00:00.000Z';
    const own = branchOwnership({
      branchId: BRANCH,
      workspaceId: WS,
      discipline: 'engineering',
      divergedAt: divergencePoint(ts),
      createdByStakeholderId: STAKEHOLDER,
    });
    const lineage = mergeLineage(own, '2026-06-30T12:00:00.000Z', HUMAN);
    expect(lineage.divergedAt).toBe(ts);
  });

  it('mergeLineage throws if mergedAt is not a valid date', () => {
    expect(() => mergeLineage(ownership(), 'not-a-date', HUMAN)).toThrow();
  });

  it('mergeLineage throws invalid-state-transition if mergedAt is before divergedAt', () => {
    // A merge cannot precede the branch divergence point — retrograde lineage
    // would produce incoherent provenance records.
    const own = branchOwnership({
      branchId: BRANCH,
      workspaceId: WS,
      discipline: 'engineering',
      divergedAt: divergencePoint('2026-06-15T10:00:00.000Z'),
      createdByStakeholderId: STAKEHOLDER,
    });
    try {
      mergeLineage(own, '2026-06-01T00:00:00.000Z', HUMAN);
      expect.fail('should have thrown');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('invalid-state-transition');
    }
  });

  it('withBranchProvenance attaches sourceBranchId, sourceWorkspaceId, sourceDiscipline to any graph item', () => {
    const own = ownership('design');
    const chunk = { label: 'IDEA-99', title: 'Some chunk' };
    const stamped = withBranchProvenance(chunk, own);

    const prov: BranchGraphProvenance = stamped.branchProvenance;
    expect(prov.sourceBranchId).toBe(BRANCH);
    expect(prov.sourceWorkspaceId).toBe(WS);
    expect(prov.sourceDiscipline).toBe('design');
  });

  it('withBranchProvenance preserves all original fields on the item', () => {
    const chunk = { label: 'IDEA-99', title: 'Some chunk' };
    const stamped = withBranchProvenance(chunk, ownership());
    expect(stamped.label).toBe('IDEA-99');
    expect(stamped.title).toBe('Some chunk');
  });

  it('withBranchProvenance returns a frozen object', () => {
    const stamped = withBranchProvenance({ label: 'IDEA-99' }, ownership());
    expect(Object.isFrozen(stamped)).toBe(true);
  });

  it('mergeLineage links back to the same branchId, enabling forward-then-back traceability', () => {
    // Represents: "given a merged mainline item stamped with sourceBranchId,
    // a stakeholder can look up that branchId in MergeLineage records."
    const own = ownership('product');
    const lineage = mergeLineage(own, '2026-06-30T12:00:00.000Z', HUMAN);
    const item = withBranchProvenance({ label: 'IDEA-42' }, own);

    // The item's provenance points at the same branch as the lineage record.
    expect(item.branchProvenance.sourceBranchId).toBe(lineage.branchId);
  });
});

// ─── Branch isolation — discipline boundary for graph writes ──────────────────

describe('assertDisciplineBoundaryForWrite — branch isolation invariant', () => {
  it('does not throw when branch and target discipline match', () => {
    expect(() => assertDisciplineBoundaryForWrite('engineering', 'engineering')).not.toThrow();
  });

  it('throws branch-isolation-violation when disciplines differ', () => {
    expect(() => assertDisciplineBoundaryForWrite('engineering', 'product'))
      .toThrow(BranchLifecycleError);

    try {
      assertDisciplineBoundaryForWrite('engineering', 'product');
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('branch-isolation-violation');
    }
  });

  it('throws for all cross-discipline combinations', () => {
    const disciplines = ['product', 'architecture', 'design', 'engineering'] as const;
    for (const branch of disciplines) {
      for (const target of disciplines) {
        if (branch === target) continue;
        expect(() => assertDisciplineBoundaryForWrite(branch, target))
          .toThrow(BranchLifecycleError);
      }
    }
  });
});

// ─── assertWorkspaceMatch — tenant boundary isolation ─────────────────────────

describe('assertWorkspaceMatch — tenant boundary invariant', () => {
  it('does not throw when workspace IDs match', () => {
    expect(() => assertWorkspaceMatch(WS, WS)).not.toThrow();
  });

  it('throws tenant-boundary-violation when workspace IDs differ', () => {
    expect(() => assertWorkspaceMatch(WS, WS2)).toThrow(BranchLifecycleError);

    try {
      assertWorkspaceMatch(WS, WS2);
    } catch (err) {
      expect((err as BranchLifecycleError).code).toBe('tenant-boundary-violation');
    }
  });
});

// ─── Full lifecycle walk-through ──────────────────────────────────────────────

describe('full branch lifecycle progression', () => {
  it('draft → submitted → verified → merged follows the expected path', () => {
    let state: BranchState = 'draft';
    state = submitBranch(state, HUMAN, 'engineering', 'engineering');
    expect(state).toBe('submitted');
    state = verifyBranch(state, HUMAN);
    expect(state).toBe('verified');
    state = mergeBranch(state, HUMAN);
    expect(state).toBe('merged');
    expect(isMergedBranch(state)).toBe(true);
  });

  it('draft → submitted → draft (return-to-draft is human-initiated, never automated)', () => {
    let state: BranchState = 'draft';
    state = submitBranch(state, HUMAN, 'product', 'product');
    expect(state).toBe('submitted');
    state = returnToDraft(state, HUMAN);
    expect(state).toBe('draft');
    expect(isDraftBranch(state)).toBe(true);
  });

  it('draft → submitted → verified → draft (return from verified is also possible)', () => {
    let state: BranchState = 'draft';
    state = submitBranch(state, HUMAN, 'architecture', 'architecture');
    state = verifyBranch(state, HUMAN);
    state = returnToDraft(state, HUMAN);
    expect(state).toBe('draft');
  });

  it('merged is terminal: cannot submit, verify, or return-to-draft from merged', () => {
    const merged: BranchState = 'merged';
    expect(() => submitBranch(merged, HUMAN, 'engineering', 'engineering')).toThrow();
    expect(() => verifyBranch(merged, HUMAN)).toThrow();
    expect(() => returnToDraft(merged, HUMAN)).toThrow();
  });

  it('write-lock applies from submission onwards', () => {
    let state: BranchState = 'draft';
    assertGraphWriteAllowed(state); // must not throw

    state = submitBranch(state, HUMAN, 'design', 'design');
    expect(() => assertGraphWriteAllowed(state)).toThrow(BranchLifecycleError);

    state = verifyBranch(state, HUMAN);
    expect(() => assertGraphWriteAllowed(state)).toThrow(BranchLifecycleError);

    state = mergeBranch(state, HUMAN);
    expect(() => assertGraphWriteAllowed(state)).toThrow(BranchLifecycleError);
  });
});

// ─── branchSubmittedRecord (S04 AC1) ─────────────────────────────────────────
//
// Accountability record for branch submission — a stakeholder can tell which
// human is accountable for the submit decision.
// Technical spec §"Human accountability"; S04 AC1.

describe('branchSubmittedRecord', () => {
  it('creates a BranchSubmittedRecord with branch ID, workspace, stakeholder, and timestamp', () => {
    const own = ownership();
    const record: BranchSubmittedRecord = branchSubmittedRecord(own, HUMAN, '2026-06-29T20:00:00.000Z');
    expect(record.branchId).toBe(BRANCH);
    expect(record.workspaceId).toBe(WS);
    expect(record.submittedByStakeholderId).toBe(STAKEHOLDER);
    expect(record.submittedAt).toBe('2026-06-29T20:00:00.000Z');
  });

  it('trims surrounding whitespace from submittedAt', () => {
    const own = ownership();
    const record = branchSubmittedRecord(own, HUMAN, '  2026-06-29T20:00:00.000Z  ');
    expect(record.submittedAt).toBe('2026-06-29T20:00:00.000Z');
  });

  it('returns a frozen (immutable) record', () => {
    const own = ownership();
    const record = branchSubmittedRecord(own, HUMAN, '2026-06-29T20:00:00.000Z');
    expect(Object.isFrozen(record)).toBe(true);
  });

  it('AC1: carries the human stakeholder ID so submission is accountable', () => {
    const own = ownership();
    const record = branchSubmittedRecord(own, HUMAN, '2026-06-29T20:00:00.000Z');
    expect(record.submittedByStakeholderId).toBe(STAKEHOLDER);
  });

  it('throws BranchLifecycleError when a delegated actor is passed — defense-in-depth runtime guard (S04 AC3)', () => {
    expect(() =>
      branchSubmittedRecord(ownership(), DELEGATED, '2026-06-29T20:00:00.000Z'),
    ).toThrow(BranchLifecycleError);
  });

  it('delegated-actor error code is unauthorized-actor', () => {
    expect(() =>
      branchSubmittedRecord(ownership(), DELEGATED, '2026-06-29T20:00:00.000Z'),
    ).toThrow(expect.objectContaining({ code: 'unauthorized-actor' }));
  });

  it('rejects an empty submittedAt timestamp', () => {
    expect(() => branchSubmittedRecord(ownership(), HUMAN, '')).toThrow(
      VocabularyValidationError,
    );
  });

  it('rejects a non-ISO submittedAt string', () => {
    expect(() =>
      branchSubmittedRecord(ownership(), HUMAN, 'not-a-date'),
    ).toThrow(VocabularyValidationError);
  });
});

// ─── branchVerifiedRecord (S04 AC1) ──────────────────────────────────────────
//
// Accountability record for branch verification — a stakeholder can tell which
// human is accountable for the verify decision.
// Technical spec §"Human accountability"; S04 AC1.

describe('branchVerifiedRecord', () => {
  it('creates a BranchVerifiedRecord with branch ID, workspace, stakeholder, and timestamp', () => {
    const own = ownership();
    const record: BranchVerifiedRecord = branchVerifiedRecord(own, HUMAN, '2026-06-29T21:00:00.000Z');
    expect(record.branchId).toBe(BRANCH);
    expect(record.workspaceId).toBe(WS);
    expect(record.verifiedByStakeholderId).toBe(STAKEHOLDER);
    expect(record.verifiedAt).toBe('2026-06-29T21:00:00.000Z');
  });

  it('trims surrounding whitespace from verifiedAt', () => {
    const own = ownership();
    const record = branchVerifiedRecord(own, HUMAN, '  2026-06-29T21:00:00.000Z  ');
    expect(record.verifiedAt).toBe('2026-06-29T21:00:00.000Z');
  });

  it('returns a frozen (immutable) record', () => {
    const own = ownership();
    const record = branchVerifiedRecord(own, HUMAN, '2026-06-29T21:00:00.000Z');
    expect(Object.isFrozen(record)).toBe(true);
  });

  it('AC1: carries the human stakeholder ID so verification is accountable', () => {
    const own = ownership();
    const record = branchVerifiedRecord(own, HUMAN, '2026-06-29T21:00:00.000Z');
    expect(record.verifiedByStakeholderId).toBe(STAKEHOLDER);
  });

  it('throws BranchLifecycleError when a delegated actor is passed — defense-in-depth runtime guard (S04 AC3)', () => {
    expect(() =>
      branchVerifiedRecord(ownership(), DELEGATED, '2026-06-29T21:00:00.000Z'),
    ).toThrow(BranchLifecycleError);
  });

  it('delegated-actor error code is unauthorized-actor', () => {
    expect(() =>
      branchVerifiedRecord(ownership(), DELEGATED, '2026-06-29T21:00:00.000Z'),
    ).toThrow(expect.objectContaining({ code: 'unauthorized-actor' }));
  });

  it('rejects an empty verifiedAt timestamp', () => {
    expect(() => branchVerifiedRecord(ownership(), HUMAN, '')).toThrow(
      VocabularyValidationError,
    );
  });

  it('rejects a non-ISO verifiedAt string', () => {
    expect(() =>
      branchVerifiedRecord(ownership(), HUMAN, 'not-a-date'),
    ).toThrow(VocabularyValidationError);
  });
});
