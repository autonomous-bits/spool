import { describe, expect, it } from 'vitest';
import { isEdgeStatus, parseEdgeStatus } from './edge-status.js';

const VALID_STATUSES = ['active', 'superseded', 'deactivated'] as const;

describe('EdgeStatus', () => {
  it.each(VALID_STATUSES)('accepts valid status %j', (status) => {
    expect(isEdgeStatus(status)).toBe(true);
    expect(parseEdgeStatus(status)).toBe(status);
  });

  it('rejects an unknown status', () => {
    expect(isEdgeStatus('archived')).toBe(false);
    expect(() => parseEdgeStatus('archived')).toThrow(TypeError);
  });

  it('rejects a non-string value', () => {
    expect(isEdgeStatus(42)).toBe(false);
    expect(() => parseEdgeStatus(null)).toThrow(TypeError);
  });
});
