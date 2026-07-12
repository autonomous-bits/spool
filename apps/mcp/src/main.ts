import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { createMcpServer } from './server.js';
import { loadStoreCredentials } from './store-client.js';
import { runStartupAuthentication } from './startup-auth.js';

const storeUrl = process.env.SPOOL_STORE_URL ?? 'http://localhost:3000';

function writeStderr(message: string): void {
  process.stderr.write(`${message}\n`);
}

function formatError(error: unknown): string {
  if (error instanceof Error) {
    return error.stack ?? error.message;
  }
  return String(error);
}

function setExitCode(exitCode: number): void {
  if (exitCode !== 0 || process.exitCode === undefined) {
    process.exitCode = exitCode;
  }
}

/**
 * Starts the Spool MCP server over stdio (Meridian IDEA-137). Mirrors
 * meridian/apps/mcp/src/main.ts's shutdown pattern: idempotent close on stdin end/close and
 * SIGINT/SIGTERM, non-zero exit only when the shutdown itself fails.
 *
 * Validates the host-held session token/workspace id (G19 SG1/SG2) before the server connects
 * its transport or registers any tool handler, so a missing/malformed `SPOOL_SESSION_TOKEN`/
 * `SPOOL_WORKSPACE_ID` fails fast instead of surfacing on the first `tools/call`.
 */
async function runCli(): Promise<void> {
  const credentials = loadStoreCredentials();
  await runStartupAuthentication({ storeUrl, credentials });

  const server = createMcpServer(storeUrl);
  const transport = new StdioServerTransport();

  await server.connect(transport);

  let shuttingDown = false;

  const shutdown = async (exitCode: number): Promise<void> => {
    if (shuttingDown) {
      return;
    }
    shuttingDown = true;

    try {
      await server.close();
      setExitCode(exitCode);
    } catch (error) {
      writeStderr(`Spool MCP shutdown failed: ${formatError(error)}`);
      setExitCode(1);
    }
  };

  process.stdin.on('end', () => {
    void shutdown(0);
  });
  process.stdin.on('close', () => {
    void shutdown(0);
  });
  process.once('SIGINT', () => {
    void shutdown(0);
  });
  process.once('SIGTERM', () => {
    void shutdown(0);
  });
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  void runCli().catch((error: unknown) => {
    writeStderr(`Spool MCP bootstrap failed: ${formatError(error)}`);
    setExitCode(1);
  });
}
