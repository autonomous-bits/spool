import { describe, expect, it } from 'vitest';
import { BranchGraphProvenance, MergeLineage } from './merge-lineage.js';
import { DivergencePoint } from './divergence-point.js';

describe('MergeLineage', () => {
  it('constructs successfully with valid inputs', () => {
    const divergedAt = new DivergencePoint('2026-07-01T00:00:00.000Z');
    const mergedAt = new Date('2026-07-06T09:00:00.000Z');

    const lineage = new MergeLineage({
      branchId: '00000000-0000-0000-0000-000000000001',
      discipline: 'product',
      divergedAt,
      mergedAt,
      mergedByStakeholderId: '00000000-0000-0000-0000-000000000002',
    });

    expect(lineage.branchId).toBe('00000000-0000-0000-0000-000000000001');
    expect(lineage.discipline).toBe('product');
    expect(lineage.divergedAt).toBe(divergedAt);
    expect(lineage.mergedAt).toBe(mergedAt);
    expect(lineage.mergedByStakeholderId).toBe('00000000-0000-0000-0000-000000000002');
  });

  it.each(['', '   '])('rejects blank branchId %j', (branchId) => {
    expect(
      () =>
        new MergeLineage({
          branchId,
          discipline: 'product',
          divergedAt: new DivergencePoint(),
          mergedAt: new Date(),
          mergedByStakeholderId: '00000000-0000-0000-0000-000000000002',
        }),
    ).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects blank mergedByStakeholderId %j', (mergedByStakeholderId) => {
    expect(
      () =>
        new MergeLineage({
          branchId: '00000000-0000-0000-0000-000000000001',
          discipline: 'product',
          divergedAt: new DivergencePoint(),
          mergedAt: new Date(),
          mergedByStakeholderId,
        }),
    ).toThrow(TypeError);
  });
});

describe('BranchGraphProvenance', () => {
  it('constructs successfully with valid inputs', () => {
    const provenance = new BranchGraphProvenance({
      sourceBranchId: '00000000-0000-0000-0000-000000000001',
    });

    expect(provenance.sourceBranchId).toBe('00000000-0000-0000-0000-000000000001');
  });

  it.each(['', '   '])('rejects blank sourceBranchId %j', (sourceBranchId) => {
    expect(() => new BranchGraphProvenance({ sourceBranchId })).toThrow(TypeError);
  });
});
