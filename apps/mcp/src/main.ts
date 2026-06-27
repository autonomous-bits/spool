import { createMcpHttpServer } from './server.js';

const harnessUrl = process.env.HARNESS_URL ?? 'http://localhost:3000';
const port = Number(process.env.MCP_PORT ?? 3001);

const server = createMcpHttpServer(harnessUrl);
server.listen(port);
