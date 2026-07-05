import { BadRequestException } from '@nestjs/common';
import type { ChunkType } from '../domain/types/vocabulary/chunk-type.js';
import { isChunkType } from '../domain/types/vocabulary/chunk-type.js';
import type { ContextKind } from '../domain/types/vocabulary/context-kind.js';
import { isContextKind } from '../domain/types/vocabulary/context-kind.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import { isDiscipline } from '../domain/types/vocabulary/discipline.js';

/**
 * Validated shape of a `POST /chunks` request body, per Meridian IDEA-52/IDEA-34 (chunk capture
 * API). Stakeholder registration is out of scope for G01 (Meridian IDEA-11 attribution still
 * requires stakeholderId to already exist). `branchId` is optional (G02): when present, the
 * chunk is attached to that draft branch; when absent, capture is branchless as in G01.
 */
export interface CreateChunkRequest {
  label: string;
  content: string;
  discipline: Discipline;
  chunkType: ChunkType;
  contextKind: ContextKind;
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
 * Parses and validates an untrusted HTTP request body into a `CreateChunkRequest`, throwing
 * `BadRequestException` (HTTP 400) for missing fields or invalid vocabulary values. This is the
 * IO boundary guard required for unknown request bodies (typescript-quality: explicit guards at
 * IO boundaries).
 */
export function parseCreateChunkRequest(body: unknown): CreateChunkRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;

  const label = requireStringField(record, 'label');
  const content = requireStringField(record, 'content');
  const stakeholderId = requireStringField(record, 'stakeholderId');

  const discipline = record['discipline'];
  if (!isDiscipline(discipline)) {
    throw new BadRequestException(`Invalid discipline: ${JSON.stringify(discipline)}`);
  }

  const chunkType = record['chunkType'];
  if (!isChunkType(chunkType)) {
    throw new BadRequestException(`Invalid chunkType: ${JSON.stringify(chunkType)}`);
  }

  const contextKind = record['contextKind'];
  if (!isContextKind(contextKind)) {
    throw new BadRequestException(`Invalid contextKind: ${JSON.stringify(contextKind)}`);
  }

  const branchIdValue = record['branchId'];
  if (branchIdValue !== undefined) {
    if (typeof branchIdValue !== 'string' || branchIdValue.trim().length === 0) {
      throw new BadRequestException('branchId must be a non-empty string when provided');
    }
    return {
      label,
      content,
      discipline,
      chunkType,
      contextKind,
      stakeholderId,
      branchId: branchIdValue,
    } satisfies CreateChunkRequest;
  }

  return {
    label,
    content,
    discipline,
    chunkType,
    contextKind,
    stakeholderId,
  } satisfies CreateChunkRequest;
}
