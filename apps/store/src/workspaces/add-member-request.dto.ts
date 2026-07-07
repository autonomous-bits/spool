import { BadRequestException } from '@nestjs/common';

/**
 * Validated shape of a `POST /workspaces/:id/members` request body, per Meridian IDEA-94's
 * direct-add contract: the caller names a target stakeholder id to add as a member.
 */
export interface AddMemberRequest {
  stakeholderId: string;
}

/**
 * Parses and validates an untrusted HTTP request body into an `AddMemberRequest`, throwing
 * `BadRequestException` (HTTP 400) for a missing/blank stakeholderId. This is the IO boundary
 * guard required for unknown request bodies (typescript-quality: explicit guards at IO
 * boundaries).
 */
export function parseAddMemberRequest(body: unknown): AddMemberRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;
  const stakeholderId = record.stakeholderId;
  if (typeof stakeholderId !== 'string' || stakeholderId.trim().length === 0) {
    throw new BadRequestException('stakeholderId must be a non-empty string');
  }

  return { stakeholderId } satisfies AddMemberRequest;
}
