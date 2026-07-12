import { afterEach, describe, expect, it, vi } from 'vitest';
import type { AuthenticatedStoreSession, EnsureAuthenticatedOptions } from './auth/login-flow.js';
import { createInMemoryTokenCache } from './auth/token-cache.js';
import {
  loadStoreCredentials,
  resetStoreCredentialsForTests,
  storeFetch,
  StoreClientConfigError,
} from './store-client.js';

function envWith(overrides: Partial<Record<string, string>>): NodeJS.ProcessEnv {
  return { ...overrides };
}

type EnsureAuthenticatedImplementation = (
  options: EnsureAuthenticatedOptions,
) => Promise<AuthenticatedStoreSession>;

type FetchImplementation = (input: string | URL, init?: RequestInit) => Promise<Response>;

describe('loadStoreCredentials', () => {
  afterEach(() => {
    resetStoreCredentialsForTests();
  });

  it('reads SPOOL_WORKSPACE_ID and an optional SPOOL_SESSION_TOKEN override from the given env', () => {
    const env = envWith({ SPOOL_SESSION_TOKEN: 'token-1', SPOOL_WORKSPACE_ID: 'workspace-1' });

    expect(loadStoreCredentials(env)).toEqual({
      sessionTokenOverride: 'token-1',
      workspaceId: 'workspace-1',
    });
  });

  it('allows interactive auth when SPOOL_SESSION_TOKEN is unset', () => {
    const env = envWith({ SPOOL_WORKSPACE_ID: 'workspace-1' });

    expect(loadStoreCredentials(env)).toEqual({ workspaceId: 'workspace-1' });
  });

  it('fails fast, naming the variable, when SPOOL_WORKSPACE_ID is missing', () => {
    const env = envWith({ SPOOL_SESSION_TOKEN: 'token-1' });

    expect(() => loadStoreCredentials(env)).toThrow(StoreClientConfigError);
    expect(() => loadStoreCredentials(env)).toThrow(/SPOOL_WORKSPACE_ID/);
  });

  it('fails fast when SPOOL_SESSION_TOKEN is an empty/blank string', () => {
    const env = envWith({ SPOOL_SESSION_TOKEN: '   ', SPOOL_WORKSPACE_ID: 'workspace-1' });

    expect(() => loadStoreCredentials(env)).toThrow(/SPOOL_SESSION_TOKEN/);
  });

  it('never includes the token value in the thrown error message', () => {
    const env = envWith({
      SPOOL_SESSION_TOKEN: 'super-secret-token-value',
      SPOOL_WORKSPACE_ID: '',
    });

    try {
      loadStoreCredentials(env);
      expect.fail('expected loadStoreCredentials to throw');
    } catch (error) {
      expect(error).toBeInstanceOf(StoreClientConfigError);
      expect((error as Error).message).not.toContain('super-secret-token-value');
    }
  });

  it('memoizes the first successful read, ignoring a subsequently changed env', () => {
    const env = envWith({ SPOOL_SESSION_TOKEN: 'token-1', SPOOL_WORKSPACE_ID: 'workspace-1' });
    expect(loadStoreCredentials(env)).toEqual({
      sessionTokenOverride: 'token-1',
      workspaceId: 'workspace-1',
    });

    const changedEnv = envWith({
      SPOOL_SESSION_TOKEN: 'token-2',
      SPOOL_WORKSPACE_ID: 'workspace-2',
    });
    expect(loadStoreCredentials(changedEnv)).toEqual({
      sessionTokenOverride: 'token-1',
      workspaceId: 'workspace-1',
    });
  });
});

