import { BadRequestException, Injectable, NotFoundException } from '@nestjs/common';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { isNotificationStatus } from '../domain/types/vocabulary/notification-status.js';
import { FeedbackNotificationRepository } from '../persistence/feedback-notification.repository.js';
import { toNotificationResponse, type NotificationResponse } from './notification-response.dto.js';

/**
 * Application service for a human stakeholder's own feedback notifications (Meridian
 * IDEA-67/IDEA-31, G09 SG3). Every method is scoped by `claims.stakeholderId` from a verified
 * session token -- never a client-supplied id -- so one stakeholder can never read or mark read
 * another stakeholder's notifications.
 */
@Injectable()
export class NotificationsService {
  constructor(private readonly notificationRepository: FeedbackNotificationRepository) {}

  /**
   * Lists the caller's own notifications, newest first, optionally filtered by status.
   */
  async findAll(claims: SessionTokenClaims, status?: string): Promise<NotificationResponse[]> {
    if (status !== undefined && !isNotificationStatus(status)) {
      throw new BadRequestException(`Invalid status filter: ${status}`);
    }

    const notifications = await this.notificationRepository.findByStakeholderId(
      claims.stakeholderId,
      status,
    );
    return notifications.map(toNotificationResponse);
  }

  /**
   * Marks one of the caller's own notifications read. 404s if the notification does not exist
   * or belongs to a different stakeholder -- both cases are indistinguishable from the caller's
   * point of view, so existence of another stakeholder's notification is never leaked.
   */
  async markAsRead(id: string, claims: SessionTokenClaims): Promise<NotificationResponse> {
    const notification = await this.notificationRepository.markAsRead(id, claims.stakeholderId);
    if (notification === undefined) {
      throw new NotFoundException(`Notification ${id} not found`);
    }
    return toNotificationResponse(notification);
  }
}
