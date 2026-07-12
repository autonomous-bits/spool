import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  AuthenticationError,
  createLoopbackCallbackServer,
  ensureAuthenticated,
  resetEnsureAuthenticatedForTests,
  type CallbackServer,
  type CreateCallbackServer,
  type FetchImplementation,
  type OpenImplementation,
} from './login-flow.js';
import { createInMemoryTokenCache, type CachedCredentials } from './token-cache.js';

function buildSessionToken(expiresAt: number): string {
  const payload = {
    claims: {
      stakeholderId: 'stakeholder-1',
      discipline: 'engineering',
      authTime: expiresAt - 900,
      workspaceId: 'workspace-1',
    },
    iat: expiresAt - 900,
    exp: expiresAt,
  };

  return `${Buffer.from(JSON.stringify(payload), 'utf8').toString('base64url')}.signature`;
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function createCachedCredentials(expiresAt: number, seed: string): CachedCredentials {
  return {
    sessionToken: buildSessionToken(expiresAt),
    sessionTokenExpiresAt: expiresAt,
    refreshToken: `refresh-${seed}`,
  };
}

function getBodyJson(init: RequestInit | undefined): unknown {
  const body = init?.body;
  if (typeof body !== 'string') {
    throw new Error('Expected RequestInit.body to be a JSON string');
  }
  return JSON.parse(body) as unknown;
}

async function triggerLoopbackCallbackFromLoginUrl(loginUrl: string, code: string): Promise<void> {
  const parsedLoginUrl = new URL(loginUrl);
  const callbackUrl = parsedLoginUrl.searchParams.get('cliRedirectUri');
  if (callbackUrl === null) {
    throw new Error('Expected cliRedirectUri query parameter in login URL');
  }

  const callbackResponse = await fetch(`${callbackUrl}?code=${encodeURIComponent(code)}`, {
    signal: AbortSignal.timeout(5_000),
  });
  expect(callbackResponse.status).toBe(200);
}

describe('ensureAuthenticated', () => {
  const nowMs = Date.parse('2026-07-11T22:30:00.000Z');
  const now = (): number => nowMs;
  const nowSeconds = Math.floor(nowMs / 1_000);

  afterEach(() => {
    resetEnsureAuthenticatedForTests();
  });

  it('returns cached credentials immediately when the session token is still valid', async () => {
    const tokenCache = createInMemoryTokenCache();
    const cached = createCachedCredentials(nowSeconds + 600, 'cached');
    await tokenCache.save('https://store.example', 'workspace-1', cached);

    const fetchImpl = vi.fn<FetchImplementation>();
    const openImpl = vi.fn<OpenImplementation>();

    await expect(
      ensureAuthenticated({
        storeUrl: 'https://store.example',
        workspaceId: 'workspace-1',
        tokenCache,
        fetchImpl,
        openImpl,
        now,
      }),
    ).resolves.toEqual({
      sessionToken: cached.sessionToken,
      workspaceId: 'workspace-1',
    });

    expect(fetchImpl).not.toHaveBeenCalled();
    expect(openImpl).not.toHaveBeenCalled();
  });

  it('refreshes an expired cached session token and updates the cache', async () => {
    const tokenCache = createInMemoryTokenCache();
    await tokenCache.save(
      'https://store.example',
      'workspace-1',
      createCachedCredentials(nowSeconds - 10, 'expired'),
    );

    const refreshedToken = buildSessionToken(nowSeconds + 900);
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      expect(new URL(String(input)).pathname).toBe('/auth/github/refresh');
      expect(getBodyJson(init)).toEqual({ refreshToken: 'refresh-expired' });
      return jsonResponse({
        sessionToken: refreshedToken,
        refreshToken: 'refresh-rotated',
        expiresAt: nowSeconds + 900,
      });
    });
    const openImpl = vi.fn<OpenImplementation>();

    await expect(
      ensureAuthenticated({
        storeUrl: 'https://store.example',
        workspaceId: 'workspace-1',
        tokenCache,
        fetchImpl,
        openImpl,
        now,
      }),
    ).resolves.toEqual({
      sessionToken: refreshedToken,
      workspaceId: 'workspace-1',
    });

    await expect(tokenCache.load('https://store.example', 'workspace-1')).resolves.toEqual({
      sessionToken: refreshedToken,
      sessionTokenExpiresAt: nowSeconds + 900,
      refreshToken: 'refresh-rotated',
    });
    expect(openImpl).not.toHaveBeenCalled();
  });

  it('falls through from a failed refresh to the interactive login flow', async () => {
    const tokenCache = createInMemoryTokenCache();
    await tokenCache.save(
      'https://store.example',
      'workspace-1',
      createCachedCredentials(nowSeconds - 10, 'expired'),
    );

    const pairedToken = buildSessionToken(nowSeconds + 1_200);
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      if (url.pathname === '/auth/github/refresh') {
        expect(getBodyJson(init)).toEqual({ refreshToken: 'refresh-expired' });
        return jsonResponse({ message: 'unauthorized' }, 401);
      }
      if (url.pathname === '/auth/github/pairing/exchange') {
        expect(getBodyJson(init)).toEqual({ code: 'pairing-code-1' });
        return jsonResponse({
          sessionToken: pairedToken,
          refreshToken: 'refresh-interactive',
        });
      }
      throw new Error(`Unexpected fetch URL: ${url.pathname}`);
    });
    const openImpl = vi.fn<OpenImplementation>().mockImplementation(async (loginUrl) => {
      await triggerLoopbackCallbackFromLoginUrl(loginUrl, 'pairing-code-1');
    });

    await expect(
      ensureAuthenticated({
        storeUrl: 'https://store.example',
        workspaceId: 'workspace-1',
        tokenCache,
        fetchImpl,
        openImpl,
        now,
        writeStderr: vi.fn<(message: string) => void>(),
      }),
    ).resolves.toEqual({
      sessionToken: pairedToken,
      workspaceId: 'workspace-1',
    });

    expect(openImpl).toHaveBeenCalledTimes(1);
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it('goes straight to interactive login when no cached credentials exist', async () => {
    const tokenCache = createInMemoryTokenCache();
    const pairedToken = buildSessionToken(nowSeconds + 1_500);
    const stderrMessages: string[] = [];
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      expect(url.pathname).toBe('/auth/github/pairing/exchange');
      expect(getBodyJson(init)).toEqual({ code: 'pairing-code-2' });
      return jsonResponse({
        sessionToken: pairedToken,
        refreshToken: 'refresh-interactive',
      });
    });
    const openImpl = vi.fn<OpenImplementation>().mockImplementation(async (loginUrl) => {
      const parsedLoginUrl = new URL(loginUrl);
      expect(parsedLoginUrl.origin).toBe('https://store.example');
      expect(parsedLoginUrl.pathname).toBe('/auth/github/login');
      expect(parsedLoginUrl.searchParams.get('workspaceId')).toBe('workspace-1');

      const callbackUrl = parsedLoginUrl.searchParams.get('cliRedirectUri');
      expect(callbackUrl).not.toBeNull();
      const parsedCallbackUrl = new URL(callbackUrl ?? '');
      expect(parsedCallbackUrl.hostname).toBe('127.0.0.1');
      expect(parsedCallbackUrl.pathname).toBe('/callback');

      await triggerLoopbackCallbackFromLoginUrl(loginUrl, 'pairing-code-2');
    });

    await expect(
      ensureAuthenticated({
        storeUrl: 'https://store.example',
        workspaceId: 'workspace-1',
        tokenCache,
        fetchImpl,
        openImpl,
        now,
        writeStderr: (message: string): void => {
          stderrMessages.push(message);
        },
      }),
    ).resolves.toEqual({
      sessionToken: pairedToken,
      workspaceId: 'workspace-1',
    });

    expect(openImpl).toHaveBeenCalledTimes(1);
    expect(fetchImpl).toHaveBeenCalledTimes(1);
    expect(stderrMessages[0]).toContain('Spool MCP is opening your browser for GitHub login');
    await expect(tokenCache.load('https://store.example', 'workspace-1')).resolves.toEqual({
      sessionToken: pairedToken,
      sessionTokenExpiresAt: nowSeconds + 1_500,
      refreshToken: 'refresh-interactive',
    });
  });

  it('times out interactive login with a clear error and closes the loopback server', async () => {
    const tokenCache = createInMemoryTokenCache();
    let callbackUrl: string | undefined;
    const openImpl = vi.fn<OpenImplementation>().mockImplementation(async (loginUrl) => {
      callbackUrl = new URL(loginUrl).searchParams.get('cliRedirectUri') ?? undefined;
    });

    const authenticationPromise = ensureAuthenticated({
      storeUrl: 'https://store.example',
      workspaceId: 'workspace-1',
      tokenCache,
      fetchImpl: vi.fn<FetchImplementation>(),
      openImpl,
      now,
      interactiveTimeoutMs: 50,
      writeStderr: vi.fn<(message: string) => void>(),
    });

    await expect(authenticationPromise).rejects.toBeInstanceOf(AuthenticationError);
    await expect(authenticationPromise).rejects.toThrow(
      'Interactive GitHub login failed: Timed out waiting for the GitHub login callback',
    );

    expect(callbackUrl).toBeDefined();
    await expect(
      fetch(callbackUrl ?? 'http://127.0.0.1:9/callback', {
        signal: AbortSignal.timeout(500),
      }),
    ).rejects.toThrow();
  });

  it('shares a single in-flight interactive login across concurrent callers', async () => {
    const tokenCache = createInMemoryTokenCache();
    const pairedToken = buildSessionToken(nowSeconds + 1_800);
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      expect(url.pathname).toBe('/auth/github/pairing/exchange');
      expect(getBodyJson(init)).toEqual({ code: 'pairing-code-3' });
      return jsonResponse({
        sessionToken: pairedToken,
        refreshToken: 'refresh-interactive',
      });
    });
    const openImpl = vi.fn<OpenImplementation>().mockImplementation(async (loginUrl) => {
      await triggerLoopbackCallbackFromLoginUrl(loginUrl, 'pairing-code-3');
    });

    const firstCall = ensureAuthenticated({
      storeUrl: 'https://store.example',
      workspaceId: 'workspace-1',
      tokenCache,
      fetchImpl,
      openImpl,
      now,
      writeStderr: vi.fn<(message: string) => void>(),
    });
    const secondCall = ensureAuthenticated({
      storeUrl: 'https://store.example',
      workspaceId: 'workspace-1',
      tokenCache,
      fetchImpl,
      openImpl,
      now,
      writeStderr: vi.fn<(message: string) => void>(),
    });

    await expect(Promise.all([firstCall, secondCall])).resolves.toEqual([
      { sessionToken: pairedToken, workspaceId: 'workspace-1' },
      { sessionToken: pairedToken, workspaceId: 'workspace-1' },
    ]);

    expect(openImpl).toHaveBeenCalledTimes(1);
    expect(fetchImpl).toHaveBeenCalledTimes(1);
  });

  it('shares a single in-flight refresh across concurrent callers for the same store and workspace', async () => {
    const tokenCache = createInMemoryTokenCache();
    await tokenCache.save(
      'https://store.example',
      'workspace-1',
      createCachedCredentials(nowSeconds - 10, 'expired-shared-refresh'),
    );

    let releaseRefresh: (() => void) | undefined;
    const refreshStarted = new Promise<void>((resolve) => {
      releaseRefresh = resolve;
    });
    const refreshedToken = buildSessionToken(nowSeconds + 1_800);
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      expect(url.pathname).toBe('/auth/github/refresh');
      expect(getBodyJson(init)).toEqual({ refreshToken: 'refresh-expired-shared-refresh' });
      await refreshStarted;
      return jsonResponse({
        sessionToken: refreshedToken,
        refreshToken: 'refresh-rotated',
        expiresAt: nowSeconds + 1_800,
      });
    });
    const openImpl = vi.fn<OpenImplementation>();

    const firstCall = ensureAuthenticated({
      storeUrl: 'https://store.example',
      workspaceId: 'workspace-1',
      tokenCache,
      fetchImpl,
      openImpl,
      now,
    });
    const secondCall = ensureAuthenticated({
      storeUrl: 'https://store.example',
      workspaceId: 'workspace-1',
      tokenCache,
      fetchImpl,
      openImpl,
      now,
    });
    releaseRefresh?.();

    await expect(Promise.all([firstCall, secondCall])).resolves.toEqual([
      { sessionToken: refreshedToken, workspaceId: 'workspace-1' },
      { sessionToken: refreshedToken, workspaceId: 'workspace-1' },
    ]);

    expect(fetchImpl).toHaveBeenCalledTimes(1);
    expect(openImpl).not.toHaveBeenCalled();
  });

  it('runs independent interactive authentications for different store and workspace keys', async () => {
    const tokenCache = createInMemoryTokenCache();
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      expect(url.pathname).toBe('/auth/github/pairing/exchange');
      const body = getBodyJson(init);
      if (
        !(
          typeof body === 'object' &&
          body !== null &&
          'code' in body &&
          typeof body.code === 'string'
        )
      ) {
        throw new Error('Expected pairing exchange request body with a code');
      }

      return jsonResponse({
        sessionToken: buildSessionToken(nowSeconds + (body.code === 'code-1' ? 1_000 : 2_000)),
        refreshToken: `refresh-${body.code}`,
      });
    });
    const openImpl = vi.fn<OpenImplementation>().mockImplementation(async (loginUrl) => {
      const workspaceId = new URL(loginUrl).searchParams.get('workspaceId');
      if (workspaceId === 'workspace-1') {
        await triggerLoopbackCallbackFromLoginUrl(loginUrl, 'code-1');
        return;
      }
      if (workspaceId === 'workspace-2') {
        await triggerLoopbackCallbackFromLoginUrl(loginUrl, 'code-2');
        return;
      }
      throw new Error(`Unexpected workspaceId in login URL: ${workspaceId ?? 'missing'}`);
    });

    await expect(
      Promise.all([
        ensureAuthenticated({
          storeUrl: 'https://store-a.example',
          workspaceId: 'workspace-1',
          tokenCache,
          fetchImpl,
          openImpl,
          now,
          writeStderr: vi.fn<(message: string) => void>(),
        }),
        ensureAuthenticated({
          storeUrl: 'https://store-b.example',
          workspaceId: 'workspace-2',
          tokenCache,
          fetchImpl,
          openImpl,
          now,
          writeStderr: vi.fn<(message: string) => void>(),
        }),
      ]),
    ).resolves.toEqual([
      { sessionToken: buildSessionToken(nowSeconds + 1_000), workspaceId: 'workspace-1' },
      { sessionToken: buildSessionToken(nowSeconds + 2_000), workspaceId: 'workspace-2' },
    ]);

    expect(openImpl).toHaveBeenCalledTimes(2);
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it('falls through gracefully when the refresh response body is malformed', async () => {
    const tokenCache = createInMemoryTokenCache();
    await tokenCache.save(
      'https://store.example',
      'workspace-1',
      createCachedCredentials(nowSeconds - 10, 'expired'),
    );

    const pairedToken = buildSessionToken(nowSeconds + 1_000);
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      if (url.pathname === '/auth/github/refresh') {
        expect(getBodyJson(init)).toEqual({ refreshToken: 'refresh-expired' });
        return jsonResponse({ sessionToken: 'missing-expires' });
      }
      if (url.pathname === '/auth/github/pairing/exchange') {
        expect(getBodyJson(init)).toEqual({ code: 'pairing-code-4' });
        return jsonResponse({
          sessionToken: pairedToken,
          refreshToken: 'refresh-interactive',
        });
      }
      throw new Error(`Unexpected fetch URL: ${url.pathname}`);
    });
    const openImpl = vi.fn<OpenImplementation>().mockImplementation(async (loginUrl) => {
      await triggerLoopbackCallbackFromLoginUrl(loginUrl, 'pairing-code-4');
    });

    await expect(
      ensureAuthenticated({
        storeUrl: 'https://store.example',
        workspaceId: 'workspace-1',
        tokenCache,
        fetchImpl,
        openImpl,
        now,
        writeStderr: vi.fn<(message: string) => void>(),
      }),
    ).resolves.toEqual({
      sessionToken: pairedToken,
      workspaceId: 'workspace-1',
    });
  });

  it('throws a clear error when the pairing exchange response body is malformed', async () => {
    const tokenCache = createInMemoryTokenCache();
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      expect(url.pathname).toBe('/auth/github/pairing/exchange');
      expect(getBodyJson(init)).toEqual({ code: 'pairing-code-5' });
      return jsonResponse({ sessionToken: 'missing-refresh-token' });
    });
    const openImpl = vi.fn<OpenImplementation>().mockImplementation(async (loginUrl) => {
      await triggerLoopbackCallbackFromLoginUrl(loginUrl, 'pairing-code-5');
    });

    await expect(
      ensureAuthenticated({
        storeUrl: 'https://store.example',
        workspaceId: 'workspace-1',
        tokenCache,
        fetchImpl,
        openImpl,
        now,
        writeStderr: vi.fn<(message: string) => void>(),
      }),
    ).rejects.toThrow(
      'Interactive GitHub login failed: Pairing exchange response body is missing a non-empty refreshToken.',
    );
  });

  it('uses an explicit pairing exchange expiresAt value instead of decoding the session token', async () => {
    const tokenCache = createInMemoryTokenCache();
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      expect(url.pathname).toBe('/auth/github/pairing/exchange');
      expect(getBodyJson(init)).toEqual({ code: 'pairing-code-explicit-expiry' });
      return jsonResponse({
        sessionToken: buildSessionToken(nowSeconds + 300),
        refreshToken: 'refresh-interactive',
        expiresAt: nowSeconds + 1_234,
      });
    });
    const openImpl = vi.fn<OpenImplementation>().mockImplementation(async (loginUrl) => {
      await triggerLoopbackCallbackFromLoginUrl(loginUrl, 'pairing-code-explicit-expiry');
    });

    await ensureAuthenticated({
      storeUrl: 'https://store.example',
      workspaceId: 'workspace-1',
      tokenCache,
      fetchImpl,
      openImpl,
      now,
      writeStderr: vi.fn<(message: string) => void>(),
    });

    await expect(tokenCache.load('https://store.example', 'workspace-1')).resolves.toEqual({
      sessionToken: buildSessionToken(nowSeconds + 300),
      sessionTokenExpiresAt: nowSeconds + 1_234,
      refreshToken: 'refresh-interactive',
    });
  });

  it('falls back to the hard-coded TTL when the pairing exchange token expiry is unavailable', async () => {
    const tokenCache = createInMemoryTokenCache();
    const fetchImpl = vi.fn<FetchImplementation>().mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      expect(url.pathname).toBe('/auth/github/pairing/exchange');
      expect(getBodyJson(init)).toEqual({ code: 'pairing-code-fallback-ttl' });
      return jsonResponse({
        sessionToken: 'not-a-jwt-like-token',
        refreshToken: 'refresh-interactive',
      });
    });
    const openImpl = vi.fn<OpenImplementation>().mockImplementation(async (loginUrl) => {
      await triggerLoopbackCallbackFromLoginUrl(loginUrl, 'pairing-code-fallback-ttl');
    });

    await ensureAuthenticated({
      storeUrl: 'https://store.example',
      workspaceId: 'workspace-1',
      tokenCache,
      fetchImpl,
      openImpl,
      now,
      writeStderr: vi.fn<(message: string) => void>(),
    });

    await expect(tokenCache.load('https://store.example', 'workspace-1')).resolves.toEqual({
      sessionToken: 'not-a-jwt-like-token',
      sessionTokenExpiresAt: nowSeconds + 60,
      refreshToken: 'refresh-interactive',
    });
  });

  it('surfaces an actionable error when opening the browser fails', async () => {
    const tokenCache = createInMemoryTokenCache();
    const close = vi.fn<() => Promise<void>>().mockResolvedValue();
    const waitForCode = vi.fn<() => Promise<string>>();
    const createCallbackServer = vi.fn<CreateCallbackServer>().mockResolvedValue({
      callbackUrl: 'http://127.0.0.1:40123/callback',
      waitForCode,
      close,
      isListening: () => true,
    } satisfies CallbackServer);

    await expect(
      ensureAuthenticated({
        storeUrl: 'https://store.example',
        workspaceId: 'workspace-1',
        tokenCache,
        fetchImpl: vi.fn<FetchImplementation>(),
        openImpl: vi.fn<OpenImplementation>().mockRejectedValue(new Error('desktop session unavailable')),
        now,
        writeStderr: vi.fn<(message: string) => void>(),
        createCallbackServer,
      }),
    ).rejects.toThrow(
      'Interactive GitHub login failed: could not open the browser automatically (desktop session unavailable).',
    );

    expect(waitForCode).not.toHaveBeenCalled();
    expect(close).toHaveBeenCalledTimes(1);
  });
});

describe('createLoopbackCallbackServer', () => {
  it('rejects wrong-path and missing-code callbacks without crashing, then accepts a valid callback', async () => {
    const callbackServer = await createLoopbackCallbackServer({
      host: '127.0.0.1',
      path: '/callback',
      port: 0,
      timeoutMs: 5_000,
    });

    try {
      const callbackUrl = new URL(callbackServer.callbackUrl);
      const wrongPathResponse = await fetch(
        `http://127.0.0.1:${callbackUrl.port}/wrong-path?code=ignored`,
        { signal: AbortSignal.timeout(5_000) },
      );
      expect(wrongPathResponse.status).toBe(404);

      const missingCodeResponse = await fetch(callbackServer.callbackUrl, {
        signal: AbortSignal.timeout(5_000),
      });
      expect(missingCodeResponse.status).toBe(400);

      const waitForCodePromise = callbackServer.waitForCode();
      const validCallbackResponse = await fetch(`${callbackServer.callbackUrl}?code=pairing-code-ok`, {
        signal: AbortSignal.timeout(5_000),
      });
      expect(validCallbackResponse.status).toBe(200);
      await expect(waitForCodePromise).resolves.toBe('pairing-code-ok');
    } finally {
      await callbackServer.close();
    }
  });
});
