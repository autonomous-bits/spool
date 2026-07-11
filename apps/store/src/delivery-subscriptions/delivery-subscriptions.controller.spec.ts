import { UnauthorizedException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import type { CreateDeliverySubscriptionResponse } from './create-delivery-subscription-response.dto.js';
import type { DeliverySubscriptionResponse } from './delivery-subscription-response.dto.js';
import { DeliverySubscriptionsController } from './delivery-subscriptions.controller.js';
import { DeliverySubscriptionsService } from './delivery-subscriptions.service.js';

const claims = {
  stakeholderId: 'stakeholder-1',
  discipline: 'product',
  authTime: 1_752_000_000,
  workspaceId: 'workspace-1',
} satisfies SessionTokenClaims;

describe('DeliverySubscriptionsController', () => {
  let controller: DeliverySubscriptionsController;
  let service: Pick<DeliverySubscriptionsService, 'create' | 'list' | 'remove'>;
  let sessionTokenService: Pick<SessionTokenService, 'verify'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [DeliverySubscriptionsController],
      providers: [
        {
          provide: DeliverySubscriptionsService,
          useValue: {
            create: vi.fn(),
            list: vi.fn(),
            remove: vi.fn(),
          } satisfies Pick<DeliverySubscriptionsService, 'create' | 'list' | 'remove'>,
        },
        {
          provide: SessionTokenService,
          useValue: {
            verify: vi.fn(),
          } satisfies Pick<SessionTokenService, 'verify'>,
        },
      ],
    }).compile();

    controller = module.get(DeliverySubscriptionsController);
    service = module.get(DeliverySubscriptionsService);
    sessionTokenService = module.get(SessionTokenService);
  });

  it('verifies the bearer token and delegates create to DeliverySubscriptionsService', async () => {
    const expected = {
      id: 'sub-1',
      workspaceId: 'workspace-1',
      url: 'https://example.com/hook',
      disciplineFilter: undefined,
      isActive: true,
      createdByStakeholderId: 'stakeholder-1',
      createdAt: new Date(),
      updatedAt: new Date(),
      signingSecret: 'secret',
    } satisfies CreateDeliverySubscriptionResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.create).mockResolvedValue(expected);

    const result = await controller.create(
      'workspace-1',
      { url: 'https://example.com/hook' },
      'Bearer signed-token',
      'workspace-1',
    );

    expect(result).toEqual(expected);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.create).toHaveBeenCalledWith(
      'workspace-1',
      { url: 'https://example.com/hook' },
      'workspace-1',
      claims,
    );
  });

  it('rejects create with a missing Authorization header', async () => {
    await expect(
      controller.create('workspace-1', { url: 'https://example.com/hook' }, undefined, 'workspace-1'),
    ).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.create).not.toHaveBeenCalled();
  });

  it('rejects create with a malformed Authorization header', async () => {
    await expect(
      controller.create('workspace-1', { url: 'https://example.com/hook' }, 'Token signed-token', 'workspace-1'),
    ).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.create).not.toHaveBeenCalled();
  });

  it('verifies the bearer token and delegates list to DeliverySubscriptionsService', async () => {
    const expected = [
      {
        id: 'sub-1',
        workspaceId: 'workspace-1',
        url: 'https://example.com/hook',
        disciplineFilter: undefined,
        isActive: true,
        createdByStakeholderId: 'stakeholder-1',
        createdAt: new Date(),
        updatedAt: new Date(),
      },
    ] satisfies DeliverySubscriptionResponse[];
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.list).mockResolvedValue(expected);

    const result = await controller.list('workspace-1', 'Bearer signed-token', 'workspace-1');

    expect(result).toEqual(expected);
    expect(service.list).toHaveBeenCalledWith('workspace-1', 'workspace-1', claims);
  });

  it('rejects list with a missing Authorization header', async () => {
    await expect(controller.list('workspace-1', undefined, 'workspace-1')).rejects.toThrow(
      UnauthorizedException,
    );
    expect(service.list).not.toHaveBeenCalled();
  });

  it('verifies the bearer token and delegates remove to DeliverySubscriptionsService', async () => {
    const expected = {
      id: 'sub-1',
      workspaceId: 'workspace-1',
      url: 'https://example.com/hook',
      disciplineFilter: undefined,
      isActive: false,
      createdByStakeholderId: 'stakeholder-1',
      createdAt: new Date(),
      updatedAt: new Date(),
    } satisfies DeliverySubscriptionResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.remove).mockResolvedValue(expected);

    const result = await controller.remove('workspace-1', 'sub-1', 'Bearer signed-token', 'workspace-1');

    expect(result).toEqual(expected);
    expect(service.remove).toHaveBeenCalledWith('workspace-1', 'sub-1', 'workspace-1', claims);
  });

  it('rejects remove with a missing Authorization header', async () => {
    await expect(controller.remove('workspace-1', 'sub-1', undefined, 'workspace-1')).rejects.toThrow(
      UnauthorizedException,
    );
    expect(service.remove).not.toHaveBeenCalled();
  });
});
