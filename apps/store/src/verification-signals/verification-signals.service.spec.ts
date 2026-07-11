import { ConflictException, ForbiddenException, NotFoundException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { VerificationSignal } from '../domain/verification-signal.js';
import { VerificationSignalRepository } from '../persistence/verification-signal.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { VerificationSignalsService } from './verification-signals.service.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';
const claims = {
  stakeholderId: '00000000-0000-0000-0000-000000000002',
  discipline: 'product',
  authTime: 1_752_000_000,
  workspaceId: WORKSPACE_ID,
} satisfies SessionTokenClaims;

describe('VerificationSignalsService', () => {
  let service: VerificationSignalsService;
  let repository: Pick<VerificationSignalRepository, 'create' | 'findByBranchId'>;
  let workspaceRepository: Pick<WorkspaceRepository, 'isMember'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        VerificationSignalsService,
        {
          provide: VerificationSignalRepository,
          useValue: {
            create: vi.fn(),
            findByBranchId: vi.fn(),
          } satisfies Pick<VerificationSignalRepository, 'create' | 'findByBranchId'>,
        },
        {
          provide: WorkspaceRepository,
          useValue: {
            isMember: vi.fn().mockResolvedValue(true),
          } satisfies Pick<WorkspaceRepository, 'isMember'>,
        },
      ],
    }).compile();

    service = module.get(VerificationSignalsService);
    repository = module.get(VerificationSignalRepository);
    workspaceRepository = module.get(WorkspaceRepository);
  });

  it('creates a signal and returns its response shape with claims-derived reporter identity', async () => {
    const signal = new VerificationSignal({
      workspaceId: WORKSPACE_ID,
      branchId: 'branch-1',
      reportedByStakeholderId: claims.stakeholderId,
      verifierName: 'ci-evaluator',
      status: 'pass',
    });
    vi.mocked(repository.create).mockResolvedValue({ kind: 'created', signal });

    const result = await service.create(
      'branch-1',
      { verifierName: 'ci-evaluator', status: 'pass' },
      WORKSPACE_ID,
      claims,
    );

    expect(result.id).toBe(signal.id);
    expect(result.branchId).toBe('branch-1');
    expect(result.reportedByStakeholderId).toBe(claims.stakeholderId);
    expect(result.status).toBe('pass');
    expect(result.reason).toBeNull();
    expect(workspaceRepository.isMember).toHaveBeenCalledWith(WORKSPACE_ID, claims.stakeholderId);
    expect(repository.create).toHaveBeenCalledWith({
      branchId: 'branch-1',
      workspaceId: WORKSPACE_ID,
      reportedByStakeholderId: claims.stakeholderId,
      verifierName: 'ci-evaluator',
      status: 'pass',
    });
  });

  it('forwards an optional reason to the repository', async () => {
    const signal = new VerificationSignal({
      workspaceId: WORKSPACE_ID,
      branchId: 'branch-1',
      reportedByStakeholderId: claims.stakeholderId,
      verifierName: 'ci-evaluator',
      status: 'fail',
      reason: 'missing tests',
    });
    vi.mocked(repository.create).mockResolvedValue({ kind: 'created', signal });

    await service.create(
      'branch-1',
      { verifierName: 'ci-evaluator', status: 'fail', reason: 'missing tests' },
      WORKSPACE_ID,
      claims,
    );

    expect(repository.create).toHaveBeenCalledWith({
      branchId: 'branch-1',
      workspaceId: WORKSPACE_ID,
      reportedByStakeholderId: claims.stakeholderId,
      verifierName: 'ci-evaluator',
      status: 'fail',
      reason: 'missing tests',
    });
  });

  it('throws NotFoundException for an unknown branch', async () => {
    vi.mocked(repository.create).mockResolvedValue({ kind: 'not_found' });

    await expect(
      service.create(
        'unknown-branch',
        { verifierName: 'ci-evaluator', status: 'pass' },
        WORKSPACE_ID,
        claims,
      ),
    ).rejects.toThrow(NotFoundException);
  });

  it('throws ConflictException for a non-reviewable branch', async () => {
    vi.mocked(repository.create).mockResolvedValue({
      kind: 'not_reviewable',
      branchStatus: 'draft',
    });

    await expect(
      service.create('branch-1', { verifierName: 'ci-evaluator', status: 'pass' }, WORKSPACE_ID, claims),
    ).rejects.toThrow(ConflictException);
  });

  it('throws ForbiddenException when the X-Workspace-Id header is missing on create', async () => {
    await expect(
      service.create('branch-1', { verifierName: 'ci-evaluator', status: 'pass' }, undefined, claims),
    ).rejects.toThrow(ForbiddenException);
    expect(workspaceRepository.isMember).not.toHaveBeenCalled();
    expect(repository.create).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException when the caller is no longer a workspace member', async () => {
    vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);

    await expect(
      service.create('branch-1', { verifierName: 'ci-evaluator', status: 'pass' }, WORKSPACE_ID, claims),
    ).rejects.toThrow(ForbiddenException);
    expect(repository.create).not.toHaveBeenCalled();
  });

  it('lists signals for a branch ordered as returned by the repository', async () => {
    const first = new VerificationSignal({
      workspaceId: WORKSPACE_ID,
      branchId: 'branch-1',
      reportedByStakeholderId: claims.stakeholderId,
      verifierName: 'first',
      status: 'pass',
    });
    const second = new VerificationSignal({
      workspaceId: WORKSPACE_ID,
      branchId: 'branch-1',
      reportedByStakeholderId: claims.stakeholderId,
      verifierName: 'second',
      status: 'fail',
    });
    vi.mocked(repository.findByBranchId).mockResolvedValue([first, second]);

    const result = await service.findAllForBranch('branch-1', WORKSPACE_ID, claims);

    expect(result.map((signal) => signal.id)).toEqual([first.id, second.id]);
    expect(result.map((signal) => signal.reportedByStakeholderId)).toEqual([
      claims.stakeholderId,
      claims.stakeholderId,
    ]);
    expect(repository.findByBranchId).toHaveBeenCalledWith('branch-1', WORKSPACE_ID);
  });

  it('throws ForbiddenException when the X-Workspace-Id header is missing on findAllForBranch', async () => {
    await expect(service.findAllForBranch('branch-1', undefined, claims)).rejects.toThrow(
      ForbiddenException,
    );
    expect(repository.findByBranchId).not.toHaveBeenCalled();
  });
});
