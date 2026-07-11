import {
  BadRequestException,
  ForbiddenException,
  NotFoundException,
} from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { DeliverySubscription } from '../domain/delivery-subscription.js';
import { DeliverySubscriptionRepository } from '../persistence/delivery-subscription.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { DeliverySubscriptionsService } from './delivery-subscriptions.service.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-0000000000aa';
const OTHER_WORKSPACE_ID = '00000000-0000-0000-0000-0000000000bb';
const STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';

function validClaims(overrides: Partial<SessionTokenClaims> = {}): SessionTokenClaims {
  return {
    stakeholderId: STAKEHOLDER_ID,
    discipline: 'product',
    authTime: 1_752_000_000,
    workspaceId: WORKSPACE_ID,
    ...overrides,
  };
}

function buildSubscription(overrides: Partial<{ id: string; isActive: boolean }> = {}): DeliverySubscription {
  return new DeliverySubscription({
    id: overrides.id ?? 'sub-1',
    workspaceId: WORKSPACE_ID,
    url: 'https://example.com/hook',
    createdByStakeholderId: STAKEHOLDER_ID,
    isActive: overrides.isActive ?? true,
  });
}

describe('DeliverySubscriptionsService', () => {
  let service: DeliverySubscriptionsService;
  let deliverySubscriptionRepository: Pick<
    DeliverySubscriptionRepository,
    'create' | 'listByWorkspace' | 'deactivate'
  >;
  let workspaceRepository: Pick<WorkspaceRepository, 'isMember'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        DeliverySubscriptionsService,
        {
          provide: DeliverySubscriptionRepository,
          useValue: {
            create: vi.fn(),
            listByWorkspace: vi.fn(),
            deactivate: vi.fn(),
          } satisfies Pick<DeliverySubscriptionRepository, 'create' | 'listByWorkspace' | 'deactivate'>,
        },
        {
          provide: WorkspaceRepository,
          useValue: {
            isMember: vi.fn(),
          } satisfies Pick<WorkspaceRepository, 'isMember'>,
        },
      ],
    }).compile();

    service = module.get(DeliverySubscriptionsService);
    deliverySubscriptionRepository = module.get(DeliverySubscriptionRepository);
    workspaceRepository = module.get(WorkspaceRepository);
  });

  describe('workspace scope + membership', () => {
    it('returns 403 when the X-Workspace-Id header is missing', async () => {
      await expect(
        service.create(WORKSPACE_ID, { url: 'https://example.com/hook' }, undefined, validClaims()),
      ).rejects.toThrow(ForbiddenException);
      expect(deliverySubscriptionRepository.create).not.toHaveBeenCalled();
    });

    it('returns 403 when the token workspace claim does not match the header workspace id', async () => {
      await expect(
        service.create(
          WORKSPACE_ID,
          { url: 'https://example.com/hook' },
          WORKSPACE_ID,
          validClaims({ workspaceId: OTHER_WORKSPACE_ID }),
        ),
      ).rejects.toThrow(ForbiddenException);
      expect(deliverySubscriptionRepository.create).not.toHaveBeenCalled();
    });

    it('returns 403 when the header workspace id does not match the route workspace id', async () => {
      await expect(
        service.create(
          WORKSPACE_ID,
          { url: 'https://example.com/hook' },
          OTHER_WORKSPACE_ID,
          validClaims({ workspaceId: OTHER_WORKSPACE_ID }),
        ),
      ).rejects.toThrow(ForbiddenException);
      expect(deliverySubscriptionRepository.create).not.toHaveBeenCalled();
    });

    it('returns 403 when the caller is not a member of the workspace', async () => {
      vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);

      await expect(
        service.create(WORKSPACE_ID, { url: 'https://example.com/hook' }, WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(ForbiddenException);
      expect(deliverySubscriptionRepository.create).not.toHaveBeenCalled();
    });
  });

  describe('create', () => {
    it('creates a subscription and returns the signingSecret exactly once', async () => {
      vi.mocked(workspaceRepository.isMember).mockResolvedValue(true);
      const created = buildSubscription();
      vi.mocked(deliverySubscriptionRepository.create).mockResolvedValue(created);

      const result = await service.create(
        WORKSPACE_ID,
        { url: 'https://example.com/hook' },
        WORKSPACE_ID,
        validClaims(),
      );

      expect(result.signingSecret).toBe(created.signingSecret);
      expect(result.id).toBe(created.id);
    });

    it('returns 400 for a non-https url', async () => {
      vi.mocked(workspaceRepository.isMember).mockResolvedValue(true);

      await expect(
        service.create(WORKSPACE_ID, { url: 'http://example.com/hook' }, WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(BadRequestException);
      expect(deliverySubscriptionRepository.create).not.toHaveBeenCalled();
    });

    it('returns 400 for an invalid discipline-filter value', async () => {
      vi.mocked(workspaceRepository.isMember).mockResolvedValue(true);

      await expect(
        service.create(
          WORKSPACE_ID,
          { url: 'https://example.com/hook', disciplineFilter: ['not-a-real-discipline'] },
          WORKSPACE_ID,
          validClaims(),
        ),
      ).rejects.toThrow(BadRequestException);
      expect(deliverySubscriptionRepository.create).not.toHaveBeenCalled();
    });
  });

  describe('list', () => {
    it('lists subscriptions without a signingSecret property', async () => {
      vi.mocked(workspaceRepository.isMember).mockResolvedValue(true);
      vi.mocked(deliverySubscriptionRepository.listByWorkspace).mockResolvedValue([buildSubscription()]);

      const result = await service.list(WORKSPACE_ID, WORKSPACE_ID, validClaims());

      expect(result).toHaveLength(1);
      expect(result[0]).not.toHaveProperty('signingSecret');
    });
  });

  describe('remove', () => {
    it('deactivates a subscription and returns it without a signingSecret property', async () => {
      vi.mocked(workspaceRepository.isMember).mockResolvedValue(true);
      const deactivated = buildSubscription({ isActive: false });
      vi.mocked(deliverySubscriptionRepository.deactivate).mockResolvedValue(deactivated);

      const result = await service.remove(WORKSPACE_ID, 'sub-1', WORKSPACE_ID, validClaims());

      expect(result.isActive).toBe(false);
      expect(result).not.toHaveProperty('signingSecret');
    });

    it('returns 404 for an unknown or cross-workspace subscription id', async () => {
      vi.mocked(workspaceRepository.isMember).mockResolvedValue(true);
      vi.mocked(deliverySubscriptionRepository.deactivate).mockResolvedValue(undefined);

      await expect(
        service.remove(WORKSPACE_ID, 'unknown-id', WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(NotFoundException);
    });
  });
});
