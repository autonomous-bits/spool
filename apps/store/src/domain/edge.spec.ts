import { describe, expect, it } from 'vitest';
import { Edge, type EdgeProps } from './edge.js';

function validProps(overrides: Partial<EdgeProps> = {}): EdgeProps {
  return {
    fromChunkLabel: 'ATOMIC-1',
    toChunkLabel: 'ATOMIC-2',
    type: 'refines',
    discipline: 'product',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

describe('Edge', () => {
  it('constructs an edge with defaulted status, id, and updatedByStakeholderId', () => {
    const edge = new Edge(validProps());

    expect(edge.fromChunkLabel).toBe('ATOMIC-1');
    expect(edge.toChunkLabel).toBe('ATOMIC-2');
    expect(edge.type).toBe('refines');
    expect(edge.status).toBe('active');
    expect(edge.discipline).toBe('product');
    expect(edge.createdByStakeholderId).toBe('00000000-0000-0000-0000-000000000001');
    expect(edge.updatedByStakeholderId).toBe('00000000-0000-0000-0000-000000000001');
    expect(edge.id).toBeTruthy();
    expect(edge.branchId).toBeUndefined();
    expect(edge.originBranchId).toBeUndefined();
    expect(edge.supersededByEdgeId).toBeUndefined();
  });

  it('constructs a branch-scoped edge with branchId and originBranchId set', () => {
    const edge = new Edge(
      validProps({
        branchId: '00000000-0000-0000-0000-0000000000b1',
        originBranchId: '00000000-0000-0000-0000-0000000000b1',
      }),
    );

    expect(edge.branchId).toBe('00000000-0000-0000-0000-0000000000b1');
    expect(edge.originBranchId).toBe('00000000-0000-0000-0000-0000000000b1');
  });

  it.each(['', '   '])('rejects blank fromChunkLabel %j', (fromChunkLabel) => {
    expect(() => new Edge(validProps({ fromChunkLabel }))).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects blank toChunkLabel %j', (toChunkLabel) => {
    expect(() => new Edge(validProps({ toChunkLabel }))).toThrow(TypeError);
  });

  it('rejects fromChunkLabel === toChunkLabel', () => {
    expect(() =>
      new Edge(validProps({ fromChunkLabel: 'ATOMIC-1', toChunkLabel: 'ATOMIC-1' })),
    ).toThrow(TypeError);
  });

  it('rejects an invalid type', () => {
    expect(() =>
      new Edge(validProps({ type: 'relates-to' as unknown as EdgeProps['type'] })),
    ).toThrow(TypeError);
  });

  it('rejects an invalid discipline', () => {
    expect(() =>
      new Edge(validProps({ discipline: 'marketing' as unknown as EdgeProps['discipline'] })),
    ).toThrow(TypeError);
  });

  it('requires a non-blank createdByStakeholderId', () => {
    expect(() => new Edge(validProps({ createdByStakeholderId: '' }))).toThrow(TypeError);
    expect(() => new Edge(validProps({ createdByStakeholderId: '   ' }))).toThrow(TypeError);
  });
});
