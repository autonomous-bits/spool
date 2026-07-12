import { describe, expect, it, vi } from 'vitest';
import {
  AuthenticationError,
  type AuthenticatedStoreSession,
  type EnsureAuthenticatedOptions,
} from './auth/login-flow.js';
import { runStartupAuthentication } from './startup-auth.js';

type EnsureAuthenticatedImplementation = (
  options: EnsureAuthenticatedOptions,
) => Promise<AuthenticatedStoreSession>;

describe('runStartupAuthentication', () => {
  it('skips interactive auth when SPOOL_SESSION_TOKEN provides an override', async () => {
    const ensureAuthenticatedMock = vi.fn<EnsureAuthenticatedImplementation>();

    await expect(
      runStartupAuthentication({
        storeUrl: 'http://store.test',
        credentials: {
          workspaceId: 'workspace-1',
          sessionTokenOverride: 'override-token',
        },
        ensureAuthenticatedImpl: ensureAuthenticatedMock,
      }),
    ).resolves.toBeUndefined();

    expect(ensureAuthenticatedMock).not.toHaveBeenCalled();
  });

  it('authenticates once at startup when no override token is configured', async () => {
    const ensureAuthenticatedMock = vi
      .fn<EnsureAuthenticatedImplementation>()
      .mockResolvedValue({ sessionToken: 'session-token-1', workspaceId: 'workspace-1' });

    await expect(
      runStartupAuthentication({
        storeUrl: 'http://store.test',
        credentials: { workspaceId: 'workspace-1' },
        ensureAuthenticatedImpl: ensureAuthenticatedMock,
      }),
    ).resolves.toBeUndefined();

    expect(ensureAuthenticatedMock).toHaveBeenCalledTimes(1);
    expect(ensureAuthenticatedMock).toHaveBeenCalledWith({
      storeUrl: 'http://store.test',
      workspaceId: 'workspace-1',
    });
  });

  it('propagates AuthenticationError failures so main.ts can fail fast before connecting stdio', async () => {
    const error = new AuthenticationError(
      'Interactive GitHub login failed: Set SPOOL_SESSION_TOKEN to bypass interactive login in headless/CI contexts.',
    );
    const ensureAuthenticatedMock = vi
      .fn<EnsureAuthenticatedImplementation>()
      .mockRejectedValue(error);

    await expect(
      runStartupAuthentication({
        storeUrl: 'http://store.test',
        credentials: { workspaceId: 'workspace-1' },
        ensureAuthenticatedImpl: ensureAuthenticatedMock,
      }),
    ).rejects.toThrow(error);
  });
});
