import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z, type ZodRawShape } from 'zod';
import { attachArtifactToChunk, parseAttachArtifactToChunkInput } from './tools/attach-artifact-to-chunk.js';
import { captureChunk, parseCaptureChunkInput } from './tools/capture-chunk.js';
import { createBranch, parseCreateBranchInput } from './tools/create-branch.js';
import { createEdge, parseCreateEdgeInput } from './tools/create-edge.js';
import { submitSuggestion, parseSubmitSuggestionInput } from './tools/submit-suggestion.js';
import { submitVerificationSignal, parseSubmitVerificationSignalInput } from './tools/submit-verification-signal.js';
import { uploadArtifact, parseUploadArtifactInput } from './tools/upload-artifact.js';
import { searchChunks, parseSearchChunksInput } from './tools/search-chunks.js';
import { getNeighbourhood, parseGetNeighbourhoodInput } from './tools/get-neighbourhood.js';

// Kept as `type` (not `interface`): the SDK's registerTool callback return type has an index
// signature, and an `interface` here is not structurally assignable to it (TS2322).
// eslint-disable-next-line @typescript-eslint/consistent-type-definitions
type ToolTextContent = { type: 'text'; text: string };
// eslint-disable-next-line @typescript-eslint/consistent-type-definitions
export type ToolResult = { content: ToolTextContent[]; isError?: true };

function logDiagnostic(level: 'info' | 'error', msg: string, extra?: Record<string, unknown>): void {
  process.stderr.write(`${JSON.stringify({ level, msg, ...extra })}\n`);
}

/**
 * Runs a tool's parse+business-logic pipeline and maps the outcome to the MCP tool-result
 * contract (Meridian IDEA-137): success is `{ content: [...] }`, any thrown `*ValidationError`
 * or upstream failure is `{ content: [...], isError: true }`. This is distinct from a
 * protocol-level rejection by the SDK's own Zod `inputSchema`, which never reaches this
 * function — the SDK returns its own error before the handler runs.
 */
async function runTool(toolName: string, fn: () => Promise<unknown>): Promise<ToolResult> {
  try {
    const result = await fn();
    return { content: [{ type: 'text', text: JSON.stringify(result) }] };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logDiagnostic('error', `${toolName} tool call failed`, { reason: message });
    return { content: [{ type: 'text', text: message }], isError: true };
  }
}

const captureChunkInputSchema = {
  label: z.string().min(1),
  content: z.string().min(1),
  discipline: z.string().min(1),
  chunkType: z.string().min(1),
  contextKind: z.string().min(1),
  branchId: z.string().min(1).optional(),
} satisfies ZodRawShape;

const createBranchInputSchema = {
  name: z.string().min(1),
  discipline: z.string().min(1),
} satisfies ZodRawShape;

const createEdgeInputSchema = {
  fromChunkLabel: z.string().min(1),
  toChunkLabel: z.string().min(1),
  type: z.string().min(1),
  discipline: z.string().min(1),
  branchId: z.string().min(1).optional(),
} satisfies ZodRawShape;

const submitSuggestionInputSchema = {
  discipline: z.string().min(1),
  label: z.string().min(1).optional(),
  content: z.string().min(1).optional(),
  fromChunkLabel: z.string().min(1).optional(),
  toChunkLabel: z.string().min(1).optional(),
  relationshipType: z.string().min(1).optional(),
} satisfies ZodRawShape;

const submitVerificationSignalInputSchema = {
  branchId: z.string().min(1),
  verifierName: z.string().min(1),
  status: z.string().min(1),
  reason: z.string().min(1).optional(),
} satisfies ZodRawShape;

const uploadArtifactInputSchema = {
  content: z.string().min(1),
  mimeType: z.string().min(1),
} satisfies ZodRawShape;

const attachArtifactToChunkInputSchema = {
  chunkLabel: z.string().min(1),
  artifactId: z.string().min(1),
  branchId: z.string().min(1).optional(),
} satisfies ZodRawShape;

const searchChunksInputSchema = {
  discipline: z.string().min(1).optional(),
  chunkType: z.string().min(1).optional(),
  status: z.string().min(1).optional(),
  contextKind: z.string().min(1).optional(),
  branchId: z.string().min(1).optional(),
  q: z.string().min(1).optional(),
  limit: z.number().optional(),
  cursor: z.string().min(1).optional(),
  activeDiscipline: z.string().min(1).optional(),
} satisfies ZodRawShape;

