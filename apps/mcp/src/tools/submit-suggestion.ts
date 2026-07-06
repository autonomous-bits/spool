/**
 * MCP `submit-suggestion` tool (Meridian IDEA-9, IDEA-27/IDEA-28/IDEA-49): lets an ideation
 * assistant submit a chunk or edge suggestion on behalf of a human stakeholder by delegating to
 * the store's `POST /suggestions` endpoint. Submission only -- this tool never accepts/rejects a
 * suggestion (Meridian IDEA-75: suggestion decisions are human-only). The tool never invents a
 * stakeholder identity -- it always forwards the caller-supplied `stakeholderId` and lets the
 * store enforce that it already exists. It also never re-validates the `relationshipType`/
 * `discipline` closed vocabularies itself: the store is the authoritative source for those
 * invariants, and this tool must surface the store's own validation errors rather than duplicate
 * or pre-empt them.
 */

/**
 * Untrusted-input shape mirroring the store's `CreateSuggestionRequest` (apps/store/src/
 * suggestions). Exactly one of the chunk-shaped (`label`/`content`) or edge-shaped
 * (`fromChunkLabel`/`toChunkLabel`/`relationshipType`) field groups must be provided; the store
 * is the authoritative enforcer of that XOR shape (`check_suggestion_type`).
 */
export interface SubmitSuggestionInput {
  discipline: string;
  stakeholderId: string;
  label?: string;
  content?: string;
  fromChunkLabel?: string;
  toChunkLabel?: string;
  relationshipType?: string;
}

/** Suggestion as returned by the store's `POST /suggestions` on success. */
export interface SubmitSuggestionResult {
  id: string;
  label: string | null;
  content: string | null;
  fromChunkLabel: string | null;
  toChunkLabel: string | null;
  relationshipType: string | null;
  discipline: string;
  status: string;
  submittedByStakeholderId: string;
  submittedByActorKind: string;
  decidedByStakeholderId: string | null;
  decidedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

/** Raised when the store rejects the request with a 4xx; carries the store's own message. */
export class SubmitSuggestionValidationError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'SubmitSuggestionValidationError';
  }
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0;
}

function requireStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (!isNonEmptyString(value)) {
    throw new SubmitSuggestionValidationError(`${field} must be a non-empty string`, 400);
  }
  return value;
}

/**
 * Validates that an untrusted tool-call body has the expected shape before it is forwarded to
 * the store. Requires `discipline` and `stakeholderId` always, plus either the chunk-shaped
 * (`label`, `content`) or edge-shaped (`fromChunkLabel`, `toChunkLabel`, `relationshipType`)
 * fields -- mixing both, or providing neither/a partial shape of either, is rejected here so the
 * tool fails fast with a clear message; the store re-validates the same invariant independently.
 * `relationshipType`/`discipline` vocabulary values are intentionally not re-validated here: the
 * store is the authoritative source for those invariants.
 */
export function parseSubmitSuggestionInput(body: unknown): SubmitSuggestionInput {
  if (typeof body !== 'object' || body === null) {
    throw new SubmitSuggestionValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  const discipline = requireStringField(record, 'discipline');
  const stakeholderId = requireStringField(record, 'stakeholderId');

  const hasAnyChunkField = isNonEmptyString(record.label) || isNonEmptyString(record.content);
  const hasAnyEdgeField =
    isNonEmptyString(record.fromChunkLabel) ||
    isNonEmptyString(record.toChunkLabel) ||
    record.relationshipType !== undefined;

  if (hasAnyChunkField && hasAnyEdgeField) {
    throw new SubmitSuggestionValidationError(
      'Request body must not mix chunk-shaped (label/content) and edge-shaped ' +
        '(fromChunkLabel/toChunkLabel/relationshipType) fields',
      400,
    );
  }

  if (hasAnyChunkField) {
    const label = requireStringField(record, 'label');
    const content = requireStringField(record, 'content');
    return { discipline, stakeholderId, label, content } satisfies SubmitSuggestionInput;
  }

  if (hasAnyEdgeField) {
    const fromChunkLabel = requireStringField(record, 'fromChunkLabel');
    const toChunkLabel = requireStringField(record, 'toChunkLabel');
    const relationshipType = requireStringField(record, 'relationshipType');
    return {
      discipline,
      stakeholderId,
      fromChunkLabel,
      toChunkLabel,
      relationshipType,
    } satisfies SubmitSuggestionInput;
  }

  throw new SubmitSuggestionValidationError(
    'Request body must provide either chunk-shaped (label, content) or edge-shaped ' +
      '(fromChunkLabel, toChunkLabel, relationshipType) fields',
    400,
  );
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
 * Forwards a validated submit-suggestion request to the store's `POST /suggestions`. Surfaces
 * the store's 4xx validation errors as `SubmitSuggestionValidationError` (never swallowed), and
 * rethrows any other failure (network errors, unexpected store status codes) for the caller to
 * handle.
 */
export async function submitSuggestion(
  input: SubmitSuggestionInput,
  harnessUrl: string,
): Promise<SubmitSuggestionResult> {
  const response = await fetch(`${harnessUrl}/suggestions`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(input),
  });

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    throw new SubmitSuggestionValidationError(
      extractErrorMessage(payload, `Store rejected submit-suggestion request (${String(response.status)})`),
      response.status,
    );
  }

  return payload as SubmitSuggestionResult;
}
