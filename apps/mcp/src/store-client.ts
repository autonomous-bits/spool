import {
  ensureAuthenticated,
  type AuthenticatedStoreSession,
  type EnsureAuthenticatedOptions,
  type FetchImplementation,
} from './auth/login-flow.js';
import { createTokenCache, type TokenCache } from './auth/token-cache.js';

export interface StoreClientConfig {
  sessionTokenOverride?: string;
  workspaceId: string;
}

export interface StoreFetchOptions {
  env?: NodeJS.ProcessEnv;
  tokenCache?: TokenCache;
  fetchImpl?: FetchImplementation;
  ensureAuthenticatedImpl?: (
    options: EnsureAuthenticatedOptions,
  ) => Promise<AuthenticatedStoreSession>;
}

/** Raised when required store-client env vars are missing or malformed. */
export class StoreClientConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'StoreClientConfigError';
  }
}

const INVALIDATED_SESSION_TOKEN_EXPIRES_AT = 1;
const INVALIDATED_SESSION_TOKEN_VALUE = 'invalidated-session-token';

let cachedConfig: StoreClientConfig | undefined;

function requireEnvVar(env: NodeJS.ProcessEnv, name: string): string {
  const value = env[name];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new StoreClientConfigError(
      `${name} environment variable is missing or empty; the MCP process cannot start without it`,
    );
  }
  return value;
}

function readOptionalSessionTokenOverride(env: NodeJS.ProcessEnv): string | undefined {
  const value = env.SPOOL_SESSION_TOKEN;
  if (value === undefined) {
    return undefined;
  }
  if (value.trim().length === 0) {
    throw new StoreClientConfigError(
      'SPOOL_SESSION_TOKEN environment variable is empty; unset it to use interactive login or set it to a non-empty override token',
    );
  }
  return value;
}

export function loadStoreCredentials(env: NodeJS.ProcessEnv = process.env): StoreClientConfig {
  if (cachedConfig !== undefined) {
    return cachedConfig;
  }

  const sessionTokenOverride = readOptionalSessionTokenOverride(env);
  cachedConfig = {
    workspaceId: requireEnvVar(env, 'SPOOL_WORKSPACE_ID'),
    ...(sessionTokenOverride === undefined ? {} : { sessionTokenOverride }),
  } satisfies StoreClientConfig;
  return cachedConfig;
}

function buildStoreUrl(storeUrl: string, path: string | URL): string {
  return path instanceof URL ? path.toString() : new URL(path, storeUrl).toString();
}

function buildHeaders(
  headersInit: RequestInit['headers'],
  sessionToken: string,
  workspaceId: string,
): Record<string, string> {
  const headers = new Headers(headersInit);
  headers.set('authorization', 'Bearer ' + sessionToken);
  headers.set('x-workspace-id', workspaceId);
  return Object.fromEntries(headers.entries());
}

async function invalidateCachedSessionToken(
  tokenCache: TokenCache,
  storeUrl: string,
  workspaceId: string,
): Promise<void> {
  const cachedCredentials = await tokenCache.load(storeUrl, workspaceId);
  if (cachedCredentials === undefined) {
    return;
  }

  await tokenCache.save(storeUrl, workspaceId, {
    sessionToken: INVALIDATED_SESSION_TOKEN_VALUE,
    sessionTokenExpiresAt: INVALIDATED_SESSION_TOKEN_EXPIRES_AT,
    refreshToken: cachedCredentials.refreshToken,
  });
}

async function resolveSessionToken(
  storeUrl: string,
  config: StoreClientConfig,
  options: Required<StoreFetchOptions>,
): Promise<string> {
  if (config.sessionTokenOverride !== undefined) {
    return config.sessionTokenOverride;
  }

  const authenticated = await options.ensureAuthenticatedImpl({
    storeUrl,
    workspaceId: config.workspaceId,
    tokenCache: options.tokenCache,
    fetchImpl: options.fetchImpl,
  });
  return authenticated.sessionToken;
}

export async function storeFetch(
  storeUrl: string,
  path: string | URL,
  init: RequestInit = {},
  options: StoreFetchOptions = {},
): Promise<Response> {
  const resolvedOptions = {
    env: options.env ?? process.env,
    tokenCache: options.tokenCache ?? createTokenCache(),
    fetchImpl: options.fetchImpl ?? fetch,
    ensureAuthenticatedImpl: options.ensureAuthenticatedImpl ?? ensureAuthenticated,
  } satisfies Required<StoreFetchOptions>;

  const config = loadStoreCredentials(resolvedOptions.env);
  const requestUrl = buildStoreUrl(storeUrl, path);
  const firstSessionToken = await resolveSessionToken(storeUrl, config, resolvedOptions);
  const firstResponse = await resolvedOptions.fetchImpl(requestUrl, {
    ...init,
    headers: buildHeaders(init.headers, firstSessionToken, config.workspaceId),
  });

  if (firstResponse.status !== 401 || config.sessionTokenOverride !== undefined) {
    return firstResponse;
  }

  await invalidateCachedSessionToken(resolvedOptions.tokenCache, storeUrl, config.workspaceId);
  const retriedSessionToken = await resolveSessionToken(storeUrl, config, resolvedOptions);
  return resolvedOptions.fetchImpl(requestUrl, {
    ...init,
    headers: buildHeaders(init.headers, retriedSessionToken, config.workspaceId),
  });
}

export function resetStoreCredentialsForTests(): void {
  cachedConfig = undefined;
}
