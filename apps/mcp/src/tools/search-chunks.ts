/**
 * MCP `search-chunks` tool: lets an ideation assistant search for chunks by full-text query
 * and strict workspace/branch boundaries, delegating to the store's `GET /chunks` endpoint.
 *
 * Neither the session token nor the workspace id is a per-call input any more (G19 SG2/SG4):
 * both come from the shared store-client helper's host-held credentials, the same as the other
 * 8 MCP tools.
 */

import { getStoreAuthHeaders } from '../store-client.js';

export interface SearchChunksInput {
  discipline?: string;
  chunkType?: string;
  status?: string;
  contextKind?: string;
  branchId?: string;
  q?: string;
  limit?: number;
  cursor?: string;
}

export interface ChunkResponse {
  id: string;
  label: string;
  content: string;
  discipline: string;
  chunkType: string;
  contextKind: string;
  status: string;
  createdByStakeholderId: string;
  updatedByStakeholderId: string;
  createdAt: string;
  updatedAt: string;
  branchId: string | null;
  originBranchId: string | null;
}

export interface SearchChunksResult {
  chunks: ChunkResponse[];
  nextCursor: string | null;
}

export class SearchChunksValidationError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'SearchChunksValidationError';
  }
}

function requireStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new SearchChunksValidationError(`${field} must be a non-empty string`, 400);
  }
  return value;
}

function optionalStringField(record: Record<string, unknown>, field: string): string | undefined {
  if (!(field in record) || record[field] === undefined) {
    return undefined;
  }
  return requireStringField(record, field);
}

function optionalNumberField(record: Record<string, unknown>, field: string): number | undefined {
  if (!(field in record) || record[field] === undefined) {
    return undefined;
  }
  if (typeof record[field] !== 'number') {
    throw new SearchChunksValidationError(`${field} must be a number`, 400);
  }
  return record[field];
}

export function parseSearchChunksInput(body: unknown): SearchChunksInput {
  if (typeof body !== 'object' || body === null) {
    throw new SearchChunksValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  const result: SearchChunksInput = {};

  const discipline = optionalStringField(record, 'discipline');
  if (discipline !== undefined) result.discipline = discipline;

  const chunkType = optionalStringField(record, 'chunkType');
  if (chunkType !== undefined) result.chunkType = chunkType;

  const status = optionalStringField(record, 'status');
  if (status !== undefined) result.status = status;

  const contextKind = optionalStringField(record, 'contextKind');
  if (contextKind !== undefined) result.contextKind = contextKind;

  const branchId = optionalStringField(record, 'branchId');
  if (branchId !== undefined) result.branchId = branchId;

  const q = optionalStringField(record, 'q');
  if (q !== undefined) result.q = q;

  const limit = optionalNumberField(record, 'limit');
  if (limit !== undefined) result.limit = limit;

  const cursor = optionalStringField(record, 'cursor');
  if (cursor !== undefined) result.cursor = cursor;

  return result;
}

function extractErrorMessage(body: unknown, fallback: string): string {
  if (
    typeof body === 'object' &&
    body !== null &&
    'message' in body &&
    typeof (body).message === 'string'
  ) {
    return (body as { message: string }).message;
  }
  return fallback;
}

export async function searchChunks(
  input: SearchChunksInput,
  storeUrl: string,
): Promise<SearchChunksResult> {
  const url = new URL(`${storeUrl}/chunks`);
  for (const [key, value] of Object.entries(input)) {
    if (key === 'chunkType') {
      url.searchParams.append('type', String(value));
    } else {
      url.searchParams.append(key, String(value));
    }
  }

  const response = await fetch(url.toString(), {
    method: 'GET',
    headers: { ...getStoreAuthHeaders() },
  });

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    throw new SearchChunksValidationError(
      extractErrorMessage(payload, `Store rejected search-chunks request (${String(response.status)})`),
      response.status,
    );
  }

  return payload as SearchChunksResult;
}
