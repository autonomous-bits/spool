import { Test, type TestingModule } from '@nestjs/testing';
import { Readable } from 'node:stream';
import { StreamableFile } from '@nestjs/common';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ArtifactsController } from './artifacts.controller.js';
import { ArtifactsService } from './artifacts.service.js';
import type { ArtifactResponse } from './artifact-response.dto.js';
import type { ChunkArtifactResponse } from './chunk-artifact-response.dto.js';
import type { EffectiveChunkArtifact } from '../persistence/chunk-artifact.repository.js';

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
      ],
    }).compile();

    controller = module.get(ArtifactsController);
    service = module.get(ArtifactsService);
  });

  it('POST /artifacts parses the body and delegates to ArtifactsService.createArtifact', async () => {
    const expected = {
      id: 'artifact-1',
      uri: 'local-file://artifact-1',
      mimeType: 'text/plain',
      createdByStakeholderId: 'stakeholder-1',
      createdAt: new Date(),
    } satisfies ArtifactResponse;
    vi.mocked(service.createArtifact).mockResolvedValue(expected);

    const result = await controller.create({
      content: 'aGVsbG8=',
      mimeType: 'text/plain',
      stakeholderId: 'stakeholder-1',
    });

    expect(result).toEqual(expected);
    expect(service.createArtifact).toHaveBeenCalledWith({
      content: 'aGVsbG8=',
      mimeType: 'text/plain',
      stakeholderId: 'stakeholder-1',
    });
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
    vi.mocked(service.attachArtifactToChunk).mockResolvedValue(expected);

    const result = await controller.attach('ATOMIC-1', {
      artifactId: 'artifact-1',
      stakeholderId: 'stakeholder-1',
    });

    expect(result).toEqual(expected);
    expect(service.attachArtifactToChunk).toHaveBeenCalledWith('ATOMIC-1', {
      artifactId: 'artifact-1',
      stakeholderId: 'stakeholder-1',
    });
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
    vi.mocked(service.detachArtifactFromChunk).mockResolvedValue(expected);

    const result = await controller.detach('ATOMIC-1', 'artifact-1', 'branch-1', 'stakeholder-1');

    expect(result).toEqual(expected);
    expect(service.detachArtifactFromChunk).toHaveBeenCalledWith(
      'ATOMIC-1',
      'artifact-1',
      'branch-1',
      'stakeholder-1',
    );
  });

  it('DELETE .../artifacts/:artifactId throws BadRequestException when branchId query param is missing', async () => {
    await expect(
      controller.detach('ATOMIC-1', 'artifact-1', undefined, 'stakeholder-1'),
    ).rejects.toThrow('branchId');
    expect(service.detachArtifactFromChunk).not.toHaveBeenCalled();
  });

  it('DELETE .../artifacts/:artifactId throws BadRequestException when stakeholderId query param is missing', async () => {
    await expect(
      controller.detach('ATOMIC-1', 'artifact-1', 'branch-1', undefined),
    ).rejects.toThrow('stakeholderId');
    expect(service.detachArtifactFromChunk).not.toHaveBeenCalled();
  });

  it('GET chunks/:label/artifacts delegates to ArtifactsService.getEffectiveArtifactsForChunk with an optional branchId', async () => {
    const expected: EffectiveChunkArtifact[] = [
      { artifactId: 'artifact-1', branchId: null, status: 'active' },
    ];
    vi.mocked(service.getEffectiveArtifactsForChunk).mockResolvedValue(expected);

    const result = await controller.listEffective('ATOMIC-1', 'branch-1');

    expect(result).toEqual(expected);
    expect(service.getEffectiveArtifactsForChunk).toHaveBeenCalledWith('ATOMIC-1', 'branch-1');
  });

  it('GET chunks/:label/artifacts passes undefined branchId when the query param is omitted', async () => {
    vi.mocked(service.getEffectiveArtifactsForChunk).mockResolvedValue([]);

    await controller.listEffective('ATOMIC-1', undefined);

    expect(service.getEffectiveArtifactsForChunk).toHaveBeenCalledWith('ATOMIC-1', undefined);
  });

  it('GET artifacts/:id/download-token delegates to ArtifactsService.issueDownloadToken', async () => {
    const issued = { token: 'signed-token', expiresAt: new Date('2026-01-01T00:00:00Z') };
    vi.mocked(service.issueDownloadToken).mockResolvedValue(issued);

    const result = await controller.downloadToken('artifact-1');

    expect(result).toEqual(issued);
    expect(service.issueDownloadToken).toHaveBeenCalledWith('artifact-1');
  });

  it('GET artifacts/content/:token streams the artifact content as a StreamableFile with its mimeType', async () => {
    const stream = Readable.from([Buffer.from('hello')]);
    vi.mocked(service.streamArtifactContent).mockResolvedValue({ stream, mimeType: 'text/plain' });

    const result = await controller.content('signed-token');

    expect(result).toBeInstanceOf(StreamableFile);
    expect(service.streamArtifactContent).toHaveBeenCalledWith('signed-token');
  });
});
