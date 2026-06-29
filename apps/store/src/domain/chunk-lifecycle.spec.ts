import { describe, expect, it } from 'vitest';
import {
  ChunkLifecycleValidationError,
  chunkLifecycleStatus,
  isDraftChunk,
  isApprovedChunk,
  isPromotedChunk,
  isActiveChunk,
  isSupersededChunk,
  isInactiveChunk,
  isSafeForImplementationUse,
  type ChunkLifecycleStatus,
} from './chunk-lifecycle.js';

/**
 * Tests for chunk-lifecycle value objects and predicates.
 *
 * Story: S02 — See whether idea context is safe to use.
 * Sources of authority:
 *   - docs/specifications/feature-01-core-domain-model/stories/S02-chunk-lifecycle-clarity.md
 *   - docs/specifications/feature-01-core-domain-model/technical-specification.md
 *     §"Required lifecycle contracts — Chunk"
 *   - Meridian IDEA-38 (superseded state is lineage, not deletion)
 */

// ─── ChunkLifecycleValidationError ───────────────────────────────────────────

describe('ChunkLifecycleValidationError', () => {
  it('is an Error with typed lifecycle and activity state fields', () => {
    const err = new ChunkLifecycleValidationError(
      'draft',
      'superseded',
      'draft chunks cannot be superseded',
    );
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe('ChunkLifecycleValidationError');
    expect(err.lifecycleState).toBe('draft');
    expect(err.activityState).toBe('superseded');
    expect(err.reason).toBe('draft chunks cannot be superseded');
    expect(err.message).toContain('draft');
    expect(err.message).toContain('superseded');
  });

  it('carries a stable machine-readable code for adapter mapping', () => {
    // Technical spec §"Required domain error categories": adapters must map
    // domain failures by category, not by inspecting free-form message text.
    const err = new ChunkLifecycleValidationError('draft', 'inactive', 'reason');
    expect(err.code).toBe('invalid-state-transition');
  });
});

// ─── chunkLifecycleStatus constructor ────────────────────────────────────────

describe('chunkLifecycleStatus', () => {
  describe('valid combinations', () => {
    it('accepts draft + active', () => {
      const s = chunkLifecycleStatus('draft', 'active');
      expect(s.lifecycleState).toBe('draft');
      expect(s.activityState).toBe('active');
    });

    it('accepts approved + active', () => {
      const s = chunkLifecycleStatus('approved', 'active');
      expect(s.lifecycleState).toBe('approved');
      expect(s.activityState).toBe('active');
    });

    it('accepts approved + superseded', () => {
      const s = chunkLifecycleStatus('approved', 'superseded');
      expect(s.lifecycleState).toBe('approved');
      expect(s.activityState).toBe('superseded');
    });

    it('accepts approved + inactive', () => {
      const s = chunkLifecycleStatus('approved', 'inactive');
      expect(s.lifecycleState).toBe('approved');
      expect(s.activityState).toBe('inactive');
    });

    it('accepts promoted + active', () => {
      const s = chunkLifecycleStatus('promoted', 'active');
      expect(s.lifecycleState).toBe('promoted');
      expect(s.activityState).toBe('active');
    });

    it('accepts promoted + superseded', () => {
      const s = chunkLifecycleStatus('promoted', 'superseded');
      expect(s.lifecycleState).toBe('promoted');
      expect(s.activityState).toBe('superseded');
    });

    it('accepts promoted + inactive', () => {
      const s = chunkLifecycleStatus('promoted', 'inactive');
      expect(s.lifecycleState).toBe('promoted');
      expect(s.activityState).toBe('inactive');
    });
  });

  describe('invalid combinations — draft may only be active', () => {
    // Technical spec: "Approved or Promoted chunks may become Superseded or inactive."
    // Draft chunks are always active; they have not yet been approved so no later-stage
    // activity state applies to them.

    it('rejects draft + superseded', () => {
      expect(() => chunkLifecycleStatus('draft', 'superseded')).toThrow(
        ChunkLifecycleValidationError,
      );
    });

    it('rejects draft + inactive', () => {
      expect(() => chunkLifecycleStatus('draft', 'inactive')).toThrow(
        ChunkLifecycleValidationError,
      );
    });
  });

  it('returns a frozen (immutable) object', () => {
    const s = chunkLifecycleStatus('approved', 'active');
    expect(Object.isFrozen(s)).toBe(true);
  });

  it('the returned value carries the expected state fields', () => {
    // The type is opaque (branded): callers cannot construct a valid
    // ChunkLifecycleStatus by structural assignment — the factory is the
    // only valid entry point. This test confirms that factory output is
    // inspectable via the predicate API.
    const s = chunkLifecycleStatus('approved', 'active');
    expect(isApprovedChunk(s)).toBe(true);
    expect(isActiveChunk(s)).toBe(true);
  });
});

