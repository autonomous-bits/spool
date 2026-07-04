import { describe, expect, it } from 'vitest';
import { artifactId, workspaceId, ideaLabel, type BranchId } from './types/index.js';
import {
  ArtifactAssociationError,
  createAssociation,
  currentAssociationVersion,
  deactivateAssociation,
  isActiveAssociation,
  isSupersededAssociation,
  isDeactivatedAssociation,
  resolveAssociationHistory,
  assertNoConflictingActiveAssociation,
  type ArtifactAssociationLineage,
} from './artifact-association-lineage.js';

/**
 * Tests for chunk-artifact association lineage value objects and
 * append-only versioning.
 *
 * Story: S05 — Keep supporting artifacts traceable through branch review.
 * Sources of authority:
 *   - docs/specifications/feature-02-postgres-persistence/stories/S05-artifact-associations-traceable-through-review.md
 *   - docs/specifications/feature-02-postgres-persistence/technical-specification.md
 *     §"Chunk-artifact association lifecycle", §"Pre-merge history reconstruction"
 *   - Meridian IDEA-60, IDEA-62, IDEA-64
 */

const ws1 = workspaceId('workspace-1');
const ws2 = workspaceId('workspace-2');
const idea1 = ideaLabel('IDEA-1');
const artifact1 = artifactId('artifact-1');
const artifact2 = artifactId('artifact-2');
const branchA = 'branch-a' as BranchId;
const branchB = 'branch-b' as BranchId;

// ─── AC1: trace an association while under branch review ───────────────────

describe('AC1 — trace which artifact is associated with an idea under branch review', () => {
  it('createAssociation with a branchId produces a branch-scoped active version', () => {
    const lineage = createAssociation(ws1, idea1, artifact1, branchA);
    const current = currentAssociationVersion(lineage);
    expect(current.branchId).toBe(branchA);
    expect(current.state).toBe('active');
    expect(current.chunkLabel).toBe(idea1);
    expect(current.artifactId).toBe(artifact1);
  });

  it('createAssociation without a branchId produces a mainline-scoped active version', () => {
    const lineage = createAssociation(ws1, idea1, artifact1);
    const current = currentAssociationVersion(lineage);
    expect(current.branchId).toBeUndefined();
    expect(current.state).toBe('active');
  });

  it('a branch-created association is provenance-tagged to itself via originBranchId', () => {
    const lineage = createAssociation(ws1, idea1, artifact1, branchA);
    expect(currentAssociationVersion(lineage).originBranchId).toBe(branchA);
  });

  it('a mainline-created association has no originBranchId', () => {
    const lineage = createAssociation(ws1, idea1, artifact1);
    expect(currentAssociationVersion(lineage).originBranchId).toBeUndefined();
  });
});

// ─── AC2: branch changes never mutate the mainline lineage ─────────────────

describe('AC2 — branch association changes do not affect the mainline association', () => {
  it('deactivating a branch lineage never touches a separately-held mainline lineage value', () => {
    const mainline = createAssociation(ws1, idea1, artifact1);
    const branchLineage = createAssociation(ws1, idea1, artifact1, branchA);

    const deactivatedBranch = deactivateAssociation(branchLineage);

    expect(currentAssociationVersion(mainline).state).toBe('active');
    expect(currentAssociationVersion(deactivatedBranch).state).toBe('deactivated');
    // The two lineages are distinct values; mutating one never touches the other.
    expect(currentAssociationVersion(mainline).branchId).toBeUndefined();
  });

  it('two different branches produce independent lineages for the same identity', () => {
    const branchLineageA = createAssociation(ws1, idea1, artifact1, branchA);
    const branchLineageB = createAssociation(ws1, idea1, artifact1, branchB);

    const deactivatedA = deactivateAssociation(branchLineageA);

    expect(currentAssociationVersion(deactivatedA).state).toBe('deactivated');
    expect(currentAssociationVersion(branchLineageB).state).toBe('active');
  });
});

// ─── AC3: current status is distinguishable and history is preserved ───────

