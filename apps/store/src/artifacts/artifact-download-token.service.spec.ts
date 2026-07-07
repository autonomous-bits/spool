import { describe, expect, it, vi } from 'vitest';
import type { ArtifactDownloadConfig } from './artifact-download-config.js';
import {
  ArtifactDownloadTokenService,
  InvalidArtifactDownloadTokenError,
} from './artifact-download-token.service.js';

function buildConfig(overrides: Partial<ArtifactDownloadConfig> = {}): ArtifactDownloadConfig {
  return {
    secret: 'download-secret',
    maxAgeSeconds: 300,
    ...overrides,
  } satisfies ArtifactDownloadConfig;
}

describe('ArtifactDownloadTokenService', () => {
  it('issues a token whose verify() returns the artifactId claim', () => {
    const service = new ArtifactDownloadTokenService(buildConfig());
    const { token } = service.issue('artifact-1', 'workspace-1');

    expect(service.verify(token)).toEqual({ artifactId: 'artifact-1', workspaceId: 'workspace-1' });
  });

  it('issues an expiresAt in the future by maxAgeSeconds', () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(0);
      const service = new ArtifactDownloadTokenService(buildConfig({ maxAgeSeconds: 300 }));
      const { expiresAt } = service.issue('artifact-1', 'workspace-1');

      expect(expiresAt.getTime()).toBe(300_000);
    } finally {
      vi.useRealTimers();
    }
  });

  it('throws InvalidArtifactDownloadTokenError for a token signed with a different secret', () => {
    const service = new ArtifactDownloadTokenService(buildConfig());
    const otherService = new ArtifactDownloadTokenService(buildConfig({ secret: 'other-secret' }));
    const { token } = otherService.issue('artifact-1', 'workspace-1');

    expect(() => service.verify(token)).toThrow(InvalidArtifactDownloadTokenError);
  });

  it('throws InvalidArtifactDownloadTokenError once the token has expired', () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(0);
      const service = new ArtifactDownloadTokenService(buildConfig({ maxAgeSeconds: 60 }));
      const { token } = service.issue('artifact-1', 'workspace-1');

      vi.setSystemTime(61_000);

      expect(() => service.verify(token)).toThrow(InvalidArtifactDownloadTokenError);
    } finally {
      vi.useRealTimers();
    }
  });

  it('throws InvalidArtifactDownloadTokenError for a tampered token', () => {
    const service = new ArtifactDownloadTokenService(buildConfig());
    const { token } = service.issue('artifact-1', 'workspace-1');
    const [payload] = token.split('.');
    const tamperedSignature = Buffer.from('tampered-signature').toString('base64url');

    expect(() => service.verify(`${String(payload)}.${tamperedSignature}`)).toThrow(
      InvalidArtifactDownloadTokenError,
    );
  });

  it('throws InvalidArtifactDownloadTokenError for a structurally invalid token', () => {
    const service = new ArtifactDownloadTokenService(buildConfig());
    expect(() => service.verify('garbage')).toThrow(InvalidArtifactDownloadTokenError);
  });
});
