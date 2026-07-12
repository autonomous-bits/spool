import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import process from 'node:process';
import open from 'open';
import type { TokenCache } from './token-cache.js';
import { createTokenCache } from './token-cache.js';

const DEFAULT_CALLBACK_HOST = '127.0.0.1';
const DEFAULT_CALLBACK_PATH = '/callback';
const DEFAULT_EXPIRY_SKEW_SECONDS = 60;
const DEFAULT_INTERACTIVE_TIMEOUT_MS = 5 * 60 * 1_000;
const FALLBACK_SESSION_TOKEN_TTL_SECONDS = 60;

const inFlightAuthentications = new Map<string, Promise<AuthenticatedStoreSession>>();

export interface AuthenticatedStoreSession {
  sessionToken: string;
  workspaceId: string;
}

export interface EnsureAuthenticatedOptions {
  storeUrl: string;
  workspaceId: string;
  tokenCache?: TokenCache;
  fetchImpl?: FetchImplementation;
  openImpl?: OpenImplementation;
  now?: () => number;
  writeStderr?: (message: string) => void;
  interactiveTimeoutMs?: number;
  expirySkewSeconds?: number;
  callbackHost?: '127.0.0.1' | 'localhost';
  callbackPath?: string;
  loopbackPort?: number;
  createCallbackServer?: CreateCallbackServer;
}

export interface CreateCallbackServerOptions {
  host: '127.0.0.1' | 'localhost';
  path: string;
  port: number;
  timeoutMs: number;
}

export interface CallbackServer {
  callbackUrl: string;
  waitForCode(): Promise<string>;
  close(): Promise<void>;
  isListening(): boolean;
}

export type CreateCallbackServer = (
  options: CreateCallbackServerOptions,
) => Promise<CallbackServer>;

export type FetchImplementation = (input: string | URL, init?: RequestInit) => Promise<Response>;

export type OpenImplementation = (target: string) => Promise<unknown>;

export class AuthenticationError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = 'AuthenticationError';
  }
}

interface RefreshResponse {
  sessionToken: string;
  refreshToken: string;
  expiresAt: number;
}

interface PairingExchangeResponse {
  sessionToken: string;
  refreshToken: string;
  expiresAt?: number;
}

interface ParsedTokenEnvelope {
  exp: number;
}

function defaultWriteStderr(message: string): void {
  process.stderr.write(`${message}\n`);
}

function formatError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

function buildInFlightKey(storeUrl: string, workspaceId: string): string {
  return `${storeUrl}::${workspaceId}`;
}

function nowSeconds(now: () => number): number {
  return Math.floor(now() / 1_000);
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0;
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function isPositiveInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0;
}

function isSessionTokenValid(expiresAt: number, now: () => number, skewSeconds: number): boolean {
  return expiresAt > nowSeconds(now) + skewSeconds;
}

function createStoreEndpoint(storeUrl: string, pathname: string): URL {
  return new URL(pathname, storeUrl);
}

async function safeReadResponseText(response: Response): Promise<string | undefined> {
  try {
    const text = await response.text();
    if (text.trim().length === 0) {
      return undefined;
    }
    return text.trim().slice(0, 200);
  } catch {
    return undefined;
  }
}

async function parseJsonResponse(response: Response, context: string): Promise<unknown> {
  try {
    return (await response.json());
  } catch (error) {
    throw new AuthenticationError(`${context} returned invalid JSON: ${formatError(error)}`, {
      cause: error,
    });
  }
}

function parseRefreshResponse(body: unknown): RefreshResponse {
  if (!isObjectRecord(body)) {
    throw new AuthenticationError('Refresh response body must be a JSON object');
  }

  const sessionToken = body.sessionToken;
  const refreshToken = body.refreshToken;
  const expiresAt = body.expiresAt;

  if (!isNonEmptyString(sessionToken)) {
    throw new AuthenticationError('Refresh response body is missing a non-empty sessionToken');
  }
  if (!isNonEmptyString(refreshToken)) {
    throw new AuthenticationError('Refresh response body is missing a non-empty refreshToken');
  }
  if (!isPositiveInteger(expiresAt)) {
    throw new AuthenticationError('Refresh response body is missing a positive-integer expiresAt');
  }

  return { sessionToken, refreshToken, expiresAt };
}

