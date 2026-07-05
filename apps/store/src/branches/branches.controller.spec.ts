import { UnauthorizedException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import { BranchesController } from './branches.controller.js';
import { BranchesService } from './branches.service.js';
import type { BranchResponse } from './branch-response.dto.js';

const claims = {
  stakeholderId: 'stakeholder-1',
  discipline: 'product',
  authTime: 1_752_000_000,
} satisfies SessionTokenClaims;

describe('BranchesController', () => {
  let controller: BranchesController;
  let service: Pick<BranchesService, 'create' | 'findById' | 'submit'>;
  let sessionTokenService: Pick<SessionTokenService, 'verify'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [BranchesController],
      providers: [
        {
          provide: BranchesService,
          useValue: {
            create: vi.fn(),
            findById: vi.fn(),
            submit: vi.fn(),
          } satisfies Pick<BranchesService, 'create' | 'findById' | 'submit'>,
        },
        {
          provide: SessionTokenService,
          useValue: {
            verify: vi.fn(),
          } satisfies Pick<SessionTokenService, 'verify'>,
        },
      ],
    }).compile();

    controller = module.get(BranchesController);
    service = module.get(BranchesService);
    sessionTokenService = module.get(SessionTokenService);
  });

  it('parses the body and delegates creation to BranchesService', async () => {
    const expected = {
      id: 'abc',
      name: 'feature-branch',
      discipline: 'product',
      status: 'draft',
      divergedAt: new Date().toISOString(),
      submittedAt: null,
      createdAt: new Date(),
      updatedAt: new Date(),
      createdByStakeholderId: 'stakeholder-1',
    } satisfies BranchResponse;
    vi.mocked(service.create).mockResolvedValue(expected);

    const result = await controller.create({
      name: 'feature-branch',
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
    });

    expect(result).toEqual(expected);
    expect(service.create).toHaveBeenCalledWith({
      name: 'feature-branch',
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
    });
  });

  it('verifies the bearer token and delegates submission to BranchesService', async () => {
    const expected = {
      id: 'abc',
      name: 'feature-branch',
      discipline: 'product',
      status: 'submitted',
      divergedAt: new Date().toISOString(),
      submittedAt: new Date().toISOString(),
      createdAt: new Date(),
      updatedAt: new Date(),
      createdByStakeholderId: 'stakeholder-1',
    } satisfies BranchResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.submit).mockResolvedValue(expected);

    const result = await controller.submit('abc', 'Bearer signed-token');

    expect(result).toEqual(expected);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.submit).toHaveBeenCalledWith('abc', claims);
  });

  it('rejects a missing Authorization header', async () => {
    await expect(controller.submit('abc', undefined)).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.submit).not.toHaveBeenCalled();
  });

  it('rejects a malformed Authorization header', async () => {
    await expect(controller.submit('abc', 'Token signed-token')).rejects.toThrow(
      UnauthorizedException,
    );
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.submit).not.toHaveBeenCalled();
  });

  it('delegates retrieval to BranchesService', async () => {
    const expected = {
      id: 'abc',
      name: 'feature-branch',
      discipline: 'product',
      status: 'draft',
      divergedAt: new Date().toISOString(),
      submittedAt: null,
      createdAt: new Date(),
      updatedAt: new Date(),
      createdByStakeholderId: 'stakeholder-1',
    } satisfies BranchResponse;
    vi.mocked(service.findById).mockResolvedValue(expected);

    const result = await controller.findOne('abc');

    expect(result).toEqual(expected);
    expect(service.findById).toHaveBeenCalledWith('abc');
  });
});
