import { BadRequestException, ConflictException, ForbiddenException, NotFoundException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { Branch } from '../domain/branch.js';
import { BranchRepository } from '../persistence/branch.repository.js';
import { StakeholderDisciplineRepository } from '../persistence/stakeholder-discipline.repository.js';
import { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { BranchesService } from './branches.service.js';
import type { CreateBranchRequest } from './create-branch-request.dto.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

function validRequest(overrides: Partial<CreateBranchRequest> = {}): CreateBranchRequest {
  return {
    name: 'feature-branch',
    discipline: 'product',
    ...overrides,
  };
}

function validClaims(overrides: Partial<SessionTokenClaims> = {}): SessionTokenClaims {
  return {
    stakeholderId: '00000000-0000-0000-0000-000000000001',
    authTime: 1_752_000_000,
    workspaceId: WORKSPACE_ID,
    ...overrides,
  };
}

describe('BranchesService', () => {
  let branchRepository: Pick<
    BranchRepository,
    'create' | 'findById' | 'submit' | 'verify' | 'reject' | 'merge'
  >;
  let stakeholderRepository: Pick<StakeholderRepository, 'findById'>;
  let workspaceRepository: Pick<WorkspaceRepository, 'isMember'>;
  let stakeholderDisciplineRepository: Pick<StakeholderDisciplineRepository, 'isAllowed'>;
  let service: BranchesService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        BranchesService,
        {
          provide: BranchRepository,
          useValue: {
            create: vi.fn(),
            findById: vi.fn(),
            submit: vi.fn(),
            verify: vi.fn(),
            reject: vi.fn(),
            merge: vi.fn(),
          } satisfies Pick<
            BranchRepository,
            'create' | 'findById' | 'submit' | 'verify' | 'reject' | 'merge'
          >,
        },
        {
          provide: StakeholderRepository,
          useValue: {
            findById: vi.fn(),
          } satisfies Pick<StakeholderRepository, 'findById'>,
        },
        {
          provide: WorkspaceRepository,
          useValue: {
            isMember: vi.fn().mockResolvedValue(true),
          } satisfies Pick<WorkspaceRepository, 'isMember'>,
        },
        {
          provide: StakeholderDisciplineRepository,
          useValue: {
            isAllowed: vi.fn().mockResolvedValue(true),
          } satisfies Pick<StakeholderDisciplineRepository, 'isAllowed'>,
        },
      ],
    }).compile();

    service = module.get(BranchesService);
    branchRepository = module.get(BranchRepository);
    stakeholderRepository = module.get(StakeholderRepository);
    workspaceRepository = module.get(WorkspaceRepository);
    stakeholderDisciplineRepository = module.get(StakeholderDisciplineRepository);
  });

  it('creates and returns the persisted branch with claims-derived authorship', async () => {
    const request = validRequest();
    const claims = validClaims();
    const persisted = new Branch({
      workspaceId: WORKSPACE_ID,
      name: request.name,
      discipline: request.discipline,
      createdByStakeholderId: claims.stakeholderId,
    });
    vi.mocked(branchRepository.create).mockResolvedValue(persisted);

    const result = await service.create(request, WORKSPACE_ID, claims);

    expect(result.id).toBe(persisted.id);
    expect(result.status).toBe('draft');
    expect(result.submittedAt).toBeNull();
    expect(branchRepository.create).toHaveBeenCalledOnce();
    const createdBranch = vi.mocked(branchRepository.create).mock.calls[0]?.[0];
    expect(createdBranch?.createdByStakeholderId).toBe(claims.stakeholderId);
  });

  it('throws ForbiddenException when the X-Workspace-Id header is missing', async () => {
    await expect(service.create(validRequest(), undefined, validClaims())).rejects.toThrow(ForbiddenException);
    expect(branchRepository.create).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException when the stakeholder is not a member of the header workspace', async () => {
    vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);

    await expect(service.create(validRequest(), WORKSPACE_ID, validClaims())).rejects.toThrow(ForbiddenException);
    expect(branchRepository.create).not.toHaveBeenCalled();
  });

  it('rejects a blank name via the domain entity as a BadRequestException', async () => {
    await expect(service.create(validRequest({ name: '   ' }), WORKSPACE_ID, validClaims())).rejects.toThrow(
      BadRequestException,
    );
    expect(branchRepository.create).not.toHaveBeenCalled();
  });

  it('translates a foreign key violation on an unknown stakeholderId into a BadRequestException', async () => {
    const request = validRequest();
    vi.mocked(branchRepository.create).mockRejectedValue(
      Object.assign(new Error('violates foreign key constraint'), { code: '23503' }),
    );

    await expect(
      service.create(request, WORKSPACE_ID, validClaims({ stakeholderId: '00000000-0000-0000-0000-0000000000ff' })),
    ).rejects.toThrow(BadRequestException);
  });

  it('translates a unique-name violation into a BadRequestException', async () => {
    const request = validRequest();
    vi.mocked(branchRepository.create).mockRejectedValue(
      Object.assign(new Error('duplicate key value violates unique constraint'), {
        code: '23505',
      }),
    );

    await expect(service.create(request, WORKSPACE_ID, validClaims())).rejects.toThrow(BadRequestException);
  });

  it('rethrows unrelated repository errors', async () => {
    const request = validRequest();
    vi.mocked(branchRepository.create).mockRejectedValue(new Error('connection lost'));

    await expect(service.create(request, WORKSPACE_ID, validClaims())).rejects.toThrow('connection lost');
  });

  it('throws ForbiddenException from submit when the X-Workspace-Id header is missing', async () => {
    await expect(service.submit('branch-1', undefined, validClaims(), undefined)).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException from submit when the token workspace claim does not match the header', async () => {
    await expect(
      service.submit('branch-1', WORKSPACE_ID, validClaims({ workspaceId: '00000000-0000-0000-0000-00000000beef' }), undefined),
    ).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('submits a matching draft branch for a stakeholder allowed to act as the branch discipline', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'product',
      createdByStakeholderId: 'creator-1',
    });
    const submitted = new Branch({
      workspaceId: WORKSPACE_ID,
      id: branch.id,
      name: branch.name,
      discipline: branch.discipline,
      status: 'submitted',
      divergedAt: branch.divergedAt,
      submittedAt: new Date('2026-07-05T12:34:56.000Z'),
      createdByStakeholderId: branch.createdByStakeholderId,
      createdAt: branch.createdAt,
      updatedAt: new Date('2026-07-05T12:34:56.000Z'),
    });
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(branchRepository.submit).mockResolvedValue(submitted);
    vi.mocked(stakeholderDisciplineRepository.isAllowed).mockResolvedValue(true);

    const result = await service.submit(branch.id, WORKSPACE_ID, validClaims(), 'product');

    expect(result.status).toBe('submitted');
    expect(result.submittedAt).toBe('2026-07-05T12:34:56.000Z');
    expect(stakeholderRepository.findById).toHaveBeenCalledWith(validClaims().stakeholderId);
    expect(stakeholderDisciplineRepository.isAllowed).toHaveBeenCalledWith(
      WORKSPACE_ID,
      validClaims().stakeholderId,
      'product',
    );
    expect(branchRepository.submit).toHaveBeenCalledWith(branch.id, WORKSPACE_ID);
  });

  it('returns 404 when the branch does not exist, before any stakeholder lookup', async () => {
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.submit('missing-branch', WORKSPACE_ID, validClaims(), 'product')).rejects.toThrow(
      NotFoundException,
    );
    expect(stakeholderRepository.findById).not.toHaveBeenCalled();
  });

  it('returns 400 when the token stakeholder does not resolve', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'product',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue(undefined);

    await expect(service.submit(branch.id, WORKSPACE_ID, validClaims(), 'product')).rejects.toThrow(BadRequestException);
    expect(branchRepository.submit).not.toHaveBeenCalled();
  });

  it('returns 400 when activeDiscipline is missing', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'product',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });

    await expect(service.submit(branch.id, WORKSPACE_ID, validClaims(), undefined)).rejects.toThrow(
      BadRequestException,
    );
    expect(branchRepository.submit).not.toHaveBeenCalled();
  });

  it('returns 400 when activeDiscipline is not a valid vocabulary value', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'product',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });

    await expect(service.submit(branch.id, WORKSPACE_ID, validClaims(), 'bogus')).rejects.toThrow(
      BadRequestException,
    );
    expect(branchRepository.submit).not.toHaveBeenCalled();
  });

  it('returns 403 when activeDiscipline is a valid vocabulary value but not in the allow-list', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderDisciplineRepository.isAllowed).mockResolvedValue(false);

    await expect(service.submit(branch.id, WORKSPACE_ID, validClaims(), 'engineering')).rejects.toThrow(
      ForbiddenException,
    );
    expect(branchRepository.submit).not.toHaveBeenCalled();
  });

  it('returns 403 when the stakeholder is not allowed to act as the branch discipline (G21 SG1 allow-list)', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderDisciplineRepository.isAllowed).mockResolvedValue(false);

    await expect(service.submit(branch.id, WORKSPACE_ID, validClaims(), 'engineering')).rejects.toThrow(ForbiddenException);
    expect(branchRepository.submit).not.toHaveBeenCalled();
  });

  it('returns 409 when the branch is not in draft status on the pre-check', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'product',
      status: 'submitted',
      submittedAt: new Date('2026-07-05T12:34:56.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);

    await expect(service.submit(branch.id, WORKSPACE_ID, validClaims(), 'product')).rejects.toThrow(ConflictException);
    expect(branchRepository.submit).not.toHaveBeenCalled();
  });

  it('returns 409 when the repository loses the draft-submit race', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'product',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(branchRepository.submit).mockResolvedValue(undefined);

    await expect(service.submit(branch.id, WORKSPACE_ID, validClaims(), 'product')).rejects.toThrow(ConflictException);
  });

  it('throws ForbiddenException from verify when the X-Workspace-Id header is missing', async () => {
    await expect(service.verify('branch-1', undefined, validClaims())).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException from verify when the token workspace claim does not match the header', async () => {
    await expect(
      service.verify('branch-1', WORKSPACE_ID, validClaims({ workspaceId: '00000000-0000-0000-0000-00000000beef' })),
    ).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('verifies a submitted branch regardless of the acting stakeholder discipline', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'submitted',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    const verified = new Branch({
      workspaceId: WORKSPACE_ID,
      id: branch.id,
      name: branch.name,
      discipline: branch.discipline,
      status: 'verified',
      divergedAt: branch.divergedAt,
      submittedAt: branch.submittedAt,
      verifiedAt: new Date('2026-07-05T13:00:00.000Z'),
      createdByStakeholderId: branch.createdByStakeholderId,
      createdAt: branch.createdAt,
      updatedAt: new Date('2026-07-05T13:00:00.000Z'),
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.verify).mockResolvedValue(verified);

    const result = await service.verify(branch.id, WORKSPACE_ID, validClaims());

    expect(result.status).toBe('verified');
    expect(result.verifiedAt).toBe('2026-07-05T13:00:00.000Z');
    expect(branchRepository.verify).toHaveBeenCalledWith(branch.id, WORKSPACE_ID);
  });

  it('verifies a submitted branch for a stakeholder with a null discipline', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'submitted',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    const verified = new Branch({
      workspaceId: WORKSPACE_ID,
      id: branch.id,
      name: branch.name,
      discipline: branch.discipline,
      status: 'verified',
      divergedAt: branch.divergedAt,
      submittedAt: branch.submittedAt,
      verifiedAt: new Date('2026-07-05T13:00:00.000Z'),
      createdByStakeholderId: branch.createdByStakeholderId,
      createdAt: branch.createdAt,
      updatedAt: new Date('2026-07-05T13:00:00.000Z'),
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });
    vi.mocked(branchRepository.verify).mockResolvedValue(verified);

    const result = await service.verify(branch.id, WORKSPACE_ID, validClaims());

    expect(result.status).toBe('verified');
  });

  it('returns 404 when verifying an unknown branch, before any stakeholder lookup', async () => {
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.verify('missing-branch', WORKSPACE_ID, validClaims())).rejects.toThrow(
      NotFoundException,
    );
    expect(stakeholderRepository.findById).not.toHaveBeenCalled();
  });

  it('returns 400 when verifying with a token stakeholder that does not resolve', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'submitted',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue(undefined);

    await expect(service.verify(branch.id, WORKSPACE_ID, validClaims())).rejects.toThrow(BadRequestException);
    expect(branchRepository.verify).not.toHaveBeenCalled();
  });

  it('returns 409 when verifying a branch that is not submitted', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });

    await expect(service.verify(branch.id, WORKSPACE_ID, validClaims())).rejects.toThrow(ConflictException);
    expect(branchRepository.verify).not.toHaveBeenCalled();
  });

  it('returns 409 when the repository loses the verify race', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'submitted',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.verify).mockResolvedValue(undefined);

    await expect(service.verify(branch.id, WORKSPACE_ID, validClaims())).rejects.toThrow(ConflictException);
  });

  it('throws ForbiddenException from reject when the X-Workspace-Id header is missing', async () => {
    await expect(service.reject('branch-1', undefined, validClaims())).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException from reject when the token workspace claim does not match the header', async () => {
    await expect(
      service.reject('branch-1', WORKSPACE_ID, validClaims({ workspaceId: '00000000-0000-0000-0000-00000000beef' })),
    ).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('rejects a submitted branch, clearing verifiedAt and submittedAt', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'submitted',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    const rejected = new Branch({
      workspaceId: WORKSPACE_ID,
      id: branch.id,
      name: branch.name,
      discipline: branch.discipline,
      status: 'draft',
      divergedAt: branch.divergedAt,
      createdByStakeholderId: branch.createdByStakeholderId,
      createdAt: branch.createdAt,
      updatedAt: new Date('2026-07-05T13:00:00.000Z'),
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.reject).mockResolvedValue(rejected);

    const result = await service.reject(branch.id, WORKSPACE_ID, validClaims());

    expect(result.status).toBe('draft');
    expect(result.verifiedAt).toBeNull();
    expect(result.submittedAt).toBeNull();
    expect(branchRepository.reject).toHaveBeenCalledWith(branch.id, WORKSPACE_ID);
  });

  it('rejects a verified branch', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'verified',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      verifiedAt: new Date('2026-07-05T12:30:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    const rejected = new Branch({
      workspaceId: WORKSPACE_ID,
      id: branch.id,
      name: branch.name,
      discipline: branch.discipline,
      status: 'draft',
      divergedAt: branch.divergedAt,
      createdByStakeholderId: branch.createdByStakeholderId,
      createdAt: branch.createdAt,
      updatedAt: new Date('2026-07-05T13:00:00.000Z'),
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.reject).mockResolvedValue(rejected);

    const result = await service.reject(branch.id, WORKSPACE_ID, validClaims());

    expect(result.status).toBe('draft');
  });

  it('returns 404 when rejecting an unknown branch', async () => {
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.reject('missing-branch', WORKSPACE_ID, validClaims())).rejects.toThrow(
      NotFoundException,
    );
  });

  it('returns 409 when rejecting a draft branch', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });

    await expect(service.reject(branch.id, WORKSPACE_ID, validClaims())).rejects.toThrow(ConflictException);
    expect(branchRepository.reject).not.toHaveBeenCalled();
  });

  it('returns 409 when the repository loses the reject race', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'submitted',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.reject).mockResolvedValue(undefined);

    await expect(service.reject(branch.id, WORKSPACE_ID, validClaims())).rejects.toThrow(ConflictException);
  });

  it('throws ForbiddenException from merge when the X-Workspace-Id header is missing', async () => {
    await expect(service.merge('branch-1', undefined, validClaims())).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException from merge when the token workspace claim does not match the header', async () => {
    await expect(
      service.merge('branch-1', WORKSPACE_ID, validClaims({ workspaceId: '00000000-0000-0000-0000-00000000beef' })),
    ).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('merges a verified branch, returning mergedAt', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'verified',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      verifiedAt: new Date('2026-07-05T12:30:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    const merged = new Branch({
      workspaceId: WORKSPACE_ID,
      id: branch.id,
      name: branch.name,
      discipline: branch.discipline,
      status: 'merged',
      divergedAt: branch.divergedAt,
      submittedAt: branch.submittedAt,
      verifiedAt: branch.verifiedAt,
      mergedAt: new Date('2026-07-05T13:00:00.000Z'),
      mergedByStakeholderId: validClaims().stakeholderId,
      createdByStakeholderId: branch.createdByStakeholderId,
      createdAt: branch.createdAt,
      updatedAt: new Date('2026-07-05T13:00:00.000Z'),
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.merge).mockResolvedValue({ kind: 'merged', branch: merged });

    const result = await service.merge(branch.id, WORKSPACE_ID, validClaims());

    expect(result.status).toBe('merged');
    expect(result.mergedAt).toBe('2026-07-05T13:00:00.000Z');
    expect(branchRepository.merge).toHaveBeenCalledWith(branch.id, WORKSPACE_ID, validClaims().stakeholderId);
  });

  it('merges a verified branch for a stakeholder with a null discipline (discipline-agnostic)', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'verified',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      verifiedAt: new Date('2026-07-05T12:30:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    const merged = new Branch({
      workspaceId: WORKSPACE_ID,
      id: branch.id,
      name: branch.name,
      discipline: branch.discipline,
      status: 'merged',
      divergedAt: branch.divergedAt,
      submittedAt: branch.submittedAt,
      verifiedAt: branch.verifiedAt,
      mergedAt: new Date('2026-07-05T13:00:00.000Z'),
      mergedByStakeholderId: validClaims().stakeholderId,
      createdByStakeholderId: branch.createdByStakeholderId,
      createdAt: branch.createdAt,
      updatedAt: new Date('2026-07-05T13:00:00.000Z'),
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });
    vi.mocked(branchRepository.merge).mockResolvedValue({ kind: 'merged', branch: merged });

    const result = await service.merge(branch.id, WORKSPACE_ID, validClaims());

    expect(result.status).toBe('merged');
  });

  it('returns 404 when merging an unknown branch, before any stakeholder lookup', async () => {
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.merge('missing-branch', WORKSPACE_ID, validClaims())).rejects.toThrow(
      NotFoundException,
    );
    expect(stakeholderRepository.findById).not.toHaveBeenCalled();
  });

  it('returns 400 when merging with a token stakeholder that does not resolve', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'verified',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      verifiedAt: new Date('2026-07-05T12:30:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue(undefined);

    await expect(service.merge(branch.id, WORKSPACE_ID, validClaims())).rejects.toThrow(BadRequestException);
    expect(branchRepository.merge).not.toHaveBeenCalled();
  });

  it('returns 409 when merging a branch that is not verified', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'submitted',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });

    await expect(service.merge(branch.id, WORKSPACE_ID, validClaims())).rejects.toThrow(ConflictException);
    expect(branchRepository.merge).not.toHaveBeenCalled();
  });

  it('returns 409 when the repository loses the merge race', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'verified',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      verifiedAt: new Date('2026-07-05T12:30:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.merge).mockResolvedValue(undefined);

    await expect(service.merge(branch.id, WORKSPACE_ID, validClaims())).rejects.toThrow(ConflictException);
  });

  it('returns 409 with a distinguishing message when the merge collides with mainline', async () => {
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'feature-branch',
      discipline: 'engineering',
      status: 'verified',
      submittedAt: new Date('2026-07-05T12:00:00.000Z'),
      verifiedAt: new Date('2026-07-05T12:30:00.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.merge).mockResolvedValue({
      kind: 'conflict',
      reason: 'mainline chunk label collision: some-label',
    });

    await expect(service.merge(branch.id, WORKSPACE_ID, validClaims())).rejects.toThrow(
      'mainline chunk label collision: some-label',
    );
  });

  it('throws ForbiddenException from findById when the header is missing', async () => {
    await expect(
      service.findById('some-id', undefined, validClaims()),
    ).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException from findById when the stakeholder is not a member of the header workspace', async () => {
    vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);

    await expect(
      service.findById('some-id', WORKSPACE_ID, validClaims()),
    ).rejects.toThrow(ForbiddenException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('returns the branch from findById', async () => {
    const request = validRequest();
    const persisted = new Branch({
      workspaceId: WORKSPACE_ID,
      name: request.name,
      discipline: request.discipline,
      createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(persisted);

    const result = await service.findById(persisted.id, WORKSPACE_ID, validClaims());

    expect(result.id).toBe(persisted.id);
    expect(result.submittedAt).toBeNull();
  });

  it('throws NotFoundException for an unknown id', async () => {
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.findById('missing', WORKSPACE_ID, validClaims())).rejects.toThrow(NotFoundException);
  });
});
