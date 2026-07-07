import type { NotificationStatus } from './types/vocabulary/notification-status.js';
import { parseNotificationStatus } from './types/vocabulary/notification-status.js';

export interface FeedbackNotificationProps {
  id: string;
  workspaceId: string;
  branchId: string;
  stakeholderId: string;
  signalId: string;
  status: NotificationStatus;
  createdAt: Date;
  updatedAt: Date;
}

/**
 * FeedbackNotification entity: one unread-by-default delivery of a verification signal to a
 * single stakeholder (Meridian IDEA-67/IDEA-68/IDEA-31, G09 SG2 fan-out + SG3 consumption).
 * Always constructed from a persisted row -- unlike `VerificationSignal`, there is no
 * client-driven creation path, so this constructor requires every field rather than defaulting
 * `id`/`createdAt`.
 */
export class FeedbackNotification {
  readonly id: string;
  readonly workspaceId: string;
  readonly branchId: string;
  readonly stakeholderId: string;
  readonly signalId: string;
  readonly status: NotificationStatus;
  readonly createdAt: Date;
  readonly updatedAt: Date;

  constructor(props: FeedbackNotificationProps) {
    this.id = props.id;
    this.workspaceId = props.workspaceId;
    this.branchId = props.branchId;
    this.stakeholderId = props.stakeholderId;
    this.signalId = props.signalId;
    this.status = parseNotificationStatus(props.status);
    this.createdAt = props.createdAt;
    this.updatedAt = props.updatedAt;
  }
}
