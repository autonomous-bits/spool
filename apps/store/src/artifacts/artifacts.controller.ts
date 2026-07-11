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
import { SessionTokenService } from '../auth/session-token.service.js';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
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
  constructor(
    private readonly artifacts: ArtifactsService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post('artifacts')
  async create(
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<ArtifactResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseCreateArtifactRequest(body);
    return this.artifacts.createArtifact(request, workspaceId, claims);
  }

  @Post('chunks/:label/artifacts')
  async attach(
    @Param('label') label: string,
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<ChunkArtifactResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseCreateChunkArtifactRequest(body);
    return this.artifacts.attachArtifactToChunk(label, request, workspaceId, claims);
  }

  @Delete('chunks/:label/artifacts/:artifactId')
  @HttpCode(HttpStatus.CREATED)
  async detach(
    @Param('label') label: string,
    @Param('artifactId') artifactId: string,
    @Query('branchId') branchId: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<ChunkArtifactResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const resolvedBranchId = requireQueryString(branchId, 'branchId');
    return this.artifacts.detachArtifactFromChunk(
      label,
      artifactId,
      resolvedBranchId,
      claims,
      workspaceId,
    );
  }

  @Get('chunks/:label/artifacts')
  async listEffective(
    @Param('label') label: string,
    @Query('branchId') branchId: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<EffectiveChunkArtifact[]> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const resolvedBranchId = optionalQueryString(branchId, 'branchId');
    return this.artifacts.getEffectiveArtifactsForChunk(
      label,
      resolvedBranchId,
      claims,
      workspaceId,
    );
  }

  @Get('artifacts/:id/download-token')
  async downloadToken(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<DownloadTokenResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const issued = await this.artifacts.issueDownloadToken(id, claims, workspaceId);
    return toDownloadTokenResponse(issued);
  }

  /**
   * IDEA-139 bearer-token policy deliberately does not apply to this one redemption endpoint. The
   * caller presents the short-lived HMAC download token itself as the narrow capability credential;
   * that token is minted only by the authenticated `GET /artifacts/:id/download-token` route and
   * remains workspace-bound and time-limited.
   */
  @Get('artifacts/content/:token')
  async content(@Param('token') token: string): Promise<StreamableFile> {
    const { stream, mimeType } = await this.artifacts.streamArtifactContent(token);
    return new StreamableFile(stream, { type: mimeType });
  }
}
