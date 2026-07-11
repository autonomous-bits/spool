import {
  BadRequestException,
  ForbiddenException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { DeliverySubscription, InvalidDeliverySubscriptionError } from '../domain/delivery-subscription.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import { assertWorkspaceScope, WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { DeliverySubscriptionRepository } from '../persistence/delivery-subscription.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import type { CreateDeliverySubscriptionRequest } from './create-delivery-subscription-request.dto.js';
import {
  toCreateDeliverySubscriptionResponse,
  type CreateDeliverySubscriptionResponse,
} from './create-delivery-subscription-response.dto.js';
import {
  toDeliverySubscriptionResponse,
  type DeliverySubscriptionResponse,
} from './delivery-subscription-response.dto.js';

/**
 * Application service for delivery-subscription CRUD (Meridian IDEA-63/IDEA-65/IDEA-104, G13
 * SG2). Human-only via session token (IDEA-57/IDEA-81) — no MCP tool exists or should be added
 * (IDEA-109's explicit ratification). Every route is workspace-scoped by the `token` tier
 * (IDEA-98/IDEA-100), mirroring `WorkspacesService.addMember`'s double-check: the header must
 * match both the token's `workspaceId` claim AND the `:workspaceId` route param, and the caller
 * must additionally be a member of that workspace (not just hold a token bound to it).
 */
@Injectable()
export class DeliverySubscriptionsService {
  constructor(
    private readonly deliverySubscriptions: DeliverySubscriptionRepository,
    private readonly workspaces: WorkspaceRepository,
  ) {}

  async create(
    workspaceId: string,
    request: CreateDeliverySubscriptionRequest,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<CreateDeliverySubscriptionResponse> {
    await this.assertCallerCanAct(workspaceId, headerWorkspaceId, claims);

    let subscription: DeliverySubscription;
    try {
      subscription = new DeliverySubscription({
        workspaceId,
        url: request.url,
        ...(request.disciplineFilter === undefined
          ? {}
          : { disciplineFilter: request.disciplineFilter as Discipline[] }),
        createdByStakeholderId: claims.stakeholderId,
      });
    } catch (error) {
      if (error instanceof InvalidDeliverySubscriptionError) {
        throw new BadRequestException(error.message);
      }
      throw error;
    }

    const created = await this.deliverySubscriptions.create(subscription);
    return toCreateDeliverySubscriptionResponse(created);
  }

  async list(
    workspaceId: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<DeliverySubscriptionResponse[]> {
    await this.assertCallerCanAct(workspaceId, headerWorkspaceId, claims);

    const subscriptions = await this.deliverySubscriptions.listByWorkspace(workspaceId);
    return subscriptions.map(toDeliverySubscriptionResponse);
  }

  async remove(
    workspaceId: string,
    id: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<DeliverySubscriptionResponse> {
    await this.assertCallerCanAct(workspaceId, headerWorkspaceId, claims);

    const deactivated = await this.deliverySubscriptions.deactivate(id, workspaceId);
    if (deactivated === undefined) {
      // Unknown id and cross-workspace id are indistinguishable by design (SG1's findById/
      // deactivate scoping) -- both surface as a plain 404.
      throw new NotFoundException(`Delivery subscription ${id} not found`);
    }

    return toDeliverySubscriptionResponse(deactivated);
  }

  /**
   * Shared workspace-scope + membership guard for every route. `assertWorkspaceScope`'s `token`
   * tier confirms the header matches the caller's own token claim; the header must additionally
   * equal the `:workspaceId` route param (same double-check as `WorkspacesService.addMember`,
   * Meridian IDEA-98/IDEA-100); finally the caller must actually be a member of the workspace
   * (not merely hold a token bound to it), 403 otherwise.
   */
  private async assertCallerCanAct(
    workspaceId: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<void> {
    try {
      assertWorkspaceScope(headerWorkspaceId, { tier: 'token', workspaceIdClaim: claims.workspaceId });
    } catch (error) {
      if (error instanceof WorkspaceScopeViolationError) {
        throw new ForbiddenException(error.message);
      }
      throw error;
    }

    if (headerWorkspaceId !== workspaceId) {
      throw new ForbiddenException(
        `X-Workspace-Id ${headerWorkspaceId} does not match the target workspace ${workspaceId}`,
      );
    }

    const isMember = await this.workspaces.isMember(workspaceId, claims.stakeholderId);
    if (!isMember) {
      throw new ForbiddenException(
        `Stakeholder ${claims.stakeholderId} is not a member of workspace ${workspaceId}`,
      );
    }
  }
}
