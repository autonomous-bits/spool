import { BadRequestException } from '@nestjs/common';
import { describe, expect, it, vi } from 'vitest';
import type { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import type { SessionTokenClaims } from './session-token.service.js';
import { resolveHumanActorContext } from './resolve-human-actor.helper.js';

const STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';

function validClaims(overrides: Partial<SessionTokenClaims> = {}): SessionTokenClaims {
  return {
    stakeholderId: STAKEHOLDER_ID,
    discipline: null,
    authTime: 1_752_000_000,
    workspaceId: '00000000-0000-0000-0000-00000000d0fa',
    ...overrides,
  };
}

describe('resolveHumanActorContext', () => {
  it('builds a HumanActorContext from claims.stakeholderId and the looked-up discipline', async () => {
    const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
      findById: vi.fn().mockResolvedValue({ id: STAKEHOLDER_ID, discipline: 'product' }),
    };

    const actor = await resolveHumanActorContext(
      stakeholderRepository as StakeholderRepository,
      validClaims(),
      { requireDiscipline: true, actionDescription: 'submit a branch' },
    );

    expect(actor).toEqual({ kind: 'human', stakeholderId: STAKEHOLDER_ID, discipline: 'product' });
    expect(stakeholderRepository.findById).toHaveBeenCalledWith(STAKEHOLDER_ID);
  });

  it('rejects a missing stakeholder when discipline is required', async () => {
    const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
      findById: vi.fn().mockResolvedValue(undefined),
    };

    await expect(
      resolveHumanActorContext(stakeholderRepository as StakeholderRepository, validClaims(), {
        requireDiscipline: true,
        actionDescription: 'submit a branch',
      }),
    ).rejects.toThrow(BadRequestException);
  });

  it('rejects an existing stakeholder with no valid discipline when discipline is required', async () => {
    const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
      findById: vi.fn().mockResolvedValue({ id: STAKEHOLDER_ID, discipline: null }),
    };

    await expect(
      resolveHumanActorContext(stakeholderRepository as StakeholderRepository, validClaims(), {
        requireDiscipline: true,
        actionDescription: 'submit a branch',
      }),
    ).rejects.toThrow(BadRequestException);
  });

  it('allows a missing/invalid discipline (resolved to null) when discipline is not required', async () => {
    const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
      findById: vi.fn().mockResolvedValue({ id: STAKEHOLDER_ID, discipline: null }),
    };

    const actor = await resolveHumanActorContext(
      stakeholderRepository as StakeholderRepository,
      validClaims(),
      { requireDiscipline: false, actionDescription: 'verify, reject, or merge a branch' },
    );

    expect(actor).toEqual({ kind: 'human', stakeholderId: STAKEHOLDER_ID, discipline: null });
  });

  it('still rejects a missing stakeholder when discipline is not required', async () => {
    const stakeholderRepository: Pick<StakeholderRepository, 'findById'> = {
      findById: vi.fn().mockResolvedValue(undefined),
    };

    await expect(
      resolveHumanActorContext(stakeholderRepository as StakeholderRepository, validClaims(), {
        requireDiscipline: false,
        actionDescription: 'accept a suggestion',
      }),
    ).rejects.toThrow(BadRequestException);
  });
});
