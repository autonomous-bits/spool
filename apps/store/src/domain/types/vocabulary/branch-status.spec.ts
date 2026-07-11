import { describe, expect, it } from 'vitest';
import { isBranchStatus, parseBranchStatus } from './branch-status.js';

describe('BranchStatus', () => {
  it.each(['draft', 'submitted', 'verified', 'merged'])('accepts %s', (value) => {
    expect(isBranchStatus(value)).toBe(true);
    expect(parseBranchStatus(value)).toBe(value);
  });

  it.each(['deleted', '', 'DRAFT', 123, null, undefined])('rejects %s', (value) => {
    expect(isBranchStatus(value)).toBe(false);
    expect(() => parseBranchStatus(value)).toThrow(TypeError);
  });
});
