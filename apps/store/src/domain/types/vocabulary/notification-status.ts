/**
 * Vocabulary: NotificationStatus enum, per Meridian IDEA-67/IDEA-31 (promoted): a
 * feedback_notification starts `unread` when fanned out to a stakeholder and becomes `read`
 * once that stakeholder consumes it (G09 SG3).
 */
export type NotificationStatus = 'unread' | 'read';

const NOTIFICATION_STATUSES: readonly NotificationStatus[] = ['unread', 'read'];

export function isNotificationStatus(value: unknown): value is NotificationStatus {
  return typeof value === 'string' && (NOTIFICATION_STATUSES as readonly string[]).includes(value);
}

export function parseNotificationStatus(value: unknown): NotificationStatus {
  if (!isNotificationStatus(value)) {
    throw new TypeError(`Invalid NotificationStatus: ${JSON.stringify(value)}`);
  }
  return value;
}
