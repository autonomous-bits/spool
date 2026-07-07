import { describe, expect, it } from 'vitest';
import { VerificationSignal, type VerificationSignalProps } from './verification-signal.js';

const BRANCH_ID = '00000000-0000-0000-0000-000000000001';
const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

function props(overrides: Partial<VerificationSignalProps> = {}): VerificationSignalProps {
  return {
    workspaceId: WORKSPACE_ID,
    branchId: BRANCH_ID,
    verifierName: 'ci-evaluator',
    status: 'pass',
    ...overrides,
  };
}

describe('VerificationSignal', () => {
  it('constructs a signal with an id and no reason by default', () => {
    const signal = new VerificationSignal(props());

    expect(signal.branchId).toBe(BRANCH_ID);
    expect(signal.verifierName).toBe('ci-evaluator');
    expect(signal.status).toBe('pass');
    expect(signal.id).toBeTruthy();
    expect(signal.reason).toBeUndefined();
    expect(signal.createdAt).toBeInstanceOf(Date);
  });

  it('retains an optional reason', () => {
    const signal = new VerificationSignal(props({ status: 'fail', reason: 'missing tests' }));

    expect(signal.status).toBe('fail');
    expect(signal.reason).toBe('missing tests');
  });

  it.each(['', '   '])('rejects a blank branchId %j', (branchId) => {
    expect(() => new VerificationSignal(props({ branchId }))).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects a blank verifierName %j', (verifierName) => {
    expect(() => new VerificationSignal(props({ verifierName }))).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects a blank workspaceId %j', (workspaceId) => {
    expect(() => new VerificationSignal(props({ workspaceId }))).toThrow(TypeError);
  });

  it('rejects an invalid status', () => {
    expect(
      () => new VerificationSignal(props({ status: 'bogus' as unknown as 'pass' })),
    ).toThrow(TypeError);
  });
});
