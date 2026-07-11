import { Module } from '@nestjs/common';
import { AuthModule } from '../auth/auth.module.js';
import { PersistenceModule } from '../persistence/persistence.module.js';
import { DeliverySubscriptionsController } from './delivery-subscriptions.controller.js';
import { DeliverySubscriptionsService } from './delivery-subscriptions.service.js';

@Module({
  imports: [PersistenceModule, AuthModule],
  controllers: [DeliverySubscriptionsController],
  providers: [DeliverySubscriptionsService],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class DeliverySubscriptionsModule {}
