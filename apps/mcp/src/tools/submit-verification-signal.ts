/**
 * MCP `submit-verification-signal` tool: lets a delegated actor submit a verification signal by
 * delegating to the store's `POST /branches/:branchId/verification-signals` endpoint. Workspace
 * scope is not a per-call input any more (G19 SG2/SG3): it comes from the shared store-client
 * helper's host-held workspace id. The tool never invents an actor identity and intentionally
 * does not re-validate the closed `status` vocabulary itself: the store is authoritative for
 * that invariant, and this tool must surface the store's own validation errors rather than
 * duplicate or pre-empt them.
 */

/**
 * Untrusted-input shape mirroring the store's `CreateVerificationSignalRequest`, plus the target
 * `branchId` path parameter. `status` is only required to be a non-empty string here so the store
 * remains the authoritative source for vocabulary validation.
 */
import { storeFetch } from '../store-client.js';

export interface SubmitVerificationSignalInput {
  branchId: string;
  verifierName: string;
  status: string;
  reason?: string;
}

/** Verification signal as returned by the store's `POST /branches/:id/verification-signals`. */
export interface SubmitVerificationSignalResult {
  id: string;
  branchId: string;
  verifierName: string;
  status: 'pass' | 'fail';
  reason: string | null;
  createdAt: string;
}

/** Raised when the store rejects the request with a 4xx; carries the store's own message. */
export class SubmitVerificationSignalValidationError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'SubmitVerificationSignalValidationError';
  }
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0;
}

function requireStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (!isNonEmptyString(value)) {
    throw new SubmitVerificationSignalValidationError(`${field} must be a non-empty string`, 400);
  }
  return value;
}

/**
 * Validates that an untrusted tool-call body has the expected shape before it is forwarded to the
 * store. `status` is intentionally not vocabulary-checked here; the store is the authoritative
 * source for that invariant and its own 4xx errors must be surfaced unchanged.
 */
export function parseSubmitVerificationSignalInput(body: unknown): SubmitVerificationSignalInput {
  if (typeof body !== 'object' || body === null) {
    throw new SubmitVerificationSignalValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  const branchId = requireStringField(record, 'branchId');
  const verifierName = requireStringField(record, 'verifierName');
  const status = requireStringField(record, 'status');

  const reasonValue = record.reason;
  if (reasonValue !== undefined) {
    if (typeof reasonValue !== 'string') {
      throw new SubmitVerificationSignalValidationError(
        'reason must be a string when provided',
        400,
      );
    }
    return {
      branchId,
      verifierName,
      status,
      reason: reasonValue,
    } satisfies SubmitVerificationSignalInput;
  }

  return {
    branchId,
    verifierName,
    status,
  } satisfies SubmitVerificationSignalInput;
}

function extractErrorMessage(body: unknown, fallback: string): string {
  if (
    typeof body === 'object' &&
    body !== null &&
    'message' in body &&
    typeof body.message === 'string'
  ) {
    return body.message;
  }
  return fallback;
}

/**
 * Forwards a validated submit-verification-signal request to the store's `POST
 * /branches/:branchId/verification-signals`. Surfaces the store's 4xx validation errors as
 * `SubmitVerificationSignalValidationError` (never swallowed), and rethrows any other failure
 * (network errors, unexpected store status codes) for the caller to handle.
 */
export async function submitVerificationSignal(
  input: SubmitVerificationSignalInput,
  storeUrl: string,
): Promise<SubmitVerificationSignalResult> {
  const { branchId, verifierName, status, ...rest } = input;
  const response = await storeFetch(
    storeUrl,
    `/branches/${encodeURIComponent(branchId)}/verification-signals`,
    {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ verifierName, status, ...rest }),
    },
  );

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    throw new SubmitVerificationSignalValidationError(
      extractErrorMessage(
        payload,
        `Store rejected submit-verification-signal request (${String(response.status)})`,
      ),
      response.status,
    );
  }

  return payload as SubmitVerificationSignalResult;
}
