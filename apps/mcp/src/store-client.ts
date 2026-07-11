/**
 * Shared host-held session-token + workspace-id client (Meridian IDEA-81, G19 SG2). Every one of
 * the 9 MCP tools gets its `Authorization`/`X-Workspace-Id` store headers from this one helper
 * instead of accepting a stakeholder/workspace id (or session token) as per-call tool input and
 * deriving the headers itself.
 *
 * `SPOOL_SESSION_TOKEN`/`SPOOL_WORKSPACE_ID` are read once and memoized for the lifetime of the
 * process, validated fail-fast per the session-token lifecycle contract documented in
 * `apps/mcp/AGENTS.md` (G19 SG1): a missing, empty, or otherwise malformed value throws a
 * `StoreClientConfigError` naming only the variable, never its value, before any store call is
 * attempted. `main.ts` calls `loadStoreCredentials()` at startup, before the MCP server connects
 * its transport or registers any tool handler, so the process fails fast rather than deferring
 * the failure to the first `tools/call`.
 *
 * Trade-off (deliberate, not a gap): because one process-held token + workspace pair now backs
 * every tool call for the lifetime of this MCP process, per-agent audit attribution collapses to
 * a single stakeholder identity per process/workspace -- see `apps/mcp/AGENTS.md`'s restart/
 * rotation story for how a fresh identity is obtained (an external process restart with a new
 * `pnpm dev:session-token`-issued token), not any in-process refresh or retry.
 */

export interface StoreCredentials {
  sessionToken: string;
  workspaceId: string;
}

/** Raised when `SPOOL_SESSION_TOKEN`/`SPOOL_WORKSPACE_ID` are missing or malformed at startup. */
export class StoreClientConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'StoreClientConfigError';
  }
}

let cachedCredentials: StoreCredentials | undefined;

/** Reads and validates one env var by name, never including its (potentially secret) value in the thrown message. */
function requireEnvVar(env: NodeJS.ProcessEnv, name: string): string {
  const value = env[name];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new StoreClientConfigError(
      `${name} environment variable is missing or empty; the MCP process cannot start without it`,
    );
  }
  return value;
}

/**
 * Reads and validates `SPOOL_SESSION_TOKEN`/`SPOOL_WORKSPACE_ID` once, memoizing the result for
 * the lifetime of the process. Call this at startup (see `main.ts`) so a missing/malformed env
 * var fails fast before any tool handler is registered or any `tools/call` is accepted, per G19
 * SG1. Subsequent calls (e.g. from `getStoreAuthHeaders`) reuse the cached value without
 * re-reading `process.env`.
 */
export function loadStoreCredentials(env: NodeJS.ProcessEnv = process.env): StoreCredentials {
  if (cachedCredentials) {
    return cachedCredentials;
  }

  const credentials: StoreCredentials = {
    sessionToken: requireEnvVar(env, 'SPOOL_SESSION_TOKEN'),
    workspaceId: requireEnvVar(env, 'SPOOL_WORKSPACE_ID'),
  };
  cachedCredentials = credentials;
  return credentials;
}

/**
 * Builds the two headers every store call needs from the host-held credentials, so none of the
 * 9 MCP tools construct an `Authorization`/`X-Workspace-Id` pair themselves or accept a
 * stakeholder/workspace id/session token from the caller in order to do so.
 */
export function getStoreAuthHeaders(env: NodeJS.ProcessEnv = process.env): Record<string, string> {
  const { sessionToken, workspaceId } = loadStoreCredentials(env);
  return { authorization: `Bearer ${sessionToken}`, 'x-workspace-id': workspaceId };
}

/** Test-only: clears the memoized credentials so each test starts from a clean cache. */
export function resetStoreCredentialsForTests(): void {
  cachedCredentials = undefined;
}
