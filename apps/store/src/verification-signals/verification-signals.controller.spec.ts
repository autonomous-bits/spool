import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { VerificationSignalResponse } from './verification-signal-response.dto.js';
import { VerificationSignalsController } from './verification-signals.controller.js';
import { VerificationSignalsService } from './verification-signals.service.js';

describe('VerificationSignalsController', () => {
  let controller: VerificationSignalsController;
  let service: Pick<VerificationSignalsService, 'create' | 'findAllForBranch'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [VerificationSignalsController],
      providers: [
        {
          provide: VerificationSignalsService,
          useValue: {
            create: vi.fn(),
            findAllForBranch: vi.fn(),
          } satisfies Pick<VerificationSignalsService, 'create' | 'findAllForBranch'>,
        },
      ],
    }).compile();

    controller = module.get(VerificationSignalsController);
    service = module.get(VerificationSignalsService);
  });

  const response = {
    id: 'signal-1',
    branchId: 'branch-1',
    verifierName: 'ci-evaluator',
    status: 'pass',
    reason: null,
    createdAt: new Date(),
  } satisfies VerificationSignalResponse;

  const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

  it('parses the body and delegates creation to VerificationSignalsService', async () => {
    vi.mocked(service.create).mockResolvedValue(response);

    const result = await controller.create(
      'branch-1',
      { verifierName: 'ci-evaluator', status: 'pass' },
      WORKSPACE_ID,
    );

    expect(result).toEqual(response);
    expect(service.create).toHaveBeenCalledWith(
      'branch-1',
      { verifierName: 'ci-evaluator', status: 'pass' },
      WORKSPACE_ID,
    );
  });

  it('delegates GET /branches/:id/verification-signals to VerificationSignalsService', async () => {
    vi.mocked(service.findAllForBranch).mockResolvedValue([response]);

    const result = await controller.findAll('branch-1', WORKSPACE_ID);

    expect(result).toEqual([response]);
    expect(service.findAllForBranch).toHaveBeenCalledWith('branch-1', WORKSPACE_ID);
  });
});
