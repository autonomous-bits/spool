import { describe, expect, it } from 'vitest';
import { Branch, type BranchProps } from './branch.js';
import { DivergencePoint } from './divergence-point.js';

function validProps(overrides: Partial<BranchProps> = {}): BranchProps {
  return {
    name: 'feature/branch-authoring',
    discipline: 'product',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

describe('Branch', () => {
  it('constructs a branch with defaulted status, id, divergedAt, and timestamps', () => {
    const branch = new Branch(validProps());

    expect(branch.name).toBe('feature/branch-authoring');
    expect(branch.discipline).toBe('product');
    expect(branch.status).toBe('draft');
    expect(branch.createdByStakeholderId).toBe('00000000-0000-0000-0000-000000000001');
    expect(branch.id).toBeTruthy();
    expect(branch.divergedAt).toBeInstanceOf(DivergencePoint);
    expect(branch.createdAt).toBeInstanceOf(Date);
    expect(branch.updatedAt).toEqual(branch.createdAt);
  });

  it('round-trips an explicit submittedAt when provided', () => {
    const submittedAt = new Date('2026-07-05T12:34:56.789Z');

    const branch = new Branch(validProps({ status: 'submitted', submittedAt }));

    expect(branch.submittedAt).toEqual(submittedAt);
  });

  it.each(['', '   '])('rejects blank name %j', (name) => {
    expect(() => new Branch(validProps({ name }))).toThrow(TypeError);
  });

  it('rejects an invalid discipline', () => {
    expect(() =>
      new Branch(validProps({ discipline: 'marketing' as unknown as BranchProps['discipline'] })),
    ).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects blank createdByStakeholderId %j', (createdByStakeholderId) => {
    expect(() => new Branch(validProps({ createdByStakeholderId }))).toThrow(TypeError);
  });

  it.each(['submitted', 'verified', 'merged'])(
    'accepts an explicit non-draft status %s for round-tripping reads',
    (status) => {
      const branch = new Branch(validProps({ status: status as BranchProps['status'] }));
      expect(branch.status).toBe(status);
    },
  );

  it('rejects an invalid status', () => {
    expect(() =>
      new Branch(validProps({ status: 'archived' as unknown as BranchProps['status'] })),
    ).toThrow(TypeError);
  });
});