// ─── AC1: A stakeholder can tell whether an idea chunk is draft, approved,
//     promoted, superseded, or inactive ─────────────────────────────────────

describe('AC1 — all observable states are distinguishable', () => {
  it('isDraftChunk returns true for a draft+active chunk', () => {
    expect(isDraftChunk(chunkLifecycleStatus('draft', 'active'))).toBe(true);
  });

  it('isDraftChunk returns false for an approved chunk', () => {
    expect(isDraftChunk(chunkLifecycleStatus('approved', 'active'))).toBe(false);
  });

  it('isDraftChunk returns false for a promoted chunk', () => {
    expect(isDraftChunk(chunkLifecycleStatus('promoted', 'active'))).toBe(false);
  });

  it('isApprovedChunk returns true for an approved+active chunk', () => {
    expect(isApprovedChunk(chunkLifecycleStatus('approved', 'active'))).toBe(true);
  });

  it('isApprovedChunk returns false for a draft chunk', () => {
    expect(isApprovedChunk(chunkLifecycleStatus('draft', 'active'))).toBe(false);
  });

  it('isApprovedChunk returns false for a promoted chunk', () => {
    expect(isApprovedChunk(chunkLifecycleStatus('promoted', 'active'))).toBe(false);
  });

  it('isPromotedChunk returns true for a promoted+active chunk', () => {
    expect(isPromotedChunk(chunkLifecycleStatus('promoted', 'active'))).toBe(true);
  });

  it('isPromotedChunk returns false for a draft chunk', () => {
    expect(isPromotedChunk(chunkLifecycleStatus('draft', 'active'))).toBe(false);
  });

  it('isSupersededChunk returns true for an approved+superseded chunk', () => {
    expect(isSupersededChunk(chunkLifecycleStatus('approved', 'superseded'))).toBe(true);
  });

  it('isSupersededChunk returns true for a promoted+superseded chunk', () => {
    expect(isSupersededChunk(chunkLifecycleStatus('promoted', 'superseded'))).toBe(true);
  });

  it('isSupersededChunk returns false for an active chunk', () => {
    expect(isSupersededChunk(chunkLifecycleStatus('approved', 'active'))).toBe(false);
  });

  it('isInactiveChunk returns true for an approved+inactive chunk', () => {
    expect(isInactiveChunk(chunkLifecycleStatus('approved', 'inactive'))).toBe(true);
  });

  it('isInactiveChunk returns true for a promoted+inactive chunk', () => {
    expect(isInactiveChunk(chunkLifecycleStatus('promoted', 'inactive'))).toBe(true);
  });

  it('isInactiveChunk returns false for an active chunk', () => {
    expect(isInactiveChunk(chunkLifecycleStatus('approved', 'active'))).toBe(false);
  });

  it('isActiveChunk returns true only for an active chunk', () => {
    expect(isActiveChunk(chunkLifecycleStatus('approved', 'active'))).toBe(true);
    expect(isActiveChunk(chunkLifecycleStatus('approved', 'superseded'))).toBe(false);
    expect(isActiveChunk(chunkLifecycleStatus('approved', 'inactive'))).toBe(false);
  });
});

// ─── AC2: A stakeholder can tell that draft context is not approved
//     implementation context ──────────────────────────────────────────────────

