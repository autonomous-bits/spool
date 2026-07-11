import { UnauthorizedException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import type { VerificationSignalResponse } from './verification-signal-response.dto.js';
import { VerificationSignalsController } from './verification-signals.controller.js';
import { VerificationSignalsService } from './verification-signals.service.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';
const claims = {
  stakeholderId: 'stakeholder-1',
  discipline: 'product',
  authTime: 1_752_000_000,
  workspaceId: WORKSPACE_ID,
} satisfies SessionTokenClaims;

describe('VerificationSignalsController', () => {
  let controller: VerificationSignalsController;
  let service: Pick<VerificationSignalsService, 'create' | 'findAllForBranch'>;
  let sessionTokenService: Pick<SessionTokenService, 'verify'>;

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
        {
          provide: SessionTokenService,
          useValue: {
            verify: vi.fn(),
          } satisfies Pick<SessionTokenService, 'verify'>,
        },
      ],
    }).compile();

    controller = module.get(VerificationSignalsController);
    service = module.get(VerificationSignalsService);
    sessionTokenService = module.get(SessionTokenService);
  });

  const response = {
    id: 'signal-1',
    branchId: 'branch-1',
    reportedByStakeholderId: 'stakeholder-1',
    verifierName: 'ci-evaluator',
    status: 'pass',
    reason: null,
    createdAt: new Date(),
  } satisfies VerificationSignalResponse;

  it('verifies the bearer token, parses the body, and delegates creation to VerificationSignalsService', async () => {
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.create).mockResolvedValue(response);

    const result = await controller.create(
      'branch-1',
      { verifierName: 'ci-evaluator', status: 'pass', reportedByStakeholderId: 'client-value' },
      'Bearer signed-token',
      WORKSPACE_ID,
    );

    expect(result).toEqual(response);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.create).toHaveBeenCalledWith(
      'branch-1',
      { verifierName: 'ci-evaluator', status: 'pass' },
      WORKSPACE_ID,
      claims,
    );
  });

  it('rejects create with a missing Authorization header', async () => {
    await expect(
      controller.create('branch-1', { verifierName: 'ci-evaluator', status: 'pass' }, undefined, WORKSPACE_ID),
    ).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.create).not.toHaveBeenCalled();
  });

  it('delegates GET /branches/:id/verification-signals to VerificationSignalsService', async () => {
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.findAllForBranch).mockResolvedValue([response]);

    const result = await controller.findAll('branch-1', 'Bearer signed-token', WORKSPACE_ID);

    expect(result).toEqual([response]);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.findAllForBranch).toHaveBeenCalledWith('branch-1', WORKSPACE_ID, claims);
  });

  it('rejects GET /branches/:id/verification-signals with a missing Authorization header', async () => {
    await expect(controller.findAll('branch-1', undefined, WORKSPACE_ID)).rejects.toThrow(
      UnauthorizedException,
    );
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.findAllForBranch).not.toHaveBeenCalled();
  });
});
