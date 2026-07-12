export interface AuthConfig {
  githubClientId: string;
  githubClientSecret: string;
  githubRedirectUri: string;
  githubAuthorizeUrl: string;
  githubTokenExchangeUrl: string;
  githubUserApiUrl: string;
  sessionTokenSecret: string;
  sessionTokenMaxAgeSeconds: number;
  refreshTokenMaxAgeSeconds: number;
  pairingCodeMaxAgeSeconds: number;
  oauthStateSecret: string;
  oauthStateMaxAgeSeconds: number;
}

/**
 * Builds the GitHub OAuth + session-token config from AUTH_* / GITHUB_* environment variables,
 * per Meridian IDEA-81 (GitHub OAuth authorization-code flow ADR). Mirrors
 * `database-config.ts`'s convention of local dev defaults so unit/e2e tests run without extra
 * setup; `githubAuthorizeUrl`/`githubTokenExchangeUrl`/`githubUserApiUrl` are independently
 * overridable so Docker Compose can point the real github.com endpoints at a local stub for
 * SG5's containerized e2e exercise, without swapping the `GithubOAuthClient` DI provider.
 *
 * `githubClientSecret`/`sessionTokenSecret`/`oauthStateSecret` have non-secret dev-only
 * placeholder defaults here (never real credentials); Docker Compose and any real deployment
 * MUST set the real values via environment, never source control.
 */
export function loadAuthConfig(env: NodeJS.ProcessEnv = process.env): AuthConfig {
  const githubClientId = env.GITHUB_CLIENT_ID ?? 'dev-github-client-id';
  const githubClientSecret = env.GITHUB_CLIENT_SECRET ?? 'dev-github-client-secret';
  const githubRedirectUri =
    env.GITHUB_OAUTH_REDIRECT_URI ?? 'http://localhost:3000/auth/github/callback';
  const githubAuthorizeUrl =
    env.GITHUB_OAUTH_AUTHORIZE_URL ?? 'https://github.com/login/oauth/authorize';
  const githubTokenExchangeUrl =
    env.GITHUB_OAUTH_TOKEN_URL ?? 'https://github.com/login/oauth/access_token';
  const githubUserApiUrl = env.GITHUB_USER_API_URL ?? 'https://api.github.com/user';
  const sessionTokenSecret = env.SESSION_TOKEN_SECRET ?? 'dev-session-token-secret';
  const sessionTokenMaxAgeSeconds = Number.parseInt(
    env.SESSION_TOKEN_MAX_AGE_SECONDS ?? '900',
    10,
  );
  const refreshTokenMaxAgeSeconds = Number.parseInt(
    env.REFRESH_TOKEN_MAX_AGE_SECONDS ?? '2592000',
    10,
  );
  const pairingCodeMaxAgeSeconds = Number.parseInt(
    env.PAIRING_CODE_MAX_AGE_SECONDS ?? '120',
    10,
  );
  const oauthStateSecret = env.OAUTH_STATE_SECRET ?? 'dev-oauth-state-secret';
  const oauthStateMaxAgeSeconds = Number.parseInt(env.OAUTH_STATE_MAX_AGE_SECONDS ?? '600', 10);

  return {
    githubClientId,
    githubClientSecret,
    githubRedirectUri,
    githubAuthorizeUrl,
    githubTokenExchangeUrl,
    githubUserApiUrl,
    sessionTokenSecret,
    sessionTokenMaxAgeSeconds,
    refreshTokenMaxAgeSeconds,
    pairingCodeMaxAgeSeconds,
    oauthStateSecret,
    oauthStateMaxAgeSeconds,
  } satisfies AuthConfig;
}
