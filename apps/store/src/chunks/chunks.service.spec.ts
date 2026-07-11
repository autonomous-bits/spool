import { BadRequestException, ConflictException, ForbiddenException, NotFoundException } from '@nestjs/common';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Branch } from '../domain/branch.js';
import { Chunk } from '../domain/chunk.js';
import type { BranchRepository } from '../persistence/branch.repository.js';
import type { ChunkRepository } from '../persistence/chunk.repository.js';
import type { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { ChunksService } from './chunks.service.js';
import type { CreateChunkRequest } from './create-chunk-request.dto.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

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
  let chunkRepository: Pick<ChunkRepository, 'create' | 'findById' | 'search'>;
  let branchRepository: Pick<BranchRepository, 'findById'>;
  let workspaceRepository: Pick<WorkspaceRepository, 'isMember'>;
  let service: ChunksService;

  beforeEach(() => {
    chunkRepository = {
      create: vi.fn(),
      findById: vi.fn(),
      search: vi.fn(),
    };
    branchRepository = {
      findById: vi.fn(),
    };
    workspaceRepository = {
      isMember: vi.fn().mockResolvedValue(true),
    };
    service = new ChunksService(
      chunkRepository as ChunkRepository,
      branchRepository as BranchRepository,
      workspaceRepository as WorkspaceRepository,
    );
  });

  it('creates and returns the persisted chunk', async () => {
    const request = validRequest();
    const persisted = new Chunk({
      workspaceId: WORKSPACE_ID,
      label: request.label,
      content: request.content,
      discipline: request.discipline,
      chunkType: request.chunkType,
      contextKind: request.contextKind,
      createdByStakeholderId: request.stakeholderId,
    });
    vi.mocked(chunkRepository.create).mockResolvedValue(persisted);

    const result = await service.create(request, WORKSPACE_ID);

    expect(result.id).toBe(persisted.id);
    expect(result.status).toBe('draft');
    expect(result.branchId).toBeNull();
    expect(result.originBranchId).toBeNull();
    expect(chunkRepository.create).toHaveBeenCalledOnce();
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException when the X-Workspace-Id header is missing', async () => {
    const request = validRequest();

    await expect(service.create(request, undefined)).rejects.toThrow(ForbiddenException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException when the stakeholder is not a member of the header workspace', async () => {
    const request = validRequest();
    vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);

    await expect(service.create(request, WORKSPACE_ID)).rejects.toThrow(ForbiddenException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('translates a foreign key violation on an unknown stakeholderId into a BadRequestException', async () => {
    const request = validRequest({ stakeholderId: '00000000-0000-0000-0000-0000000000ff' });
    vi.mocked(chunkRepository.create).mockRejectedValue(
      Object.assign(new Error('violates foreign key constraint'), { code: '23503' }),
    );

    await expect(service.create(request, WORKSPACE_ID)).rejects.toThrow(BadRequestException);
  });

  it('rethrows unrelated repository errors', async () => {
    const request = validRequest();
    vi.mocked(chunkRepository.create).mockRejectedValue(new Error('connection lost'));

    await expect(service.create(request, WORKSPACE_ID)).rejects.toThrow('connection lost');
  });

  it('returns the chunk from findById', async () => {
    const request = validRequest();
    const persisted = new Chunk({
      workspaceId: WORKSPACE_ID,
      ...request,
      createdByStakeholderId: request.stakeholderId,
    });
    vi.mocked(chunkRepository.findById).mockResolvedValue(persisted);

    const result = await service.findById(persisted.id, WORKSPACE_ID, request.stakeholderId);

    expect(result.id).toBe(persisted.id);
  });

  it('throws ForbiddenException from findById when the header is missing', async () => {
    await expect(
      service.findById('some-id', undefined, '00000000-0000-0000-0000-000000000001'),
    ).rejects.toThrow(ForbiddenException);
    expect(chunkRepository.findById).not.toHaveBeenCalled();
  });

  it('throws NotFoundException for an unknown id', async () => {
    vi.mocked(chunkRepository.findById).mockResolvedValue(undefined);

    await expect(
      service.findById('missing', WORKSPACE_ID, '00000000-0000-0000-0000-000000000001'),
    ).rejects.toThrow(NotFoundException);
  });

  it('throws NotFoundException when branchId does not resolve to a branch', async () => {
    const request = validRequest({ branchId: '00000000-0000-0000-0000-0000000000b1' });
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.create(request, WORKSPACE_ID)).rejects.toThrow(NotFoundException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('throws ConflictException when branch discipline does not match the request', async () => {
    const request = validRequest({ branchId: '00000000-0000-0000-0000-0000000000b1', discipline: 'product' });
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'design-work',
      discipline: 'design',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);

    await expect(service.create(request, WORKSPACE_ID)).rejects.toThrow(ConflictException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('throws ConflictException when the branch is not in draft status', async () => {
    const request = validRequest({ branchId: '00000000-0000-0000-0000-0000000000b1' });
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'submitted-work',
      discipline: 'product',
      status: 'submitted',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);

    await expect(service.create(request, WORKSPACE_ID)).rejects.toThrow(ConflictException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('sets branchId and originBranchId on the persisted chunk when branchId is valid', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'product-work',
      discipline: 'product',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    const request = validRequest({ branchId: branch.id });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(chunkRepository.create).mockImplementation((chunk) => chunk);

    const result = await service.create(request, WORKSPACE_ID);

    expect(result.branchId).toBe(branch.id);
    expect(result.originBranchId).toBe(branch.id);
    const createdArg = vi.mocked(chunkRepository.create).mock.calls[0]?.[0];
    expect(createdArg?.branchId).toBe(branch.id);
    expect(createdArg?.originBranchId).toBe(branch.id);
  });
});
