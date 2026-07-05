import { BadRequestException, ConflictException, NotFoundException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { Branch } from '../domain/branch.js';
import { BranchRepository } from '../persistence/branch.repository.js';
import { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import { BranchesService } from './branches.service.js';
import type { CreateBranchRequest } from './create-branch-request.dto.js';

function validRequest(overrides: Partial<CreateBranchRequest> = {}): CreateBranchRequest {
  return {
    name: 'feature-branch',
    discipline: 'product',
    stakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

function validClaims(overrides: Partial<SessionTokenClaims> = {}): SessionTokenClaims {
  return {
    stakeholderId: '00000000-0000-0000-0000-000000000001',
    discipline: 'product',
    authTime: 1_752_000_000,
    ...overrides,
  };
}

describe('BranchesService', () => {
  let branchRepository: Pick<BranchRepository, 'create' | 'findById' | 'submit'>;
  let stakeholderRepository: Pick<StakeholderRepository, 'findById'>;
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
          } satisfies Pick<BranchRepository, 'create' | 'findById' | 'submit'>,
        },
        {
          provide: StakeholderRepository,
          useValue: {
            findById: vi.fn(),
          } satisfies Pick<StakeholderRepository, 'findById'>,
        },
      ],
    }).compile();

    service = module.get(BranchesService);
    branchRepository = module.get(BranchRepository);
    stakeholderRepository = module.get(StakeholderRepository);
  });

  it('creates and returns the persisted branch', async () => {
    const request = validRequest();
    const persisted = new Branch({
      name: request.name,
      discipline: request.discipline,
      createdByStakeholderId: request.stakeholderId,
    });
    vi.mocked(branchRepository.create).mockResolvedValue(persisted);

    const result = await service.create(request);

    expect(result.id).toBe(persisted.id);
    expect(result.status).toBe('draft');
    expect(result.submittedAt).toBeNull();
    expect(branchRepository.create).toHaveBeenCalledOnce();
  });

  it('rejects a blank name via the domain entity as a BadRequestException', async () => {
    await expect(service.create(validRequest({ name: '   ' }))).rejects.toThrow(
      BadRequestException,
    );
    expect(branchRepository.create).not.toHaveBeenCalled();
  });

  it('translates a foreign key violation on an unknown stakeholderId into a BadRequestException', async () => {
    const request = validRequest({ stakeholderId: '00000000-0000-0000-0000-0000000000ff' });
    vi.mocked(branchRepository.create).mockRejectedValue(
      Object.assign(new Error('violates foreign key constraint'), { code: '23503' }),
    );

    await expect(service.create(request)).rejects.toThrow(BadRequestException);
  });

  it('translates a unique-name violation into a BadRequestException', async () => {
    const request = validRequest();
    vi.mocked(branchRepository.create).mockRejectedValue(
      Object.assign(new Error('duplicate key value violates unique constraint'), {
        code: '23505',
      }),
    );

    await expect(service.create(request)).rejects.toThrow(BadRequestException);
  });

  it('rethrows unrelated repository errors', async () => {
    const request = validRequest();
    vi.mocked(branchRepository.create).mockRejectedValue(new Error('connection lost'));

    await expect(service.create(request)).rejects.toThrow('connection lost');
  });

  it('submits a matching draft branch for a stakeholder with a valid discipline', async () => {
    const branch = new Branch({
      name: 'feature-branch',
      discipline: 'product',
      createdByStakeholderId: 'creator-1',
    });
    const submitted = new Branch({
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
      discipline: 'product',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(branchRepository.submit).mockResolvedValue(submitted);

    const result = await service.submit(branch.id, validClaims());

    expect(result.status).toBe('submitted');
    expect(result.submittedAt).toBe('2026-07-05T12:34:56.000Z');
    expect(stakeholderRepository.findById).toHaveBeenCalledWith(validClaims().stakeholderId);
    expect(branchRepository.submit).toHaveBeenCalledWith(branch.id);
  });

  it('returns 400 when the token stakeholder does not resolve', async () => {
    vi.mocked(stakeholderRepository.findById).mockResolvedValue(undefined);

    await expect(service.submit('branch-1', validClaims())).rejects.toThrow(BadRequestException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('returns 400 when the resolved stakeholder discipline is null', async () => {
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: null,
    });

    await expect(service.submit('branch-1', validClaims())).rejects.toThrow(BadRequestException);
    expect(branchRepository.findById).not.toHaveBeenCalled();
  });

  it('returns 404 when the branch does not exist', async () => {
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.submit('missing-branch', validClaims())).rejects.toThrow(
      NotFoundException,
    );
  });

  it('returns 409 when the stakeholder discipline does not match the branch discipline', async () => {
    const branch = new Branch({
      name: 'feature-branch',
      discipline: 'engineering',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);

    await expect(service.submit(branch.id, validClaims())).rejects.toThrow(ConflictException);
    expect(branchRepository.submit).not.toHaveBeenCalled();
  });

  it('returns 409 when the branch is not in draft status on the pre-check', async () => {
    const branch = new Branch({
      name: 'feature-branch',
      discipline: 'product',
      status: 'submitted',
      submittedAt: new Date('2026-07-05T12:34:56.000Z'),
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);

    await expect(service.submit(branch.id, validClaims())).rejects.toThrow(ConflictException);
    expect(branchRepository.submit).not.toHaveBeenCalled();
  });

  it('returns 409 when the repository loses the draft-submit race', async () => {
    const branch = new Branch({
      name: 'feature-branch',
      discipline: 'product',
      createdByStakeholderId: 'creator-1',
    });
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: validClaims().stakeholderId,
      discipline: 'product',
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(branch);
    vi.mocked(branchRepository.submit).mockResolvedValue(undefined);

    await expect(service.submit(branch.id, validClaims())).rejects.toThrow(ConflictException);
  });

  it('returns the branch from findById', async () => {
    const request = validRequest();
    const persisted = new Branch({
      name: request.name,
      discipline: request.discipline,
      createdByStakeholderId: request.stakeholderId,
    });
    vi.mocked(branchRepository.findById).mockResolvedValue(persisted);

    const result = await service.findById(persisted.id);

    expect(result.id).toBe(persisted.id);
    expect(result.submittedAt).toBeNull();
  });

  it('throws NotFoundException for an unknown id', async () => {
    vi.mocked(branchRepository.findById).mockResolvedValue(undefined);

    await expect(service.findById('missing')).rejects.toThrow(NotFoundException);
  });
});
