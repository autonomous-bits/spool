import type { Readable } from 'node:stream';
import { BadRequestException, Inject, Injectable, NotFoundException, UnauthorizedException } from '@nestjs/common';
import { ChunkArtifactAssociation } from '../domain/chunk-artifact-association.js';
import type { ArtifactBlobStore } from '../persistence/artifact-blob-store.js';
import { ARTIFACT_BLOB_STORE } from '../persistence/artifact-blob-store.token.js';
import { ArtifactRepository } from '../persistence/artifact.repository.js';
import type { EffectiveChunkArtifact } from '../persistence/chunk-artifact.repository.js';
import { ChunkArtifactRepository } from '../persistence/chunk-artifact.repository.js';
import { ChunkRepository } from '../persistence/chunk.repository.js';
import {
  ArtifactDownloadTokenService,
  InvalidArtifactDownloadTokenError,
  type IssuedArtifactDownloadToken,
} from './artifact-download-token.service.js';
import { toArtifactResponse, type ArtifactResponse } from './artifact-response.dto.js';
import { toChunkArtifactResponse, type ChunkArtifactResponse } from './chunk-artifact-response.dto.js';
import type { CreateArtifactRequest } from './create-artifact-request.dto.js';
import type { CreateChunkArtifactRequest } from './create-chunk-artifact-request.dto.js';

export interface StreamedArtifactContent {
  stream: Readable;
  mimeType: string;
}

const FOREIGN_KEY_VIOLATION = '23503';

function isForeignKeyViolation(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error).code === FOREIGN_KEY_VIOLATION
  );
}

/**
 * Application service for artifact ingestion and chunk-artifact association writes (Meridian
 * IDEA-58/IDEA-59/IDEA-60/IDEA-52/IDEA-34), sitting between the HTTP controller and the
 * `ArtifactRepository`/`ChunkArtifactRepository`/`ChunkRepository` persistence layers.
 */
@Injectable()
export class ArtifactsService {
  constructor(
    private readonly artifactRepository: ArtifactRepository,
    private readonly chunkArtifactRepository: ChunkArtifactRepository,
    private readonly chunkRepository: ChunkRepository,
    private readonly downloadTokenService: ArtifactDownloadTokenService,
    @Inject(ARTIFACT_BLOB_STORE) private readonly blobStore: ArtifactBlobStore,
  ) {}

  async createArtifact(request: CreateArtifactRequest): Promise<ArtifactResponse> {
    try {
      const artifact = await this.artifactRepository.create({
        content: Buffer.from(request.content, 'base64'),
        mimeType: request.mimeType,
        createdByStakeholderId: request.stakeholderId,
      });
      return toArtifactResponse(artifact);
    } catch (error) {
      if (isForeignKeyViolation(error)) {
        throw new BadRequestException(`Unknown stakeholderId: ${request.stakeholderId}`);
      }
      throw error;
    }
  }

  async attachArtifactToChunk(
    chunkLabel: string,
    request: CreateChunkArtifactRequest,
  ): Promise<ChunkArtifactResponse> {
    const chunk = await this.chunkRepository.findByLabel(chunkLabel, request.branchId);
    if (chunk === undefined) {
      throw new NotFoundException(`Chunk with label ${chunkLabel} not found in this scope`);
    }

    const artifact = await this.artifactRepository.findById(request.artifactId);
    if (artifact === undefined) {
      throw new NotFoundException(`Artifact ${request.artifactId} not found`);
    }

    let association: ChunkArtifactAssociation;
    try {
      association = new ChunkArtifactAssociation({
        chunkLabel,
        artifactId: request.artifactId,
        status: 'active',
        createdByStakeholderId: request.stakeholderId,
        ...(request.branchId === undefined
          ? {}
          : { branchId: request.branchId, originBranchId: request.branchId }),
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Invalid chunk artifact association';
      throw new BadRequestException(message);
    }

    try {
      const created = await this.chunkArtifactRepository.create(association);
      return toChunkArtifactResponse(created);
    } catch (error) {
      if (isForeignKeyViolation(error)) {
        throw new BadRequestException(`Unknown stakeholderId: ${request.stakeholderId}`);
      }
      throw error;
    }
  }

  async detachArtifactFromChunk(
    chunkLabel: string,
    artifactId: string,
    branchId: string,
    stakeholderId: string,
  ): Promise<ChunkArtifactResponse> {
    const chunk = await this.chunkRepository.findByLabel(chunkLabel, branchId);
    if (chunk === undefined) {
      throw new NotFoundException(`Chunk with label ${chunkLabel} not found in this scope`);
    }

    const artifact = await this.artifactRepository.findById(artifactId);
    if (artifact === undefined) {
      throw new NotFoundException(`Artifact ${artifactId} not found`);
    }

    let association: ChunkArtifactAssociation;
    try {
      association = new ChunkArtifactAssociation({
        chunkLabel,
        artifactId,
        status: 'deactivated',
        createdByStakeholderId: stakeholderId,
        branchId,
        originBranchId: branchId,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Invalid chunk artifact association';
      throw new BadRequestException(message);
    }

    try {
      const created = await this.chunkArtifactRepository.create(association);
      return toChunkArtifactResponse(created);
    } catch (error) {
      if (isForeignKeyViolation(error)) {
        throw new BadRequestException(`Unknown stakeholderId: ${stakeholderId}`);
      }
      throw error;
    }
  }

  /**
   * Set-based overlay read for `GET /chunks/:label/artifacts?branchId=` (Meridian IDEA-32/
   * IDEA-60/IDEA-62). 404s for an unknown chunk in the queried scope, mirroring the write paths
   * above, before delegating to `ChunkArtifactRepository.findEffectiveForChunk`.
   */
  async getEffectiveArtifactsForChunk(
    chunkLabel: string,
    branchId: string | undefined,
  ): Promise<EffectiveChunkArtifact[]> {
    const chunk = await this.chunkRepository.findByLabel(chunkLabel, branchId);
    if (chunk === undefined) {
      throw new NotFoundException(`Chunk with label ${chunkLabel} not found in this scope`);
    }

    return this.chunkArtifactRepository.findEffectiveForChunk(chunkLabel, branchId);
  }

  /**
   * Issues a signed, time-limited download token for `artifactId` (Meridian IDEA-85's resolution
   * of the IDEA-84 gap report). 404s if the artifact does not exist rather than minting a token
   * for a blob that can never be streamed.
   */
  async issueDownloadToken(artifactId: string): Promise<IssuedArtifactDownloadToken> {
    const artifact = await this.artifactRepository.findById(artifactId);
    if (artifact === undefined) {
      throw new NotFoundException(`Artifact ${artifactId} not found`);
    }

    return this.downloadTokenService.issue(artifactId);
  }

  /**
   * Verifies `token` and streams the referenced artifact's original bytes back (Meridian
   * IDEA-85). An invalid/expired/tampered token yields 401 (mirrors `BranchesController`'s
   * session-token-invalid handling); a token that verifies but whose artifact no longer exists
   * yields 404.
   */
  async streamArtifactContent(token: string): Promise<StreamedArtifactContent> {
    let claims;
    try {
      claims = this.downloadTokenService.verify(token);
    } catch (error) {
      if (error instanceof InvalidArtifactDownloadTokenError) {
        throw new UnauthorizedException(error.message);
      }
      throw error;
    }

    const artifact = await this.artifactRepository.findById(claims.artifactId);
    if (artifact === undefined) {
      throw new NotFoundException(`Artifact ${claims.artifactId} not found`);
    }

    return { stream: this.blobStore.createReadStream(artifact.uri), mimeType: artifact.mimeType };
  }
}
