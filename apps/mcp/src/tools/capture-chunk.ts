/**
 * MCP `capture-chunk` tool (Meridian IDEA-9): lets an ideation assistant capture a chunk on
 * behalf of a human stakeholder by delegating to the store's `POST /chunks` endpoint. The tool
 * never invents a stakeholder identity — it always forwards the caller-supplied
 * `stakeholderId` and lets the store enforce that it already exists.
 */

/** Untrusted-input shape mirroring the store's `CreateChunkRequest` (apps/store/src/chunks). */
export interface CaptureChunkInput {
  label: string;
  content: string;
  discipline: string;
  chunkType: string;
  contextKind: string;
  stakeholderId: string;
}

/** Chunk as returned by the store's `POST /chunks` on success. */
export interface CaptureChunkResult {
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
}

/** Raised when the store rejects the request with a 4xx; carries the store's own message. */
export class CaptureChunkValidationError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'CaptureChunkValidationError';
  }
}

function requireStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new CaptureChunkValidationError(`${field} must be a non-empty string`, 400);
  }
  return value;
}

/**
 * Validates that an untrusted tool-call body has the expected shape before it is forwarded to
 * the store. Vocabulary values (discipline/chunkType/contextKind) are intentionally not
 * re-validated here: the store is the authoritative source for those invariants, and this tool
 * must surface the store's own validation errors rather than duplicate or pre-empt them.
 */
export function parseCaptureChunkInput(body: unknown): CaptureChunkInput {
  if (typeof body !== 'object' || body === null) {
    throw new CaptureChunkValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  return {
    label: requireStringField(record, 'label'),
    content: requireStringField(record, 'content'),
    discipline: requireStringField(record, 'discipline'),
    chunkType: requireStringField(record, 'chunkType'),
    contextKind: requireStringField(record, 'contextKind'),
    stakeholderId: requireStringField(record, 'stakeholderId'),
  } satisfies CaptureChunkInput;
}

function extractErrorMessage(body: unknown, fallback: string): string {
  if (
    typeof body === 'object' &&
    body !== null &&
    'message' in body &&
    typeof (body as { message: unknown }).message === 'string'
  ) {
    return (body as { message: string }).message;
  }
  return fallback;
}

/**
 * Forwards a validated capture request to the store's `POST /chunks`. Surfaces the store's 400
 * validation errors as `CaptureChunkValidationError` (never swallowed), and rethrows any other
 * failure (network errors, unexpected store status codes) for the caller to handle.
 */
export async function captureChunk(
  input: CaptureChunkInput,
  harnessUrl: string,
): Promise<CaptureChunkResult> {
  const response = await fetch(`${harnessUrl}/chunks`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(input),
  });

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    throw new CaptureChunkValidationError(
      extractErrorMessage(payload, `Store rejected capture-chunk request (${String(response.status)})`),
      response.status,
    );
  }

  return payload as CaptureChunkResult;
}
