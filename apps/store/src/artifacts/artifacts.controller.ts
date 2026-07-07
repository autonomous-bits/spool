import {
  BadRequestException,
  Body,
  Controller,
  Delete,
  Get,
  Headers,
  HttpCode,
  HttpStatus,
  Param,
  Post,
  Query,
  StreamableFile,
} from '@nestjs/common';
import type { ArtifactResponse } from './artifact-response.dto.js';
import { ArtifactsService } from './artifacts.service.js';
import type { ChunkArtifactResponse } from './chunk-artifact-response.dto.js';
import { parseCreateArtifactRequest } from './create-artifact-request.dto.js';
import { parseCreateChunkArtifactRequest } from './create-chunk-artifact-request.dto.js';
import { toDownloadTokenResponse, type DownloadTokenResponse } from './download-token-response.dto.js';
import type { EffectiveChunkArtifact } from '../persistence/chunk-artifact.repository.js';

function requireQueryString(value: unknown, field: string): string {
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException(`${field} query parameter must be a non-empty string`);
  }
  return value;
}

function optionalQueryString(value: unknown, field: string): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  return requireQueryString(value, field);
}

@Controller()
export class ArtifactsController {
  constructor(private readonly artifacts: ArtifactsService) {}

  @Post('artifacts')
  async create(
    @Body() body: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<ArtifactResponse> {
    const request = parseCreateArtifactRequest(body);
    return this.artifacts.createArtifact(request, workspaceId);
  }

  @Post('chunks/:label/artifacts')
  async attach(
    @Param('label') label: string,
    @Body() body: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<ChunkArtifactResponse> {
    const request = parseCreateChunkArtifactRequest(body);
    return this.artifacts.attachArtifactToChunk(label, request, workspaceId);
  }

  @Delete('chunks/:label/artifacts/:artifactId')
  @HttpCode(HttpStatus.CREATED)
  async detach(
    @Param('label') label: string,
    @Param('artifactId') artifactId: string,
    @Query('branchId') branchId: unknown,
    @Query('stakeholderId') stakeholderId: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<ChunkArtifactResponse> {
    const resolvedBranchId = requireQueryString(branchId, 'branchId');
    const resolvedStakeholderId = requireQueryString(stakeholderId, 'stakeholderId');
    return this.artifacts.detachArtifactFromChunk(
      label,
      artifactId,
      resolvedBranchId,
      resolvedStakeholderId,
      workspaceId,
    );
  }

  @Get('chunks/:label/artifacts')
  async listEffective(
    @Param('label') label: string,
    @Query('branchId') branchId: unknown,
    @Query('stakeholderId') stakeholderId: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<EffectiveChunkArtifact[]> {
    const resolvedBranchId = optionalQueryString(branchId, 'branchId');
    const resolvedStakeholderId = requireQueryString(stakeholderId, 'stakeholderId');
    return this.artifacts.getEffectiveArtifactsForChunk(
      label,
      resolvedBranchId,
      resolvedStakeholderId,
      workspaceId,
    );
  }

  @Get('artifacts/:id/download-token')
  async downloadToken(
    @Param('id') id: string,
    @Query('stakeholderId') stakeholderId: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<DownloadTokenResponse> {
    const resolvedStakeholderId = requireQueryString(stakeholderId, 'stakeholderId');
    const issued = await this.artifacts.issueDownloadToken(id, resolvedStakeholderId, workspaceId);
    return toDownloadTokenResponse(issued);
  }

  @Get('artifacts/content/:token')
  async content(@Param('token') token: string): Promise<StreamableFile> {
    const { stream, mimeType } = await this.artifacts.streamArtifactContent(token);
    return new StreamableFile(stream, { type: mimeType });
  }
}