function parsePairingExchangeResponse(body: unknown): PairingExchangeResponse {
  if (!isObjectRecord(body)) {
    throw new AuthenticationError('Pairing exchange response body must be a JSON object');
  }

  const sessionToken = body.sessionToken;
  const refreshToken = body.refreshToken;
  const expiresAt = body.expiresAt;

  if (!isNonEmptyString(sessionToken)) {
    throw new AuthenticationError('Pairing exchange response body is missing a non-empty sessionToken');
  }
  if (!isNonEmptyString(refreshToken)) {
    throw new AuthenticationError('Pairing exchange response body is missing a non-empty refreshToken');
  }
  if (expiresAt !== undefined && !isPositiveInteger(expiresAt)) {
    throw new AuthenticationError('Pairing exchange response body has an invalid expiresAt value');
  }

  return expiresAt === undefined
    ? { sessionToken, refreshToken }
    : { sessionToken, refreshToken, expiresAt };
}

function parseTokenEnvelope(token: string): ParsedTokenEnvelope | undefined {
  const [encodedPayload] = token.split('.');
  if (encodedPayload === undefined || encodedPayload.length === 0) {
    return undefined;
  }

  try {
    const rawPayload = Buffer.from(encodedPayload, 'base64url').toString('utf8');
    const parsed: unknown = JSON.parse(rawPayload);
    if (!isObjectRecord(parsed)) {
      return undefined;
    }

    const exp = parsed.exp;
    if (!isPositiveInteger(exp)) {
      return undefined;
    }

    return { exp };
  } catch {
    return undefined;
  }
}

function deriveSessionTokenExpiresAt(response: PairingExchangeResponse, now: () => number): number {
  if (response.expiresAt !== undefined) {
    return response.expiresAt;
  }

  const tokenEnvelope = parseTokenEnvelope(response.sessionToken);
  if (tokenEnvelope !== undefined) {
    return tokenEnvelope.exp;
  }

  return nowSeconds(now) + FALLBACK_SESSION_TOKEN_TTL_SECONDS;
}

async function closeServer(server: Server): Promise<void> {
  if (!server.listening) {
    return;
  }

  await new Promise<void>((resolve, reject) => {
    server.close((error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });
}

function respondHtml(response: ServerResponse, statusCode: number, message: string): void {
  response.statusCode = statusCode;
  response.setHeader('content-type', 'text/html; charset=utf-8');
  response.end(`<!doctype html><html><body><p>${message}</p></body></html>`);
}

function matchesCallbackPath(request: IncomingMessage, expectedPath: string): boolean {
  const requestUrl = request.url;
  if (requestUrl === undefined) {
    return false;
  }

  return new URL(requestUrl, 'http://127.0.0.1').pathname === expectedPath;
}

export async function createLoopbackCallbackServer(
  options: CreateCallbackServerOptions,
): Promise<CallbackServer> {
  let settled = false;
  let timeout: NodeJS.Timeout | undefined;
  let resolveCode: ((code: string) => void) | undefined;
  let rejectCode: ((reason?: unknown) => void) | undefined;

  const codePromise = new Promise<string>((resolve, reject) => {
    resolveCode = resolve;
    rejectCode = reject;
  });

  const server = createServer((request, response) => {
    if (request.method !== 'GET' || !matchesCallbackPath(request, options.path)) {
      respondHtml(response, 404, 'Not found.');
      return;
    }

    const requestUrl = request.url;
    if (requestUrl === undefined) {
      respondHtml(response, 400, 'Missing callback URL.');
      return;
    }

    const parsedUrl = new URL(requestUrl, `http://${options.host}`);
    const code = parsedUrl.searchParams.get('code');
    if (!isNonEmptyString(code)) {
      respondHtml(response, 400, 'Missing pairing code.');
      return;
    }

    settled = true;
    if (timeout !== undefined) {
      clearTimeout(timeout);
      timeout = undefined;
    }

    respondHtml(response, 200, 'GitHub login complete. You can close this tab and return to your terminal.');
    resolveCode?.(code);
    void closeServer(server).catch((error: unknown) => {
      rejectCode?.(error);
    });
  });

  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(options.port, options.host, () => {
      server.off('error', reject);
      resolve();
    });
  });

  const address = server.address();
  if (address === null || typeof address === 'string') {
    await closeServer(server);
    throw new AuthenticationError('Loopback callback server did not expose a TCP address');
  }

  timeout = setTimeout(() => {
    if (settled) {
      return;
    }

    settled = true;
    rejectCode?.(
      new AuthenticationError(
        `Timed out waiting for the GitHub login callback after ${Math.round(options.timeoutMs / 1_000).toString()} seconds`,
      ),
    );
    void closeServer(server).catch(() => {
      // Best-effort close after timeout.
    });
  }, options.timeoutMs);

  return {
    callbackUrl: `http://${options.host}:${address.port.toString()}${options.path}`,
    waitForCode: async (): Promise<string> => codePromise,
    close: async (): Promise<void> => {
      if (timeout !== undefined) {
        clearTimeout(timeout);
        timeout = undefined;
      }
      await closeServer(server);
    },
    isListening(): boolean {
      return server.listening;
    },
  };
}

