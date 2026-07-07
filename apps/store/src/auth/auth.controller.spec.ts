import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthController } from './auth.controller.js';
import { AuthService } from './auth.service.js';

describe('AuthController', () => {
  let controller: AuthController;
  let service: Pick<AuthService, 'buildLoginRedirectUrl' | 'handleCallback'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [AuthController],
      providers: [
        {
          provide: AuthService,
          useValue: {
            buildLoginRedirectUrl: vi.fn(),
            handleCallback: vi.fn(),
          } satisfies Pick<AuthService, 'buildLoginRedirectUrl' | 'handleCallback'>,
        },
      ],
    }).compile();

    controller = module.get(AuthController);
    service = module.get(AuthService);
  });

  it('login redirects (302) to the URL built by AuthService', () => {
    vi.mocked(service.buildLoginRedirectUrl).mockReturnValue('https://github.com/login/oauth/authorize?state=abc');

    const result = controller.login();

    expect(service.buildLoginRedirectUrl).toHaveBeenCalledWith(undefined);
    expect(result).toEqual({
      url: 'https://github.com/login/oauth/authorize?state=abc',
      statusCode: 302,
    });
  });

  it('login passes an explicit workspaceId query param through to AuthService', () => {
    vi.mocked(service.buildLoginRedirectUrl).mockReturnValue('https://github.com/login/oauth/authorize?state=abc');

    controller.login('workspace-1');

    expect(service.buildLoginRedirectUrl).toHaveBeenCalledWith('workspace-1');
  });

  it('callback delegates to AuthService and wraps the token in a response body', async () => {
    vi.mocked(service.handleCallback).mockResolvedValue('signed-session-token');

    const result = await controller.callback('a-code', 'a-state');

    expect(service.handleCallback).toHaveBeenCalledWith('a-code', 'a-state');
    expect(result).toEqual({ sessionToken: 'signed-session-token' });
  });
});
