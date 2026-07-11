import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChunksController } from './chunks.controller.js';
import { ChunksService } from './chunks.service.js';
import { SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import type { ChunkResponse } from './chunk-response.dto.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

const claims = {
  stakeholderId: 'stakeholder-1',
  workspaceId: WORKSPACE_ID,
  discipline: 'product',
  authTime: 1_752_000_000,
} satisfies SessionTokenClaims;

describe('ChunksController', () => {
  let controller: ChunksController;
  let service: Pick<ChunksService, 'create' | 'findById'>;
  let sessionTokenService: Pick<SessionTokenService, 'verify'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [ChunksController],
      providers: [
        {
          provide: ChunksService,
          useValue: {
            create: vi.fn(),
            findById: vi.fn(),
            search: vi.fn(),
          },
        },
        {
          provide: SessionTokenService,
          useValue: {
            sign: vi.fn(),
            verify: vi.fn(),
          },
        },
      ],
    }).compile();

    controller = module.get(ChunksController);
    service = module.get(ChunksService);
    sessionTokenService = module.get(SessionTokenService);
  });

  it('parses the body and delegates creation to ChunksService', async () => {
    const expected = {
      id: 'abc',
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
      status: 'draft',
      createdByStakeholderId: 'stakeholder-1',
      updatedByStakeholderId: 'stakeholder-1',
      branchId: null,
      originBranchId: null,
      workspaceId: WORKSPACE_ID,
      createdAt: new Date(),
      updatedAt: new Date(),
    } satisfies ChunkResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.create).mockResolvedValue(expected);

    const result = await controller.create({
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
    }, 'Bearer signed-token', WORKSPACE_ID);

    expect(result).toEqual(expected);
    expect(service.create).toHaveBeenCalledWith({
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
    }, WORKSPACE_ID, claims);
  });

  it('delegates retrieval to ChunksService', async () => {
    const expected = { id: 'abc' } as ChunkResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.findById).mockResolvedValue(expected);

    const result = await controller.findOne('abc', 'Bearer signed-token', WORKSPACE_ID);

    expect(result).toEqual(expected);
    expect(service.findById).toHaveBeenCalledWith('abc', WORKSPACE_ID, claims);
  });
});