const getNeighbourhoodInputSchema = {
  id: z.string().min(1),
  depth: z.number().optional(),
  branchId: z.string().min(1).optional(),
  activeDiscipline: z.string().min(1).optional(),
} satisfies ZodRawShape;

/**
 * Builds the Spool MCP server (Meridian IDEA-137): a real stdio JSON-RPC `McpServer` exposing
 * the 9 existing tool handlers with explicit Zod input schemas. Each handler still calls the
 * tool's existing `parse*Input` function and business-logic function unchanged; only the
 * transport and result-shaping are new.
 */
export function createMcpServer(storeUrl = process.env.SPOOL_STORE_URL ?? 'http://localhost:3000'): McpServer {
  const server = new McpServer({ name: 'spool-mcp', version: '0.1.0' });

  server.registerTool(
    'capture-chunk',
    {
      title: 'Capture Chunk',
      description:
        'Captures a chunk on behalf of a human stakeholder by delegating to the store (host-held session token, G19).',
      inputSchema: captureChunkInputSchema,
    },
    async (args) =>
      runTool('capture-chunk', async () => {
        const input = parseCaptureChunkInput(args);
        return captureChunk(input, storeUrl);
      }),
  );

  server.registerTool(
    'create-branch',
    {
      title: 'Create Branch',
      description: 'Creates a new branch for a discipline on behalf of a human stakeholder (host-held session token, G19).',
      inputSchema: createBranchInputSchema,
    },
    async (args) =>
      runTool('create-branch', async () => {
        const input = parseCreateBranchInput(args);
        return createBranch(input, storeUrl);
      }),
  );

  server.registerTool(
    'create-edge',
    {
      title: 'Create Edge',
      description: 'Creates a typed relationship edge between two chunks (host-held session token, G19).',
      inputSchema: createEdgeInputSchema,
    },
    async (args) =>
      runTool('create-edge', async () => {
        const input = parseCreateEdgeInput(args);
        return createEdge(input, storeUrl);
      }),
  );

  server.registerTool(
    'submit-suggestion',
    {
      title: 'Submit Suggestion',
      description: 'Submits a chunk or edge suggestion for later review (host-held session token, G19).',
      inputSchema: submitSuggestionInputSchema,
    },
    async (args) =>
      runTool('submit-suggestion', async () => {
        const input = parseSubmitSuggestionInput(args);
        return submitSuggestion(input, storeUrl);
      }),
  );

  server.registerTool(
    'submit-verification-signal',
    {
      title: 'Submit Verification Signal',
      description: 'Submits a pass/fail verification signal for a branch (host-held session token, G19).',
      inputSchema: submitVerificationSignalInputSchema,
    },
    async (args) =>
      runTool('submit-verification-signal', async () => {
        const input = parseSubmitVerificationSignalInput(args);
        return submitVerificationSignal(input, storeUrl);
      }),
  );

  server.registerTool(
    'upload-artifact',
    {
      title: 'Upload Artifact',
      description: 'Uploads a base64-encoded artifact on behalf of a human stakeholder (host-held session token, G19).',
      inputSchema: uploadArtifactInputSchema,
    },
    async (args) =>
      runTool('upload-artifact', async () => {
        const input = parseUploadArtifactInput(args);
        return uploadArtifact(input, storeUrl);
      }),
  );

  server.registerTool(
    'attach-artifact-to-chunk',
    {
      title: 'Attach Artifact To Chunk',
      description: 'Attaches a previously uploaded artifact to a chunk (host-held session token, G19).',
      inputSchema: attachArtifactToChunkInputSchema,
    },
    async (args) =>
      runTool('attach-artifact-to-chunk', async () => {
        const input = parseAttachArtifactToChunkInput(args);
        return attachArtifactToChunk(input, storeUrl);
      }),
  );

  server.registerTool(
    'search-chunks',
    {
      title: 'Search Chunks',
      description: 'Searches chunks within a workspace; requires a human-authenticated session token.',
      inputSchema: searchChunksInputSchema,
    },
    async (args) =>
      runTool('search-chunks', async () => {
        const input = parseSearchChunksInput(args);
        return searchChunks(input, storeUrl);
      }),
  );

  server.registerTool(
    'get-neighbourhood',
    {
      title: 'Get Neighbourhood',
      description: 'Returns a chunk and its typed-edge neighbours; requires a human-authenticated session token.',
      inputSchema: getNeighbourhoodInputSchema,
    },
    async (args) =>
      runTool('get-neighbourhood', async () => {
        const input = parseGetNeighbourhoodInput(args);
        return getNeighbourhood(input, storeUrl);
      }),
  );

  return server;
}
