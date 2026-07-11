import { BadRequestException } from '@nestjs/common';

/**
 * Validated shape of a `POST /workspaces` request body, per Meridian IDEA-94. The creating
 * stakeholder is not part of the body — it is derived from the caller's verified session token
 * (Meridian IDEA-81), not a caller-declared field.
 */
export interface CreateWorkspaceRequest {
  name: string;
}

/**
 * Parses and validates an untrusted HTTP request body into a `CreateWorkspaceRequest`, throwing
 * `BadRequestException` (HTTP 400) for a missing/blank name. This is the IO boundary guard
 * required for unknown request bodies (typescript-quality: explicit guards at IO boundaries).
 */
export function parseCreateWorkspaceRequest(body: unknown): CreateWorkspaceRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;
  const name = record.name;
  if (typeof name !== 'string' || name.trim().length === 0) {
    throw new BadRequestException('name must be a non-empty string');
  }

  return { name } satisfies CreateWorkspaceRequest;
}
