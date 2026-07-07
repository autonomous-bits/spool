import {
  BadRequestException,
  Body,
  Controller,
  Get,
  Headers,
  Param,
  Post,
  Query,
} from '@nestjs/common';
import { SessionTokenService } from '../auth/session-token.service.js';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import type { BranchResponse } from '../branches/branch-response.dto.js';
import { parseAcceptSuggestionRequest } from './accept-suggestion-request.dto.js';
import type { SuggestionResponse } from './suggestion-response.dto.js';
import { parseCreateSuggestionRequest } from './create-suggestion-request.dto.js';
import { SuggestionsService } from './suggestions.service.js';

function requireStakeholderId(value: string | undefined): string {
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException('stakeholderId query parameter must be a non-empty string');
  }
  return value;
}

/**
 * SG1 exposes submission only (Meridian IDEA-28); SG2 adds human-only acceptance (Meridian
 * IDEA-27/IDEA-75). SG3 adds human-only rejection plus GET /suggestions and GET /suggestions/:id
 * reads, matching this codebase's existing GET /branches, GET /chunks, GET /edges precedent
 * (Meridian IDEA-27). G11 SG5 (Meridian IDEA-98/IDEA-100) adds the `X-Workspace-Id` header to
 * every route and a required `stakeholderId` query param to the two GET reads (create/accept/
 * reject already carry a caller identity in their body/token).
 */
@Controller('suggestions')
export class SuggestionsController {
  constructor(
    private readonly suggestions: SuggestionsService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post()
  async create(
    @Body() body: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<SuggestionResponse> {
    const request = parseCreateSuggestionRequest(body);
    return this.suggestions.create(request, workspaceId);
  }

  @Post(':id/accept')
  async accept(
    @Param('id') id: string,
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseAcceptSuggestionRequest(body);
    return this.suggestions.accept(id, request, claims, workspaceId);
  }

  @Post(':id/reject')
  async reject(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<SuggestionResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.suggestions.reject(id, claims, workspaceId);
  }

  @Get()
  async findAll(
    @Query('status') status: string | undefined,
    @Query('stakeholderId') stakeholderId: string | undefined,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<SuggestionResponse[]> {
    return this.suggestions.findAll(status, requireStakeholderId(stakeholderId), workspaceId);
  }

  @Get(':id')
  async findOne(
    @Param('id') id: string,
    @Query('stakeholderId') stakeholderId: string | undefined,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<SuggestionResponse> {
    return this.suggestions.findById(id, requireStakeholderId(stakeholderId), workspaceId);
  }
}
