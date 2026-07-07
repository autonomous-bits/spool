import { UnauthorizedException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import type { BranchResponse } from '../branches/branch-response.dto.js';
import { SuggestionsController } from './suggestions.controller.js';
import { SuggestionsService } from './suggestions.service.js';
import type { SuggestionResponse } from './suggestion-response.dto.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

const claims = {
  stakeholderId: 'stakeholder-1',
  discipline: 'product',
  authTime: 1_752_000_000,
  workspaceId: WORKSPACE_ID,
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

    const result = await controller.create(
      {
        label: 'ATOMIC-1',
        content: 'content',
        discipline: 'product',
        stakeholderId: 'stakeholder-1',
      },
      WORKSPACE_ID,
    );

    expect(result).toEqual(expected);
    expect(service.create).toHaveBeenCalledWith(
      {
        variant: { kind: 'chunk', label: 'ATOMIC-1', content: 'content' },
        discipline: 'product',
        stakeholderId: 'stakeholder-1',
      },
      WORKSPACE_ID,
    );
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
      WORKSPACE_ID,
    );

    expect(result).toEqual(expected);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.accept).toHaveBeenCalledWith(
      'suggestion-1',
      { name: 'accepted-branch' },
      claims,
      WORKSPACE_ID,
    );
  });

  it('rejects accept with a missing Authorization header', async () => {
    await expect(
      controller.accept('suggestion-1', { name: 'accepted-branch' }, undefined, WORKSPACE_ID),
    ).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.accept).not.toHaveBeenCalled();
  });

  it('rejects accept with a malformed Authorization header', async () => {
    await expect(
      controller.accept(
        'suggestion-1',
        { name: 'accepted-branch' },
        'Token signed-token',
        WORKSPACE_ID,
      ),
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

    const result = await controller.reject('suggestion-1', 'Bearer signed-token', WORKSPACE_ID);

    expect(result).toEqual(suggestionResponse);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.reject).toHaveBeenCalledWith('suggestion-1', claims, WORKSPACE_ID);
  });

  it('rejects reject with a missing Authorization header', async () => {
    await expect(controller.reject('suggestion-1', undefined, WORKSPACE_ID)).rejects.toThrow(
      UnauthorizedException,
    );
    expect(service.reject).not.toHaveBeenCalled();
  });

  it('delegates GET /suggestions/:id to SuggestionsService', async () => {
    vi.mocked(service.findById).mockResolvedValue(suggestionResponse);

    const result = await controller.findOne('suggestion-1', 'stakeholder-1', WORKSPACE_ID);

    expect(result).toEqual(suggestionResponse);
    expect(service.findById).toHaveBeenCalledWith('suggestion-1', 'stakeholder-1', WORKSPACE_ID);
  });

  it('throws BadRequestException from GET /suggestions/:id when stakeholderId query param is missing', async () => {
    await expect(controller.findOne('suggestion-1', undefined, WORKSPACE_ID)).rejects.toThrow(
      'stakeholderId',
    );
    expect(service.findById).not.toHaveBeenCalled();
  });

  it('delegates GET /suggestions with an optional status filter to SuggestionsService', async () => {
    vi.mocked(service.findAll).mockResolvedValue([suggestionResponse]);

    const result = await controller.findAll('pending', 'stakeholder-1', WORKSPACE_ID);

    expect(result).toEqual([suggestionResponse]);
    expect(service.findAll).toHaveBeenCalledWith('pending', 'stakeholder-1', WORKSPACE_ID);
  });

  it('delegates GET /suggestions with no status filter to SuggestionsService', async () => {
    vi.mocked(service.findAll).mockResolvedValue([suggestionResponse]);

    const result = await controller.findAll(undefined, 'stakeholder-1', WORKSPACE_ID);

    expect(result).toEqual([suggestionResponse]);
    expect(service.findAll).toHaveBeenCalledWith(undefined, 'stakeholder-1', WORKSPACE_ID);
  });

  it('throws BadRequestException from GET /suggestions when stakeholderId query param is missing', async () => {
    await expect(controller.findAll(undefined, undefined, WORKSPACE_ID)).rejects.toThrow(
      'stakeholderId',
    );
    expect(service.findAll).not.toHaveBeenCalled();
  });
});
