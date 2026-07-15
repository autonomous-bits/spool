import { BadRequestException, ConflictException, ForbiddenException, NotFoundException } from '@nestjs/common';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Branch } from '../domain/branch.js';
import { Chunk } from '../domain/chunk.js';
import type { BranchRepository } from '../persistence/branch.repository.js';
import type { ChunkRepository } from '../persistence/chunk.repository.js';
import type { StakeholderDisciplineRepository } from '../persistence/stakeholder-discipline.repository.js';
import type { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import type { WorkspaceRepository } from '../persistence/workspace.repository.js';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { ChunksService } from './chunks.service.js';
import type { CreateChunkRequest } from './create-chunk-request.dto.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';
const STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';

function validRequest(overrides: Partial<CreateChunkRequest> = {}): CreateChunkRequest {
  return {
    label: 'ATOMIC-1',
    content: 'A raw captured idea.',
    discipline: 'product',
    chunkType: 'feature',
    contextKind: 'permanent',
    ...overrides,
  };
}

function validClaims(overrides: Partial<SessionTokenClaims> = {}): SessionTokenClaims {
  return {
    stakeholderId: STAKEHOLDER_ID,
    workspaceId: WORKSPACE_ID,
    authTime: 1_752_000_000,
    ...overrides,
  };
}

describe('ChunksService', () => {
  let chunkRepository: Pick<ChunkRepository, 'create' | 'findById' | 'search'>;
  let branchRepository: Pick<BranchRepository, 'findById'>;
  let workspaceRepository: Pick<WorkspaceRepository, 'isMember'>;
  let stakeholderDisciplineRepository: Pick<StakeholderDisciplineRepository, 'isAllowed'>;
  let stakeholderRepository: Pick<StakeholderRepository, 'findById'>;
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
    stakeholderDisciplineRepository = {
      isAllowed: vi.fn().mockResolvedValue(true),
    };
    stakeholderRepository = {
      findById: vi.fn().mockResolvedValue({ id: STAKEHOLDER_ID, discipline: null }),
    };
    service = new ChunksService(
      chunkRepository as ChunkRepository,
      branchRepository as BranchRepository,
      workspaceRepository as WorkspaceRepository,
      stakeholderDisciplineRepository as StakeholderDisciplineRepository,
      stakeholderRepository as StakeholderRepository,
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
      createdByStakeholderId: STAKEHOLDER_ID,
    });
    vi.mocked(chunkRepository.create).mockResolvedValue(persisted);

    const result = await service.create(request, WORKSPACE_ID, validClaims());

    expect(result.id).toBe(persisted.id);
    expect(result.status).toBe('draft');
    expect(result.branchId).toBeNull();
    expect(result.originBranchId).toBeNull();
    expect(chunkRepository.create).toHaveBeenCalledOnce();
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException when the X-Workspace-Id header is missing', async () => {
    const request = validRequest();

    await expect(service.create(request, undefined, validClaims())).rejects.toThrow(ForbiddenException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException when the stakeholder is not a member of the header workspace', async () => {
    const request = validRequest();
    vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);

    await expect(service.create(request, WORKSPACE_ID, validClaims())).rejects.toThrow(ForbiddenException);
    expect(chunkRepository.create).not.toHaveBeenCalled();
  });

  it('translates a foreign key violation on an unknown stakeholderId into a BadRequestException', async () => {
    const request = validRequest();
    const claims = validClaims({ stakeholderId: '00000000-0000-0000-0000-0000000000ff' });
    vi.mocked(chunkRepository.create).mockRejectedValue(
      Object.assign(new Error('violates foreign key constraint'), { code: '23503' }),
    );

    await expect(service.create(request, WORKSPACE_ID, claims)).rejects.toThrow(BadRequestException);
  });

  it('rethrows unrelated repository errors', async () => {
    const request = validRequest();
    vi.mocked(chunkRepository.create).mockRejectedValue(new Error('connection lost'));

    await expect(service.create(request, WORKSPACE_ID, validClaims())).rejects.toThrow('connection lost');
  });

  it('returns the chunk from findById', async () => {
    const request = validRequest();
    const persisted = new Chunk({
      workspaceId: WORKSPACE_ID,
      ...request,
      createdByStakeholderId: STAKEHOLDER_ID,
    });
    vi.mocked(chunkRepository.findById).mockResolvedValue(persisted);

    const result = await service.findById(persisted.id, WORKSPACE_ID, validClaims());

    expect(result.id).toBe(persisted.id);
  });

  it('throws ForbiddenException from findById when the header is missing', async () => {
    await expect(
      service.findById('some-id', undefined, validClaims()),
    ).rejects.toThrow(ForbiddenException);
    expect(chunkRepository.findById).not.toHaveBeenCalled();
  });

  it('throws NotFoundException for an unknown id', async () => {
    vi.mocked(chunkRepository.findById).mockResolvedValue(undefined);

    await expect(
      service.findById('missing', WORKSPACE_ID, validClaims()),
    ).rejects.toThrow(NotFoundException);
  });

  it('throws NotFoundException when branchId does not resolve to a branch', async () => {
    const request = validRequest({ branchId: '00000000-0000-0000-0000-0000000000b1' });
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.create(request, WORKSPACE_ID, validClaims())).rejects.toThrow(NotFoundException);
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

    await expect(service.create(request, WORKSPACE_ID, validClaims())).rejects.toThrow(ConflictException);
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

    await expect(service.create(request, WORKSPACE_ID, validClaims())).rejects.toThrow(ConflictException);
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

    const result = await service.create(request, WORKSPACE_ID, validClaims());

    expect(result.branchId).toBe(branch.id);
    expect(result.originBranchId).toBe(branch.id);
    const createdArg = vi.mocked(chunkRepository.create).mock.calls[0]?.[0];
    expect(createdArg?.branchId).toBe(branch.id);
    expect(createdArg?.originBranchId).toBe(branch.id);
  });

  describe('branch-scoped search/getNeighbourhood (G21 SG4 activeDiscipline gate)', () => {
    function branchScopedRequestFilters(branchId: string) {
      return { workspaceId: WORKSPACE_ID, branchId } as const;
    }

    it('search() succeeds when activeDiscipline is allowed for the stakeholder', async () => {
      const branch = new Branch({
        workspaceId: WORKSPACE_ID,
        name: 'design-work',
        discipline: 'design',
        createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
      });
      vi.mocked(branchRepository.findById).mockResolvedValue(branch);
      vi.mocked(stakeholderDisciplineRepository.isAllowed).mockResolvedValue(true);
      vi.mocked(chunkRepository.search).mockResolvedValue({ chunks: [], nextCursor: null });

      await service.search(branchScopedRequestFilters(branch.id), 10, validClaims(), undefined, 'design');

      expect(stakeholderDisciplineRepository.isAllowed).toHaveBeenCalledWith(
        WORKSPACE_ID,
        STAKEHOLDER_ID,
        'design',
      );
    });

    it('search() throws BadRequestException when activeDiscipline is missing and branchId is present', async () => {
      const branch = new Branch({
        workspaceId: WORKSPACE_ID,
        name: 'design-work',
        discipline: 'design',
        createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
      });
      vi.mocked(branchRepository.findById).mockResolvedValue(branch);

      await expect(
        service.search(branchScopedRequestFilters(branch.id), 10, validClaims(), undefined, undefined),
      ).rejects.toThrow(BadRequestException);
      expect(chunkRepository.search).not.toHaveBeenCalled();
    });

    it('search() throws ForbiddenException when activeDiscipline is a valid vocabulary value but disallowed', async () => {
      const branch = new Branch({
        workspaceId: WORKSPACE_ID,
        name: 'design-work',
        discipline: 'design',
        createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
      });
      vi.mocked(branchRepository.findById).mockResolvedValue(branch);
      vi.mocked(stakeholderDisciplineRepository.isAllowed).mockResolvedValue(false);

      await expect(
        service.search(branchScopedRequestFilters(branch.id), 10, validClaims(), undefined, 'design'),
      ).rejects.toThrow(ForbiddenException);
      expect(chunkRepository.search).not.toHaveBeenCalled();
    });

    it('getNeighbourhood() throws BadRequestException when activeDiscipline is missing and branchId is present', async () => {
      const branch = new Branch({
        workspaceId: WORKSPACE_ID,
        name: 'design-work',
        discipline: 'design',
        createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
      });
      vi.mocked(branchRepository.findById).mockResolvedValue(branch);

      await expect(
        service.getNeighbourhood('chunk-1', WORKSPACE_ID, validClaims(), 1, branch.id, undefined),
      ).rejects.toThrow(BadRequestException);
      expect(chunkRepository.findById).not.toHaveBeenCalled();
    });

    it('getNeighbourhood() throws ForbiddenException when activeDiscipline is a valid vocabulary value but disallowed', async () => {
      const branch = new Branch({
        workspaceId: WORKSPACE_ID,
        name: 'design-work',
        discipline: 'design',
        createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
      });
      vi.mocked(branchRepository.findById).mockResolvedValue(branch);
      vi.mocked(stakeholderDisciplineRepository.isAllowed).mockResolvedValue(false);

      await expect(
        service.getNeighbourhood('chunk-1', WORKSPACE_ID, validClaims(), 1, branch.id, 'design'),
      ).rejects.toThrow(ForbiddenException);
      expect(chunkRepository.findById).not.toHaveBeenCalled();
    });
  });
});
