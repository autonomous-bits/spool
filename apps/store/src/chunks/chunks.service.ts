import { BadRequestException, Injectable, NotFoundException } from '@nestjs/common';
import { Chunk } from '../domain/chunk.js';
import { ChunkRepository } from '../persistence/chunk.repository.js';
import { toChunkResponse, type ChunkResponse } from './chunk-response.dto.js';
import type { CreateChunkRequest } from './create-chunk-request.dto.js';

const FOREIGN_KEY_VIOLATION = '23503';

function isForeignKeyViolation(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error as { code: unknown }).code === FOREIGN_KEY_VIOLATION
  );
}

/**
 * Application service for chunk capture and retrieval (Meridian IDEA-52/IDEA-34), sitting between
 * the HTTP controller and the `ChunkRepository` persistence layer.
 */
@Injectable()
export class ChunksService {
  constructor(private readonly chunkRepository: ChunkRepository) {}

  async create(request: CreateChunkRequest): Promise<ChunkResponse> {
    let chunk: Chunk;
    try {
      chunk = new Chunk({
        label: request.label,
        content: request.content,
        discipline: request.discipline,
        chunkType: request.chunkType,
        contextKind: request.contextKind,
        createdByStakeholderId: request.stakeholderId,
      });
    } catch (error) {
      // Domain invariants (blank label/content, etc.) surfaced as 400s, not 500s.
      const message = error instanceof Error ? error.message : 'Invalid chunk';
      throw new BadRequestException(message);
    }

    try {
      const created = await this.chunkRepository.create(chunk);
      return toChunkResponse(created);
    } catch (error) {
      if (isForeignKeyViolation(error)) {
        throw new BadRequestException(`Unknown stakeholderId: ${request.stakeholderId}`);
      }
      throw error;
    }
  }

  async findById(id: string): Promise<ChunkResponse> {
    const chunk = await this.chunkRepository.findById(id);
    if (chunk === undefined) {
      throw new NotFoundException(`Chunk ${id} not found`);
    }
    return toChunkResponse(chunk);
  }
}