describe('AC3 — current status is distinguishable and prior associations are preserved', () => {
  it('deactivateAssociation appends a new version rather than mutating the current one in place', () => {
    const lineage = createAssociation(ws1, idea1, artifact1);
    const deactivated = deactivateAssociation(lineage);

    expect(deactivated.versions).toHaveLength(2);
    expect(deactivated.versions[0]?.state).toBe('superseded');
    expect(deactivated.versions[1]?.state).toBe('deactivated');
    // The original lineage value is untouched (append-only, immutable).
    expect(lineage.versions).toHaveLength(1);
    expect(currentAssociationVersion(lineage).state).toBe('active');
  });

  it('resolveAssociationHistory returns every version, newest first', () => {
    const lineage = createAssociation(ws1, idea1, artifact1);
    const deactivated = deactivateAssociation(lineage);
    const history = resolveAssociationHistory(deactivated);

    expect(history.map((v) => v.state)).toEqual(['deactivated', 'superseded']);
  });

  it('a lineage`s first version can only be active; deactivated only ever appears last', () => {
    const lineage = createAssociation(ws1, idea1, artifact1);
    expect(lineage.versions[0]?.state).toBe('active');
    const deactivated = deactivateAssociation(lineage);
    expect(deactivated.versions[deactivated.versions.length - 1]?.state).toBe('deactivated');
  });

  it('isActiveAssociation/isSupersededAssociation/isDeactivatedAssociation classify states correctly', () => {
    const lineage = createAssociation(ws1, idea1, artifact1);
    const active = currentAssociationVersion(lineage);
    expect(isActiveAssociation(active)).toBe(true);
    expect(isSupersededAssociation(active)).toBe(false);
    expect(isDeactivatedAssociation(active)).toBe(false);

    const deactivated = deactivateAssociation(lineage);
    const [superseded, terminal] = deactivated.versions;
    expect(isSupersededAssociation(superseded!)).toBe(true);
    expect(isDeactivatedAssociation(terminal!)).toBe(true);
  });

  it('deactivating an already-deactivated association throws invalid-state-transition', () => {
    const lineage = createAssociation(ws1, idea1, artifact1);
    const deactivated = deactivateAssociation(lineage);
    expect(() => deactivateAssociation(deactivated)).toThrow(ArtifactAssociationError);
    try {
      deactivateAssociation(deactivated);
      expect.fail('expected deactivateAssociation to throw');
    } catch (error) {
      expect((error as ArtifactAssociationError).code).toBe('invalid-state-transition');
    }
  });
});

// ─── Duplicate-active determinism (IDEA-64) ────────────────────────────────

describe('duplicate-active-relationship — at most one active association per identity per scope', () => {
  it('throws when a candidate mainline association conflicts with an existing active mainline lineage', () => {
    const existing: ArtifactAssociationLineage[] = [createAssociation(ws1, idea1, artifact1)];
    const candidate = currentAssociationVersion(createAssociation(ws1, idea1, artifact1));

    expect(() => assertNoConflictingActiveAssociation(candidate, existing)).toThrow(
      ArtifactAssociationError,
    );
  });

  it('does not throw when the candidate is scoped to a different branch than the existing active lineage', () => {
    const existing: ArtifactAssociationLineage[] = [createAssociation(ws1, idea1, artifact1)];
    const candidate = currentAssociationVersion(
      createAssociation(ws1, idea1, artifact1, branchA),
    );

    expect(() => assertNoConflictingActiveAssociation(candidate, existing)).not.toThrow();
  });

  it('does not throw for a different artifact on the same chunk', () => {
    const existing: ArtifactAssociationLineage[] = [createAssociation(ws1, idea1, artifact1)];
    const candidate = currentAssociationVersion(createAssociation(ws1, idea1, artifact2));

    expect(() => assertNoConflictingActiveAssociation(candidate, existing)).not.toThrow();
  });

  it('does not throw across different workspaces', () => {
    const existing: ArtifactAssociationLineage[] = [createAssociation(ws1, idea1, artifact1)];
    const candidate = currentAssociationVersion(createAssociation(ws2, idea1, artifact1));

    expect(() => assertNoConflictingActiveAssociation(candidate, existing)).not.toThrow();
  });

  it('does not throw when the existing lineage for the same identity is inactive', () => {
    const deactivated = deactivateAssociation(createAssociation(ws1, idea1, artifact1));
    const candidate = currentAssociationVersion(createAssociation(ws1, idea1, artifact1));

    expect(() =>
      assertNoConflictingActiveAssociation(candidate, [deactivated]),
    ).not.toThrow();
  });
});
