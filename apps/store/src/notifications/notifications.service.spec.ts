import { BadRequestException, NotFoundException } from '@nestjs/common';
import { describe, expect, it, vi } from 'vitest';
import { FeedbackNotification } from '../domain/feedback-notification.js';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import type { FeedbackNotificationRepository } from '../persistence/feedback-notification.repository.js';
import { NotificationsService } from './notifications.service.js';

const STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';
const claims = { stakeholderId: STAKEHOLDER_ID, discipline: 'product', authTime: 1_752_000_000 } satisfies SessionTokenClaims;

function notification(overrides: Partial<ConstructorParameters<typeof FeedbackNotification>[0]> = {}): FeedbackNotification {
  return new FeedbackNotification({
    id: 'notification-1',
    branchId: 'branch-1',
    stakeholderId: STAKEHOLDER_ID,
    signalId: 'signal-1',
    status: 'unread',
    createdAt: new Date('2026-07-01T00:00:00Z'),
    updatedAt: new Date('2026-07-01T00:00:00Z'),
    ...overrides,
  });
}

function setUp() {
  const notificationRepository: Pick<FeedbackNotificationRepository, 'findByStakeholderId' | 'markAsRead'> = {
    findByStakeholderId: vi.fn(),
    markAsRead: vi.fn(),
  };
  const service = new NotificationsService(notificationRepository as FeedbackNotificationRepository);
  return { notificationRepository, service };
}

describe('NotificationsService', () => {
  describe('findAll', () => {
    it('lists the caller\'s own notifications, mapped to responses', async () => {
      const { notificationRepository, service } = setUp();
      vi.mocked(notificationRepository.findByStakeholderId).mockResolvedValue([notification()]);

      const result = await service.findAll(claims);

      expect(notificationRepository.findByStakeholderId).toHaveBeenCalledWith(STAKEHOLDER_ID, undefined);
      expect(result).toEqual([
        {
          id: 'notification-1',
          branchId: 'branch-1',
          stakeholderId: STAKEHOLDER_ID,
          signalId: 'signal-1',
          status: 'unread',
          createdAt: new Date('2026-07-01T00:00:00Z'),
          updatedAt: new Date('2026-07-01T00:00:00Z'),
        },
      ]);
    });

    it('passes a valid status filter through to the repository', async () => {
      const { notificationRepository, service } = setUp();
      vi.mocked(notificationRepository.findByStakeholderId).mockResolvedValue([]);

      await service.findAll(claims, 'unread');

      expect(notificationRepository.findByStakeholderId).toHaveBeenCalledWith(STAKEHOLDER_ID, 'unread');
    });

    it('rejects an invalid status filter with 400, never reaching the repository', async () => {
      const { notificationRepository, service } = setUp();

      await expect(service.findAll(claims, 'bogus')).rejects.toBeInstanceOf(BadRequestException);
      expect(notificationRepository.findByStakeholderId).not.toHaveBeenCalled();
    });
  });

  describe('markAsRead', () => {
    it('marks the caller\'s own notification read', async () => {
      const { notificationRepository, service } = setUp();
      const read = notification({ status: 'read' });
      vi.mocked(notificationRepository.markAsRead).mockResolvedValue(read);

      const result = await service.markAsRead('notification-1', claims);

      expect(notificationRepository.markAsRead).toHaveBeenCalledWith('notification-1', STAKEHOLDER_ID);
      expect(result.status).toBe('read');
    });

    it('404s when the repository reports no matching row (not found or not owned)', async () => {
      const { notificationRepository, service } = setUp();
      vi.mocked(notificationRepository.markAsRead).mockResolvedValue(undefined);

      await expect(service.markAsRead('missing', claims)).rejects.toBeInstanceOf(NotFoundException);
    });
  });
});
