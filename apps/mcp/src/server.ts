import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import { captureChunk, CaptureChunkValidationError, parseCaptureChunkInput } from './tools/capture-chunk.js';
import { createBranch, CreateBranchValidationError, parseCreateBranchInput } from './tools/create-branch.js';
import { createEdge, CreateEdgeValidationError, parseCreateEdgeInput } from './tools/create-edge.js';
import {
  submitSuggestion,
  SubmitSuggestionValidationError,
  parseSubmitSuggestionInput,
} from './tools/submit-suggestion.js';

interface McpHealthResponse {
  status: 'ok';
  service: 'mcp';
  harnessUrl: string;
}

export function createMcpHealthResponse(
  harnessUrl = process.env.HARNESS_URL ?? 'http://localhost:3000',
): McpHealthResponse {
  return {
    status: 'ok',
    service: 'mcp',
    harnessUrl,
  };
}

const CAPTURE_CHUNK_ROUTE = '/tools/capture-chunk';
const CREATE_BRANCH_ROUTE = '/tools/create-branch';
const CREATE_EDGE_ROUTE = '/tools/create-edge';
const SUBMIT_SUGGESTION_ROUTE = '/tools/submit-suggestion';
// Bounds the in-memory body buffer for a single tool call (node-memory-management: avoid
// unbounded buffering of untrusted input).
const MAX_BODY_BYTES = 1_000_000;

function logDiagnostic(level: 'info' | 'error', msg: string, extra?: Record<string, unknown>): void {
  process.stderr.write(`${JSON.stringify({ level, msg, ...extra })}\n`);
}

/** Generic 4xx error raised for malformed/oversized request bodies, before tool-specific parsing. */
class RequestBodyError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    this.name = 'RequestBodyError';
  }
}

async function readRequestBody(request: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  let totalBytes = 0;

  for await (const chunk of request) {
    const buffer = chunk as Buffer;
    totalBytes += buffer.length;
    if (totalBytes > MAX_BODY_BYTES) {
      throw new RequestBodyError('Request body too large', 413);
    }
    chunks.push(buffer);
  }

  return Buffer.concat(chunks).toString('utf8');
}

function sendJson(response: ServerResponse, statusCode: number, body: unknown): void {
  response.writeHead(statusCode, { 'content-type': 'application/json' });
  response.end(JSON.stringify(body));
}

async function handleCaptureChunk(
  request: IncomingMessage,
  response: ServerResponse,
  harnessUrl: string,
): Promise<void> {
  try {
    const raw = await readRequestBody(request);
    let parsedBody: unknown;
    try {
      parsedBody = raw.length === 0 ? undefined : JSON.parse(raw);
    } catch {
      throw new CaptureChunkValidationError('Request body must be valid JSON', 400);
    }

    const input = parseCaptureChunkInput(parsedBody);
    const chunk = await captureChunk(input, harnessUrl);
    sendJson(response, 201, chunk);
  } catch (error) {
    if (error instanceof RequestBodyError || error instanceof CaptureChunkValidationError) {
      sendJson(response, error.statusCode, { message: error.message });
      return;
    }

    const reason = error instanceof Error ? error.message : 'Unknown error';
    logDiagnostic('error', 'capture-chunk tool call failed', { reason });
    sendJson(response, 502, { message: 'Failed to reach the store' });
  }
}

async function handleCreateBranch(
  request: IncomingMessage,
  response: ServerResponse,
  harnessUrl: string,
): Promise<void> {
  try {
    const raw = await readRequestBody(request);
    let parsedBody: unknown;
    try {
      parsedBody = raw.length === 0 ? undefined : JSON.parse(raw);
    } catch {
      throw new CreateBranchValidationError('Request body must be valid JSON', 400);
    }

    const input = parseCreateBranchInput(parsedBody);
    const branch = await createBranch(input, harnessUrl);
    sendJson(response, 201, branch);
  } catch (error) {
    if (error instanceof RequestBodyError || error instanceof CreateBranchValidationError) {
      sendJson(response, error.statusCode, { message: error.message });
      return;
    }

    const reason = error instanceof Error ? error.message : 'Unknown error';
    logDiagnostic('error', 'create-branch tool call failed', { reason });
    sendJson(response, 502, { message: 'Failed to reach the store' });
  }
}

async function handleCreateEdge(
  request: IncomingMessage,
  response: ServerResponse,
  harnessUrl: string,
): Promise<void> {
  try {
    const raw = await readRequestBody(request);
    let parsedBody: unknown;
    try {
      parsedBody = raw.length === 0 ? undefined : JSON.parse(raw);
    } catch {
      throw new CreateEdgeValidationError('Request body must be valid JSON', 400);
    }

    const input = parseCreateEdgeInput(parsedBody);
    const edge = await createEdge(input, harnessUrl);
    sendJson(response, 201, edge);
  } catch (error) {
    if (error instanceof RequestBodyError || error instanceof CreateEdgeValidationError) {
      sendJson(response, error.statusCode, { message: error.message });
      return;
    }

    const reason = error instanceof Error ? error.message : 'Unknown error';
    logDiagnostic('error', 'create-edge tool call failed', { reason });
    sendJson(response, 502, { message: 'Failed to reach the store' });
  }
}

async function handleSubmitSuggestion(
  request: IncomingMessage,
  response: ServerResponse,
  harnessUrl: string,
): Promise<void> {
  try {
    const raw = await readRequestBody(request);
    let parsedBody: unknown;
    try {
      parsedBody = raw.length === 0 ? undefined : JSON.parse(raw);
    } catch {
      throw new SubmitSuggestionValidationError('Request body must be valid JSON', 400);
    }

    const input = parseSubmitSuggestionInput(parsedBody);
    const suggestion = await submitSuggestion(input, harnessUrl);
    sendJson(response, 201, suggestion);
  } catch (error) {
    if (error instanceof RequestBodyError || error instanceof SubmitSuggestionValidationError) {
      sendJson(response, error.statusCode, { message: error.message });
      return;
    }

    const reason = error instanceof Error ? error.message : 'Unknown error';
    logDiagnostic('error', 'submit-suggestion tool call failed', { reason });
    sendJson(response, 502, { message: 'Failed to reach the store' });
  }
}

export function createMcpHttpServer(
  harnessUrl = process.env.HARNESS_URL ?? 'http://localhost:3000',
): Server {
  return createServer((request: IncomingMessage, response: ServerResponse) => {
    if (request.method === 'POST' && request.url === CAPTURE_CHUNK_ROUTE) {
      void handleCaptureChunk(request, response, harnessUrl);
      return;
    }

    if (request.method === 'POST' && request.url === CREATE_BRANCH_ROUTE) {
      void handleCreateBranch(request, response, harnessUrl);
      return;
    }

    if (request.method === 'POST' && request.url === CREATE_EDGE_ROUTE) {
      void handleCreateEdge(request, response, harnessUrl);
      return;
    }

    if (request.method === 'POST' && request.url === SUBMIT_SUGGESTION_ROUTE) {
      void handleSubmitSuggestion(request, response, harnessUrl);
      return;
    }

    response.writeHead(200, { 'content-type': 'application/json' });
    response.end(JSON.stringify(createMcpHealthResponse(harnessUrl)));
  });
}
