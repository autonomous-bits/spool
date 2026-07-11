import { Module } from '@nestjs/common';
import { PersistenceModule } from '../persistence/persistence.module.js';
import { AuthModule } from '../auth/auth.module.js';
import { EdgesController } from './edges.controller.js';
import { EdgesService } from './edges.service.js';

@Module({
  imports: [PersistenceModule, AuthModule],
  controllers: [EdgesController],
  providers: [EdgesService],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class EdgesModule {}
