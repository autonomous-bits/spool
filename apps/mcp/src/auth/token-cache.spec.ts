import { randomUUID } from 'node:crypto';

import { describe, expect, it, vi } from 'vitest';

import {
  createInMemoryTokenCache,
  createTokenCache,
  type CachedCredentials,
  TokenCacheError,
  type TokenCacheKeyringClient,
} from './token-cache.js';

function createCredentials(seed: string): CachedCredentials {
  return {
    sessionToken: `session-${seed}`,
    sessionTokenExpiresAt: 1_735_689_600,
    refreshToken: `refresh-${seed}`,
  };
}

function createStubKeyring(): TokenCacheKeyringClient {
  const entries = new Map<string, string>();

  return {
    getPassword(service: string, account: string): Promise<string | undefined> {
      return Promise.resolve(entries.get(`${service}::${account}`));
    },
    setPassword(service: string, account: string, password: string): Promise<void> {
      entries.set(`${service}::${account}`, password);
      return Promise.resolve();
    },
    deletePassword(service: string, account: string): Promise<boolean> {
      return Promise.resolve(entries.delete(`${service}::${account}`));
    },
  };
}

describe('createInMemoryTokenCache', () => {
  it('round-trips credentials through save, load, and clear', async () => {
    const cache = createInMemoryTokenCache();
    const credentials = createCredentials('alpha');

    await cache.save('https://store.example', 'workspace-1', credentials);
    await expect(cache.load('https://store.example', 'workspace-1')).resolves.toEqual(credentials);

    await cache.clear('https://store.example', 'workspace-1');
    await expect(cache.load('https://store.example', 'workspace-1')).resolves.toBeUndefined();
  });

  it('isolates entries by storeUrl and workspaceId', async () => {
    const cache = createInMemoryTokenCache();
    const first = createCredentials('first');
    const second = createCredentials('second');
    const third = createCredentials('third');

    await cache.save('https://store-a.example', 'workspace-1', first);
    await cache.save('https://store-a.example', 'workspace-2', second);
    await cache.save('https://store-b.example', 'workspace-1', third);

    await expect(cache.load('https://store-a.example', 'workspace-1')).resolves.toEqual(first);
    await expect(cache.load('https://store-a.example', 'workspace-2')).resolves.toEqual(second);
    await expect(cache.load('https://store-b.example', 'workspace-1')).resolves.toEqual(third);
  });

  it('returns undefined for a cache miss', async () => {
    const cache = createInMemoryTokenCache();

    await expect(cache.load('https://store.example', 'missing-workspace')).resolves.toBeUndefined();
  });
});

