import { BadRequestException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthController } from './auth.controller.js';
import { AuthService } from './auth.service.js';

describe('AuthController', () => {
  let controller: AuthController;
  let service: Pick<
    AuthService,
    'buildLoginRedirectUrl' | 'handleCallback' | 'exchangePairingCode' | 'refreshSession'
  >;
  let response: { redirect: ReturnType<typeof vi.fn> };

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [AuthController],
      providers: [
        {
          provide: AuthService,
          useValue: {
            buildLoginRedirectUrl: vi.fn(),
            handleCallback: vi.fn(),
            exchangePairingCode: vi.fn(),
            refreshSession: vi.fn(),
          } satisfies Pick<
            AuthService,
            'buildLoginRedirectUrl' | 'handleCallback' | 'exchangePairingCode' | 'refreshSession'
          >,
        },
      ],
    }).compile();

    controller = module.get(AuthController);
    service = module.get(AuthService);
    response = {
      redirect: vi.fn(),
    };
  });

  it('login redirects (302) to the URL built by AuthService', () => {
    vi.mocked(service.buildLoginRedirectUrl).mockReturnValue('https://github.com/login/oauth/authorize?state=abc');

    const result = controller.login();

    expect(service.buildLoginRedirectUrl).toHaveBeenCalledWith(undefined, undefined);
    expect(result).toEqual({
      url: 'https://github.com/login/oauth/authorize?state=abc',
      statusCode: 302,
    });
  });

  it('login passes workspaceId and cliRedirectUri query params through to AuthService', () => {
    vi.mocked(service.buildLoginRedirectUrl).mockReturnValue('https://github.com/login/oauth/authorize?state=abc');

    controller.login('workspace-1', 'http://127.0.0.1:4318/callback');

    expect(service.buildLoginRedirectUrl).toHaveBeenCalledWith(
      'workspace-1',
      'http://127.0.0.1:4318/callback',
    );
  });

  it('callback delegates to AuthService and wraps the token set in a response body', async () => {
    vi.mocked(service.handleCallback).mockResolvedValue({
      kind: 'tokens',
      sessionToken: 'signed-session-token',
      refreshToken: 'refresh-token-1',
      expiresAt: 1_756_360_200,
    });

    const result = await controller.callback('a-code', 'a-state', response);

    expect(service.handleCallback).toHaveBeenCalledWith('a-code', 'a-state');
    expect(result).toEqual({
      sessionToken: 'signed-session-token',
      refreshToken: 'refresh-token-1',
      expiresAt: 1_756_360_200,
    });
  });

  it('callback returns a redirect response when AuthService requests loopback hand-off', async () => {
    vi.mocked(service.handleCallback).mockResolvedValue({
      kind: 'redirect',
      redirectUrl: 'http://127.0.0.1:4318/callback?code=pairing-code-1',
    });

    await expect(controller.callback('a-code', 'a-state', response)).resolves.toBeUndefined();
    expect(response.redirect).toHaveBeenCalledWith(
      302,
      'http://127.0.0.1:4318/callback?code=pairing-code-1',
    );
  });

  it('pairing exchange delegates to AuthService and returns the exchanged tokens', async () => {
    vi.mocked(service.exchangePairingCode).mockResolvedValue({
      sessionToken: 'signed-session-token',
      refreshToken: 'refresh-token-1',
    });

    const result = await controller.exchangePairingCode({ code: 'pairing-code-1' });

    expect(service.exchangePairingCode).toHaveBeenCalledWith('pairing-code-1');
    expect(result).toEqual({
      sessionToken: 'signed-session-token',
      refreshToken: 'refresh-token-1',
    });
  });

  it('pairing exchange rejects a malformed body before calling AuthService', async () => {
    await expect(controller.exchangePairingCode({})).rejects.toThrow(BadRequestException);
    expect(service.exchangePairingCode).not.toHaveBeenCalled();
  });

  it('pairing exchange propagates generic invalid-or-expired errors for unknown codes', async () => {
    vi.mocked(service.exchangePairingCode).mockRejectedValue(
      new BadRequestException('Invalid or expired pairing code'),
    );

    await expect(controller.exchangePairingCode({ code: 'unknown-code' })).rejects.toThrow(
      BadRequestException,
    );
  });

  it('pairing exchange propagates generic invalid-or-expired errors for expired codes', async () => {
    vi.mocked(service.exchangePairingCode).mockRejectedValue(
      new BadRequestException('Invalid or expired pairing code'),
    );

    await expect(controller.exchangePairingCode({ code: 'expired-code' })).rejects.toThrow(
      BadRequestException,
    );
  });

  it('pairing exchange propagates generic invalid-or-expired errors for already consumed codes', async () => {
    vi.mocked(service.exchangePairingCode).mockRejectedValue(
      new BadRequestException('Invalid or expired pairing code'),
    );

    await expect(controller.exchangePairingCode({ code: 'consumed-code' })).rejects.toThrow(
      BadRequestException,
    );
  });

  it('refresh delegates to AuthService and wraps the rotated token set in a response body', async () => {
    vi.mocked(service.refreshSession).mockResolvedValue({
      sessionToken: 'signed-session-token-2',
      refreshToken: 'refresh-token-2',
      expiresAt: 1_756_360_500,
    });

    const result = await controller.refresh({ refreshToken: 'refresh-token-1' });

    expect(service.refreshSession).toHaveBeenCalledWith('refresh-token-1');
    expect(result).toEqual({
      sessionToken: 'signed-session-token-2',
      refreshToken: 'refresh-token-2',
      expiresAt: 1_756_360_500,
    });
  });

  it('refresh rejects a malformed body before calling AuthService', async () => {
    await expect(controller.refresh({})).rejects.toThrow(BadRequestException);
    expect(service.refreshSession).not.toHaveBeenCalled();
  });
});
