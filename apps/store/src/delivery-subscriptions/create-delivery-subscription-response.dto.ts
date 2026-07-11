import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import type { DeliverySubscription } from '../domain/delivery-subscription.js';

/**
 * HTTP-facing shape of a newly-created DeliverySubscription, returned ONLY by
 * `POST /workspaces/:workspaceId/delivery-subscriptions` (Meridian IDEA-104 point 1/2). Includes
 * `signingSecret` — a single-disclosure webhook secret the consumer must capture at registration
 * time, since it is never returned again (see `DeliverySubscriptionResponse`, the distinct DTO
 * used by every other route, which omits the property entirely rather than making it optional).
 */
export interface CreateDeliverySubscriptionResponse {
  id: string;
  workspaceId: string;
  url: string;
  disciplineFilter: readonly Discipline[] | undefined;
  isActive: boolean;
  createdByStakeholderId: string;
  createdAt: Date;
  updatedAt: Date;
  signingSecret: string;
}

export function toCreateDeliverySubscriptionResponse(
  subscription: DeliverySubscription,
): CreateDeliverySubscriptionResponse {
  return {
    id: subscription.id,
    workspaceId: subscription.workspaceId,
    url: subscription.url,
    disciplineFilter: subscription.disciplineFilter,
    isActive: subscription.isActive,
    createdByStakeholderId: subscription.createdByStakeholderId,
    createdAt: subscription.createdAt,
    updatedAt: subscription.updatedAt,
    signingSecret: subscription.signingSecret,
  } satisfies CreateDeliverySubscriptionResponse;
}
