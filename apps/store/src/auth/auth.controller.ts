import {
  Body,
  Controller,
  Get,
  HttpCode,
  Post,
  Query,
  Redirect,
  Res,
} from '@nestjs/common';
import type { PairingExchangeResult } from './auth.service.js';
import { AuthService } from './auth.service.js';
import { parsePairingCodeExchangeRequest } from './pairing-code-exchange-request.dto.js';
import { parseRefreshTokenRequest } from './refresh-token-request.dto.js';
import { toSessionTokenResponse, type SessionTokenResponse } from './session-token-response.dto.js';

interface LoginRedirect {
  url: string;
  statusCode: number;
}

interface RedirectResponse {
  redirect(statusCode: number, url: string): void;
}

/**
 * GitHub OAuth login/callback endpoints (Meridian IDEA-81). Human-only per IDEA-9/IDEA-40/
 * IDEA-57 — there is deliberately no MCP-facing equivalent (see G04's scope notes).
 */
@Controller('auth/github')
export class AuthController {
  constructor(private readonly authService: AuthService) {}

  /**
   * `workspaceId` is optional (Meridian IDEA-92/IDEA-101, G11 SG2): supplying it requests a
   * workspace-bound token (subject to membership at the callback); omitting it is only valid for
   * a stakeholder with zero workspace memberships, who receives a workspace-less bootstrap token.
   */
  @Get('login')
  @Redirect()
  login(
    @Query('workspaceId') workspaceId?: string,
    @Query('cliRedirectUri') cliRedirectUri?: string,
  ): LoginRedirect {
    const url = this.authService.buildLoginRedirectUrl(workspaceId, cliRedirectUri);
    return { url, statusCode: 302 };
  }

  @Get('callback')
  async callback(
    @Query('code') code: unknown,
    @Query('state') state: unknown,
    @Res({ passthrough: true }) response: RedirectResponse,
  ): Promise<SessionTokenResponse | void> {
    const result = await this.authService.handleCallback(code, state);
    if (result.kind === 'redirect') {
      response.redirect(302, result.redirectUrl);
      return;
    }

    return toSessionTokenResponse(result);
  }

  @Post('pairing/exchange')
  @HttpCode(200)
  async exchangePairingCode(@Body() body: unknown): Promise<PairingExchangeResult> {
    const request = parsePairingCodeExchangeRequest(body);
    return this.authService.exchangePairingCode(request.code);
  }

  @Post('refresh')
  @HttpCode(200)
  async refresh(@Body() body: unknown): Promise<SessionTokenResponse> {
    const request = parseRefreshTokenRequest(body);
    const tokens = await this.authService.refreshSession(request.refreshToken);
    return toSessionTokenResponse(tokens);
  }
}
