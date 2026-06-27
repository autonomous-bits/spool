import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';

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

export function createMcpHttpServer(
  harnessUrl = process.env.HARNESS_URL ?? 'http://localhost:3000',
): Server {
  return createServer((_request: IncomingMessage, response: ServerResponse) => {
    response.writeHead(200, { 'content-type': 'application/json' });
    response.end(JSON.stringify(createMcpHealthResponse(harnessUrl)));
  });
}
