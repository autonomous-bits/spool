import { UnauthorizedException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import type { BranchResponse } from '../branches/branch-response.dto.js';
import { SuggestionsController } from './suggestions.controller.js';
import { SuggestionsService } from './suggestions.service.js';
import type { SuggestionResponse } from './suggestion-response.dto.js';

const claims = {
  stakeholderId: 'stakeholder-1',
  discipline: 'product',
  authTime: 1_752_000_000,
} satisfies SessionTokenClaims;

describe('SuggestionsController', () => {
  let controller: SuggestionsController;
  let service: Pick<SuggestionsService, 'create' | 'accept' | 'reject' | 'findById' | 'findAll'>;
  let sessionTokenService: Pick<SessionTokenService, 'verify'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [SuggestionsController],
      providers: [
        {
          provide: SuggestionsService,
          useValue: {
            create: vi.fn(),
            accept: vi.fn(),
            reject: vi.fn(),
            findById: vi.fn(),
            findAll: vi.fn(),
          } satisfies Pick<SuggestionsService, 'create' | 'accept' | 'reject' | 'findById' | 'findAll'>,
        },
        {
          provide: SessionTokenService,
          useValue: {
            verify: vi.fn(),
          } satisfies Pick<SessionTokenService, 'verify'>,
        },
      ],
    }).compile();

    controller = module.get(SuggestionsController);
    service = module.get(SuggestionsService);
    sessionTokenService = module.get(SessionTokenService);
  });

  it('parses the body and delegates creation to SuggestionsService', async () => {
    const expected = {
      id: 'abc',
      label: 'ATOMIC-1',
      content: 'content',
      fromChunkLabel: null,
      toChunkLabel: null,
      relationshipType: null,
      discipline: 'product',
      status: 'pending',
      submittedByStakeholderId: 'stakeholder-1',
      submittedByActorKind: 'delegated',
      decidedByStakeholderId: null,
      decidedAt: null,
      createdAt: new Date(),
      updatedAt: new Date(),
    } satisfies SuggestionResponse;
    vi.mocked(service.create).mockResolvedValue(expected);

    const result = await controller.create({
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
    });

    expect(result).toEqual(expected);
  });

  it('verifies the bearer token, parses the body, and delegates acceptance to SuggestionsService', async () => {
    const expected = {
      id: 'branch-1',
      name: 'accepted-branch',
      discipline: 'product',
      status: 'draft',
      divergedAt: new Date().toISOString(),
      submittedAt: null,
      verifiedAt: null,
      mergedAt: null,
      originSuggestionId: 'suggestion-1',
      createdAt: new Date(),
      updatedAt: new Date(),
      createdByStakeholderId: 'stakeholder-1',
    } satisfies BranchResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.accept).mockResolvedValue(expected);

    const result = await controller.accept(
      'suggestion-1',
      { name: 'accepted-branch' },
      'Bearer signed-token',
    );

    expect(result).toEqual(expected);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.accept).toHaveBeenCalledWith(
      'suggestion-1',
      { name: 'accepted-branch' },
      claims,
    );
  });

  it('rejects accept with a missing Authorization header', async () => {
    await expect(
      controller.accept('suggestion-1', { name: 'accepted-branch' }, undefined),
    ).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.accept).not.toHaveBeenCalled();
  });

  it('rejects accept with a malformed Authorization header', async () => {
    await expect(
      controller.accept('suggestion-1', { name: 'accepted-branch' }, 'Token signed-token'),
    ).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.accept).not.toHaveBeenCalled();
  });

  const suggestionResponse = {
    id: 'suggestion-1',
    label: 'ATOMIC-1',
    content: 'content',
    fromChunkLabel: null,
    toChunkLabel: null,
    relationshipType: null,
    discipline: 'product',
    status: 'rejected',
    submittedByStakeholderId: 'stakeholder-1',
    submittedByActorKind: 'delegated',
    decidedByStakeholderId: 'stakeholder-1',
    decidedAt: new Date().toISOString(),
    createdAt: new Date(),
    updatedAt: new Date(),
  } satisfies SuggestionResponse;

  it('verifies the bearer token and delegates rejection to SuggestionsService', async () => {
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.reject).mockResolvedValue(suggestionResponse);

    const result = await controller.reject('suggestion-1', 'Bearer signed-token');

    expect(result).toEqual(suggestionResponse);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.reject).toHaveBeenCalledWith('suggestion-1', claims);
  });

  it('rejects reject with a missing Authorization header', async () => {
    await expect(controller.reject('suggestion-1', undefined)).rejects.toThrow(
      UnauthorizedException,
    );
    expect(service.reject).not.toHaveBeenCalled();
  });

  it('delegates GET /suggestions/:id to SuggestionsService', async () => {
    vi.mocked(service.findById).mockResolvedValue(suggestionResponse);

    const result = await controller.findOne('suggestion-1');

    expect(result).toEqual(suggestionResponse);
    expect(service.findById).toHaveBeenCalledWith('suggestion-1');
  });

  it('delegates GET /suggestions with an optional status filter to SuggestionsService', async () => {
    vi.mocked(service.findAll).mockResolvedValue([suggestionResponse]);

    const result = await controller.findAll('pending');

    expect(result).toEqual([suggestionResponse]);
    expect(service.findAll).toHaveBeenCalledWith('pending');
  });

  it('delegates GET /suggestions with no status filter to SuggestionsService', async () => {
    vi.mocked(service.findAll).mockResolvedValue([suggestionResponse]);

    const result = await controller.findAll(undefined);

    expect(result).toEqual([suggestionResponse]);
    expect(service.findAll).toHaveBeenCalledWith(undefined);
  });
});
