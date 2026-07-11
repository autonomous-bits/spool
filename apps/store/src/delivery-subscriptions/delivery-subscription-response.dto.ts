import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import type { DeliverySubscription } from '../domain/delivery-subscription.js';

/**
 * HTTP-facing shape of a persisted DeliverySubscription, returned by
 * `GET /workspaces/:workspaceId/delivery-subscriptions` (list) and
 * `DELETE .../:id` (Meridian IDEA-104 point 1). Deliberately has NO `signingSecret` property at
 * all — not an optional field left undefined — so the secret can never be accidentally disclosed
 * on any route other than the one-time `POST` response
 * (`CreateDeliverySubscriptionResponse`).
 */
export interface DeliverySubscriptionResponse {
  id: string;
  workspaceId: string;
  url: string;
  disciplineFilter: readonly Discipline[] | undefined;
  isActive: boolean;
  createdByStakeholderId: string;
  createdAt: Date;
  updatedAt: Date;
}

export function toDeliverySubscriptionResponse(
  subscription: DeliverySubscription,
): DeliverySubscriptionResponse {
  return {
    id: subscription.id,
    workspaceId: subscription.workspaceId,
    url: subscription.url,
    disciplineFilter: subscription.disciplineFilter,
    isActive: subscription.isActive,
    createdByStakeholderId: subscription.createdByStakeholderId,
    createdAt: subscription.createdAt,
    updatedAt: subscription.updatedAt,
  } satisfies DeliverySubscriptionResponse;
}
