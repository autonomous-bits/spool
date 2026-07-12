import { Module } from '@nestjs/common';
import { PersistenceModule } from '../persistence/persistence.module.js';
import { AUTH_CONFIG } from './auth-config.token.js';
import { loadAuthConfig } from './auth-config.js';
import { AuthController } from './auth.controller.js';
import { AuthService } from './auth.service.js';
import { GITHUB_OAUTH_CLIENT } from './github-oauth-client.js';
import { HttpGithubOAuthClient } from './http-github-oauth-client.js';
import { OAuthStateService } from './oauth-state.service.js';
import { RefreshTokenService } from './refresh-token.service.js';
import { SessionTokenService } from './session-token.service.js';

@Module({
  imports: [PersistenceModule],
  controllers: [AuthController],
  providers: [
    { provide: AUTH_CONFIG, useValue: loadAuthConfig() },
    { provide: GITHUB_OAUTH_CLIENT, useClass: HttpGithubOAuthClient },
    AuthService,
    OAuthStateService,
    RefreshTokenService,
    SessionTokenService,
  ],
  exports: [SessionTokenService],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class AuthModule {}
