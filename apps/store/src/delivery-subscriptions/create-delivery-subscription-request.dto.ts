import { BadRequestException } from '@nestjs/common';

/**
 * Validated shape of a `POST /workspaces/:workspaceId/delivery-subscriptions` request body, per
 * Meridian IDEA-104 point 1. Only the JSON shape is guarded here (IO boundary); the domain
 * invariants (https-only url, discipline vocabulary membership) are enforced by
 * `DeliverySubscription` construction in the service layer, mirroring the
 * `create-workspace-request.dto.ts` split between shape validation and domain validation.
 */
export interface CreateDeliverySubscriptionRequest {
  url: string;
  disciplineFilter?: string[];
}

export function parseCreateDeliverySubscriptionRequest(body: unknown): CreateDeliverySubscriptionRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;
  const url = record.url;
  if (typeof url !== 'string' || url.trim().length === 0) {
    throw new BadRequestException('url must be a non-empty string');
  }

  const disciplineFilter = record.disciplineFilter;
  if (disciplineFilter === undefined) {
    return { url };
  }

  if (!Array.isArray(disciplineFilter) || disciplineFilter.some((value) => typeof value !== 'string')) {
    throw new BadRequestException('disciplineFilter must be an array of strings when present');
  }

  return { url, disciplineFilter: disciplineFilter as string[] } satisfies CreateDeliverySubscriptionRequest;
}
