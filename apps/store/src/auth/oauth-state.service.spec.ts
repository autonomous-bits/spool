import { describe, expect, it, vi } from 'vitest';
import type { AuthConfig } from './auth-config.js';
import { InvalidOAuthStateError, OAuthStateService } from './oauth-state.service.js';

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
    oauthStateSecret: 'state-secret',
    oauthStateMaxAgeSeconds: 600,
    ...overrides,
  } satisfies AuthConfig;
}

describe('OAuthStateService', () => {
  it('issues a state that verifies successfully', () => {
    const service = new OAuthStateService(buildConfig());
    const state = service.issue();
    expect(() => service.verify(state)).not.toThrow();
  });

  it('rejects a missing state', () => {
    const service = new OAuthStateService(buildConfig());
    expect(() => service.verify(undefined)).toThrow(InvalidOAuthStateError);
  });

  it('rejects a blank state', () => {
    const service = new OAuthStateService(buildConfig());
    expect(() => service.verify('   ')).toThrow(InvalidOAuthStateError);
  });

  it('rejects a tampered/foreign state', () => {
    const service = new OAuthStateService(buildConfig());
    const otherService = new OAuthStateService(buildConfig({ oauthStateSecret: 'other-secret' }));
    const state = otherService.issue();
    expect(() => service.verify(state)).toThrow(InvalidOAuthStateError);
  });

  it('rejects an expired state', () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(0);
      const service = new OAuthStateService(buildConfig({ oauthStateMaxAgeSeconds: 60 }));
      const state = service.issue();

      vi.setSystemTime(61_000);

      expect(() => service.verify(state)).toThrow(InvalidOAuthStateError);
    } finally {
      vi.useRealTimers();
    }
  });
});
