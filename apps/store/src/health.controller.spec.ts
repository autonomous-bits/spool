import { Test, type TestingModule } from '@nestjs/testing';
import { describe, expect, it } from 'vitest';
import { HealthController } from './health.controller.js';

describe('HealthController', () => {
  it('returns the store health payload', async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [HealthController],
    }).compile();

    const controller = module.get(HealthController);

    expect(controller.getHealth()).toEqual({
      status: 'ok',
      service: 'store',
    });
  });
});
