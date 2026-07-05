import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { BranchesController } from './branches.controller.js';
import { BranchesService } from './branches.service.js';
import type { BranchResponse } from './branch-response.dto.js';

describe('BranchesController', () => {
  let controller: BranchesController;
  let service: Pick<BranchesService, 'create' | 'findById'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [BranchesController],
      providers: [
        {
          provide: BranchesService,
          useValue: {
            create: vi.fn(),
            findById: vi.fn(),
          } satisfies Pick<BranchesService, 'create' | 'findById'>,
        },
      ],
    }).compile();

    controller = module.get(BranchesController);
    service = module.get(BranchesService);
  });

  it('parses the body and delegates creation to BranchesService', async () => {
    const expected = {
      id: 'abc',
      name: 'feature-branch',
      discipline: 'product',
      status: 'draft',
      divergedAt: new Date().toISOString(),
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

  it('delegates retrieval to BranchesService', async () => {
    const expected = { id: 'abc' } as BranchResponse;
    vi.mocked(service.findById).mockResolvedValue(expected);

    const result = await controller.findOne('abc');

    expect(result).toEqual(expected);
    expect(service.findById).toHaveBeenCalledWith('abc');
  });
});
