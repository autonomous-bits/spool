/**
 * MCP `create-branch` tool (Meridian IDEA-9, IDEA-52/IDEA-34): lets an ideation assistant create
 * a draft branch on behalf of a human stakeholder by delegating to the store's `POST /branches`
 * endpoint. Neither stakeholder identity nor workspace scope is a per-call input any more (G19
 * SG2/SG3): both come from the shared store-client helper's host-held session token/workspace
 * id, and the store derives authorship from the verified token's `stakeholderId` claim.
 */

import { getStoreAuthHeaders } from '../store-client.js';

/** Untrusted-input shape mirroring the store's `CreateBranchRequest` (apps/store/src/branches). */
export interface CreateBranchInput {
  name: string;
  discipline: string;
}

/** Branch as returned by the store's `POST /branches` on success. */
export interface CreateBranchResult {
  id: string;
  name: string;
  discipline: string;
  status: string;
  divergedAt: string;
  createdAt: string;
  updatedAt: string;
  createdByStakeholderId: string;
}

/** Raised when the store rejects the request with a 4xx; carries the store's own message. */
export class CreateBranchValidationError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'CreateBranchValidationError';
  }
}

function requireStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new CreateBranchValidationError(`${field} must be a non-empty string`, 400);
  }
  return value;
}

/**
 * Validates that an untrusted tool-call body has the expected shape before it is forwarded to
 * the store. The discipline vocabulary value is intentionally not re-validated here: the store
 * is the authoritative source for that invariant, and this tool must surface the store's own
 * validation errors rather than duplicate or pre-empt them.
 */
export function parseCreateBranchInput(body: unknown): CreateBranchInput {
  if (typeof body !== 'object' || body === null) {
    throw new CreateBranchValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  return {
    name: requireStringField(record, 'name'),
    discipline: requireStringField(record, 'discipline'),
  } satisfies CreateBranchInput;
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
 * Forwards a validated create-branch request to the store's `POST /branches`. Surfaces the
 * store's 4xx validation errors as `CreateBranchValidationError` (never swallowed), and
 * rethrows any other failure (network errors, unexpected store status codes) for the caller to
 * handle.
 */
export async function createBranch(
  input: CreateBranchInput,
  storeUrl: string,
): Promise<CreateBranchResult> {
  const response = await fetch(`${storeUrl}/branches`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', ...getStoreAuthHeaders() },
    body: JSON.stringify(input),
  });

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    throw new CreateBranchValidationError(
      extractErrorMessage(payload, `Store rejected create-branch request (${String(response.status)})`),
      response.status,
    );
  }

  return payload as CreateBranchResult;
}
