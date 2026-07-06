import { BadRequestException } from '@nestjs/common';

/**
 * Validated shape of a `POST /suggestions/:id/accept` request body, per Meridian IDEA-49/IDEA-27
 * (G07 SG2). Discipline comes from the suggestion row, not the request body -- the body only
 * carries the new branch's name.
 */
export interface AcceptSuggestionRequest {
  name: string;
}

/**
 * Parses and validates an untrusted HTTP request body into an `AcceptSuggestionRequest`,
 * throwing `BadRequestException` (HTTP 400) when `name` is missing or blank.
 */
export function parseAcceptSuggestionRequest(body: unknown): AcceptSuggestionRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;
  const name = record['name'];
  if (typeof name !== 'string' || name.trim().length === 0) {
    throw new BadRequestException('name must be a non-empty string');
  }

  return { name } satisfies AcceptSuggestionRequest;
}
