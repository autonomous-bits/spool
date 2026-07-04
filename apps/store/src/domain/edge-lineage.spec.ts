import { describe, expect, it } from 'vitest';
import { ideaLabel, workspaceId } from './types/index.js';
import {
  EdgeLineageError,
  createEdge,
  currentEdgeVersion,
  supersedeEdge,
  deactivateEdge,
  isActiveEdge,
  isSupersededEdge,
  isDeactivatedEdge,
  resolveLineage,
  assertNoConflictingActiveEdge,
  assertDeterministicEdgeSet,
  type EdgeLineage,
} from './edge-lineage.js';

/**
 * Tests for edge lineage value objects, determinism, and lineage
 * preservation.
 *
 * Story: S06 — Keep relationships traceable as ideas evolve.
 * Sources of authority:
 *   - docs/specifications/feature-01-core-domain-model/stories/S06-traceable-relationships.md
 *   - docs/specifications/feature-01-core-domain-model/technical-specification.md
 *     §"Logical edge endpoints", §"Edge determinism", §"Edge lineage"
 *   - Meridian IDEA-36, IDEA-37, IDEA-38
 */

const ws1 = workspaceId('workspace-1');
const ws2 = workspaceId('workspace-2');
const ideaA = ideaLabel('IDEA-A');
const ideaB = ideaLabel('IDEA-B');
const ideaC = ideaLabel('IDEA-C');

// ─── AC1: relationships are visible using stable idea labels ────────────────

describe('AC1 — relationships are visible using stable idea labels', () => {
  it('createEdge produces an active version keyed by IdeaLabel endpoints', () => {
    const lineage = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const current = currentEdgeVersion(lineage);
    expect(current.sourceLabel).toBe(ideaA);
    expect(current.targetLabel).toBe(ideaB);
    expect(current.relationshipType).toBe('depends-on');
    expect(current.state).toBe('active');
  });

  it('the workspace is part of the relationship identity', () => {
    const lineage = createEdge(ws1, ideaA, ideaB, 'refines');
    expect(currentEdgeVersion(lineage).workspaceId).toBe(ws1);
  });
});

// ─── AC2: current active relationship is distinguishable from replaced ones ─

describe('AC2 — the currently active relationship is distinguishable from replaced ones', () => {
  it('a freshly created edge is active', () => {
    const lineage = createEdge(ws1, ideaA, ideaB, 'depends-on');
    expect(isActiveEdge(currentEdgeVersion(lineage))).toBe(true);
  });

  it('after supersession, the new current version is active and the old one is superseded', () => {
    const original = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const superseded = supersedeEdge(original, {
      workspaceId: ws1,
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'depends-on',
    });
    const current = currentEdgeVersion(superseded);
    expect(isActiveEdge(current)).toBe(true);

    const history = resolveLineage(superseded);
    expect(history).toHaveLength(2);
    expect(isActiveEdge(history[0]!)).toBe(true);
    expect(isSupersededEdge(history[1]!)).toBe(true);
  });

  it('after deactivation, the current version is deactivated, not active', () => {
    const original = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const deactivated = deactivateEdge(original);
    expect(isActiveEdge(currentEdgeVersion(deactivated))).toBe(false);
    expect(isDeactivatedEdge(currentEdgeVersion(deactivated))).toBe(true);
  });
});

// ─── AC3: a replaced relationship can be traced through its prior versions ──

describe('AC3 — a replaced relationship can be traced through its prior versions', () => {
  it('resolveLineage returns a single-element history for a never-superseded edge', () => {
    const lineage = createEdge(ws1, ideaA, ideaB, 'depends-on');
    expect(resolveLineage(lineage)).toHaveLength(1);
  });

  it('resolveLineage returns newest-first history across multiple supersessions', () => {
    const v1 = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const identity = {
      workspaceId: ws1,
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'depends-on' as const,
    };
    const v2 = supersedeEdge(v1, identity);
    const v3 = supersedeEdge(v2, identity);

    const history = resolveLineage(v3);
    expect(history).toHaveLength(3);
    expect(isActiveEdge(history[0]!)).toBe(true);
    expect(isSupersededEdge(history[1]!)).toBe(true);
    expect(isSupersededEdge(history[2]!)).toBe(true);
  });

  it('every version in a deep lineage keeps the same relationship identity', () => {
    const v1 = createEdge(ws1, ideaA, ideaB, 'refines');
    const identity = {
      workspaceId: ws1,
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'refines' as const,
    };
    const v2 = supersedeEdge(v1, identity);
    const v3 = supersedeEdge(v2, identity);

    for (const version of resolveLineage(v3)) {
      expect(version.sourceLabel).toBe(ideaA);
      expect(version.targetLabel).toBe(ideaB);
      expect(version.relationshipType).toBe('refines');
    }
  });
});

// ─── AC4: no conflicting active relationships for the same triple ───────────

