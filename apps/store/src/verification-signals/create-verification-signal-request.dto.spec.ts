import { BadRequestException } from '@nestjs/common';
import { describe, expect, it } from 'vitest';
import { parseCreateVerificationSignalRequest } from './create-verification-signal-request.dto.js';

describe('parseCreateVerificationSignalRequest', () => {
  it('parses a valid body without a reason', () => {
    const request = parseCreateVerificationSignalRequest({
      verifierName: 'ci-evaluator',
      status: 'pass',
    });

    expect(request).toEqual({ verifierName: 'ci-evaluator', status: 'pass' });
  });

  it('parses a valid body with a reason', () => {
    const request = parseCreateVerificationSignalRequest({
      verifierName: 'ci-evaluator',
      status: 'fail',
      reason: 'missing tests',
    });

    expect(request).toEqual({
      verifierName: 'ci-evaluator',
      status: 'fail',
      reason: 'missing tests',
    });
  });

  it('does not accept a client-supplied reportedByStakeholderId field', () => {
    const request = parseCreateVerificationSignalRequest({
      verifierName: 'ci-evaluator',
      status: 'pass',
      reportedByStakeholderId: 'caller-controlled',
    });

    expect(request).toEqual({ verifierName: 'ci-evaluator', status: 'pass' });
    expect('reportedByStakeholderId' in request).toBe(false);
  });

  it('rejects a non-object body', () => {
    expect(() => parseCreateVerificationSignalRequest(null)).toThrow(BadRequestException);
    expect(() => parseCreateVerificationSignalRequest('nope')).toThrow(BadRequestException);
  });

  it.each(['', '   ', undefined, 42])('rejects an invalid verifierName %j', (verifierName) => {
    expect(() =>
      parseCreateVerificationSignalRequest({ verifierName, status: 'pass' }),
    ).toThrow(BadRequestException);
  });

  it.each(['PASS', 'unknown', undefined, 42])('rejects an invalid status %j', (status) => {
    expect(() =>
      parseCreateVerificationSignalRequest({ verifierName: 'ci-evaluator', status }),
    ).toThrow(BadRequestException);
  });

  it('rejects a non-string reason', () => {
    expect(() =>
      parseCreateVerificationSignalRequest({
        verifierName: 'ci-evaluator',
        status: 'pass',
        reason: 42,
      }),
    ).toThrow(BadRequestException);
  });
});
