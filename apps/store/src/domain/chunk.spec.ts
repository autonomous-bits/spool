import { describe, expect, it } from 'vitest';
import { Chunk, type ChunkProps } from './chunk.js';

function validProps(overrides: Partial<ChunkProps> = {}): ChunkProps {
  return {
    workspaceId: '00000000-0000-0000-0000-00000000d0fa',
    label: 'ATOMIC-1',
    content: 'A raw captured idea.',
    discipline: 'product',
    chunkType: 'feature',
    contextKind: 'permanent',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

describe('Chunk', () => {
  it('constructs a chunk with defaulted status, id, and updatedByStakeholderId', () => {
    const chunk = new Chunk(validProps());

    expect(chunk.label).toBe('ATOMIC-1');
    expect(chunk.content).toBe('A raw captured idea.');
    expect(chunk.discipline).toBe('product');
    expect(chunk.chunkType).toBe('feature');
    expect(chunk.contextKind).toBe('permanent');
    expect(chunk.status).toBe('draft');
    expect(chunk.createdByStakeholderId).toBe('00000000-0000-0000-0000-000000000001');
    expect(chunk.updatedByStakeholderId).toBe('00000000-0000-0000-0000-000000000001');
    expect(chunk.id).toBeTruthy();
    expect(chunk.branchId).toBeUndefined();
    expect(chunk.originBranchId).toBeUndefined();
  });

  it('constructs a branch-scoped chunk with branchId and originBranchId set', () => {
    const chunk = new Chunk(
      validProps({
        branchId: '00000000-0000-0000-0000-0000000000b1',
        originBranchId: '00000000-0000-0000-0000-0000000000b1',
      }),
    );

    expect(chunk.branchId).toBe('00000000-0000-0000-0000-0000000000b1');
    expect(chunk.originBranchId).toBe('00000000-0000-0000-0000-0000000000b1');
  });

  it.each(['', '   '])('rejects blank label %j', (label) => {
    expect(() => new Chunk(validProps({ label }))).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects blank content %j', (content) => {
    expect(() => new Chunk(validProps({ content }))).toThrow(TypeError);
  });

  it('requires a non-blank createdByStakeholderId', () => {
    expect(() => new Chunk(validProps({ createdByStakeholderId: '' }))).toThrow(TypeError);
    expect(() => new Chunk(validProps({ createdByStakeholderId: '   ' }))).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects blank workspaceId %j', (workspaceId) => {
    expect(() => new Chunk(validProps({ workspaceId }))).toThrow(TypeError);
  });

  it('rejects an invalid discipline', () => {
    expect(() =>
      new Chunk(validProps({ discipline: 'marketing' as unknown as ChunkProps['discipline'] })),
    ).toThrow(TypeError);
  });

  it('rejects an invalid chunkType', () => {
    expect(() =>
      new Chunk(validProps({ chunkType: 'epic' as unknown as ChunkProps['chunkType'] })),
    ).toThrow(TypeError);
  });

  it('rejects an invalid contextKind', () => {
    expect(() =>
      new Chunk(
        validProps({ contextKind: 'temporary' as unknown as ChunkProps['contextKind'] }),
      ),
    ).toThrow(TypeError);
  });
});
