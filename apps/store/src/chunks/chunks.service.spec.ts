import { BadRequestException, NotFoundException } from '@nestjs/common';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Chunk } from '../domain/chunk.js';
import type { ChunkRepository } from '../persistence/chunk.repository.js';
import { ChunksService } from './chunks.service.js';
import type { CreateChunkRequest } from './create-chunk-request.dto.js';

function validRequest(overrides: Partial<CreateChunkRequest> = {}): CreateChunkRequest {
  return {
    label: 'ATOMIC-1',
    content: 'A raw captured idea.',
    discipline: 'product',
    chunkType: 'feature',
    contextKind: 'permanent',
    stakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

describe('ChunksService', () => {
  let repository: Pick<ChunkRepository, 'create' | 'findById'>;
  let service: ChunksService;

  beforeEach(() => {
    repository = {
      create: vi.fn(),
      findById: vi.fn(),
    };
    service = new ChunksService(repository as ChunkRepository);
  });

  it('creates and returns the persisted chunk', async () => {
    const request = validRequest();
    const persisted = new Chunk({
      label: request.label,
      content: request.content,
      discipline: request.discipline,
      chunkType: request.chunkType,
      contextKind: request.contextKind,
      createdByStakeholderId: request.stakeholderId,
    });
    vi.mocked(repository.create).mockResolvedValue(persisted);

    const result = await service.create(request);

    expect(result.id).toBe(persisted.id);
    expect(result.status).toBe('draft');
    expect(repository.create).toHaveBeenCalledOnce();
  });

  it('translates a foreign key violation on an unknown stakeholderId into a BadRequestException', async () => {
    const request = validRequest({ stakeholderId: '00000000-0000-0000-0000-0000000000ff' });
    vi.mocked(repository.create).mockRejectedValue(
      Object.assign(new Error('violates foreign key constraint'), { code: '23503' }),
    );

    await expect(service.create(request)).rejects.toThrow(BadRequestException);
  });

  it('rethrows unrelated repository errors', async () => {
    const request = validRequest();
    vi.mocked(repository.create).mockRejectedValue(new Error('connection lost'));

    await expect(service.create(request)).rejects.toThrow('connection lost');
  });

  it('returns the chunk from findById', async () => {
    const persisted = new Chunk({ ...validRequest(), createdByStakeholderId: validRequest().stakeholderId });
    vi.mocked(repository.findById).mockResolvedValue(persisted);

    const result = await service.findById(persisted.id);

    expect(result.id).toBe(persisted.id);
  });

  it('throws NotFoundException for an unknown id', async () => {
    vi.mocked(repository.findById).mockResolvedValue(undefined);

    await expect(service.findById('missing')).rejects.toThrow(NotFoundException);
  });
});
