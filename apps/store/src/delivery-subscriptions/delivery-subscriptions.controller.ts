import { Body, Controller, Delete, Get, Headers, Param, Post } from '@nestjs/common';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import { SessionTokenService } from '../auth/session-token.service.js';
import { parseCreateDeliverySubscriptionRequest } from './create-delivery-subscription-request.dto.js';
import type { CreateDeliverySubscriptionResponse } from './create-delivery-subscription-response.dto.js';
import type { DeliverySubscriptionResponse } from './delivery-subscription-response.dto.js';
import { DeliverySubscriptionsService } from './delivery-subscriptions.service.js';

/**
 * Human-only delivery-subscription CRUD (Meridian IDEA-63/IDEA-65/IDEA-104, G13 SG2). No MCP
 * tool is exposed for any of these routes (IDEA-109's explicit ratification, consistent with
 * every other human-only precedent -- branch submit/verify/merge, workspace registry/membership).
 * Every route mirrors `WorkspacesController.addMember`'s auth pattern exactly: verify the bearer
 * session token, then let the service enforce workspace scope (`token` tier) plus the
 * route-param/header match and caller-membership checks.
 */
@Controller('workspaces/:workspaceId/delivery-subscriptions')
export class DeliverySubscriptionsController {
  constructor(
    private readonly deliverySubscriptions: DeliverySubscriptionsService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post()
  async create(
    @Param('workspaceId') workspaceId: string,
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') headerWorkspaceId: string | undefined,
  ): Promise<CreateDeliverySubscriptionResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseCreateDeliverySubscriptionRequest(body);
    return this.deliverySubscriptions.create(workspaceId, request, headerWorkspaceId, claims);
  }

  @Get()
  async list(
    @Param('workspaceId') workspaceId: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') headerWorkspaceId: string | undefined,
  ): Promise<DeliverySubscriptionResponse[]> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.deliverySubscriptions.list(workspaceId, headerWorkspaceId, claims);
  }

  @Delete(':id')
  async remove(
    @Param('workspaceId') workspaceId: string,
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') headerWorkspaceId: string | undefined,
  ): Promise<DeliverySubscriptionResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.deliverySubscriptions.remove(workspaceId, id, headerWorkspaceId, claims);
  }
}
