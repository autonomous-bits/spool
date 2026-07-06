import { NotFoundException, UnauthorizedException } from '@nestjs/common';
import { Readable } from 'node:stream';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Artifact } from '../domain/artifact.js';
import { Chunk } from '../domain/chunk.js';
import { ChunkArtifactAssociation } from '../domain/chunk-artifact-association.js';
import type { ArtifactBlobStore } from '../persistence/artifact-blob-store.js';
import type { ArtifactRepository } from '../persistence/artifact.repository.js';
import type { ChunkArtifactRepository } from '../persistence/chunk-artifact.repository.js';
import type { ChunkRepository } from '../persistence/chunk.repository.js';
import type {
  ArtifactDownloadTokenService} from './artifact-download-token.service.js';
import {
  InvalidArtifactDownloadTokenError,
} from './artifact-download-token.service.js';
import { ArtifactsService } from './artifacts.service.js';

describe('ArtifactsService', () => {
  let artifactRepository: Pick<ArtifactRepository, 'create' | 'findById'>;
  let chunkArtifactRepository: Pick<ChunkArtifactRepository, 'create' | 'findEffectiveForChunk'>;
  let chunkRepository: Pick<ChunkRepository, 'findByLabel'>;
  let downloadTokenService: Pick<ArtifactDownloadTokenService, 'issue' | 'verify'>;
  let blobStore: Pick<ArtifactBlobStore, 'createReadStream'>;
  let service: ArtifactsService;

  const stakeholderId = '11111111-1111-1111-1111-111111111111';

  beforeEach(() => {
    artifactRepository = { create: vi.fn(), findById: vi.fn() };
    chunkArtifactRepository = { create: vi.fn(), findEffectiveForChunk: vi.fn() };
    chunkRepository = { findByLabel: vi.fn() };
    downloadTokenService = { issue: vi.fn(), verify: vi.fn() };
    blobStore = { createReadStream: vi.fn() };
    service = new ArtifactsService(
      artifactRepository as ArtifactRepository,
      chunkArtifactRepository as ChunkArtifactRepository,
      chunkRepository as ChunkRepository,
      downloadTokenService as ArtifactDownloadTokenService,
      blobStore as ArtifactBlobStore,
    );
  });

  describe('createArtifact', () => {
    it('decodes base64 content and delegates to ArtifactRepository.create', async () => {
      const artifact = new Artifact({
        uri: 'local-file://artifact-1',
        mimeType: 'text/plain',
        createdByStakeholderId: stakeholderId,
      });
      vi.mocked(artifactRepository.create).mockResolvedValue(artifact);

      const result = await service.createArtifact({
        content: Buffer.from('hello').toString('base64'),
        mimeType: 'text/plain',
        stakeholderId,
      });

      expect(result).toEqual({
        id: artifact.id,
        uri: artifact.uri,
        mimeType: artifact.mimeType,
        createdByStakeholderId: artifact.createdByStakeholderId,
        createdAt: artifact.createdAt,
      });
      expect(artifactRepository.create).toHaveBeenCalledWith({
        content: Buffer.from('hello'),
        mimeType: 'text/plain',
        createdByStakeholderId: stakeholderId,
      });
    });
  });

  describe('attachArtifactToChunk', () => {
    it('throws NotFoundException when the chunk does not resolve in scope', async () => {
      vi.mocked(chunkRepository.findByLabel).mockResolvedValue(undefined);

      await expect(
        service.attachArtifactToChunk('ATOMIC-1', {
          artifactId: 'artifact-1',
          stakeholderId,
        }),
      ).rejects.toThrow(NotFoundException);
      expect(chunkArtifactRepository.create).not.toHaveBeenCalled();
    });

    it('throws NotFoundException when the artifact does not exist', async () => {
      vi.mocked(chunkRepository.findByLabel).mockResolvedValue(
        new Chunk({
          label: 'ATOMIC-1',
          content: 'content',
          discipline: 'engineering',
          chunkType: 'feature',
          contextKind: 'permanent',
          createdByStakeholderId: stakeholderId,
        }),
      );
      vi.mocked(artifactRepository.findById).mockResolvedValue(undefined);

      await expect(
        service.attachArtifactToChunk('ATOMIC-1', {
          artifactId: 'artifact-1',
          stakeholderId,
        }),
      ).rejects.toThrow(NotFoundException);
      expect(chunkArtifactRepository.create).not.toHaveBeenCalled();
    });

    it('creates an active association when chunk and artifact both resolve', async () => {
      const chunk = new Chunk({
        label: 'ATOMIC-1',
        content: 'content',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
        createdByStakeholderId: stakeholderId,
      });
      const artifact = new Artifact({
        uri: 'local-file://artifact-1',
        mimeType: 'text/plain',
        createdByStakeholderId: stakeholderId,
      });
      vi.mocked(chunkRepository.findByLabel).mockResolvedValue(chunk);
      vi.mocked(artifactRepository.findById).mockResolvedValue(artifact);
      const created = new ChunkArtifactAssociation({
        chunkLabel: 'ATOMIC-1',
        artifactId: artifact.id,
        status: 'active',
        createdByStakeholderId: stakeholderId,
      });
      vi.mocked(chunkArtifactRepository.create).mockResolvedValue(created);

      const result = await service.attachArtifactToChunk('ATOMIC-1', {
        artifactId: artifact.id,
        stakeholderId,
      });

      expect(result.status).toBe('active');
      expect(chunkArtifactRepository.create).toHaveBeenCalledWith(
        expect.objectContaining({
          chunkLabel: 'ATOMIC-1',
          artifactId: artifact.id,
          status: 'active',
          branchId: undefined,
        }),
      );
    });
  });

  describe('detachArtifactFromChunk', () => {
    it('creates a branch-scoped deactivated association', async () => {
      const chunk = new Chunk({
        label: 'ATOMIC-1',
        content: 'content',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
        createdByStakeholderId: stakeholderId,
        branchId: 'branch-1',
        originBranchId: 'branch-1',
      });
      const artifact = new Artifact({
        uri: 'local-file://artifact-1',
        mimeType: 'text/plain',
        createdByStakeholderId: stakeholderId,
      });
      vi.mocked(chunkRepository.findByLabel).mockResolvedValue(chunk);
      vi.mocked(artifactRepository.findById).mockResolvedValue(artifact);
      const created = new ChunkArtifactAssociation({
        chunkLabel: 'ATOMIC-1',
        artifactId: artifact.id,
        status: 'deactivated',
        createdByStakeholderId: stakeholderId,
        branchId: 'branch-1',
        originBranchId: 'branch-1',
      });
      vi.mocked(chunkArtifactRepository.create).mockResolvedValue(created);

      const result = await service.detachArtifactFromChunk(
        'ATOMIC-1',
        artifact.id,
        'branch-1',
        stakeholderId,
      );

      expect(result.status).toBe('deactivated');
      expect(chunkArtifactRepository.create).toHaveBeenCalledWith(
        expect.objectContaining({
          chunkLabel: 'ATOMIC-1',
          artifactId: artifact.id,
          status: 'deactivated',
          branchId: 'branch-1',
          originBranchId: 'branch-1',
        }),
      );
    });

    it('throws NotFoundException when the chunk does not resolve in the given branch scope', async () => {
      vi.mocked(chunkRepository.findByLabel).mockResolvedValue(undefined);

      await expect(
        service.detachArtifactFromChunk('ATOMIC-1', 'artifact-1', 'branch-1', stakeholderId),
      ).rejects.toThrow(NotFoundException);
      expect(chunkArtifactRepository.create).not.toHaveBeenCalled();
    });
  });

  describe('getEffectiveArtifactsForChunk', () => {
    it('throws NotFoundException when the chunk does not resolve in scope', async () => {
      vi.mocked(chunkRepository.findByLabel).mockResolvedValue(undefined);

      await expect(
        service.getEffectiveArtifactsForChunk('ATOMIC-1', undefined),
      ).rejects.toThrow(NotFoundException);
      expect(chunkArtifactRepository.findEffectiveForChunk).not.toHaveBeenCalled();
    });

    it('delegates to ChunkArtifactRepository.findEffectiveForChunk when the chunk resolves', async () => {
      const chunk = new Chunk({
        label: 'ATOMIC-1',
        content: 'content',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
        createdByStakeholderId: stakeholderId,
      });
      vi.mocked(chunkRepository.findByLabel).mockResolvedValue(chunk);
      const effective = [{ artifactId: 'artifact-1', branchId: null, status: 'active' as const }];
      vi.mocked(chunkArtifactRepository.findEffectiveForChunk).mockResolvedValue(effective);

      const result = await service.getEffectiveArtifactsForChunk('ATOMIC-1', 'branch-1');

      expect(result).toEqual(effective);
      expect(chunkArtifactRepository.findEffectiveForChunk).toHaveBeenCalledWith(
        'ATOMIC-1',
        'branch-1',
      );
    });
  });

  describe('issueDownloadToken', () => {
    it('throws NotFoundException when the artifact does not exist', async () => {
      vi.mocked(artifactRepository.findById).mockResolvedValue(undefined);

      await expect(service.issueDownloadToken('artifact-1')).rejects.toThrow(NotFoundException);
      expect(downloadTokenService.issue).not.toHaveBeenCalled();
    });

    it('delegates to ArtifactDownloadTokenService.issue when the artifact exists', async () => {
      const artifact = new Artifact({
        uri: 'local-file://artifact-1',
        mimeType: 'text/plain',
        createdByStakeholderId: stakeholderId,
      });
      vi.mocked(artifactRepository.findById).mockResolvedValue(artifact);
      const issued = { token: 'signed-token', expiresAt: new Date('2026-01-01T00:00:00Z') };
      vi.mocked(downloadTokenService.issue).mockReturnValue(issued);

      const result = await service.issueDownloadToken(artifact.id);

      expect(result).toEqual(issued);
      expect(downloadTokenService.issue).toHaveBeenCalledWith(artifact.id);
    });
  });

  describe('streamArtifactContent', () => {
    it('throws UnauthorizedException when the token is invalid', async () => {
      vi.mocked(downloadTokenService.verify).mockImplementation(() => {
        throw new InvalidArtifactDownloadTokenError('token expired');
      });

      await expect(service.streamArtifactContent('bad-token')).rejects.toThrow(
        UnauthorizedException,
      );
      expect(artifactRepository.findById).not.toHaveBeenCalled();
    });

    it('throws NotFoundException when the verified artifact no longer exists', async () => {
      vi.mocked(downloadTokenService.verify).mockReturnValue({ artifactId: 'artifact-1' });
      vi.mocked(artifactRepository.findById).mockResolvedValue(undefined);

      await expect(service.streamArtifactContent('good-token')).rejects.toThrow(
        NotFoundException,
      );
    });

    it('streams the artifact blob content with its mimeType when the token is valid', async () => {
      const artifact = new Artifact({
        uri: 'local-file://artifact-1',
        mimeType: 'text/plain',
        createdByStakeholderId: stakeholderId,
      });
      vi.mocked(downloadTokenService.verify).mockReturnValue({ artifactId: artifact.id });
      vi.mocked(artifactRepository.findById).mockResolvedValue(artifact);
      const stream = Readable.from([Buffer.from('hello')]);
      vi.mocked(blobStore.createReadStream).mockReturnValue(stream);

      const result = await service.streamArtifactContent('good-token');

      expect(result).toEqual({ stream, mimeType: 'text/plain' });
      expect(blobStore.createReadStream).toHaveBeenCalledWith(artifact.uri);
    });
  });
});