async function tryRefreshSession(
  options: Required<
    Pick<
      EnsureAuthenticatedOptions,
      'storeUrl' | 'workspaceId' | 'tokenCache' | 'fetchImpl' | 'now' | 'expirySkewSeconds'
    >
  >,
  refreshToken: string,
): Promise<AuthenticatedStoreSession | undefined> {
  let response: Response;
  try {
    response = await options.fetchImpl(createStoreEndpoint(options.storeUrl, '/auth/github/refresh'), {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ refreshToken }),
    });
  } catch {
    return undefined;
  }

  if (!response.ok) {
    return undefined;
  }

  let parsed: RefreshResponse;
  try {
    parsed = parseRefreshResponse(await parseJsonResponse(response, 'Refresh endpoint'));
  } catch (error) {
    if (error instanceof AuthenticationError) {
      return undefined;
    }
    throw error;
  }
  await options.tokenCache.save(options.storeUrl, options.workspaceId, {
    sessionToken: parsed.sessionToken,
    sessionTokenExpiresAt: parsed.expiresAt,
    refreshToken: parsed.refreshToken,
  });

  if (!isSessionTokenValid(parsed.expiresAt, options.now, options.expirySkewSeconds)) {
    return undefined;
  }

  return { sessionToken: parsed.sessionToken, workspaceId: options.workspaceId };
}

async function runInteractiveLogin(
  options: Required<
    Pick<
      EnsureAuthenticatedOptions,
      | 'storeUrl'
      | 'workspaceId'
      | 'tokenCache'
      | 'fetchImpl'
      | 'openImpl'
      | 'now'
      | 'writeStderr'
      | 'interactiveTimeoutMs'
      | 'callbackHost'
      | 'callbackPath'
      | 'loopbackPort'
      | 'createCallbackServer'
    >
  >,
): Promise<AuthenticatedStoreSession> {
  const callbackServer = await options.createCallbackServer({
    host: options.callbackHost,
    path: options.callbackPath,
    port: options.loopbackPort,
    timeoutMs: options.interactiveTimeoutMs,
  });

  try {
    const loginUrl = createStoreEndpoint(options.storeUrl, '/auth/github/login');
    loginUrl.searchParams.set('workspaceId', options.workspaceId);
    loginUrl.searchParams.set('cliRedirectUri', callbackServer.callbackUrl);

    try {
      await options.openImpl(loginUrl.toString());
    } catch (error) {
      throw new AuthenticationError(
        `Interactive GitHub login failed: could not open the browser automatically (${formatError(error)}). ` +
          `Open this URL manually: ${loginUrl.toString()}. Set SPOOL_SESSION_TOKEN to bypass interactive login in headless/CI contexts.`,
        { cause: error },
      );
    }

    options.writeStderr(
      'Spool MCP is opening your browser for GitHub login. If no browser appears, open this URL manually: ' +
        loginUrl.toString(),
    );

    const code = await callbackServer.waitForCode();
    const response = await options.fetchImpl(
      createStoreEndpoint(options.storeUrl, '/auth/github/pairing/exchange'),
      {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ code }),
      },
    );

    if (!response.ok) {
      const responseText = await safeReadResponseText(response);
      const responseSuffix = responseText === undefined ? '' : `: ${responseText}`;
      throw new AuthenticationError(
        `Interactive GitHub login failed: pairing-code exchange was rejected with HTTP ${response.status.toString()}${responseSuffix}. ` +
          'Set SPOOL_SESSION_TOKEN to bypass interactive login in headless/CI contexts.',
      );
    }

    const parsed = parsePairingExchangeResponse(await parseJsonResponse(response, 'Pairing exchange endpoint'));
    const expiresAt = deriveSessionTokenExpiresAt(parsed, options.now);

    await options.tokenCache.save(options.storeUrl, options.workspaceId, {
      sessionToken: parsed.sessionToken,
      sessionTokenExpiresAt: expiresAt,
      refreshToken: parsed.refreshToken,
    });

    return { sessionToken: parsed.sessionToken, workspaceId: options.workspaceId };
  } catch (error) {
    if (error instanceof AuthenticationError) {
      if (error.message.startsWith('Interactive GitHub login failed:')) {
        throw error;
      }
      throw new AuthenticationError(
        `Interactive GitHub login failed: ${error.message}. ` +
          'Set SPOOL_SESSION_TOKEN to bypass interactive login in headless/CI contexts.',
        { cause: error },
      );
    }

    throw new AuthenticationError(
      `Interactive GitHub login failed: ${formatError(error)}. ` +
        'Set SPOOL_SESSION_TOKEN to bypass interactive login in headless/CI contexts.',
      { cause: error },
    );
  } finally {
    await callbackServer.close().catch(() => {
      // Best-effort cleanup only.
    });
  }
}

