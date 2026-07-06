import { describe, expect, it } from 'vitest';
import {
  isVerificationSignalStatus,
  parseVerificationSignalStatus,
} from './verification-signal-status.js';

describe('VerificationSignalStatus vocabulary', () => {
  it.each(['pass', 'fail'] as const)('accepts %s as a valid status', (status) => {
    expect(isVerificationSignalStatus(status)).toBe(true);
    expect(parseVerificationSignalStatus(status)).toBe(status);
  });

  it.each([undefined, null, '', 'PASS', 'unknown', 42])(
    'rejects %j as an invalid status',
    (value) => {
      expect(isVerificationSignalStatus(value)).toBe(false);
      expect(() => parseVerificationSignalStatus(value)).toThrow(TypeError);
    },
  );
});
