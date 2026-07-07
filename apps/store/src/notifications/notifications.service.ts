import { BadRequestException, ForbiddenException, Injectable, NotFoundException } from '@nestjs/common';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { isNotificationStatus } from '../domain/types/vocabulary/notification-status.js';
import { assertWorkspaceScope, WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { FeedbackNotificationRepository } from '../persistence/feedback-notification.repository.js';
import { toNotificationResponse, type NotificationResponse } from './notification-response.dto.js';

/**
 * Application service for a human stakeholder's own feedback notifications (Meridian
 * IDEA-67/IDEA-31, G09 SG3). Every method is scoped by `claims.stakeholderId` from a verified
 * session token -- never a client-supplied id -- so one stakeholder can never read or mark read
 * another stakeholder's notifications.
 *
 * G11 SG5 (Meridian IDEA-98/IDEA-100): both routes are already token-gated, so they sit on the
 * token auth tier -- `X-Workspace-Id` is validated against the token's `workspaceId` claim
 * (no async membership lookup needed), mirroring `SuggestionsService.accept`/`reject`.
 */
@Injectable()
export class NotificationsService {
  constructor(private readonly notificationRepository: FeedbackNotificationRepository) {}

  private assertTokenScope(
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): string {
    try {
      assertWorkspaceScope(headerWorkspaceId, { tier: 'token', workspaceIdClaim: claims.workspaceId });
    } catch (error) {
      if (error instanceof WorkspaceScopeViolationError) {
        throw new ForbiddenException(error.message);
      }
      throw error;
    }

    return headerWorkspaceId;
  }

  /**
   * Lists the caller's own notifications, newest first, optionally filtered by status.
   */
  async findAll(
    claims: SessionTokenClaims,
    headerWorkspaceId: string | null | undefined,
    status?: string,
  ): Promise<NotificationResponse[]> {
    const workspaceId = this.assertTokenScope(headerWorkspaceId, claims);

    if (status !== undefined && !isNotificationStatus(status)) {
      throw new BadRequestException(`Invalid status filter: ${status}`);
    }

    const notifications = await this.notificationRepository.findByStakeholderId(
      claims.stakeholderId,
      workspaceId,
      status,
    );
    return notifications.map(toNotificationResponse);
  }

  /**
   * Marks one of the caller's own notifications read. 404s if the notification does not exist
   * or belongs to a different stakeholder -- both cases are indistinguishable from the caller's
   * point of view, so existence of another stakeholder's notification is never leaked.
   */
  async markAsRead(
    id: string,
    claims: SessionTokenClaims,
    headerWorkspaceId: string | null | undefined,
  ): Promise<NotificationResponse> {
    const workspaceId = this.assertTokenScope(headerWorkspaceId, claims);

    const notification = await this.notificationRepository.markAsRead(
      id,
      claims.stakeholderId,
      workspaceId,
    );
    if (notification === undefined) {
      throw new NotFoundException(`Notification ${id} not found`);
    }
    return toNotificationResponse(notification);
  }
}