async function ensureAuthenticatedOnce(
  options: Required<
    Pick<
      EnsureAuthenticatedOptions,
      | 'storeUrl'
      | 'workspaceId'
      | 'tokenCache'
      | 'fetchImpl'
      | 'openImpl'
      | 'now'
      | 'writeStderr'
      | 'interactiveTimeoutMs'
      | 'expirySkewSeconds'
      | 'callbackHost'
      | 'callbackPath'
      | 'loopbackPort'
      | 'createCallbackServer'
    >
  >,
): Promise<AuthenticatedStoreSession> {
  const cachedCredentials = await options.tokenCache.load(options.storeUrl, options.workspaceId);
  if (
    cachedCredentials !== undefined &&
    isSessionTokenValid(cachedCredentials.sessionTokenExpiresAt, options.now, options.expirySkewSeconds)
  ) {
    return { sessionToken: cachedCredentials.sessionToken, workspaceId: options.workspaceId };
  }

  if (cachedCredentials !== undefined) {
    const refreshed = await tryRefreshSession(options, cachedCredentials.refreshToken);
    if (refreshed !== undefined) {
      return refreshed;
    }
  }

  return runInteractiveLogin(options);
}

export function resetEnsureAuthenticatedForTests(): void {
  inFlightAuthentications.clear();
}

export async function ensureAuthenticated(
  options: EnsureAuthenticatedOptions,
): Promise<AuthenticatedStoreSession> {
  const resolvedOptions = {
    storeUrl: options.storeUrl,
    workspaceId: options.workspaceId,
    tokenCache: options.tokenCache ?? createTokenCache(),
    fetchImpl: options.fetchImpl ?? fetch,
    openImpl: options.openImpl ?? open,
    now: options.now ?? Date.now,
    writeStderr: options.writeStderr ?? defaultWriteStderr,
    interactiveTimeoutMs: options.interactiveTimeoutMs ?? DEFAULT_INTERACTIVE_TIMEOUT_MS,
    expirySkewSeconds: options.expirySkewSeconds ?? DEFAULT_EXPIRY_SKEW_SECONDS,
    callbackHost: options.callbackHost ?? DEFAULT_CALLBACK_HOST,
    callbackPath: options.callbackPath ?? DEFAULT_CALLBACK_PATH,
    loopbackPort: options.loopbackPort ?? 0,
    createCallbackServer: options.createCallbackServer ?? createLoopbackCallbackServer,
  } satisfies Required<EnsureAuthenticatedOptions>;

  const inFlightKey = buildInFlightKey(resolvedOptions.storeUrl, resolvedOptions.workspaceId);
  const existingPromise = inFlightAuthentications.get(inFlightKey);
  if (existingPromise !== undefined) {
    return existingPromise;
  }

  const authenticationPromise = ensureAuthenticatedOnce(resolvedOptions).finally(() => {
    const current = inFlightAuthentications.get(inFlightKey);
    if (current === authenticationPromise) {
      inFlightAuthentications.delete(inFlightKey);
    }
  });

  inFlightAuthentications.set(inFlightKey, authenticationPromise);
  return authenticationPromise;
}
