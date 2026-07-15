import { BadRequestException, ForbiddenException } from '@nestjs/common';
import { describe, expect, it, vi } from 'vitest';
import type { StakeholderDisciplineRepository } from '../persistence/stakeholder-discipline.repository.js';
import type { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import type { SessionTokenClaims } from './session-token.service.js';
import { resolveHumanActorContext } from './resolve-human-actor.helper.js';

const STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';
const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

function validClaims(overrides: Partial<SessionTokenClaims> = {}): SessionTokenClaims {
  return {
    stakeholderId: STAKEHOLDER_ID,
    authTime: 1_752_000_000,
    workspaceId: WORKSPACE_ID,
    ...overrides,
  };
}

function stubDisciplineRepository(
  overrides: Partial<Pick<StakeholderDisciplineRepository, 'isAllowed'>> = {},
): Pick<StakeholderDisciplineRepository, 'isAllowed'> {
  return {
    isAllowed: vi.fn().mockResolvedValue(false),
    ...overrides,
  };
}

describe('resolveHumanActorContext', () => {
  describe('requireDiscipline: false', () => {
    it('builds a HumanActorContext from claims.stakeholderId and the looked-up legacy discipline', async () => {
      const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
        findById: vi.fn().mockResolvedValue({ id: STAKEHOLDER_ID, discipline: 'product' }),
      };
      const stakeholderDisciplineRepository = stubDisciplineRepository();

      const actor = await resolveHumanActorContext(
        stakeholderRepository as StakeholderRepository,
        stakeholderDisciplineRepository as StakeholderDisciplineRepository,
        validClaims(),
        { requireDiscipline: false, actionDescription: 'verify, reject, or merge a branch' },
      );

      expect(actor).toEqual({ kind: 'human', stakeholderId: STAKEHOLDER_ID, discipline: 'product' });
      expect(stakeholderRepository.findById).toHaveBeenCalledWith(STAKEHOLDER_ID);
      expect(stakeholderDisciplineRepository.isAllowed).not.toHaveBeenCalled();
    });

    it('allows a missing/invalid discipline (resolved to null), no allow-list check', async () => {
      const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
        findById: vi.fn().mockResolvedValue({ id: STAKEHOLDER_ID, discipline: null }),
      };
      const stakeholderDisciplineRepository = stubDisciplineRepository();

      const actor = await resolveHumanActorContext(
        stakeholderRepository as StakeholderRepository,
        stakeholderDisciplineRepository as StakeholderDisciplineRepository,
        validClaims(),
        { requireDiscipline: false, actionDescription: 'accept a suggestion' },
      );

      expect(actor).toEqual({ kind: 'human', stakeholderId: STAKEHOLDER_ID, discipline: null });
      expect(stakeholderDisciplineRepository.isAllowed).not.toHaveBeenCalled();
    });

    it('still rejects a missing stakeholder with 400', async () => {
      const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
        findById: vi.fn().mockResolvedValue(undefined),
      };
      const stakeholderDisciplineRepository = stubDisciplineRepository();

      await expect(
        resolveHumanActorContext(
          stakeholderRepository as StakeholderRepository,
          stakeholderDisciplineRepository as StakeholderDisciplineRepository,
          validClaims(),
          { requireDiscipline: false, actionDescription: 'reject a suggestion' },
        ),
      ).rejects.toThrow(BadRequestException);
    });
  });

  describe('requireDiscipline: true', () => {
    function stakeholderRepositoryFound(): Pick<StakeholderRepository, 'findById'> {
      return { findById: vi.fn().mockResolvedValue({ id: STAKEHOLDER_ID, discipline: null }) };
    }

    it('rejects a missing stakeholder with 400', async () => {
      const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
        findById: vi.fn().mockResolvedValue(undefined),
      };
      const stakeholderDisciplineRepository = stubDisciplineRepository();

      await expect(
        resolveHumanActorContext(
          stakeholderRepository as StakeholderRepository,
          stakeholderDisciplineRepository as StakeholderDisciplineRepository,
          validClaims(),
          { requireDiscipline: true, actionDescription: 'submit a branch', activeDiscipline: 'product' },
        ),
      ).rejects.toThrow(BadRequestException);
    });

    it('rejects a missing activeDiscipline with 400', async () => {
      const stakeholderDisciplineRepository = stubDisciplineRepository();

      await expect(
        resolveHumanActorContext(
          stakeholderRepositoryFound() as StakeholderRepository,
          stakeholderDisciplineRepository as StakeholderDisciplineRepository,
          validClaims(),
          { requireDiscipline: true, actionDescription: 'submit a branch', activeDiscipline: undefined },
        ),
      ).rejects.toThrow(BadRequestException);
      expect(stakeholderDisciplineRepository.isAllowed).not.toHaveBeenCalled();
    });

    it('rejects a null activeDiscipline with 400', async () => {
      const stakeholderDisciplineRepository = stubDisciplineRepository();

      await expect(
        resolveHumanActorContext(
          stakeholderRepositoryFound() as StakeholderRepository,
          stakeholderDisciplineRepository as StakeholderDisciplineRepository,
          validClaims(),
          { requireDiscipline: true, actionDescription: 'submit a branch', activeDiscipline: null },
        ),
      ).rejects.toThrow(BadRequestException);
    });

    it('rejects a blank activeDiscipline with 400', async () => {
      const stakeholderDisciplineRepository = stubDisciplineRepository();

      await expect(
        resolveHumanActorContext(
          stakeholderRepositoryFound() as StakeholderRepository,
          stakeholderDisciplineRepository as StakeholderDisciplineRepository,
          validClaims(),
          { requireDiscipline: true, actionDescription: 'submit a branch', activeDiscipline: '   ' },
        ),
      ).rejects.toThrow(BadRequestException);
    });

    it('rejects an invalid-vocabulary activeDiscipline with 400', async () => {
      const stakeholderDisciplineRepository = stubDisciplineRepository();

      await expect(
        resolveHumanActorContext(
          stakeholderRepositoryFound() as StakeholderRepository,
          stakeholderDisciplineRepository as StakeholderDisciplineRepository,
          validClaims(),
          { requireDiscipline: true, actionDescription: 'submit a branch', activeDiscipline: 'not-a-real-discipline' },
        ),
      ).rejects.toThrow(BadRequestException);
    });

    it('rejects a valid-vocabulary-but-disallowed activeDiscipline with 403', async () => {
      const stakeholderDisciplineRepository = stubDisciplineRepository({
        isAllowed: vi.fn().mockResolvedValue(false),
      });

      await expect(
        resolveHumanActorContext(
          stakeholderRepositoryFound() as StakeholderRepository,
          stakeholderDisciplineRepository as StakeholderDisciplineRepository,
          validClaims(),
          { requireDiscipline: true, actionDescription: 'submit a branch', activeDiscipline: 'product' },
        ),
      ).rejects.toThrow(ForbiddenException);
      expect(stakeholderDisciplineRepository.isAllowed).toHaveBeenCalledWith(
        WORKSPACE_ID,
        STAKEHOLDER_ID,
        'product',
      );
    });

    it('rejects when the token has no workspace (bootstrap token) with 400', async () => {
      const stakeholderDisciplineRepository = stubDisciplineRepository();

      await expect(
        resolveHumanActorContext(
          stakeholderRepositoryFound() as StakeholderRepository,
          stakeholderDisciplineRepository as StakeholderDisciplineRepository,
          validClaims({ workspaceId: null }),
          { requireDiscipline: true, actionDescription: 'submit a branch', activeDiscipline: 'product' },
        ),
      ).rejects.toThrow(BadRequestException);
      expect(stakeholderDisciplineRepository.isAllowed).not.toHaveBeenCalled();
    });

    it('builds a HumanActorContext with the validated activeDiscipline when allowed', async () => {
      const stakeholderDisciplineRepository = stubDisciplineRepository({
        isAllowed: vi.fn().mockResolvedValue(true),
      });

      const actor = await resolveHumanActorContext(
        stakeholderRepositoryFound() as StakeholderRepository,
        stakeholderDisciplineRepository as StakeholderDisciplineRepository,
        validClaims(),
        { requireDiscipline: true, actionDescription: 'submit a branch', activeDiscipline: 'product' },
      );

      expect(actor).toEqual({ kind: 'human', stakeholderId: STAKEHOLDER_ID, discipline: 'product' });
    });
  });
});