describe('AC2 — draft context is not approved implementation context', () => {
  it('a draft chunk is not approved', () => {
    const draft = chunkLifecycleStatus('draft', 'active');
    expect(isApprovedChunk(draft)).toBe(false);
    expect(isPromotedChunk(draft)).toBe(false);
  });

  it('a draft chunk is not safe for implementation use', () => {
    const draft = chunkLifecycleStatus('draft', 'active');
    expect(isSafeForImplementationUse(draft)).toBe(false);
  });

  it('an approved chunk is not draft', () => {
    const approved = chunkLifecycleStatus('approved', 'active');
    expect(isDraftChunk(approved)).toBe(false);
  });

  it('draft and approved are mutually exclusive lifecycle stages', () => {
    const draft = chunkLifecycleStatus('draft', 'active');
    const approved = chunkLifecycleStatus('approved', 'active');
    expect(isDraftChunk(draft) && isApprovedChunk(draft)).toBe(false);
    expect(isDraftChunk(approved) && isApprovedChunk(approved)).toBe(false);
  });
});

// ─── AC3: An implementation agent can receive only context that is safe for
//     implementation use — all nine valid state combinations verified ────────

describe('AC3 — isSafeForImplementationUse covers all valid state combinations', () => {
  it('approved + active → safe', () => {
    expect(isSafeForImplementationUse(chunkLifecycleStatus('approved', 'active'))).toBe(true);
  });

  it('promoted + active → safe', () => {
    expect(isSafeForImplementationUse(chunkLifecycleStatus('promoted', 'active'))).toBe(true);
  });

  it('draft + active → NOT safe (lifecycle stage wins)', () => {
    expect(isSafeForImplementationUse(chunkLifecycleStatus('draft', 'active'))).toBe(false);
  });

  it('approved + superseded → NOT safe (activity state wins)', () => {
    expect(isSafeForImplementationUse(chunkLifecycleStatus('approved', 'superseded'))).toBe(false);
  });

  it('promoted + superseded → NOT safe (activity state wins)', () => {
    expect(isSafeForImplementationUse(chunkLifecycleStatus('promoted', 'superseded'))).toBe(false);
  });

  it('approved + inactive → NOT safe', () => {
    expect(isSafeForImplementationUse(chunkLifecycleStatus('approved', 'inactive'))).toBe(false);
  });

  it('promoted + inactive → NOT safe', () => {
    expect(isSafeForImplementationUse(chunkLifecycleStatus('promoted', 'inactive'))).toBe(false);
  });
});

// ─── AC4: A stakeholder can tell when an approved or promoted idea has been
//     replaced without losing the fact that it previously existed ─────────────

describe('AC4 — superseded chunks remain inspectable (Meridian IDEA-38: lineage, not deletion)', () => {
  it('an approved chunk that was replaced is representable as approved+superseded', () => {
    // The chunk still carries its lifecycle stage so the stakeholder can see
    // it was previously approved — history is preserved.
    const replaced = chunkLifecycleStatus('approved', 'superseded');
    expect(isApprovedChunk(replaced)).toBe(true);
    expect(isSupersededChunk(replaced)).toBe(true);
  });

  it('a promoted chunk that was replaced is representable as promoted+superseded', () => {
    const replaced = chunkLifecycleStatus('promoted', 'superseded');
    expect(isPromotedChunk(replaced)).toBe(true);
    expect(isSupersededChunk(replaced)).toBe(true);
  });

  it('a superseded chunk retains both its original lifecycle stage and its superseded activity state', () => {
    const approvedThenSuperseded = chunkLifecycleStatus('approved', 'superseded');
    // Both facts are retained simultaneously:
    expect(approvedThenSuperseded.lifecycleState).toBe('approved');
    expect(approvedThenSuperseded.activityState).toBe('superseded');
  });

  it('an approved+superseded chunk is distinguishable from a promoted+superseded chunk', () => {
    const approvedReplaced = chunkLifecycleStatus('approved', 'superseded');
    const promotedReplaced = chunkLifecycleStatus('promoted', 'superseded');
    expect(approvedReplaced.lifecycleState).not.toBe(promotedReplaced.lifecycleState);
    expect(isApprovedChunk(approvedReplaced)).toBe(true);
    expect(isPromotedChunk(promotedReplaced)).toBe(true);
  });

  it('a superseded chunk is never safe for implementation use, preserving replacement semantics', () => {
    const replaced: ChunkLifecycleStatus = chunkLifecycleStatus('approved', 'superseded');
    expect(isSafeForImplementationUse(replaced)).toBe(false);
  });
});
