import { BadRequestException, ConflictException, NotFoundException } from '@nestjs/common';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Branch } from '../domain/branch.js';
import { Chunk } from '../domain/chunk.js';
import type { BranchRepository } from '../persistence/branch.repository.js';
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
  let chunkRepository: Pick<ChunkRepository, 'create' | 'findById'>;
  let branchRepository: Pick<BranchRepository, 'findById'>;
  let service: ChunksService;

  beforeEach(() => {
    chunkRepository = {
      create: vi.fn(),
      findById: vi.fn(),
    };
    branchRepository = {
      findById: vi.fn(),
    };
    service = new ChunksService(
      chunkRepository as ChunkRepository,
      branchRepository as BranchRepository,
    );
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
    vi.mocked(chunkRepository.create).mockResolvedValue(persisted);

    const result = await service.create(request);

    expect(result.id).toBe(persisted.id);
    expect(result.status).toBe('draft');
    expect(result.branchId).toBeNull();
    expect(result.originBranchId).toBeNull();
    expect(chunkRepository.create).toHaveBeenCalledOnce();
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('translates a foreign key violation on an unknown stakeholderId into a BadRequestException', async () => {
    const request = validRequest({ stakeholderId: '00000000-0000-0000-0000-0000000000ff' });
    vi.mocked(chunkRepository.create).mockRejectedValue(
      Object.assign(new Error('violates foreign key constraint'), { code: '23503' }),
    );

    await expect(service.create(request)).rejects.toThrow(BadRequestException);
  });

  it('rethrows unrelated repository errors', async () => {
    const request = validRequest();
    vi.mocked(chunkRepository.create).mockRejectedValue(new Error('connection lost'));

    await expect(service.create(request)).rejects.toThrow('connection lost');
  });

  it('returns the chunk from findById', async () => {
    const persisted = new Chunk({ ...validRequest(), createdByStakeholderId: validRequest().stakeholderId });
    vi.mocked(chunkRepository.findById).mockResolvedValue(persisted);

    const result = await service.findById(persisted.id);

    expect(result.id).toBe(persisted.id);
  });

  it('throws NotFoundException for an unknown id', async () => {
    vi.mocked(chunkRepository.findById).mockResolvedValue(undefined);

    await expect(service.findById('missing')).rejects.toThrow(NotFoundException);
  });

  it('throws NotFoundException when branchId does not resolve to a branch', async () => {
    const request = validRequest({ branchId: '00000000-0000-0000-0000-0000000000b1' });
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.create(request)).rejects.toThrow(NotFoundException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('throws ConflictException when branch discipline does not match the request', async () => {
    const request = validRequest({ branchId: '00000000-0000-0000-0000-0000000000b1', discipline: 'product' });
    const branch = new Branch({
      name: 'design-work',
      discipline: 'design',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);

    await expect(service.create(request)).rejects.toThrow(ConflictException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('throws ConflictException when the branch is not in draft status', async () => {
    const request = validRequest({ branchId: '00000000-0000-0000-0000-0000000000b1' });
    const branch = new Branch({
      name: 'submitted-work',
      discipline: 'product',
      status: 'submitted',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);

    await expect(service.create(request)).rejects.toThrow(ConflictException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('sets branchId and originBranchId on the persisted chunk when branchId is valid', async () => {
    const branch = new Branch({
      name: 'product-work',
      discipline: 'product',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    const request = validRequest({ branchId: branch.id });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(chunkRepository.create).mockImplementation(async (chunk) => chunk);

    const result = await service.create(request);

    expect(result.branchId).toBe(branch.id);
    expect(result.originBranchId).toBe(branch.id);
    const createdArg = vi.mocked(chunkRepository.create).mock.calls[0]?.[0];
    expect(createdArg?.branchId).toBe(branch.id);
    expect(createdArg?.originBranchId).toBe(branch.id);
  });
});
