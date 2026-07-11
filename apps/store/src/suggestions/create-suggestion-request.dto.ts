import { BadRequestException } from '@nestjs/common';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import { isDiscipline } from '../domain/types/vocabulary/discipline.js';
import type { EdgeType } from '../domain/types/vocabulary/edge-type.js';
import { isEdgeType } from '../domain/types/vocabulary/edge-type.js';

export type CreateSuggestionVariant =
  | { kind: 'chunk'; label: string; content: string }
  | { kind: 'edge'; fromChunkLabel: string; toChunkLabel: string; relationshipType: EdgeType };

/**
 * Validated shape of a `POST /suggestions` request body, per Meridian IDEA-49 (suggestions
 * queue). The body carries exactly one variant's fields plus a `discipline`.
 *
 * Meridian IDEA-139: submission still authenticates with a verified session token, and authorship
 * attribution (`submittedByStakeholderId`) is derived from the token's `stakeholderId` claim, not
 * a client-supplied body field — this interface intentionally has no `stakeholderId` field.
 *
 * Meridian IDEA-75: submission remains a delegated-actor operation, so the server always assigns
 * `submittedByActorKind: 'delegated'` as business logic, never from client input.
 */
export interface CreateSuggestionRequest {
  variant: CreateSuggestionVariant;
  discipline: Discipline;
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0;
}

function requireStringField(body: Record<string, unknown>, field: string): string {
  const value = body[field];
  if (!isNonEmptyString(value)) {
    throw new BadRequestException(`${field} must be a non-empty string`);
  }
  return value;
}

/**
 * Parses and validates an untrusted HTTP request body into a `CreateSuggestionRequest`, throwing
 * `BadRequestException` (HTTP 400) for missing fields, invalid vocabulary values, or a body that
 * mixes chunk-shaped and edge-shaped fields (or provides neither/a partial shape of either) --
 * mirroring the `check_suggestion_type` discriminated-union constraint (Meridian IDEA-49).
 */
export function parseCreateSuggestionRequest(body: unknown): CreateSuggestionRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;

  const discipline = record.discipline;
  if (!isDiscipline(discipline)) {
    throw new BadRequestException(`Invalid discipline: ${JSON.stringify(discipline)}`);
  }

  const hasAnyChunkField = isNonEmptyString(record.label) || isNonEmptyString(record.content);
  const hasAnyEdgeField =
    isNonEmptyString(record.fromChunkLabel) ||
    isNonEmptyString(record.toChunkLabel) ||
    record.relationshipType !== undefined;

  if (hasAnyChunkField && hasAnyEdgeField) {
    throw new BadRequestException(
      'Request body must not mix chunk-shaped (label/content) and edge-shaped ' +
        '(fromChunkLabel/toChunkLabel/relationshipType) fields',
    );
  }

  if (hasAnyChunkField) {
    const label = requireStringField(record, 'label');
    const content = requireStringField(record, 'content');
    return {
      variant: { kind: 'chunk', label, content },
      discipline,
    } satisfies CreateSuggestionRequest;
  }

  if (hasAnyEdgeField) {
    const fromChunkLabel = requireStringField(record, 'fromChunkLabel');
    const toChunkLabel = requireStringField(record, 'toChunkLabel');
    if (fromChunkLabel === toChunkLabel) {
      throw new BadRequestException(
        'fromChunkLabel and toChunkLabel must not be the same label',
      );
    }

    const relationshipType = record.relationshipType;
    if (!isEdgeType(relationshipType)) {
      throw new BadRequestException(`Invalid relationshipType: ${JSON.stringify(relationshipType)}`);
    }

    return {
      variant: { kind: 'edge', fromChunkLabel, toChunkLabel, relationshipType },
      discipline,
    } satisfies CreateSuggestionRequest;
  }

  throw new BadRequestException(
    'Request body must provide either chunk-shaped (label, content) or edge-shaped ' +
      '(fromChunkLabel, toChunkLabel, relationshipType) fields',
  );
}
