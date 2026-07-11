import type { FeedbackNotification } from '../domain/feedback-notification.js';
import type { NotificationStatus } from '../domain/types/vocabulary/notification-status.js';

/**
 * HTTP-facing shape of a persisted FeedbackNotification, per Meridian IDEA-67/IDEA-31 (G09 SG3).
 * Kept as an explicit interface (rather than returning the domain entity directly) so the API
 * response contract is typed independently of the domain entity's internal shape.
 */
export interface NotificationResponse {
  id: string;
  branchId: string;
  stakeholderId: string;
  signalId: string;
  status: NotificationStatus;
  createdAt: Date;
  updatedAt: Date;
}

export function toNotificationResponse(notification: FeedbackNotification): NotificationResponse {
  return {
    id: notification.id,
    branchId: notification.branchId,
    stakeholderId: notification.stakeholderId,
    signalId: notification.signalId,
    status: notification.status,
    createdAt: notification.createdAt,
    updatedAt: notification.updatedAt,
  } satisfies NotificationResponse;
}
