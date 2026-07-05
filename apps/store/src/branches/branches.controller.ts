import {
  Body,
  Controller,
  Get,
  Headers,
  Param,
  Post,
  UnauthorizedException,
} from '@nestjs/common';
import { InvalidSessionTokenError, SessionTokenService, type SessionTokenClaims } from '../auth/session-token.service.js';
import type { BranchResponse } from './branch-response.dto.js';
import { parseCreateBranchRequest } from './create-branch-request.dto.js';
import { BranchesService } from './branches.service.js';

function extractBearerToken(authorizationHeader: unknown): string {
  if (typeof authorizationHeader !== 'string') {
    throw new UnauthorizedException('Missing Authorization header');
  }

  const [scheme, token, ...rest] = authorizationHeader.split(' ');
  if (scheme !== 'Bearer' || token === undefined || token.trim().length === 0 || rest.length > 0) {
    throw new UnauthorizedException('Authorization header must use Bearer token format');
  }

  return token;
}

function verifySessionClaims(
  authorizationHeader: unknown,
  sessionTokenService: SessionTokenService,
): SessionTokenClaims {
  const token = extractBearerToken(authorizationHeader);

  try {
    return sessionTokenService.verify(token);
  } catch (error) {
    if (error instanceof InvalidSessionTokenError) {
      throw new UnauthorizedException(error.message);
    }
    throw error;
  }
}

@Controller('branches')
export class BranchesController {
  constructor(
    private readonly branches: BranchesService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post()
  async create(@Body() body: unknown): Promise<BranchResponse> {
    const request = parseCreateBranchRequest(body);
    return this.branches.create(request);
  }

  @Post(':id/submit')
  async submit(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.submit(id, claims);
  }

  @Get(':id')
  async findOne(@Param('id') id: string): Promise<BranchResponse> {
    return this.branches.findById(id);
  }
}
