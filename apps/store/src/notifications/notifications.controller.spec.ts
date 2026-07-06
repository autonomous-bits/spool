import { UnauthorizedException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import { NotificationsController } from './notifications.controller.js';
import { NotificationsService } from './notifications.service.js';
import type { NotificationResponse } from './notification-response.dto.js';

const claims = {
  stakeholderId: 'stakeholder-1',
  discipline: 'product',
  authTime: 1_752_000_000,
} satisfies SessionTokenClaims;

const AUTH_HEADER = 'Bearer test-signed-token';

describe('NotificationsController', () => {
  let controller: NotificationsController;
  let service: Pick<NotificationsService, 'findAll' | 'markAsRead'>;
  let sessionTokenService: Pick<SessionTokenService, 'verify'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [NotificationsController],
      providers: [
        {
          provide: NotificationsService,
          useValue: {
            findAll: vi.fn(),
            markAsRead: vi.fn(),
          } satisfies Pick<NotificationsService, 'findAll' | 'markAsRead'>,
        },
        {
          provide: SessionTokenService,
          useValue: {
            verify: vi.fn(),
          } satisfies Pick<SessionTokenService, 'verify'>,
        },
      ],
    }).compile();

    controller = module.get(NotificationsController);
    service = module.get(NotificationsService);
    sessionTokenService = module.get(SessionTokenService);
  });

  const notificationResponse = {
    id: 'notification-1',
    branchId: 'branch-1',
    stakeholderId: 'stakeholder-1',
    signalId: 'signal-1',
    status: 'unread',
    createdAt: new Date(),
    updatedAt: new Date(),
  } satisfies NotificationResponse;

  it('verifies the bearer token and delegates listing to NotificationsService', async () => {
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.findAll).mockResolvedValue([notificationResponse]);

    const result = await controller.findAll(AUTH_HEADER, undefined);

    expect(result).toEqual([notificationResponse]);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('test-signed-token');
    expect(service.findAll).toHaveBeenCalledWith(claims, undefined);
  });

  it('forwards the status query param to NotificationsService', async () => {
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.findAll).mockResolvedValue([]);

    await controller.findAll(AUTH_HEADER, 'unread');

    expect(service.findAll).toHaveBeenCalledWith(claims, 'unread');
  });

  it('rejects findAll with a missing Authorization header', async () => {
    await expect(controller.findAll(undefined, undefined)).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.findAll).not.toHaveBeenCalled();
  });

  it('rejects findAll with a malformed Authorization header', async () => {
    await expect(controller.findAll('Token not-a-bearer-token', undefined)).rejects.toThrow(
      UnauthorizedException,
    );
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.findAll).not.toHaveBeenCalled();
  });

  it('verifies the bearer token and delegates marking read to NotificationsService', async () => {
    const readResponse = { ...notificationResponse, status: 'read' } satisfies NotificationResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.markAsRead).mockResolvedValue(readResponse);

    const result = await controller.markAsRead('notification-1', AUTH_HEADER);

    expect(result).toEqual(readResponse);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('test-signed-token');
    expect(service.markAsRead).toHaveBeenCalledWith('notification-1', claims);
  });

  it('rejects markAsRead with a missing Authorization header', async () => {
    await expect(controller.markAsRead('notification-1', undefined)).rejects.toThrow(
      UnauthorizedException,
    );
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.markAsRead).not.toHaveBeenCalled();
  });
});
