import { BadRequestException, ConflictException, ForbiddenException, NotFoundException } from '@nestjs/common';
import { describe, expect, it, vi } from 'vitest';
import { Branch } from '../domain/branch.js';
import { Chunk } from '../domain/chunk.js';
import { Edge } from '../domain/edge.js';
import type { BranchRepository } from '../persistence/branch.repository.js';
import type { ChunkRepository } from '../persistence/chunk.repository.js';
import type { EdgeRepository } from '../persistence/edge.repository.js';
import type { WorkspaceRepository } from '../persistence/workspace.repository.js';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { EdgesService } from './edges.service.js';
import type { CreateEdgeRequest } from './create-edge-request.dto.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';
const STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';

function validClaims(overrides: Partial<SessionTokenClaims> = {}): SessionTokenClaims {
  return {
    stakeholderId: STAKEHOLDER_ID,
    workspaceId: WORKSPACE_ID,
    discipline: 'product',
    authTime: 1_752_000_000,
    ...overrides,
  };
}

function validRequest(overrides: Partial<CreateEdgeRequest> = {}): CreateEdgeRequest {
  return {
    fromChunkLabel: 'ATOMIC-1',
    toChunkLabel: 'ATOMIC-2',
    type: 'refines',
    discipline: 'product',
    stakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

function chunkWithLabel(label: string, discipline: Chunk['discipline'] = 'product'): Chunk {
  return new Chunk({
    workspaceId: WORKSPACE_ID,
    label,
    content: 'content',
    discipline,
    chunkType: 'feature',
    contextKind: 'permanent',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
  });
}

describe('EdgesService', () => {
  function setUp() {
    const edgeRepository: Pick<EdgeRepository, 'create' | 'findById'> = {
      create: vi.fn(),
      findById: vi.fn(),
    };
    const chunkRepository: Pick<ChunkRepository, 'findByLabel'> = {
      findByLabel: vi.fn(),
    };
    const branchRepository: Pick<BranchRepository, 'findById'> = {
      findById: vi.fn(),
    };
    const workspaceRepository: Pick<WorkspaceRepository, 'isMember'> = {
      isMember: vi.fn().mockResolvedValue(true),
    };
    const service = new EdgesService(
      edgeRepository as EdgeRepository,
      chunkRepository as ChunkRepository,
      branchRepository as BranchRepository,
      workspaceRepository as WorkspaceRepository,
    );
    return { edgeRepository, chunkRepository, branchRepository, workspaceRepository, service };
  }

  it('creates and returns the persisted edge (branchless)', async () => {
    const { edgeRepository, chunkRepository, service } = setUp();
    const request = validRequest();
    vi.mocked(chunkRepository.findByLabel).mockImplementation((label) =>
      chunkWithLabel(label),
    );
    vi.mocked(edgeRepository.create).mockImplementation((edge) => edge);

    const result = await service.create(request, WORKSPACE_ID, validClaims());

    expect(result.fromChunkLabel).toBe(request.fromChunkLabel);
    expect(result.toChunkLabel).toBe(request.toChunkLabel);
    expect(result.status).toBe('active');
    expect(result.branchId).toBeNull();
    expect(result.supersededByEdgeId).toBeNull();
    expect(chunkRepository.findByLabel).toHaveBeenCalledWith(request.fromChunkLabel, undefined, WORKSPACE_ID);
    expect(chunkRepository.findByLabel).toHaveBeenCalledWith(request.toChunkLabel, undefined, WORKSPACE_ID);
  });

  it('throws ForbiddenException when the X-Workspace-Id header is missing', async () => {
    const { edgeRepository, service } = setUp();

    await expect(service.create(validRequest(), undefined, validClaims())).rejects.toThrow(ForbiddenException);
    expect(edgeRepository.create).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException when the stakeholder is not a member of the header workspace', async () => {
    const { edgeRepository, workspaceRepository, service } = setUp();
    vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);

    await expect(service.create(validRequest(), WORKSPACE_ID, validClaims())).rejects.toThrow(ForbiddenException);
    expect(edgeRepository.create).not.toHaveBeenCalled();
  });

  it('throws NotFoundException when fromChunkLabel does not resolve', async () => {
    const { chunkRepository, edgeRepository, service } = setUp();
    vi.mocked(chunkRepository.findByLabel).mockResolvedValue(undefined);

    await expect(service.create(validRequest(), WORKSPACE_ID, validClaims())).rejects.toThrow(NotFoundException);
    expect(edgeRepository.create).not.toHaveBeenCalled();
  });

  it('throws NotFoundException when toChunkLabel does not resolve', async () => {
    const { chunkRepository, edgeRepository, service } = setUp();
    vi.mocked(chunkRepository.findByLabel).mockImplementation((label) =>
      label === 'ATOMIC-1' ? chunkWithLabel(label) : undefined,
    );

    await expect(service.create(validRequest(), WORKSPACE_ID, validClaims())).rejects.toThrow(NotFoundException);
    expect(edgeRepository.create).not.toHaveBeenCalled();
  });

  it('throws NotFoundException when branchId does not resolve to a branch', async () => {
    const { branchRepository, chunkRepository, edgeRepository, service } = setUp();
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(
      service.create(validRequest({ branchId: '00000000-0000-0000-0000-0000000000b1' }), WORKSPACE_ID, validClaims()),
    ).rejects.toThrow(NotFoundException);
    expect(chunkRepository.findByLabel).not.toHaveBeenCalled();
    expect(edgeRepository.create).not.toHaveBeenCalled();
  });

  it('throws ConflictException when branch discipline does not match the request', async () => {
    const { branchRepository, edgeRepository, service } = setUp();
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'design-work',
      discipline: 'design',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);

    await expect(
      service.create(validRequest({ branchId: branch.id, discipline: 'product' }), WORKSPACE_ID, validClaims()),
    ).rejects.toThrow(ConflictException);
    expect(edgeRepository.create).not.toHaveBeenCalled();
  });

  it('throws ConflictException when the branch is not in draft status', async () => {
    const { branchRepository, edgeRepository, service } = setUp();
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'submitted-work',
      discipline: 'product',
      status: 'submitted',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);

    await expect(
      service.create(validRequest({ branchId: branch.id }), WORKSPACE_ID, validClaims()),
    ).rejects.toThrow(ConflictException);
    expect(edgeRepository.create).not.toHaveBeenCalled();
  });

  it('scopes findByLabel lookups to the resolved branchId', async () => {
    const { branchRepository, chunkRepository, edgeRepository, service } = setUp();
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'product-work',
      discipline: 'product',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(chunkRepository.findByLabel).mockImplementation((label) =>
      chunkWithLabel(label),
    );
    vi.mocked(edgeRepository.create).mockImplementation((edge) => edge);

    const result = await service.create(validRequest({ branchId: branch.id }), WORKSPACE_ID, validClaims());

    expect(result.branchId).toBe(branch.id);
    expect(result.originBranchId).toBe(branch.id);
    expect(chunkRepository.findByLabel).toHaveBeenCalledWith('ATOMIC-1', branch.id, WORKSPACE_ID);
    expect(chunkRepository.findByLabel).toHaveBeenCalledWith('ATOMIC-2', branch.id, WORKSPACE_ID);
  });

  it('translates a foreign key violation on an unknown stakeholderId into a BadRequestException', async () => {
    const { chunkRepository, edgeRepository, service } = setUp();
    vi.mocked(chunkRepository.findByLabel).mockImplementation((label) =>
      chunkWithLabel(label),
    );
    vi.mocked(edgeRepository.create).mockRejectedValue(
      Object.assign(new Error('violates foreign key constraint'), { code: '23503' }),
    );

    await expect(service.create(validRequest(), WORKSPACE_ID, validClaims())).rejects.toThrow(BadRequestException);
  });

  it('translates a unique violation on a duplicate active edge into a ConflictException', async () => {
    const { chunkRepository, edgeRepository, service } = setUp();
    vi.mocked(chunkRepository.findByLabel).mockImplementation((label) =>
      chunkWithLabel(label),
    );
    vi.mocked(edgeRepository.create).mockRejectedValue(
      Object.assign(new Error('duplicate key value violates unique constraint'), {
        code: '23505',
      }),
    );

    await expect(service.create(validRequest(), WORKSPACE_ID, validClaims())).rejects.toThrow(ConflictException);
  });

  it('rethrows unrelated repository errors', async () => {
    const { chunkRepository, edgeRepository, service } = setUp();
    vi.mocked(chunkRepository.findByLabel).mockImplementation((label) =>
      chunkWithLabel(label),
    );
    vi.mocked(edgeRepository.create).mockRejectedValue(new Error('connection lost'));

    await expect(service.create(validRequest(), WORKSPACE_ID, validClaims())).rejects.toThrow('connection lost');
  });

  it('returns the edge from findById', async () => {
    const { edgeRepository, service } = setUp();
    const persisted = new Edge({
      workspaceId: WORKSPACE_ID,
      fromChunkLabel: 'ATOMIC-1',
      toChunkLabel: 'ATOMIC-2',
      type: 'refines',
      discipline: 'product',
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    vi.mocked(edgeRepository.findById).mockResolvedValue(persisted);

    const result = await service.findById(persisted.id, WORKSPACE_ID, validClaims());

    expect(result.id).toBe(persisted.id);
    expect(result.supersededByEdgeId).toBeNull();
  });

  it('throws ForbiddenException from findById when the header is missing', async () => {
    const { edgeRepository, service } = setUp();

    await expect(
      service.findById('some-id', undefined, validClaims()),
    ).rejects.toThrow(ForbiddenException);
    expect(edgeRepository.findById).not.toHaveBeenCalled();
  });

  it('throws NotFoundException for an unknown id', async () => {
    const { edgeRepository, service } = setUp();
    vi.mocked(edgeRepository.findById).mockResolvedValue(undefined);

    await expect(
      service.findById('missing', WORKSPACE_ID, validClaims()),
    ).rejects.toThrow(NotFoundException);
  });
});
