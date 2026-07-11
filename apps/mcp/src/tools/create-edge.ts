/**
 * MCP `create-edge` tool (Meridian IDEA-9, IDEA-52/IDEA-34, IDEA-36/IDEA-37/IDEA-38): lets an
 * ideation assistant create a typed edge between two chunks on behalf of a human stakeholder by
 * delegating to the store's `POST /edges` endpoint. Neither stakeholder identity nor workspace
 * scope is a per-call input any more (G19 SG2/SG3): both come from the shared store-client
 * helper's host-held session token/workspace id, and the store derives authorship from the
 * verified token's `stakeholderId` claim. It also never re-validates the `type`/`discipline`
 * closed vocabularies itself: the store is the authoritative source for those invariants, and
 * this tool must surface the store's own validation errors rather than duplicate or pre-empt
 * them.
 */

import { getStoreAuthHeaders } from '../store-client.js';

/** Untrusted-input shape mirroring the store's `CreateEdgeRequest` (apps/store/src/edges). */
export interface CreateEdgeInput {
  fromChunkLabel: string;
  toChunkLabel: string;
  type: string;
  discipline: string;
  branchId?: string;
}

/** Edge as returned by the store's `POST /edges` on success. */
export interface CreateEdgeResult {
  id: string;
  fromChunkLabel: string;
  toChunkLabel: string;
  type: string;
  status: string;
  discipline: string;
  branchId: string | null;
  originBranchId: string | null;
  supersededByEdgeId: string | null;
  createdByStakeholderId: string;
  updatedByStakeholderId: string;
  createdAt: string;
  updatedAt: string;
}

/** Raised when the store rejects the request with a 4xx; carries the store's own message. */
export class CreateEdgeValidationError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'CreateEdgeValidationError';
  }
}

function requireStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new CreateEdgeValidationError(`${field} must be a non-empty string`, 400);
  }
  return value;
}

/**
 * Validates that an untrusted tool-call body has the expected shape before it is forwarded to
 * the store. The `type`/`discipline` vocabulary values are intentionally not re-validated here:
 * the store is the authoritative source for those invariants, and this tool must surface the
 * store's own validation errors rather than duplicate or pre-empt them. `branchId` is optional.
 */
export function parseCreateEdgeInput(body: unknown): CreateEdgeInput {
  if (typeof body !== 'object' || body === null) {
    throw new CreateEdgeValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  const fromChunkLabel = requireStringField(record, 'fromChunkLabel');
  const toChunkLabel = requireStringField(record, 'toChunkLabel');
  const type = requireStringField(record, 'type');
  const discipline = requireStringField(record, 'discipline');

  const branchIdValue = record.branchId;
  if (branchIdValue !== undefined) {
    if (typeof branchIdValue !== 'string' || branchIdValue.trim().length === 0) {
      throw new CreateEdgeValidationError('branchId must be a non-empty string when provided', 400);
    }
    return {
      fromChunkLabel,
      toChunkLabel,
      type,
      discipline,
      branchId: branchIdValue,
    } satisfies CreateEdgeInput;
  }

  return {
    fromChunkLabel,
    toChunkLabel,
    type,
    discipline,
  } satisfies CreateEdgeInput;
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

/**
 * Forwards a validated create-edge request to the store's `POST /edges`. Surfaces the store's
 * 4xx validation errors as `CreateEdgeValidationError` (never swallowed), and rethrows any other
 * failure (network errors, unexpected store status codes) for the caller to handle.
 */
export async function createEdge(
  input: CreateEdgeInput,
  storeUrl: string,
): Promise<CreateEdgeResult> {
  const response = await fetch(`${storeUrl}/edges`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', ...getStoreAuthHeaders() },
    body: JSON.stringify(input),
  });

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    throw new CreateEdgeValidationError(
      extractErrorMessage(payload, `Store rejected create-edge request (${String(response.status)})`),
      response.status,
    );
  }

  return payload as CreateEdgeResult;
}
