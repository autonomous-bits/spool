/**
 * MCP `capture-chunk` tool (Meridian IDEA-9): lets an ideation assistant capture a chunk on
 * behalf of a human stakeholder by delegating to the store's `POST /chunks` endpoint. The tool
 * never invents a stakeholder identity — it always forwards the caller-supplied
 * `stakeholderId` and lets the store enforce that it already exists.
 *
 * G11 SG6 (Meridian IDEA-92/IDEA-98/IDEA-100): `POST /chunks` sits on the delegated, tokenless
 * auth tier, so this tool requires a `workspaceId` input and forwards it as the store's
 * `X-Workspace-Id` header (not a body field) — the store validates it against a
 * `workspace_memberships` row for the caller-supplied `stakeholderId`.
 */

/** Untrusted-input shape mirroring the store's `CreateChunkRequest` (apps/store/src/chunks). */
export interface CaptureChunkInput {
  label: string;
  content: string;
  discipline: string;
  chunkType: string;
  contextKind: string;
  stakeholderId: string;
  workspaceId: string;
  branchId?: string;
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
  branchId: string | null;
  originBranchId: string | null;
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
 * Validates the optional `branchId` field, if present. When present it must be a non-empty
 * string; when absent it is omitted entirely (not set to `undefined`) so callers preserve
 * G01's branchless-path behavior unchanged.
 */
function optionalStringField(record: Record<string, unknown>, field: string): string | undefined {
  if (!(field in record) || record[field] === undefined) {
    return undefined;
  }
  return requireStringField(record, field);
}

/**
 * Validates that an untrusted tool-call body has the expected shape before it is forwarded to
 * the store. Vocabulary values (discipline/chunkType/contextKind) are intentionally not
 * re-validated here: the store is the authoritative source for those invariants, and this tool
 * must surface the store's own validation errors rather than duplicate or pre-empt them.
 * `branchId` is likewise forwarded as-is: branch existence/discipline/status invariants are the
 * store's responsibility (Meridian IDEA-52/IDEA-34/G02 SG3).
 */
export function parseCaptureChunkInput(body: unknown): CaptureChunkInput {
  if (typeof body !== 'object' || body === null) {
    throw new CaptureChunkValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  const branchId = optionalStringField(record, 'branchId');

  return {
    label: requireStringField(record, 'label'),
    content: requireStringField(record, 'content'),
    discipline: requireStringField(record, 'discipline'),
    chunkType: requireStringField(record, 'chunkType'),
    contextKind: requireStringField(record, 'contextKind'),
    stakeholderId: requireStringField(record, 'stakeholderId'),
    workspaceId: requireStringField(record, 'workspaceId'),
    ...(branchId !== undefined ? { branchId } : {}),
  } satisfies CaptureChunkInput;
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
 * Forwards a validated capture request to the store's `POST /chunks`. Surfaces the store's 400
 * validation errors as `CaptureChunkValidationError` (never swallowed), and rethrows any other
 * failure (network errors, unexpected store status codes) for the caller to handle.
 */
export async function captureChunk(
  input: CaptureChunkInput,
  storeUrl: string,
): Promise<CaptureChunkResult> {
  const { workspaceId, ...body } = input;
  const response = await fetch(`${storeUrl}/chunks`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', 'x-workspace-id': workspaceId },
    body: JSON.stringify(body),
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
