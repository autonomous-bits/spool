/**
 * MCP `attach-artifact-to-chunk` tool (Meridian IDEA-9, IDEA-60, IDEA-52/IDEA-34): lets an
 * ideation assistant associate an already-uploaded artifact with a chunk on behalf of a human
 * stakeholder by delegating to the store's `POST /chunks/:label/artifacts` endpoint. The tool
 * never invents a stakeholder identity -- it always forwards the caller-supplied `stakeholderId`
 * and lets the store enforce that it already exists. `branchId` is forwarded as-is when present:
 * branch-scoped vs. mainline association semantics are the store's responsibility (Meridian
 * IDEA-32/IDEA-60), and this tool must surface the store's own validation/404 errors rather than
 * duplicate or pre-empt them.
 *
 * G11 SG6 (Meridian IDEA-92/IDEA-98/IDEA-100): `POST /chunks/:label/artifacts` sits on the
 * delegated, tokenless auth tier, so this tool requires a `workspaceId` input and forwards it as
 * the store's `X-Workspace-Id` header (not a body field).
 */

/**
 * Untrusted-input shape mirroring the store's `CreateChunkArtifactRequest`
 * (apps/store/src/artifacts). `chunkLabel` is carried here (rather than as a separate route
 * param) because this tool speaks to a single MCP route and forwards it into the store's
 * `:label` path segment itself.
 */
export interface AttachArtifactToChunkInput {
  chunkLabel: string;
  artifactId: string;
  stakeholderId: string;
  workspaceId: string;
  branchId?: string;
}

/** Association as returned by the store's `POST /chunks/:label/artifacts` on success. */
export interface AttachArtifactToChunkResult {
  id: string;
  chunkLabel: string;
  artifactId: string;
  status: string;
  branchId: string | null;
  originBranchId: string | null;
  createdByStakeholderId: string;
  updatedByStakeholderId: string;
  createdAt: string;
  updatedAt: string;
}

/** Raised when the store rejects the request with a 4xx; carries the store's own message. */
export class AttachArtifactToChunkValidationError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'AttachArtifactToChunkValidationError';
  }
}

function requireStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new AttachArtifactToChunkValidationError(`${field} must be a non-empty string`, 400);
  }
  return value;
}

/**
 * Validates that an untrusted tool-call body has the expected shape before it is forwarded to
 * the store. `branchId` is optional; when present it must be a non-empty string, mirroring the
 * `create-edge`/`capture-chunk` precedent for optional branch scoping.
 */
export function parseAttachArtifactToChunkInput(body: unknown): AttachArtifactToChunkInput {
  if (typeof body !== 'object' || body === null) {
    throw new AttachArtifactToChunkValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  const chunkLabel = requireStringField(record, 'chunkLabel');
  const artifactId = requireStringField(record, 'artifactId');
  const stakeholderId = requireStringField(record, 'stakeholderId');
  const workspaceId = requireStringField(record, 'workspaceId');

  const branchIdValue = record.branchId;
  if (branchIdValue !== undefined) {
    if (typeof branchIdValue !== 'string' || branchIdValue.trim().length === 0) {
      throw new AttachArtifactToChunkValidationError('branchId must be a non-empty string when provided', 400);
    }
    return {
      chunkLabel,
      artifactId,
      stakeholderId,
      workspaceId,
      branchId: branchIdValue,
    } satisfies AttachArtifactToChunkInput;
  }

  return { chunkLabel, artifactId, stakeholderId, workspaceId } satisfies AttachArtifactToChunkInput;
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
 * Forwards a validated attach request to the store's `POST /chunks/:label/artifacts`. Surfaces
 * the store's 4xx errors (including 404 for an unknown chunk/artifact) as
 * `AttachArtifactToChunkValidationError` (never swallowed), and rethrows any other failure
 * (network errors, unexpected store status codes) for the caller to handle.
 */
export async function attachArtifactToChunk(
  input: AttachArtifactToChunkInput,
  harnessUrl: string,
): Promise<AttachArtifactToChunkResult> {
  const { chunkLabel, workspaceId, ...body } = input;
  const response = await fetch(`${harnessUrl}/chunks/${encodeURIComponent(chunkLabel)}/artifacts`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', 'x-workspace-id': workspaceId },
    body: JSON.stringify(body),
  });

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    throw new AttachArtifactToChunkValidationError(
      extractErrorMessage(
        payload,
        `Store rejected attach-artifact-to-chunk request (${String(response.status)})`,
      ),
      response.status,
    );
  }

  return payload as AttachArtifactToChunkResult;
}
