import { afterEach, describe, expect, it } from 'vitest';
import {
  getStoreAuthHeaders,
  loadStoreCredentials,
  resetStoreCredentialsForTests,
  StoreClientConfigError,
} from './store-client.js';

function envWith(overrides: Partial<Record<string, string>>): NodeJS.ProcessEnv {
  return { ...overrides } as NodeJS.ProcessEnv;
}

describe('loadStoreCredentials', () => {
  afterEach(() => {
    resetStoreCredentialsForTests();
  });

  it('reads SPOOL_SESSION_TOKEN and SPOOL_WORKSPACE_ID from the given env', () => {
    const env = envWith({ SPOOL_SESSION_TOKEN: 'token-1', SPOOL_WORKSPACE_ID: 'workspace-1' });

    expect(loadStoreCredentials(env)).toEqual({ sessionToken: 'token-1', workspaceId: 'workspace-1' });
  });

  it('fails fast, naming the variable, when SPOOL_SESSION_TOKEN is missing', () => {
    const env = envWith({ SPOOL_WORKSPACE_ID: 'workspace-1' });

    expect(() => loadStoreCredentials(env)).toThrow(StoreClientConfigError);
    expect(() => loadStoreCredentials(env)).toThrow(/SPOOL_SESSION_TOKEN/);
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
    const env = envWith({ SPOOL_SESSION_TOKEN: 'super-secret-token-value', SPOOL_WORKSPACE_ID: '' });

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
    expect(loadStoreCredentials(env)).toEqual({ sessionToken: 'token-1', workspaceId: 'workspace-1' });

    const changedEnv = envWith({ SPOOL_SESSION_TOKEN: 'token-2', SPOOL_WORKSPACE_ID: 'workspace-2' });
    expect(loadStoreCredentials(changedEnv)).toEqual({ sessionToken: 'token-1', workspaceId: 'workspace-1' });
  });
});

describe('getStoreAuthHeaders', () => {
  afterEach(() => {
    resetStoreCredentialsForTests();
  });

  it('builds Authorization and X-Workspace-Id headers from the host-held credentials', () => {
    const env = envWith({ SPOOL_SESSION_TOKEN: 'token-1', SPOOL_WORKSPACE_ID: 'workspace-1' });

    expect(getStoreAuthHeaders(env)).toEqual({
      authorization: 'Bearer token-1',
      'x-workspace-id': 'workspace-1',
    });
  });

  it('propagates the same fail-fast validation as loadStoreCredentials', () => {
    const env = envWith({});

    expect(() => getStoreAuthHeaders(env)).toThrow(StoreClientConfigError);
  });
});
