import {
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

/**
 * SG1 exposes submission only (Meridian IDEA-28); SG2 adds human-only acceptance (Meridian
 * IDEA-27/IDEA-75). SG3 adds human-only rejection plus unauthenticated GET /suggestions and
 * GET /suggestions/:id reads, matching this codebase's existing GET /branches, GET /chunks,
 * GET /edges precedent (Meridian IDEA-27).
 */
@Controller('suggestions')
export class SuggestionsController {
  constructor(
    private readonly suggestions: SuggestionsService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post()
  async create(@Body() body: unknown): Promise<SuggestionResponse> {
    const request = parseCreateSuggestionRequest(body);
    return this.suggestions.create(request);
  }

  @Post(':id/accept')
  async accept(
    @Param('id') id: string,
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseAcceptSuggestionRequest(body);
    return this.suggestions.accept(id, request, claims);
  }

  @Post(':id/reject')
  async reject(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
  ): Promise<SuggestionResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.suggestions.reject(id, claims);
  }

  @Get()
  async findAll(@Query('status') status?: string): Promise<SuggestionResponse[]> {
    return this.suggestions.findAll(status);
  }

  @Get(':id')
  async findOne(@Param('id') id: string): Promise<SuggestionResponse> {
    return this.suggestions.findById(id);
  }
}
