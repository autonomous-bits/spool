import { BadRequestException, NotFoundException } from '@nestjs/common';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Branch } from '../domain/branch.js';
import type { BranchRepository } from '../persistence/branch.repository.js';
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

describe('BranchesService', () => {
  let repository: Pick<BranchRepository, 'create' | 'findById'>;
  let service: BranchesService;

  beforeEach(() => {
    repository = {
      create: vi.fn(),
      findById: vi.fn(),
    };
    service = new BranchesService(repository as BranchRepository);
  });

  it('creates and returns the persisted branch', async () => {
    const request = validRequest();
    const persisted = new Branch({
      name: request.name,
      discipline: request.discipline,
      createdByStakeholderId: request.stakeholderId,
    });
    vi.mocked(repository.create).mockResolvedValue(persisted);

    const result = await service.create(request);

    expect(result.id).toBe(persisted.id);
    expect(result.status).toBe('draft');
    expect(repository.create).toHaveBeenCalledOnce();
  });

  it('rejects a blank name via the domain entity as a BadRequestException', async () => {
    await expect(service.create(validRequest({ name: '   ' }))).rejects.toThrow(
      BadRequestException,
    );
    expect(repository.create).not.toHaveBeenCalled();
  });

  it('translates a foreign key violation on an unknown stakeholderId into a BadRequestException', async () => {
    const request = validRequest({ stakeholderId: '00000000-0000-0000-0000-0000000000ff' });
    vi.mocked(repository.create).mockRejectedValue(
      Object.assign(new Error('violates foreign key constraint'), { code: '23503' }),
    );

    await expect(service.create(request)).rejects.toThrow(BadRequestException);
  });

  it('translates a unique-name violation into a BadRequestException', async () => {
    const request = validRequest();
    vi.mocked(repository.create).mockRejectedValue(
      Object.assign(new Error('duplicate key value violates unique constraint'), {
        code: '23505',
      }),
    );

    await expect(service.create(request)).rejects.toThrow(BadRequestException);
  });

  it('rethrows unrelated repository errors', async () => {
    const request = validRequest();
    vi.mocked(repository.create).mockRejectedValue(new Error('connection lost'));

    await expect(service.create(request)).rejects.toThrow('connection lost');
  });

  it('returns the branch from findById', async () => {
    const request = validRequest();
    const persisted = new Branch({
      name: request.name,
      discipline: request.discipline,
      createdByStakeholderId: request.stakeholderId,
    });
    vi.mocked(repository.findById).mockResolvedValue(persisted);

    const result = await service.findById(persisted.id);

    expect(result.id).toBe(persisted.id);
  });

  it('throws NotFoundException for an unknown id', async () => {
    vi.mocked(repository.findById).mockResolvedValue(undefined);

    await expect(service.findById('missing')).rejects.toThrow(NotFoundException);
  });
});
