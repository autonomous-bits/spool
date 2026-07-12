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
 * `loadStoreCredentials()` validates that `SPOOL_WORKSPACE_ID` (and, if set, `SPOOL_SESSION_TOKEN`)
 * is present and well-formed before the transport connects, so malformed env vars still fail fast.
 * The network/interactive authentication preflight (`runStartupAuthentication`) runs after
 * `server.connect()` instead of before it: interactive GitHub login can take minutes, and running
 * it ahead of connect() blocks the MCP initialize handshake long enough for clients' own connection
 * timeouts to report "Failed to connect to MCP server" even though the process is alive and only
 * waiting on the user.
 */
async function runCli(): Promise<void> {
  const credentials = loadStoreCredentials();

  const server = createMcpServer(storeUrl);
  const transport = new StdioServerTransport();

  await server.connect(transport);
  await runStartupAuthentication({ storeUrl, credentials });

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
