import { UnauthorizedException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import type { WorkspaceMembershipResponse } from './workspace-membership-response.dto.js';
import type { WorkspaceResponse } from './workspace-response.dto.js';
import { WorkspacesController } from './workspaces.controller.js';
import { WorkspacesService } from './workspaces.service.js';

const claims = {
  stakeholderId: 'stakeholder-1',
  discipline: 'product',
  authTime: 1_752_000_000,
} satisfies SessionTokenClaims;

describe('WorkspacesController', () => {
  let controller: WorkspacesController;
  let service: Pick<WorkspacesService, 'create' | 'addMember'>;
  let sessionTokenService: Pick<SessionTokenService, 'verify'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [WorkspacesController],
      providers: [
        {
          provide: WorkspacesService,
          useValue: {
            create: vi.fn(),
            addMember: vi.fn(),
          } satisfies Pick<WorkspacesService, 'create' | 'addMember'>,
        },
        {
          provide: SessionTokenService,
          useValue: {
            verify: vi.fn(),
          } satisfies Pick<SessionTokenService, 'verify'>,
        },
      ],
    }).compile();

    controller = module.get(WorkspacesController);
    service = module.get(WorkspacesService);
    sessionTokenService = module.get(SessionTokenService);
  });

  it('verifies the bearer token and delegates creation to WorkspacesService', async () => {
    const expected = {
      id: 'workspace-1',
      name: 'acme',
      createdByStakeholderId: 'stakeholder-1',
      createdAt: new Date(),
    } satisfies WorkspaceResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.create).mockResolvedValue(expected);

    const result = await controller.create({ name: 'acme' }, 'Bearer signed-token');

    expect(result).toEqual(expected);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.create).toHaveBeenCalledWith({ name: 'acme' }, claims);
  });

  it('rejects create with a missing Authorization header', async () => {
    await expect(controller.create({ name: 'acme' }, undefined)).rejects.toThrow(
      UnauthorizedException,
    );
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.create).not.toHaveBeenCalled();
  });

  it('rejects create with a malformed Authorization header', async () => {
    await expect(
      controller.create({ name: 'acme' }, 'Token signed-token'),
    ).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.create).not.toHaveBeenCalled();
  });

  it('verifies the bearer token and delegates add-member to WorkspacesService', async () => {
    const expected = {
      workspaceId: 'workspace-1',
      stakeholderId: 'stakeholder-2',
      createdAt: new Date(),
    } satisfies WorkspaceMembershipResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.addMember).mockResolvedValue(expected);

    const result = await controller.addMember(
      'workspace-1',
      { stakeholderId: 'stakeholder-2' },
      'Bearer signed-token',
      '00000000-0000-0000-0000-00000000d0fa',
    );

    expect(result).toEqual(expected);
    expect(sessionTokenService.verify).toHaveBeenCalledWith('signed-token');
    expect(service.addMember).toHaveBeenCalledWith('workspace-1', 'stakeholder-2', '00000000-0000-0000-0000-00000000d0fa', claims);
  });

  it('rejects add-member with a missing Authorization header', async () => {
    await expect(
      controller.addMember('workspace-1', { stakeholderId: 'stakeholder-2' }, undefined, '00000000-0000-0000-0000-00000000d0fa'),
    ).rejects.toThrow(UnauthorizedException);
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
    expect(service.addMember).not.toHaveBeenCalled();
  });
});
