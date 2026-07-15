import {
  BadRequestException,
  ConflictException,
  ForbiddenException,
  NotFoundException,
} from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { WorkspaceMembershipAlreadyExistsError } from '../domain/workspace-membership.js';
import { StakeholderDisciplineRepository } from '../persistence/stakeholder-discipline.repository.js';
import { StakeholderRepository, type StakeholderRecord } from '../persistence/stakeholder.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { WorkspacesService } from './workspaces.service.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-0000000000aa';
const CALLER_ID = '00000000-0000-0000-0000-000000000001';
const TARGET_ID = '00000000-0000-0000-0000-000000000002';
const OTHER_WORKSPACE_ID = '00000000-0000-0000-0000-0000000000bb';

function validClaims(overrides: Partial<SessionTokenClaims> = {}): SessionTokenClaims {
  return {
    stakeholderId: CALLER_ID,
    discipline: 'product',
    authTime: 1_752_000_000,
    workspaceId: WORKSPACE_ID,
    ...overrides,
  };
}

describe('WorkspacesService', () => {
  let workspaceRepository: Pick<
    WorkspaceRepository,
    'createWithFirstMember' | 'addMember' | 'findById' | 'isMember'
  >;
  let stakeholderRepository: Pick<StakeholderRepository, 'findById'>;
  let stakeholderDisciplineRepository: Pick<StakeholderDisciplineRepository, 'assign' | 'revoke'>;
  let service: WorkspacesService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        WorkspacesService,
        {
          provide: WorkspaceRepository,
          useValue: {
            createWithFirstMember: vi.fn(),
            addMember: vi.fn(),
            findById: vi.fn(),
            isMember: vi.fn().mockResolvedValue(true),
          } satisfies Pick<
            WorkspaceRepository,
            'createWithFirstMember' | 'addMember' | 'findById' | 'isMember'
          >,
        },
        {
          provide: StakeholderRepository,
          useValue: {
            findById: vi.fn(),
          } satisfies Pick<StakeholderRepository, 'findById'>,
        },
        {
          provide: StakeholderDisciplineRepository,
          useValue: {
            assign: vi.fn(),
            revoke: vi.fn(),
          } satisfies Pick<StakeholderDisciplineRepository, 'assign' | 'revoke'>,
        },
      ],
    }).compile();

    workspaceRepository = module.get(WorkspaceRepository);
    stakeholderRepository = module.get(StakeholderRepository);
    stakeholderDisciplineRepository = module.get(StakeholderDisciplineRepository);
    service = module.get(WorkspacesService);
  });

  describe('create', () => {
    it('persists a new workspace with the caller as creator', async () => {
      const created = {
        id: WORKSPACE_ID,
        name: 'acme',
        createdByStakeholderId: CALLER_ID,
        createdAt: new Date(),
        updatedAt: new Date(),
      };
      vi.mocked(workspaceRepository.createWithFirstMember).mockResolvedValue(created);

      const result = await service.create({ name: 'acme' }, validClaims());

      expect(result).toMatchObject({
        id: WORKSPACE_ID,
        name: 'acme',
        createdByStakeholderId: CALLER_ID,
      });
      expect(workspaceRepository.createWithFirstMember).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'acme', createdByStakeholderId: CALLER_ID }),
      );
    });

    it('rejects a blank name with 400', async () => {
      await expect(service.create({ name: '   ' }, validClaims())).rejects.toThrow(
        BadRequestException,
      );
      expect(workspaceRepository.createWithFirstMember).not.toHaveBeenCalled();
    });

    it('maps a foreign-key violation on the creating stakeholder to 400', async () => {
      vi.mocked(workspaceRepository.createWithFirstMember).mockRejectedValue({ code: '23503' });

      await expect(service.create({ name: 'acme' }, validClaims())).rejects.toThrow(
        BadRequestException,
      );
    });
  });

  describe('addMember', () => {
    it('returns 403 when the X-Workspace-Id header is missing', async () => {
      await expect(
        service.addMember(WORKSPACE_ID, TARGET_ID, undefined, validClaims()),
      ).rejects.toThrow(ForbiddenException);
      expect(workspaceRepository.addMember).not.toHaveBeenCalled();
    });

    it('returns 403 when the token workspace claim does not match the header workspace id', async () => {
      await expect(
        service.addMember(
          WORKSPACE_ID,
          TARGET_ID,
          WORKSPACE_ID,
          validClaims({ workspaceId: OTHER_WORKSPACE_ID }),
        ),
      ).rejects.toThrow(ForbiddenException);
      expect(workspaceRepository.addMember).not.toHaveBeenCalled();
    });

    it('returns 403 when the header workspace id does not match the route workspace id', async () => {
      await expect(
        service.addMember(
          WORKSPACE_ID,
          TARGET_ID,
          OTHER_WORKSPACE_ID,
          validClaims({ workspaceId: OTHER_WORKSPACE_ID }),
        ),
      ).rejects.toThrow(ForbiddenException);
      expect(workspaceRepository.addMember).not.toHaveBeenCalled();
    });

    it('returns 404 when the target stakeholder does not exist', async () => {
      vi.mocked(stakeholderRepository.findById).mockResolvedValue(undefined);

      await expect(
        service.addMember(WORKSPACE_ID, TARGET_ID, WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(NotFoundException);
      expect(workspaceRepository.addMember).not.toHaveBeenCalled();
    });

    it('returns 404 when the workspace does not exist', async () => {
      vi.mocked(stakeholderRepository.findById).mockResolvedValue({
        id: TARGET_ID,
        discipline: null,
      } satisfies StakeholderRecord);
      vi.mocked(workspaceRepository.addMember).mockResolvedValue({ kind: 'workspace_not_found' });

      await expect(
        service.addMember(WORKSPACE_ID, TARGET_ID, WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(NotFoundException);
    });

    it('returns 403 when the caller is not a member', async () => {
      vi.mocked(stakeholderRepository.findById).mockResolvedValue({
        id: TARGET_ID,
        discipline: null,
      } satisfies StakeholderRecord);
      vi.mocked(workspaceRepository.addMember).mockResolvedValue({ kind: 'caller_not_member' });

      await expect(
        service.addMember(WORKSPACE_ID, TARGET_ID, WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(ForbiddenException);
    });

    it('returns 409 when the target is already a member', async () => {
      vi.mocked(stakeholderRepository.findById).mockResolvedValue({
        id: TARGET_ID,
        discipline: null,
      } satisfies StakeholderRecord);
      vi.mocked(workspaceRepository.addMember).mockRejectedValue(
        new WorkspaceMembershipAlreadyExistsError(WORKSPACE_ID, TARGET_ID),
      );

      await expect(
        service.addMember(WORKSPACE_ID, TARGET_ID, WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(ConflictException);
    });

    it('adds the member and returns 201 shape on success', async () => {
      vi.mocked(stakeholderRepository.findById).mockResolvedValue({
        id: TARGET_ID,
        discipline: null,
      } satisfies StakeholderRecord);
      const membership = {
        workspaceId: WORKSPACE_ID,
        stakeholderId: TARGET_ID,
        createdAt: new Date(),
      };
      vi.mocked(workspaceRepository.addMember).mockResolvedValue({
        kind: 'added',
        membership: membership,
      });

      const result = await service.addMember(WORKSPACE_ID, TARGET_ID, WORKSPACE_ID, validClaims());

      expect(result).toMatchObject({ workspaceId: WORKSPACE_ID, stakeholderId: TARGET_ID });
      expect(workspaceRepository.addMember).toHaveBeenCalledWith(
        WORKSPACE_ID,
        CALLER_ID,
        TARGET_ID,
      );
    });
  });

  describe('assignDiscipline', () => {
    it('assigns a discipline for another existing member and it is visible via isAllowed repository semantics', async () => {
      vi.mocked(workspaceRepository.isMember)
        .mockResolvedValueOnce(true)
        .mockResolvedValueOnce(true);
      vi.mocked(stakeholderDisciplineRepository.assign).mockResolvedValue({
        workspaceId: WORKSPACE_ID,
        stakeholderId: TARGET_ID,
        discipline: 'security',
        createdAt: new Date('2026-07-15T07:00:00.000Z'),
      });

      const result = await service.assignDiscipline(
        WORKSPACE_ID,
        TARGET_ID,
        'security',
        WORKSPACE_ID,
        validClaims(),
      );

      expect(result).toEqual({
        workspaceId: WORKSPACE_ID,
        stakeholderId: TARGET_ID,
        discipline: 'security',
        createdAt: '2026-07-15T07:00:00.000Z',
      });
      expect(stakeholderDisciplineRepository.assign).toHaveBeenCalledWith(
        WORKSPACE_ID,
        TARGET_ID,
        'security',
      );
    });

    it('returns 403 when a non-member caller attempts to assign a discipline', async () => {
      vi.mocked(workspaceRepository.isMember).mockResolvedValueOnce(false);

      await expect(
        service.assignDiscipline(WORKSPACE_ID, TARGET_ID, 'security', WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(ForbiddenException);
      expect(stakeholderDisciplineRepository.assign).not.toHaveBeenCalled();
    });

    it('returns 404 when the target stakeholder is not a member of the workspace', async () => {
      vi.mocked(workspaceRepository.isMember)
        .mockResolvedValueOnce(true)
        .mockResolvedValueOnce(false);

      await expect(
        service.assignDiscipline(WORKSPACE_ID, TARGET_ID, 'security', WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(NotFoundException);
      expect(stakeholderDisciplineRepository.assign).not.toHaveBeenCalled();
    });

    it('maps a late foreign-key violation to 404 when the target membership disappears after pre-check', async () => {
      vi.mocked(workspaceRepository.isMember)
        .mockResolvedValueOnce(true)
        .mockResolvedValueOnce(true);
      vi.mocked(stakeholderDisciplineRepository.assign).mockRejectedValue({ code: '23503' });

      await expect(
        service.assignDiscipline(WORKSPACE_ID, TARGET_ID, 'security', WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(NotFoundException);
    });
  });

  describe('revokeDiscipline', () => {
    it('removes the row and returns 404 on repeat delete', async () => {
      vi.mocked(workspaceRepository.isMember).mockResolvedValue(true);
      vi.mocked(stakeholderDisciplineRepository.revoke)
        .mockResolvedValueOnce(true)
        .mockResolvedValueOnce(false);

      await expect(
        service.revokeDiscipline(WORKSPACE_ID, TARGET_ID, 'security', WORKSPACE_ID, validClaims()),
      ).resolves.toBeUndefined();
      await expect(
        service.revokeDiscipline(WORKSPACE_ID, TARGET_ID, 'security', WORKSPACE_ID, validClaims()),
      ).rejects.toThrow(NotFoundException);
    });
  });
});