describe('createTokenCache', () => {
  it('wraps OS keychain load failures in TokenCacheError', async () => {
    const cause = new Error('keychain offline');
    const keyring = {
      getPassword: vi.fn<TokenCacheKeyringClient['getPassword']>().mockRejectedValue(cause),
      setPassword: vi.fn<TokenCacheKeyringClient['setPassword']>(),
      deletePassword: vi.fn<TokenCacheKeyringClient['deletePassword']>(),
    } satisfies TokenCacheKeyringClient;
    const cache = createTokenCache({ keyring });

    await expect(cache.load('https://store.example', 'workspace-1')).rejects.toMatchObject({
      name: 'TokenCacheError',
      message: expect.stringContaining('Failed to load cached Spool MCP credentials'),
      cause,
    });
    await expect(cache.load('https://store.example', 'workspace-1')).rejects.toBeInstanceOf(
      TokenCacheError,
    );
  });

  it('wraps OS keychain save failures in TokenCacheError', async () => {
    const cause = new Error('keychain write failed');
    const keyring = {
      getPassword: vi.fn<TokenCacheKeyringClient['getPassword']>(),
      setPassword: vi.fn<TokenCacheKeyringClient['setPassword']>().mockRejectedValue(cause),
      deletePassword: vi.fn<TokenCacheKeyringClient['deletePassword']>(),
    } satisfies TokenCacheKeyringClient;
    const cache = createTokenCache({ keyring });

    await expect(
      cache.save('https://store.example', 'workspace-1', createCredentials('save-error')),
    ).rejects.toMatchObject({
      name: 'TokenCacheError',
      message: expect.stringContaining('Failed to save cached Spool MCP credentials'),
      cause,
    });
  });

  it('wraps OS keychain clear failures in TokenCacheError', async () => {
    const cause = new Error('keychain delete failed');
    const keyring = {
      getPassword: vi.fn<TokenCacheKeyringClient['getPassword']>(),
      setPassword: vi.fn<TokenCacheKeyringClient['setPassword']>(),
      deletePassword: vi.fn<TokenCacheKeyringClient['deletePassword']>().mockRejectedValue(cause),
    } satisfies TokenCacheKeyringClient;
    const cache = createTokenCache({ keyring });

    await expect(cache.clear('https://store.example', 'workspace-1')).rejects.toMatchObject({
      name: 'TokenCacheError',
      message: expect.stringContaining('Failed to clear cached Spool MCP credentials'),
      cause,
    });
  });

  it('returns undefined and logs a warning when the stored value is malformed', async () => {
    const logger = { warn: vi.fn<(message: string) => void>() };
    const keyring: TokenCacheKeyringClient = {
      getPassword(): Promise<string> {
        return Promise.resolve('{"sessionToken":"ok","sessionTokenExpiresAt":"wrong-type"}');
      },
      setPassword(): Promise<void> {
        return Promise.reject(new Error('not used in this test'));
      },
      deletePassword(): Promise<boolean> {
        return Promise.resolve(false);
      },
    };
    const cache = createTokenCache({ keyring, logger });

    await expect(cache.load('https://store.example', 'workspace-1')).resolves.toBeUndefined();
    expect(logger.warn).toHaveBeenCalledTimes(1);
    expect(logger.warn.mock.calls[0]?.[0]).toContain('Ignoring malformed cached Spool MCP credentials');
  });

  it('returns undefined and logs a warning when the stored JSON is corrupt', async () => {
    const logger = { warn: vi.fn<(message: string) => void>() };
    const keyring = {
      getPassword: vi.fn<TokenCacheKeyringClient['getPassword']>().mockResolvedValue('{not-json'),
      setPassword: vi.fn<TokenCacheKeyringClient['setPassword']>(),
      deletePassword: vi.fn<TokenCacheKeyringClient['deletePassword']>(),
    } satisfies TokenCacheKeyringClient;
    const cache = createTokenCache({ keyring, logger });

    await expect(cache.load('https://store.example', 'workspace-1')).resolves.toBeUndefined();
    expect(logger.warn).toHaveBeenCalledTimes(1);
    expect(logger.warn.mock.calls[0]?.[0]).toContain('Ignoring malformed cached Spool MCP credentials');
  });

  it('persists credentials in the OS keychain when a backend is available', async (context) => {
    const cache = createTokenCache();
    const storeUrl = `https://store-${randomUUID()}.example`;
    const workspaceId = `workspace-${randomUUID()}`;
    const credentials = createCredentials('keychain');

    try {
      await cache.clear(storeUrl, workspaceId);
      await cache.save(storeUrl, workspaceId, credentials);
      await expect(cache.load(storeUrl, workspaceId)).resolves.toEqual(credentials);
      await cache.clear(storeUrl, workspaceId);
      await expect(cache.load(storeUrl, workspaceId)).resolves.toBeUndefined();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      context.skip(`OS keychain unavailable in this environment: ${message}`);
    } finally {
      try {
        await cache.clear(storeUrl, workspaceId);
      } catch {
        // Best-effort cleanup only.
      }
    }
  });

  it('can use an injected keyring client instead of the OS keychain', async () => {
    const cache = createTokenCache({ keyring: createStubKeyring() });
    const credentials = createCredentials('stub');

    await cache.save('https://store.example', 'workspace-1', credentials);
    await expect(cache.load('https://store.example', 'workspace-1')).resolves.toEqual(credentials);
  });

  it('isolates cache entries for store and workspace pairs that would collide without encoding', async () => {
    const cache = createTokenCache({ keyring: createStubKeyring() });
    const firstCredentials = createCredentials('first');
    const secondCredentials = createCredentials('second');

    await cache.save('https://store.example/team%2Falpha', 'workspace::one', firstCredentials);
    await cache.save('https://store.example/team', 'alpha::workspace%2Fone', secondCredentials);

    await expect(
      cache.load('https://store.example/team%2Falpha', 'workspace::one'),
    ).resolves.toEqual(firstCredentials);
    await expect(
      cache.load('https://store.example/team', 'alpha::workspace%2Fone'),
    ).resolves.toEqual(secondCredentials);
  });
});
