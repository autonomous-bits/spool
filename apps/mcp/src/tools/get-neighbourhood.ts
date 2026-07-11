export interface GetNeighbourhoodInput {
  id: string;
  sessionToken: string;
  workspaceId: string;
  depth?: number;
  branchId?: string;
}

export interface NeighbourResponse {
  edgeId: string;
  chunkId: string;
  label: string;
  content: string;
  type: string;
  status: string;
  discipline: string;
  contextKind: string;
  direction: 'outgoing' | 'incoming';
  hop: number;
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

export interface GetNeighbourhoodResult {
  chunk: ChunkResponse;
  neighbours: NeighbourResponse[];
}

export class GetNeighbourhoodValidationError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'GetNeighbourhoodValidationError';
  }
}

function requireStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new GetNeighbourhoodValidationError(`${field} must be a non-empty string`, 400);
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
    throw new GetNeighbourhoodValidationError(`${field} must be a number`, 400);
  }
  return record[field];
}

export function parseGetNeighbourhoodInput(body: unknown): GetNeighbourhoodInput {
  if (typeof body !== 'object' || body === null) {
    throw new GetNeighbourhoodValidationError('Request body must be a JSON object', 400);
  }

  const record = body as Record<string, unknown>;

  const result: GetNeighbourhoodInput = {
    id: requireStringField(record, 'id'),
    sessionToken: requireStringField(record, 'sessionToken'),
    workspaceId: requireStringField(record, 'workspaceId'),
  };
  
  const depth = optionalNumberField(record, 'depth');
  if (depth !== undefined) result.depth = depth;
  
  const branchId = optionalStringField(record, 'branchId');
  if (branchId !== undefined) result.branchId = branchId;
  
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

export async function getNeighbourhood(
  input: GetNeighbourhoodInput,
  storeUrl: string,
): Promise<GetNeighbourhoodResult> {
  const { id, sessionToken, workspaceId, ...queryParams } = input;
  
  const url = new URL(`${storeUrl}/chunks/${id}/neighbourhood`);
  for (const [key, value] of Object.entries(queryParams)) {
    url.searchParams.append(key, String(value));
  }

  const response = await fetch(url.toString(), {
    method: 'GET',
    headers: {
      authorization: `Bearer ${sessionToken}`,
      'x-workspace-id': workspaceId,
    },
  });

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    throw new GetNeighbourhoodValidationError(
      extractErrorMessage(payload, `Store rejected get-neighbourhood request (${String(response.status)})`),
      response.status,
    );
  }

  return payload as GetNeighbourhoodResult;
}
