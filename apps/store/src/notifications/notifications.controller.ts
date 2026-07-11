import { Controller, Get, Headers, Param, Post, Query } from '@nestjs/common';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import { SessionTokenService } from '../auth/session-token.service.js';
import type { NotificationResponse } from './notification-response.dto.js';
import { NotificationsService } from './notifications.service.js';

/**
 * G09 SG3: a human stakeholder's own notification inbox (Meridian IDEA-67/IDEA-31). Both routes
 * are session-token gated (mirrors BranchesController/SuggestionsController's human-only
 * routes) -- the stakeholder id always comes from verified claims, never a client-supplied
 * value, path param, or query param.
 */
@Controller('notifications')
export class NotificationsController {
  constructor(
    private readonly notifications: NotificationsService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Get()
  async findAll(
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
    @Query('status') status?: string,
  ): Promise<NotificationResponse[]> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.notifications.findAll(claims, workspaceId, status);
  }

  @Post(':id/read')
  async markAsRead(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<NotificationResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.notifications.markAsRead(id, claims, workspaceId);
  }
}
