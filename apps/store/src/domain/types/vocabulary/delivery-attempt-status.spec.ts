import { describe, expect, it } from 'vitest';
import { isDeliveryAttemptStatus, parseDeliveryAttemptStatus } from './delivery-attempt-status.js';

describe('DeliveryAttemptStatus vocabulary', () => {
  it.each(['pending', 'in_progress', 'succeeded', 'failed'] as const)(
    'accepts %s as a valid status',
    (status) => {
      expect(isDeliveryAttemptStatus(status)).toBe(true);
      expect(parseDeliveryAttemptStatus(status)).toBe(status);
    },
  );

  it.each([undefined, null, '', 'PENDING', 'unknown', 42])(
    'rejects %j as an invalid status',
    (value) => {
      expect(isDeliveryAttemptStatus(value)).toBe(false);
      expect(() => parseDeliveryAttemptStatus(value)).toThrow(TypeError);
    },
  );
});