describe('storeFetch', () => {
  afterEach(() => {
    resetStoreCredentialsForTests();
  });

  it('uses the SPOOL_SESSION_TOKEN override as-is and skips ensureAuthenticated', async () => {
    const fetchMock = vi
      .fn<FetchImplementation>()
      .mockResolvedValue(new Response('ok', { status: 200 }));
    const ensureAuthenticatedMock = vi.fn<EnsureAuthenticatedImplementation>();

    const response = await storeFetch(
      'http://store.test',
      '/chunks',
      {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ label: 'IDEA-1' }),
      },
      {
        env: envWith({
          SPOOL_SESSION_TOKEN: 'override-token',
          SPOOL_WORKSPACE_ID: 'workspace-1',
        }),
        fetchImpl: fetchMock,
        ensureAuthenticatedImpl: ensureAuthenticatedMock,
      },
    );

    expect(response.status).toBe(200);
    expect(ensureAuthenticatedMock).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledWith('http://store.test/chunks', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: 'Bearer override-token',
        'x-workspace-id': 'workspace-1',
      },
      body: JSON.stringify({ label: 'IDEA-1' }),
    });
  });

  it('does not retry or invalidate cached credentials when an override token receives a 401', async () => {
    const tokenCache = createInMemoryTokenCache();
    await tokenCache.save('http://store.test', 'workspace-1', {
      sessionToken: 'cached-session-token',
      sessionTokenExpiresAt: 1_700_000_000,
      refreshToken: 'refresh-token-1',
    });
    const fetchMock = vi
      .fn<FetchImplementation>()
      .mockResolvedValue(new Response('unauthorized', { status: 401 }));
    const ensureAuthenticatedMock = vi.fn<EnsureAuthenticatedImplementation>();

    const response = await storeFetch(
      'http://store.test',
      '/chunks',
      { method: 'GET' },
      {
        env: envWith({
          SPOOL_SESSION_TOKEN: 'override-token',
          SPOOL_WORKSPACE_ID: 'workspace-1',
        }),
        tokenCache,
        fetchImpl: fetchMock,
        ensureAuthenticatedImpl: ensureAuthenticatedMock,
      },
    );

    expect(response.status).toBe(401);
    expect(ensureAuthenticatedMock).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await expect(tokenCache.load('http://store.test', 'workspace-1')).resolves.toEqual({
      sessionToken: 'cached-session-token',
      sessionTokenExpiresAt: 1_700_000_000,
      refreshToken: 'refresh-token-1',
    });
  });

  it('calls ensureAuthenticated when no override token is configured', async () => {
    const fetchMock = vi
      .fn<FetchImplementation>()
      .mockResolvedValue(new Response('ok', { status: 200 }));
    const ensureAuthenticatedMock = vi
      .fn<EnsureAuthenticatedImplementation>()
      .mockResolvedValue({ sessionToken: 'session-1', workspaceId: 'workspace-1' });
    const tokenCache = createInMemoryTokenCache();

    await storeFetch(
      'http://store.test',
      '/chunks',
      { method: 'GET' },
      {
        env: envWith({ SPOOL_WORKSPACE_ID: 'workspace-1' }),
        fetchImpl: fetchMock,
        ensureAuthenticatedImpl: ensureAuthenticatedMock,
        tokenCache,
      },
    );

    expect(ensureAuthenticatedMock).toHaveBeenCalledTimes(1);
    expect(ensureAuthenticatedMock).toHaveBeenCalledWith({
      storeUrl: 'http://store.test',
      workspaceId: 'workspace-1',
      tokenCache,
      fetchImpl: fetchMock,
    });
    expect(fetchMock).toHaveBeenCalledWith('http://store.test/chunks', {
      method: 'GET',
      headers: {
        authorization: 'Bearer session-1',
        'x-workspace-id': 'workspace-1',
      },
    });
  });

  it('invalidates only the cached session token after a 401, then re-authenticates and retries once', async () => {
    const env = envWith({ SPOOL_WORKSPACE_ID: 'workspace-1' });
    const tokenCache = createInMemoryTokenCache();
    await tokenCache.save('http://store.test', 'workspace-1', {
      sessionToken: 'cached-session-token',
      sessionTokenExpiresAt: 1_700_000_000,
      refreshToken: 'refresh-token-1',
    });

    const ensureAuthenticatedMock = vi
      .fn<EnsureAuthenticatedImplementation>()
      .mockImplementationOnce(() => Promise.resolve({
        sessionToken: 'session-token-1',
        workspaceId: 'workspace-1',
      }))
      .mockImplementationOnce(async (options) => {
        const cached = await options.tokenCache?.load('http://store.test', 'workspace-1');
        expect(cached?.sessionTokenExpiresAt).toBe(1);
        expect(cached?.refreshToken).toBe('refresh-token-1');
        return { sessionToken: 'session-token-2', workspaceId: 'workspace-1' };
      });
    const fetchMock = vi
      .fn<FetchImplementation>()
      .mockResolvedValueOnce(new Response('unauthorized', { status: 401 }))
      .mockResolvedValueOnce(new Response('ok', { status: 200 }));

    const response = await storeFetch(
      'http://store.test',
      '/chunks',
      { method: 'GET' },
      {
        env,
        tokenCache,
        fetchImpl: fetchMock,
        ensureAuthenticatedImpl: ensureAuthenticatedMock,
      },
    );

    expect(response.status).toBe(200);
    expect(ensureAuthenticatedMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenNthCalledWith(1, 'http://store.test/chunks', {
      method: 'GET',
      headers: {
        authorization: 'Bearer session-token-1',
        'x-workspace-id': 'workspace-1',
      },
    });
    expect(fetchMock).toHaveBeenNthCalledWith(2, 'http://store.test/chunks', {
      method: 'GET',
      headers: {
        authorization: 'Bearer session-token-2',
        'x-workspace-id': 'workspace-1',
      },
    });
  });

  it('retries at most once after a 401 and then returns the second 401 response', async () => {
    const ensureAuthenticatedMock = vi
      .fn<EnsureAuthenticatedImplementation>()
      .mockResolvedValueOnce({ sessionToken: 'session-token-1', workspaceId: 'workspace-1' })
      .mockResolvedValueOnce({ sessionToken: 'session-token-2', workspaceId: 'workspace-1' });
    const fetchMock = vi
      .fn<FetchImplementation>()
      .mockResolvedValueOnce(new Response('unauthorized', { status: 401 }))
      .mockResolvedValueOnce(new Response('still unauthorized', { status: 401 }));

    const response = await storeFetch(
      'http://store.test',
      '/chunks',
      { method: 'GET' },
      {
        env: envWith({ SPOOL_WORKSPACE_ID: 'workspace-1' }),
        fetchImpl: fetchMock,
        ensureAuthenticatedImpl: ensureAuthenticatedMock,
        tokenCache: createInMemoryTokenCache(),
      },
    );

    expect(response.status).toBe(401);
    expect(ensureAuthenticatedMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
