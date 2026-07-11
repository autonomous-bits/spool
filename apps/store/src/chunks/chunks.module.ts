import { Module } from '@nestjs/common';
import { PersistenceModule } from '../persistence/persistence.module.js';
import { ChunksController } from './chunks.controller.js';
import { ChunksService } from './chunks.service.js';

@Module({
  imports: [PersistenceModule],
  controllers: [ChunksController],
  providers: [ChunksService],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class ChunksModule {}
