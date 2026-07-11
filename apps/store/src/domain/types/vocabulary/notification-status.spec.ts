import { describe, expect, it } from 'vitest';
import { isNotificationStatus, parseNotificationStatus } from './notification-status.js';

describe('NotificationStatus vocabulary', () => {
  it.each(['unread', 'read'] as const)('accepts %s as a valid status', (status) => {
    expect(isNotificationStatus(status)).toBe(true);
    expect(parseNotificationStatus(status)).toBe(status);
  });

  it.each([undefined, null, '', 'UNREAD', 'unknown', 42])(
    'rejects %j as an invalid status',
    (value) => {
      expect(isNotificationStatus(value)).toBe(false);
      expect(() => parseNotificationStatus(value)).toThrow(TypeError);
    },
  );
});
