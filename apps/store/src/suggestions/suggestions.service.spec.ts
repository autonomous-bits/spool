import {
  BadRequestException,
  ConflictException,
  ForbiddenException,
  NotFoundException,
} from '@nestjs/common';
import { describe, expect, it, vi } from 'vitest';
import { Branch } from '../domain/branch.js';
import { Suggestion } from '../domain/suggestion.js';
import type { StakeholderDisciplineRepository } from '../persistence/stakeholder-discipline.repository.js';
import type { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import type { SuggestionRepository } from '../persistence/suggestion.repository.js';
import type { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { SuggestionsService } from './suggestions.service.js';
import type { AcceptSuggestionRequest } from './accept-suggestion-request.dto.js';
import type { CreateSuggestionRequest } from './create-suggestion-request.dto.js';

const STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';
const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

function chunkRequest(overrides: Partial<CreateSuggestionRequest> = {}): CreateSuggestionRequest {
  return {
    variant: { kind: 'chunk', label: 'ATOMIC-1', content: 'Some proposed content.' },
    discipline: 'product',
    ...overrides,
  };
}

function edgeRequest(): CreateSuggestionRequest {
  return {
    variant: {
      kind: 'edge',
      fromChunkLabel: 'ATOMIC-1',
      toChunkLabel: 'ATOMIC-2',
      relationshipType: 'refines',
    },
    discipline: 'product',
  };
}

const claims = {
  stakeholderId: STAKEHOLDER_ID,
  authTime: 1_752_000_000,
  workspaceId: WORKSPACE_ID,
};

function acceptRequest(overrides: Partial<AcceptSuggestionRequest> = {}): AcceptSuggestionRequest {
  return { name: 'accepted-branch', ...overrides };
}

function setUp() {
  const suggestionRepository: Pick<
    SuggestionRepository,
    'create' | 'accept' | 'reject' | 'findById' | 'findAll'
  > = {
    create: vi.fn(),
    accept: vi.fn(),
    reject: vi.fn(),
    findById: vi.fn(),
    findAll: vi.fn(),
  };
  const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
    findById: vi.fn(),
  };
  const workspaceRepository: Pick<WorkspaceRepository, 'isMember'> = {
    isMember: vi.fn().mockResolvedValue(true),
  };
  const stakeholderDisciplineRepository: Pick<StakeholderDisciplineRepository, 'isAllowed'> = {
    isAllowed: vi.fn().mockResolvedValue(true),
  };
  const service = new SuggestionsService(
    suggestionRepository as SuggestionRepository,
    stakeholderRepository as StakeholderRepository,
    workspaceRepository as WorkspaceRepository,
    stakeholderDisciplineRepository as StakeholderDisciplineRepository,
  );
  return { suggestionRepository, stakeholderRepository, workspaceRepository, service };
}

describe('SuggestionsService', () => {
  it('creates the persisted chunk-shaped suggestion from token claims, always as delegated', async () => {
    const { suggestionRepository, service } = setUp();
    vi.mocked(suggestionRepository.create).mockImplementation((suggestion) => suggestion);

    const result = await service.create(chunkRequest(), WORKSPACE_ID, claims);

    expect(result.label).toBe('ATOMIC-1');
    expect(result.content).toBe('Some proposed content.');
    expect(result.status).toBe('pending');
    expect(result.submittedByActorKind).toBe('delegated');
    expect(result.decidedByStakeholderId).toBeNull();
    expect(result.decidedAt).toBeNull();
    const created = vi.mocked(suggestionRepository.create).mock.calls[0]?.[0];
    expect(created?.submittedByActorKind).toBe('delegated');
    expect(created?.submittedByStakeholderId).toBe(STAKEHOLDER_ID);
    expect(created?.workspaceId).toBe(WORKSPACE_ID);
  });

  it('creates and returns the persisted edge-shaped suggestion', async () => {
    const { suggestionRepository, service } = setUp();
    vi.mocked(suggestionRepository.create).mockImplementation((suggestion) => suggestion);

    const result = await service.create(edgeRequest(), WORKSPACE_ID, claims);

    expect(result.fromChunkLabel).toBe('ATOMIC-1');
    expect(result.toChunkLabel).toBe('ATOMIC-2');
    expect(result.relationshipType).toBe('refines');
    expect(result.status).toBe('pending');
  });

  it('throws ForbiddenException when the X-Workspace-Id header is missing', async () => {
    const { suggestionRepository, service } = setUp();

    await expect(service.create(chunkRequest(), undefined, claims)).rejects.toThrow(ForbiddenException);
    expect(suggestionRepository.create).not.toHaveBeenCalled();
  });

  it('throws ForbiddenException when the stakeholder is not a member of the header workspace', async () => {
    const { suggestionRepository, stakeholderRepository, workspaceRepository, service } = setUp();
    vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: STAKEHOLDER_ID,
      discipline: 'product',
    });

    await expect(service.create(chunkRequest(), WORKSPACE_ID, claims)).rejects.toThrow(
      ForbiddenException,
    );
    expect(suggestionRepository.create).not.toHaveBeenCalled();
  });

  it('throws BadRequestException when the domain entity rejects the variant', async () => {
    const { suggestionRepository, service } = setUp();

    await expect(
      service.create(
        chunkRequest({ variant: { kind: 'chunk', label: '   ', content: 'content' } }),
        WORKSPACE_ID,
        claims,
      ),
    ).rejects.toThrow(BadRequestException);
    expect(suggestionRepository.create).not.toHaveBeenCalled();
  });

  it('maps a foreign key violation on an unknown stakeholderId claim to BadRequestException', async () => {
    const { stakeholderRepository, suggestionRepository, workspaceRepository, service } = setUp();
    vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);
    vi.mocked(stakeholderRepository.findById).mockResolvedValue(undefined);
    vi.mocked(suggestionRepository.create).mockRejectedValue({ code: '23503' });

    await expect(service.create(chunkRequest(), WORKSPACE_ID, claims)).rejects.toThrow(
      BadRequestException,
    );
  });

  it('rethrows unexpected repository errors unchanged', async () => {
    const { suggestionRepository, service } = setUp();
    const unexpected = new Error('boom');
    vi.mocked(suggestionRepository.create).mockRejectedValue(unexpected);

    await expect(service.create(chunkRequest(), WORKSPACE_ID, claims)).rejects.toBe(unexpected);
  });

  it('accepts a pending suggestion, returning the created branch response', async () => {
    const { suggestionRepository, stakeholderRepository, service } = setUp();
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: STAKEHOLDER_ID,
      discipline: 'product',
    });
    const branch = new Branch({
      workspaceId: WORKSPACE_ID,
      name: 'accepted-branch',
      discipline: 'product',
      createdByStakeholderId: STAKEHOLDER_ID,
      originSuggestionId: 'suggestion-1',
    });
    vi.mocked(suggestionRepository.accept).mockResolvedValue({ kind: 'accepted', branch });

    const result = await service.accept('suggestion-1', acceptRequest(), claims, WORKSPACE_ID);

    expect(result.originSuggestionId).toBe('suggestion-1');
    expect(suggestionRepository.accept).toHaveBeenCalledWith(
      'suggestion-1',
      'accepted-branch',
      STAKEHOLDER_ID,
      WORKSPACE_ID,
    );
  });

  it('throws ForbiddenException on accept when the header workspaceId mismatches the token claim', async () => {
    const { service } = setUp();

    await expect(
      service.accept(
        'suggestion-1',
        acceptRequest(),
        claims,
        '11111111-1111-1111-1111-111111111111',
      ),
    ).rejects.toThrow(ForbiddenException);
  });

  it('throws NotFoundException when the suggestion does not exist', async () => {
    const { suggestionRepository, stakeholderRepository, service } = setUp();
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: STAKEHOLDER_ID,
      discipline: 'product',
    });
    vi.mocked(suggestionRepository.accept).mockResolvedValue({ kind: 'not_found' });

    await expect(
      service.accept('missing', acceptRequest(), claims, WORKSPACE_ID),
    ).rejects.toThrow(NotFoundException);
  });

  it('throws ConflictException when the suggestion is not pending', async () => {
    const { suggestionRepository, stakeholderRepository, service } = setUp();
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: STAKEHOLDER_ID,
      discipline: 'product',
    });
    vi.mocked(suggestionRepository.accept).mockResolvedValue({ kind: 'not_pending' });

    await expect(
      service.accept('suggestion-1', acceptRequest(), claims, WORKSPACE_ID),
    ).rejects.toThrow(ConflictException);
  });

  it('throws BadRequestException when the acting stakeholder does not resolve', async () => {
    const { stakeholderRepository, service } = setUp();
    vi.mocked(stakeholderRepository.findById).mockResolvedValue(undefined);

    await expect(
      service.accept('suggestion-1', acceptRequest(), claims, WORKSPACE_ID),
    ).rejects.toThrow(BadRequestException);
  });

  it('maps a duplicate branch name to BadRequestException', async () => {
    const { suggestionRepository, stakeholderRepository, service } = setUp();
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: STAKEHOLDER_ID,
      discipline: 'product',
    });
    vi.mocked(suggestionRepository.accept).mockRejectedValue({ code: '23505' });

    await expect(
      service.accept('suggestion-1', acceptRequest(), claims, WORKSPACE_ID),
    ).rejects.toThrow(BadRequestException);
  });

  function pendingSuggestion(overrides: Partial<ConstructorParameters<typeof Suggestion>[0]> = {}) {
    return new Suggestion({
      workspaceId: WORKSPACE_ID,
      variant: { kind: 'chunk', label: 'ATOMIC-1', content: 'content' },
      discipline: 'product',
      submittedByStakeholderId: STAKEHOLDER_ID,
      submittedByActorKind: 'delegated',
      ...overrides,
    });
  }

  it('rejects a pending suggestion, returning the rejected suggestion response', async () => {
    const { suggestionRepository, stakeholderRepository, service } = setUp();
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: STAKEHOLDER_ID,
      discipline: 'product',
    });
    vi.mocked(suggestionRepository.reject).mockResolvedValue({ kind: 'rejected' });
    vi.mocked(suggestionRepository.findById).mockResolvedValue(
      pendingSuggestion({ id: 'suggestion-1', status: 'rejected' }),
    );

    const result = await service.reject('suggestion-1', claims, WORKSPACE_ID);

    expect(result.status).toBe('rejected');
    expect(suggestionRepository.reject).toHaveBeenCalledWith(
      'suggestion-1',
      STAKEHOLDER_ID,
      WORKSPACE_ID,
    );
  });

  it('throws ForbiddenException on reject when the header workspaceId mismatches the token claim', async () => {
    const { service } = setUp();

    await expect(
      service.reject('suggestion-1', claims, '11111111-1111-1111-1111-111111111111'),
    ).rejects.toThrow(ForbiddenException);
  });

  it('throws NotFoundException when rejecting an unknown suggestion', async () => {
    const { suggestionRepository, stakeholderRepository, service } = setUp();
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: STAKEHOLDER_ID,
      discipline: 'product',
    });
    vi.mocked(suggestionRepository.reject).mockResolvedValue({ kind: 'not_found' });

    await expect(service.reject('missing', claims, WORKSPACE_ID)).rejects.toThrow(
      NotFoundException,
    );
  });

  it('throws ConflictException when rejecting a non-pending suggestion', async () => {
    const { suggestionRepository, stakeholderRepository, service } = setUp();
    vi.mocked(stakeholderRepository.findById).mockResolvedValue({
      id: STAKEHOLDER_ID,
      discipline: 'product',
    });
    vi.mocked(suggestionRepository.reject).mockResolvedValue({ kind: 'not_pending' });

    await expect(service.reject('suggestion-1', claims, WORKSPACE_ID)).rejects.toThrow(
      ConflictException,
    );
  });

  it('findById returns the mapped response for an existing suggestion', async () => {
    const { suggestionRepository, service } = setUp();
    vi.mocked(suggestionRepository.findById).mockResolvedValue(
      pendingSuggestion({ id: 'suggestion-1' }),
    );

    const result = await service.findById('suggestion-1', claims, WORKSPACE_ID);

    expect(result.id).toBe('suggestion-1');
  });

  it('findById throws NotFoundException for an unknown suggestion', async () => {
    const { suggestionRepository, service } = setUp();
    vi.mocked(suggestionRepository.findById).mockResolvedValue(undefined);

    await expect(service.findById('missing', claims, WORKSPACE_ID)).rejects.toThrow(
      NotFoundException,
    );
  });

  it('findById throws ForbiddenException when the X-Workspace-Id header is missing', async () => {
    const { suggestionRepository, service } = setUp();

    await expect(service.findById('missing', claims, undefined)).rejects.toThrow(
      ForbiddenException,
    );
    expect(suggestionRepository.findById).not.toHaveBeenCalled();
  });

  it('findAll delegates to the repository and maps every row', async () => {
    const { suggestionRepository, service } = setUp();
    vi.mocked(suggestionRepository.findAll).mockResolvedValue([
      pendingSuggestion({ id: 'suggestion-1' }),
      pendingSuggestion({ id: 'suggestion-2' }),
    ]);

    const result = await service.findAll('pending', claims, WORKSPACE_ID);

    expect(result).toHaveLength(2);
    expect(suggestionRepository.findAll).toHaveBeenCalledWith('pending', WORKSPACE_ID);
  });

  it('findAll throws BadRequestException for an invalid status filter', async () => {
    const { service } = setUp();

    await expect(
      service.findAll('not-a-status', claims, WORKSPACE_ID),
    ).rejects.toThrow(BadRequestException);
  });

  it('findAll throws ForbiddenException when the X-Workspace-Id header is missing', async () => {
    const { suggestionRepository, service } = setUp();

    await expect(service.findAll('pending', claims, undefined)).rejects.toThrow(
      ForbiddenException,
    );
    expect(suggestionRepository.findAll).not.toHaveBeenCalled();
  });
});
