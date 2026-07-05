import { BadRequestException } from '@nestjs/common';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import { isDiscipline } from '../domain/types/vocabulary/discipline.js';
import type { EdgeType } from '../domain/types/vocabulary/edge-type.js';
import { isEdgeType } from '../domain/types/vocabulary/edge-type.js';

/**
 * Validated shape of a `POST /edges` request body, per Meridian IDEA-52/IDEA-34 (API gateway
 * boundary) and IDEA-36/IDEA-37/IDEA-38 (typed edges referenced by logical chunk labels).
 * `branchId` is optional: when present, the edge is scoped to that draft branch; when absent,
 * endpoint resolution is branchless (mirrors G02's chunk capture precedent).
 */
export interface CreateEdgeRequest {
  fromChunkLabel: string;
  toChunkLabel: string;
  type: EdgeType;
  discipline: Discipline;
  stakeholderId: string;
  branchId?: string;
}

function requireStringField(body: Record<string, unknown>, field: string): string {
  const value = body[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException(`${field} must be a non-empty string`);
  }
  return value;
}

/**
 * Parses and validates an untrusted HTTP request body into a `CreateEdgeRequest`, throwing
 * `BadRequestException` (HTTP 400) for missing fields or invalid vocabulary values. This is the
 * IO boundary guard required for unknown request bodies (typescript-quality: explicit guards at
 * IO boundaries).
 */
export function parseCreateEdgeRequest(body: unknown): CreateEdgeRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;

  const fromChunkLabel = requireStringField(record, 'fromChunkLabel');
  const toChunkLabel = requireStringField(record, 'toChunkLabel');
  const stakeholderId = requireStringField(record, 'stakeholderId');

  if (fromChunkLabel === toChunkLabel) {
    throw new BadRequestException('fromChunkLabel and toChunkLabel must not be the same label');
  }

  const type = record['type'];
  if (!isEdgeType(type)) {
    throw new BadRequestException(`Invalid type: ${JSON.stringify(type)}`);
  }

  const discipline = record['discipline'];
  if (!isDiscipline(discipline)) {
    throw new BadRequestException(`Invalid discipline: ${JSON.stringify(discipline)}`);
  }

  const branchIdValue = record['branchId'];
  if (branchIdValue !== undefined) {
    if (typeof branchIdValue !== 'string' || branchIdValue.trim().length === 0) {
      throw new BadRequestException('branchId must be a non-empty string when provided');
    }
    return {
      fromChunkLabel,
      toChunkLabel,
      type,
      discipline,
      stakeholderId,
      branchId: branchIdValue,
    } satisfies CreateEdgeRequest;
  }

  return {
    fromChunkLabel,
    toChunkLabel,
    type,
    discipline,
    stakeholderId,
  } satisfies CreateEdgeRequest;
}
