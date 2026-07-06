import { ConflictException, NotFoundException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { VerificationSignal } from '../domain/verification-signal.js';
import { VerificationSignalRepository } from '../persistence/verification-signal.repository.js';
import { VerificationSignalsService } from './verification-signals.service.js';

describe('VerificationSignalsService', () => {
  let service: VerificationSignalsService;
  let repository: Pick<VerificationSignalRepository, 'create' | 'findByBranchId'>;

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
      ],
    }).compile();

    service = module.get(VerificationSignalsService);
    repository = module.get(VerificationSignalRepository);
  });

  it('creates a signal and returns its response shape', async () => {
    const signal = new VerificationSignal({
      branchId: 'branch-1',
      verifierName: 'ci-evaluator',
      status: 'pass',
    });
    vi.mocked(repository.create).mockResolvedValue({ kind: 'created', signal });

    const result = await service.create('branch-1', { verifierName: 'ci-evaluator', status: 'pass' });

    expect(result.id).toBe(signal.id);
    expect(result.branchId).toBe('branch-1');
    expect(result.status).toBe('pass');
    expect(result.reason).toBeNull();
    expect(repository.create).toHaveBeenCalledWith({
      branchId: 'branch-1',
      verifierName: 'ci-evaluator',
      status: 'pass',
    });
  });

  it('forwards an optional reason to the repository', async () => {
    const signal = new VerificationSignal({
      branchId: 'branch-1',
      verifierName: 'ci-evaluator',
      status: 'fail',
      reason: 'missing tests',
    });
    vi.mocked(repository.create).mockResolvedValue({ kind: 'created', signal });

    await service.create('branch-1', {
      verifierName: 'ci-evaluator',
      status: 'fail',
      reason: 'missing tests',
    });

    expect(repository.create).toHaveBeenCalledWith({
      branchId: 'branch-1',
      verifierName: 'ci-evaluator',
      status: 'fail',
      reason: 'missing tests',
    });
  });

  it('throws NotFoundException for an unknown branch', async () => {
    vi.mocked(repository.create).mockResolvedValue({ kind: 'not_found' });

    await expect(
      service.create('unknown-branch', { verifierName: 'ci-evaluator', status: 'pass' }),
    ).rejects.toThrow(NotFoundException);
  });

  it('throws ConflictException for a non-reviewable branch', async () => {
    vi.mocked(repository.create).mockResolvedValue({
      kind: 'not_reviewable',
      branchStatus: 'draft',
    });

    await expect(
      service.create('branch-1', { verifierName: 'ci-evaluator', status: 'pass' }),
    ).rejects.toThrow(ConflictException);
  });

  it('lists signals for a branch ordered as returned by the repository', async () => {
    const first = new VerificationSignal({
      branchId: 'branch-1',
      verifierName: 'first',
      status: 'pass',
    });
    const second = new VerificationSignal({
      branchId: 'branch-1',
      verifierName: 'second',
      status: 'fail',
    });
    vi.mocked(repository.findByBranchId).mockResolvedValue([first, second]);

    const result = await service.findAllForBranch('branch-1');

    expect(result.map((signal) => signal.id)).toEqual([first.id, second.id]);
    expect(repository.findByBranchId).toHaveBeenCalledWith('branch-1');
  });
});