describe('AC4 — a resolved view contains no conflicting active relationships', () => {
  it('assertNoConflictingActiveEdge throws when another lineage in the same workspace is already active for the same triple', () => {
    const existing = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const candidate = currentEdgeVersion(createEdge(ws1, ideaA, ideaB, 'depends-on'));
    expect(() => assertNoConflictingActiveEdge(candidate, [existing])).toThrow(
      EdgeLineageError,
    );
  });

  it('throws with the duplicate-active-relationship code', () => {
    const existing = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const candidate = currentEdgeVersion(createEdge(ws1, ideaA, ideaB, 'depends-on'));
    try {
      assertNoConflictingActiveEdge(candidate, [existing]);
      expect.unreachable('expected assertNoConflictingActiveEdge to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(EdgeLineageError);
      expect((err as EdgeLineageError).code).toBe('duplicate-active-relationship');
    }
  });

  it('a superseded prior version, considered on its own, does not conflict with a new active edge', () => {
    // The prior (now-superseded) version of a lineage is history, not a live
    // relationship — assertNoConflictingActiveEdge only compares against a
    // lineage's *current* version, so wrapping a superseded version alone
    // must not be treated as a conflict.
    const identity = {
      workspaceId: ws1,
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'depends-on' as const,
    };
    const superseded = supersedeEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'), identity);
    const originalVersion = resolveLineage(superseded)[1]!;
    expect(originalVersion.state).toBe('superseded');

    const candidate = currentEdgeVersion(createEdge(ws1, ideaA, ideaB, 'depends-on'));
    // Comparing against the full (still-active-headed) lineage correctly conflicts:
    expect(() => assertNoConflictingActiveEdge(candidate, [superseded])).toThrow(
      EdgeLineageError,
    );
  });

  it('does not throw when the existing edge for the same triple has been deactivated', () => {
    const existing = deactivateEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'));
    const candidate = currentEdgeVersion(createEdge(ws1, ideaA, ideaB, 'depends-on'));
    expect(() => assertNoConflictingActiveEdge(candidate, [existing])).not.toThrow();
  });

  it('does not throw for a different relationship type on the same idea pair', () => {
    const existing = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const candidate = currentEdgeVersion(createEdge(ws1, ideaA, ideaB, 'refines'));
    expect(() => assertNoConflictingActiveEdge(candidate, [existing])).not.toThrow();
  });

  it('does not throw for the reverse direction of the same idea pair (ordered triple)', () => {
    const existing = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const candidate = currentEdgeVersion(createEdge(ws1, ideaB, ideaA, 'depends-on'));
    expect(() => assertNoConflictingActiveEdge(candidate, [existing])).not.toThrow();
  });

  it('does not throw for an identical triple in a different workspace (tenant isolation)', () => {
    const existing = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const candidate = currentEdgeVersion(createEdge(ws2, ideaA, ideaB, 'depends-on'));
    expect(() => assertNoConflictingActiveEdge(candidate, [existing])).not.toThrow();
  });

  it('assertDeterministicEdgeSet passes for a set of distinct active triples within one workspace', () => {
    const lineages: EdgeLineage[] = [
      createEdge(ws1, ideaA, ideaB, 'depends-on'),
      createEdge(ws1, ideaB, ideaC, 'refines'),
    ];
    expect(() => assertDeterministicEdgeSet(lineages)).not.toThrow();
  });

  it('assertDeterministicEdgeSet throws tenant-boundary-violation for a mixed-workspace set', () => {
    const lineages: EdgeLineage[] = [
      createEdge(ws1, ideaA, ideaB, 'depends-on'),
      createEdge(ws2, ideaB, ideaC, 'refines'),
    ];
    try {
      assertDeterministicEdgeSet(lineages);
      expect.unreachable('expected assertDeterministicEdgeSet to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(EdgeLineageError);
      expect((err as EdgeLineageError).code).toBe('tenant-boundary-violation');
    }
  });

  it('assertDeterministicEdgeSet throws when two lineages share an active triple in the same workspace', () => {
    const lineages: EdgeLineage[] = [
      createEdge(ws1, ideaA, ideaB, 'depends-on'),
      createEdge(ws1, ideaA, ideaB, 'depends-on'),
    ];
    expect(() => assertDeterministicEdgeSet(lineages)).toThrow(EdgeLineageError);
  });

  it('assertDeterministicEdgeSet ignores superseded/deactivated lineages when checking determinism', () => {
    const identity = {
      workspaceId: ws1,
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'depends-on' as const,
    };
    const supersededLineage = supersedeEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'), identity);
    const deactivatedLineage = deactivateEdge(createEdge(ws1, ideaB, ideaC, 'refines'));
    expect(() =>
      assertDeterministicEdgeSet([supersededLineage, deactivatedLineage]),
    ).not.toThrow();
  });
});

// ─── AC5: replacing or deactivating a relationship does not erase history ───

describe('AC5 — replacing or deactivating a relationship does not erase its history', () => {
  it('supersedeEdge preserves the prior version rather than deleting it', () => {
    const original = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const superseded = supersedeEdge(original, {
      workspaceId: ws1,
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'depends-on',
    });
    expect(resolveLineage(superseded)).toHaveLength(2);
    // The original lineage value itself is untouched (immutability).
    expect(resolveLineage(original)).toHaveLength(1);
    expect(isActiveEdge(currentEdgeVersion(original))).toBe(true);
  });

  it('deactivateEdge preserves history rather than deleting it', () => {
    const identity = {
      workspaceId: ws1,
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'depends-on' as const,
    };
    const superseded = supersedeEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'), identity);
    const deactivated = deactivateEdge(superseded);
    const history = resolveLineage(deactivated);
    // deactivateEdge appends a new version rather than mutating the current
    // version in place (technical spec §"Edge lineage persistence"): the
    // superseded lineage already had 2 versions, deactivation appends a 3rd.
    expect(history).toHaveLength(3);
    expect(isDeactivatedEdge(history[0]!)).toBe(true);
    expect(isSupersededEdge(history[1]!)).toBe(true);
    expect(isSupersededEdge(history[2]!)).toBe(true);
  });

  it('deactivateEdge supersedes the previously-current version instead of leaving it active', () => {
    const original = createEdge(ws1, ideaA, ideaB, 'depends-on');
    const deactivated = deactivateEdge(original);
    const history = resolveLineage(deactivated);
    expect(history).toHaveLength(2);
    expect(isDeactivatedEdge(history[0]!)).toBe(true);
    expect(isSupersededEdge(history[1]!)).toBe(true);
    expect(history[1]!.sourceLabel).toBe(ideaA);
    expect(history[1]!.targetLabel).toBe(ideaB);
    expect(history[1]!.relationshipType).toBe('depends-on');
    // The original lineage value itself is untouched (immutability).
    expect(resolveLineage(original)).toHaveLength(1);
    expect(isActiveEdge(currentEdgeVersion(original))).toBe(true);
  });

  it('returned lineages are frozen (immutable)', () => {
    const lineage = createEdge(ws1, ideaA, ideaB, 'depends-on');
    expect(Object.isFrozen(lineage)).toBe(true);
    expect(Object.isFrozen(lineage.versions)).toBe(true);
    expect(Object.isFrozen(lineage.versions[0])).toBe(true);
  });

  it('supersedeEdge throws invalid-state-transition when the current version is not active', () => {
    const deactivated = deactivateEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'));
    expect(() =>
      supersedeEdge(deactivated, {
        workspaceId: ws1,
        sourceLabel: ideaA,
        targetLabel: ideaB,
        relationshipType: 'depends-on',
      }),
    ).toThrow(EdgeLineageError);
  });

  it('deactivateEdge throws invalid-state-transition when already deactivated', () => {
    const deactivated = deactivateEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'));
    expect(() => deactivateEdge(deactivated)).toThrow(EdgeLineageError);
  });

  it('a lineage whose current head was re-superseded remains active and deactivatable', () => {
    // The state guard on deactivateEdge is based on the lineage's *current*
    // state, not on whether the lineage has ever been superseded before.
    const identity = {
      workspaceId: ws1,
      sourceLabel: ideaA,
      targetLabel: ideaB,
      relationshipType: 'depends-on' as const,
    };
    const superseded = supersedeEdge(createEdge(ws1, ideaA, ideaB, 'depends-on'), identity);
    expect(isActiveEdge(currentEdgeVersion(superseded))).toBe(true);
    expect(() => deactivateEdge(superseded)).not.toThrow();
  });

  it('supersedeEdge throws lineage-violation when next changes the source label', () => {
    const original = createEdge(ws1, ideaA, ideaB, 'depends-on');
    expect(() =>
      supersedeEdge(original, {
        workspaceId: ws1,
        sourceLabel: ideaC,
        targetLabel: ideaB,
        relationshipType: 'depends-on',
      }),
    ).toThrow(EdgeLineageError);
  });

  it('supersedeEdge throws lineage-violation when next changes the relationship type', () => {
    const original = createEdge(ws1, ideaA, ideaB, 'depends-on');
    expect(() =>
      supersedeEdge(original, {
        workspaceId: ws1,
        sourceLabel: ideaA,
        targetLabel: ideaB,
        relationshipType: 'refines',
      }),
    ).toThrow(EdgeLineageError);
  });

  it('supersedeEdge throws lineage-violation when next changes the workspace', () => {
    const original = createEdge(ws1, ideaA, ideaB, 'depends-on');
    expect(() =>
      supersedeEdge(original, {
        workspaceId: ws2,
        sourceLabel: ideaA,
        targetLabel: ideaB,
        relationshipType: 'depends-on',
      }),
    ).toThrow(EdgeLineageError);
  });

  it('lineage-violation carries the correct machine-readable code', () => {
    const original = createEdge(ws1, ideaA, ideaB, 'depends-on');
    try {
      supersedeEdge(original, {
        workspaceId: ws1,
        sourceLabel: ideaC,
        targetLabel: ideaB,
        relationshipType: 'depends-on',
      });
      expect.unreachable('expected supersedeEdge to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(EdgeLineageError);
      expect((err as EdgeLineageError).code).toBe('lineage-violation');
    }
  });
});
