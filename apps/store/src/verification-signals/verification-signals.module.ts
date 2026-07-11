import { Module } from '@nestjs/common';
import { PersistenceModule } from '../persistence/persistence.module.js';
import { VerificationSignalsController } from './verification-signals.controller.js';
import { VerificationSignalsService } from './verification-signals.service.js';

@Module({
  imports: [PersistenceModule],
  controllers: [VerificationSignalsController],
  providers: [VerificationSignalsService],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class VerificationSignalsModule {}
