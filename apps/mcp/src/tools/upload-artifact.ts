/**
 * MCP `upload-artifact` tool (Meridian IDEA-9, IDEA-58/IDEA-59, IDEA-52/IDEA-34): lets an
 * ideation assistant upload a standalone artifact blob on behalf of a human stakeholder by
 * delegating to the store's `POST /artifacts` endpoint. Inline base64 content only -- there is no
 * multipart/streaming path in G08. The tool never invents a stakeholder identity -- it always
 * forwards the caller-supplied `stakeholderId` and lets the store enforce that it already exists.
 * It also never re-validates `mimeType` itself: the store is the authoritative source for that
 * invariant, and this tool must surface the store's own validation errors rather than duplicate
 * or pre-empt them.
 *
 * It does own one guard the store does not: decoded-content size. The store's raw-body cap
 * (`MAX_BODY_BYTES` in server.ts) already prevents unbounded buffering of the whole tool-call
 * request, but a clear, tool-scoped rejection of an oversized artifact -- rather than a generic
 * "body too large" -- gives the calling agent an unambiguous, non-crashing error to react to.
 */

/** Untrusted-input shape mirroring the store's `CreateArtifactRequest` (apps/store/src/artifacts). */
export interface UploadArtifactInput {
  content: string;
  mimeType: string;
  stakeholderId: string;
}

/** Artifact metadata as returned by the store's `POST /artifacts` on success. */
export interface UploadArtifactResult {
  id: string;
  uri: string;
  mimeType: string;
  createdByStakeholderId: string;
  createdAt: string;
}

/** Raised for tool-local validation failures and for surfaced store 4xx errors. */
export class UploadArtifactValidationError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'UploadArtifactValidationError';
  }
}

// Decoded-content size guard owned by this tool, distinct from (and deliberately smaller than) the
// MCP server's raw request-body cap (`MAX_BODY_BYTES` = 1,000,000 bytes in server.ts). Base64
// inflates payload size by ~4/3, so a decoded artifact anywhere near 1 MiB would already trip that
// generic cap first; keeping this guard well under it ensures an oversized upload always yields
// this tool's specific, actionable message rather than the server's generic "body too large".
export const MAX_ARTIFACT_CONTENT_BYTES = 700_000;

const BASE64_PATTERN = /^[A-Za-z0-9+/]*={0,2}$/;

function requireStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new UploadArtifactValidationError(`${field} must be a non-empty string`, 400);
  }
  return value;
}

/** Computes decoded byte length from a base64 string's length and trailing padding, without allocating a Buffer. */
function decodedByteLength(base64: string): number {
  const padding = base64.endsWith('==') ? 2 : base64.endsWith('=') ? 1 : 0;
  return (base64.length / 4) * 3 - padding;
}

/**
 * Validates that an untrusted tool-call body has the expected shape before it is forwarded to
 * the store. `mimeType` is intentionally not re-validated beyond non-empty: the store is the
 * authoritative source for that invariant. `content` must be well-formed base64 and, once
 * decoded, must not exceed `MAX_ARTIFACT_CONTENT_BYTES` -- both checked here so an oversized or
 * malformed upload never reaches the store and always yields a clear client error.
 */
export function parseUploadArtifactInput(body: unknown): UploadArtifactInput {
  if (typeof body !== 'object' || body === null) {
    throw new UploadArtifactValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  const content = requireStringField(record, 'content');
  const mimeType = requireStringField(record, 'mimeType');
  const stakeholderId = requireStringField(record, 'stakeholderId');

  if (!BASE64_PATTERN.test(content) || content.length % 4 !== 0) {
    throw new UploadArtifactValidationError('content must be base64-encoded', 400);
  }

  if (decodedByteLength(content) > MAX_ARTIFACT_CONTENT_BYTES) {
    throw new UploadArtifactValidationError(
      `Decoded content exceeds the maximum artifact size of ${String(MAX_ARTIFACT_CONTENT_BYTES)} bytes`,
      400,
    );
  }

  return { content, mimeType, stakeholderId } satisfies UploadArtifactInput;
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
 * Forwards a validated upload request to the store's `POST /artifacts`. Surfaces the store's 4xx
 * validation errors as `UploadArtifactValidationError` (never swallowed), and rethrows any other
 * failure (network errors, unexpected store status codes) for the caller to handle.
 */
export async function uploadArtifact(
  input: UploadArtifactInput,
  harnessUrl: string,
): Promise<UploadArtifactResult> {
  const response = await fetch(`${harnessUrl}/artifacts`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(input),
  });

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    throw new UploadArtifactValidationError(
      extractErrorMessage(payload, `Store rejected upload-artifact request (${String(response.status)})`),
      response.status,
    );
  }

  return payload as UploadArtifactResult;
}
