/**
 * Config for signed artifact-download tokens (Meridian IDEA-85's resolution of the IDEA-84 gap
 * report): a deliberately separate secret/lifetime from `AuthConfig`'s session-token secret,
 * because this token authorizes downloading one specific blob, not stakeholder identity — mirrors
 * `auth-config.ts`'s env-var-with-dev-default convention.
 */
export interface ArtifactDownloadConfig {
  secret: string;
  maxAgeSeconds: number;
}

export function loadArtifactDownloadConfig(
  env: NodeJS.ProcessEnv = process.env,
): ArtifactDownloadConfig {
  const secret = env.ARTIFACT_DOWNLOAD_TOKEN_SECRET ?? 'dev-artifact-download-token-secret';
  const maxAgeSeconds = Number.parseInt(env.ARTIFACT_DOWNLOAD_TOKEN_MAX_AGE_SECONDS ?? '300', 10);

  return { secret, maxAgeSeconds } satisfies ArtifactDownloadConfig;
}
