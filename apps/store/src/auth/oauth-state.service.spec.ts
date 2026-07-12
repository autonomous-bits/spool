import { describe, expect, it, vi } from 'vitest';
import type { AuthConfig } from './auth-config.js';
import { signHmacToken } from './hmac-token.js';
import {
  InvalidCliRedirectUriError,
  InvalidOAuthStateError,
  OAuthStateService,
} from './oauth-state.service.js';

function buildConfig(overrides: Partial<AuthConfig> = {}): AuthConfig {
  return {
    githubClientId: 'client-id',
    githubClientSecret: 'client-secret',
    githubRedirectUri: 'http://localhost:3000/auth/github/callback',
    githubAuthorizeUrl: 'https://github.com/login/oauth/authorize',
    githubTokenExchangeUrl: 'https://github.com/login/oauth/access_token',
    githubUserApiUrl: 'https://api.github.com/user',
    sessionTokenSecret: 'session-secret',
    sessionTokenMaxAgeSeconds: 900,
    refreshTokenMaxAgeSeconds: 2_592_000,
    pairingCodeMaxAgeSeconds: 120,
    oauthStateSecret: 'state-secret',
    oauthStateMaxAgeSeconds: 600,
    ...overrides,
  } satisfies AuthConfig;
}

describe('OAuthStateService', () => {
  it('issues a state that verifies successfully with a null workspaceId when none was supplied', () => {
    const service = new OAuthStateService(buildConfig());
    const state = service.issue();
    expect(service.verify(state)).toEqual({ workspaceId: null, cliRedirectUri: null });
  });

  it('round-trips a supplied workspaceId through issue/verify', () => {
    const service = new OAuthStateService(buildConfig());
    const state = service.issue('workspace-1');
    expect(service.verify(state)).toEqual({ workspaceId: 'workspace-1', cliRedirectUri: null });
  });

  it('round-trips a supplied loopback cliRedirectUri through issue/verify', () => {
    const service = new OAuthStateService(buildConfig());
    const state = service.issue('workspace-1', 'http://127.0.0.1:4318/callback');
    expect(service.verify(state)).toEqual({
      workspaceId: 'workspace-1',
      cliRedirectUri: 'http://127.0.0.1:4318/callback',
    });
  });

  it('round-trips localhost cliRedirectUri values through issue/verify', () => {
    const service = new OAuthStateService(buildConfig());
    const state = service.issue(undefined, 'http://localhost:8787/callback?source=cli');
    expect(service.verify(state)).toEqual({
      workspaceId: null,
      cliRedirectUri: 'http://localhost:8787/callback?source=cli',
    });
  });

  it('rejects a non-loopback cliRedirectUri during issue', () => {
    const service = new OAuthStateService(buildConfig());
    expect(() => {
      service.issue(undefined, 'http://example.com/callback');
    }).toThrow(InvalidCliRedirectUriError);
  });

  it('rejects a malformed cliRedirectUri during issue', () => {
    const service = new OAuthStateService(buildConfig());
    expect(() => {
      service.issue(undefined, 'not a url');
    }).toThrow(InvalidCliRedirectUriError);
  });

  it('rejects an https loopback cliRedirectUri during issue', () => {
    const service = new OAuthStateService(buildConfig());
    expect(() => {
      service.issue(undefined, 'https://127.0.0.1/callback');
    }).toThrow(InvalidCliRedirectUriError);
  });

  it('rejects a non-loopback cliRedirectUri when verifying a tampered state payload', () => {
    const config = buildConfig();
    const service = new OAuthStateService(config);
    const state = signHmacToken(
      { workspaceId: null, cliRedirectUri: 'http://example.com/callback' },
      config.oauthStateSecret,
      config.oauthStateMaxAgeSeconds,
    );

    expect(() => {
      service.verify(state);
    }).toThrow(InvalidOAuthStateError);
  });

  it('rejects an https loopback cliRedirectUri when verifying a tampered state payload', () => {
    const config = buildConfig();
    const service = new OAuthStateService(config);
    const state = signHmacToken(
      { workspaceId: null, cliRedirectUri: 'https://127.0.0.1/callback' },
      config.oauthStateSecret,
      config.oauthStateMaxAgeSeconds,
    );

    expect(() => {
      service.verify(state);
    }).toThrow(InvalidOAuthStateError);
  });

  it('rejects a missing state', () => {
    const service = new OAuthStateService(buildConfig());
    expect(() => { service.verify(undefined); }).toThrow(InvalidOAuthStateError);
  });

  it('rejects a blank state', () => {
    const service = new OAuthStateService(buildConfig());
    expect(() => { service.verify('   '); }).toThrow(InvalidOAuthStateError);
  });

  it('rejects a tampered/foreign state', () => {
    const service = new OAuthStateService(buildConfig());
    const otherService = new OAuthStateService(buildConfig({ oauthStateSecret: 'other-secret' }));
    const state = otherService.issue();
    expect(() => { service.verify(state); }).toThrow(InvalidOAuthStateError);
  });

  it('rejects an expired state', () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(0);
      const service = new OAuthStateService(buildConfig({ oauthStateMaxAgeSeconds: 60 }));
      const state = service.issue();

      vi.setSystemTime(61_000);

      expect(() => {
        service.verify(state);
      }).toThrow(InvalidOAuthStateError);
    } finally {
      vi.useRealTimers();
    }
  });
});
