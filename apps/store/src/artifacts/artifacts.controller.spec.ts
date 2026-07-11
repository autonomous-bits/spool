import { Test, type TestingModule } from '@nestjs/testing';
import { Readable } from 'node:stream';
import { StreamableFile } from '@nestjs/common';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ArtifactsController } from './artifacts.controller.js';
import { ArtifactsService } from './artifacts.service.js';
import { SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import type { ArtifactResponse } from './artifact-response.dto.js';
import type { ChunkArtifactResponse } from './chunk-artifact-response.dto.js';
import type { EffectiveChunkArtifact } from '../persistence/chunk-artifact.repository.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

const claims = {
  stakeholderId: 'stakeholder-1',
  workspaceId: WORKSPACE_ID,
  discipline: 'product',
  authTime: 1_752_000_000,
} satisfies SessionTokenClaims;

describe('ArtifactsController', () => {
  let controller: ArtifactsController;
  let service: Pick<
    ArtifactsService,
    | 'createArtifact'
    | 'attachArtifactToChunk'
    | 'detachArtifactFromChunk'
    | 'getEffectiveArtifactsForChunk'
    | 'issueDownloadToken'
    | 'streamArtifactContent'
  >;
  let sessionTokenService: Pick<SessionTokenService, 'verify'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [ArtifactsController],
      providers: [
        {
          provide: ArtifactsService,
          useValue: {
            createArtifact: vi.fn(),
            attachArtifactToChunk: vi.fn(),
            detachArtifactFromChunk: vi.fn(),
            getEffectiveArtifactsForChunk: vi.fn(),
            issueDownloadToken: vi.fn(),
            streamArtifactContent: vi.fn(),
          } satisfies Pick<
            ArtifactsService,
            | 'createArtifact'
            | 'attachArtifactToChunk'
            | 'detachArtifactFromChunk'
            | 'getEffectiveArtifactsForChunk'
            | 'issueDownloadToken'
            | 'streamArtifactContent'
          >,
        },
        {
          provide: SessionTokenService,
          useValue: {
            verify: vi.fn(),
          } satisfies Pick<SessionTokenService, 'verify'>,
        },
      ],
    }).compile();

    controller = module.get(ArtifactsController);
    service = module.get(ArtifactsService);
    sessionTokenService = module.get(SessionTokenService);
  });

  it('POST /artifacts parses the body and delegates to ArtifactsService.createArtifact', async () => {
    const expected = {
      id: 'artifact-1',
      uri: 'local-file://artifact-1',
      mimeType: 'text/plain',
      createdByStakeholderId: 'stakeholder-1',
      createdAt: new Date(),
    } satisfies ArtifactResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.createArtifact).mockResolvedValue(expected);

    const result = await controller.create(
      {
        content: 'aGVsbG8=',
        mimeType: 'text/plain',
      },
      'Bearer signed-token',
      WORKSPACE_ID,
    );

    expect(result).toEqual(expected);
    expect(service.createArtifact).toHaveBeenCalledWith(
      {
        content: 'aGVsbG8=',
        mimeType: 'text/plain',
      },
      WORKSPACE_ID,
      claims,
    );
  });

  it('POST /artifacts rejects a missing Authorization header', async () => {
    await expect(
      controller.create(
        {
          content: 'aGVsbG8=',
          mimeType: 'text/plain',
        },
        undefined,
        WORKSPACE_ID,
      ),
    ).rejects.toThrow();
    expect(service.createArtifact).not.toHaveBeenCalled();
  });

  it('POST /chunks/:label/artifacts parses the body and delegates to ArtifactsService.attachArtifactToChunk', async () => {
    const expected = {
      id: 'assoc-1',
      chunkLabel: 'ATOMIC-1',
      artifactId: 'artifact-1',
      status: 'active',
      branchId: null,
      originBranchId: null,
      createdByStakeholderId: 'stakeholder-1',
      updatedByStakeholderId: 'stakeholder-1',
      createdAt: new Date(),
      updatedAt: new Date(),
    } satisfies ChunkArtifactResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.attachArtifactToChunk).mockResolvedValue(expected);

    const result = await controller.attach(
      'ATOMIC-1',
      {
        artifactId: 'artifact-1',
      },
      'Bearer signed-token',
      WORKSPACE_ID,
    );

    expect(result).toEqual(expected);
    expect(service.attachArtifactToChunk).toHaveBeenCalledWith(
      'ATOMIC-1',
      {
        artifactId: 'artifact-1',
      },
      WORKSPACE_ID,
      claims,
    );
  });

  it('POST /chunks/:label/artifacts rejects a missing Authorization header', async () => {
    await expect(
      controller.attach('ATOMIC-1', { artifactId: 'artifact-1' }, undefined, WORKSPACE_ID),
    ).rejects.toThrow();
    expect(service.attachArtifactToChunk).not.toHaveBeenCalled();
  });

  it('DELETE .../artifacts/:artifactId parses query params and delegates to ArtifactsService.detachArtifactFromChunk', async () => {
    const expected = {
      id: 'assoc-2',
      chunkLabel: 'ATOMIC-1',
      artifactId: 'artifact-1',
      status: 'deactivated',
      branchId: 'branch-1',
      originBranchId: 'branch-1',
      createdByStakeholderId: 'stakeholder-1',
      updatedByStakeholderId: 'stakeholder-1',
      createdAt: new Date(),
      updatedAt: new Date(),
    } satisfies ChunkArtifactResponse;
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.detachArtifactFromChunk).mockResolvedValue(expected);

    const result = await controller.detach(
      'ATOMIC-1',
      'artifact-1',
      'branch-1',
      'Bearer signed-token',
      WORKSPACE_ID,
    );

    expect(result).toEqual(expected);
    expect(service.detachArtifactFromChunk).toHaveBeenCalledWith(
      'ATOMIC-1',
      'artifact-1',
      'branch-1',
      claims,
      WORKSPACE_ID,
    );
  });

  it('DELETE .../artifacts/:artifactId throws BadRequestException when branchId query param is missing', async () => {
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    await expect(
      controller.detach('ATOMIC-1', 'artifact-1', undefined, 'Bearer signed-token', WORKSPACE_ID),
    ).rejects.toThrow('branchId');
    expect(service.detachArtifactFromChunk).not.toHaveBeenCalled();
  });

  it('DELETE .../artifacts/:artifactId rejects a missing Authorization header', async () => {
    await expect(
      controller.detach('ATOMIC-1', 'artifact-1', 'branch-1', undefined, WORKSPACE_ID),
    ).rejects.toThrow();
    expect(service.detachArtifactFromChunk).not.toHaveBeenCalled();
  });

  it('GET chunks/:label/artifacts delegates to ArtifactsService.getEffectiveArtifactsForChunk with an optional branchId', async () => {
    const expected: EffectiveChunkArtifact[] = [
      { artifactId: 'artifact-1', branchId: null, status: 'active' },
    ];
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.getEffectiveArtifactsForChunk).mockResolvedValue(expected);

    const result = await controller.listEffective(
      'ATOMIC-1',
      'branch-1',
      'Bearer signed-token',
      WORKSPACE_ID,
    );

    expect(result).toEqual(expected);
    expect(service.getEffectiveArtifactsForChunk).toHaveBeenCalledWith(
      'ATOMIC-1',
      'branch-1',
      claims,
      WORKSPACE_ID,
    );
  });

  it('GET chunks/:label/artifacts passes undefined branchId when the query param is omitted', async () => {
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.getEffectiveArtifactsForChunk).mockResolvedValue([]);

    await controller.listEffective('ATOMIC-1', undefined, 'Bearer signed-token', WORKSPACE_ID);

    expect(service.getEffectiveArtifactsForChunk).toHaveBeenCalledWith(
      'ATOMIC-1',
      undefined,
      claims,
      WORKSPACE_ID,
    );
  });

  it('GET chunks/:label/artifacts rejects a missing Authorization header', async () => {
    await expect(
      controller.listEffective('ATOMIC-1', undefined, undefined, WORKSPACE_ID),
    ).rejects.toThrow();
    expect(service.getEffectiveArtifactsForChunk).not.toHaveBeenCalled();
  });

  it('GET artifacts/:id/download-token delegates to ArtifactsService.issueDownloadToken', async () => {
    const issued = { token: 'signed-token', expiresAt: new Date('2026-01-01T00:00:00Z') };
    vi.mocked(sessionTokenService.verify).mockReturnValue(claims);
    vi.mocked(service.issueDownloadToken).mockResolvedValue(issued);

    const result = await controller.downloadToken('artifact-1', 'Bearer signed-token', WORKSPACE_ID);

    expect(result).toEqual(issued);
    expect(service.issueDownloadToken).toHaveBeenCalledWith(
      'artifact-1',
      claims,
      WORKSPACE_ID,
    );
  });

  it('GET artifacts/:id/download-token rejects a missing Authorization header', async () => {
    await expect(controller.downloadToken('artifact-1', undefined, WORKSPACE_ID)).rejects.toThrow();
    expect(service.issueDownloadToken).not.toHaveBeenCalled();
  });

  it('GET artifacts/content/:token remains reachable without a bearer token and streams the artifact content', async () => {
    const stream = Readable.from([Buffer.from('hello')]);
    vi.mocked(service.streamArtifactContent).mockResolvedValue({ stream, mimeType: 'text/plain' });

    const result = await controller.content('signed-token');

    expect(result).toBeInstanceOf(StreamableFile);
    expect(service.streamArtifactContent).toHaveBeenCalledWith('signed-token');
    expect(sessionTokenService.verify).not.toHaveBeenCalled();
  });
});
