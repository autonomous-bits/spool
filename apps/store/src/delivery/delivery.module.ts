import { Module } from '@nestjs/common';
import { PersistenceModule } from '../persistence/persistence.module.js';
import { DeliveryWorkerService } from './delivery-worker.service.js';

@Module({
  imports: [PersistenceModule],
  providers: [DeliveryWorkerService],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class DeliveryModule {}
