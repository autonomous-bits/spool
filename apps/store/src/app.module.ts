import { Module } from '@nestjs/common';
import { ArtifactsModule } from './artifacts/artifacts.module.js';
import { AuthModule } from './auth/auth.module.js';
import { BranchesModule } from './branches/branches.module.js';
import { ChunksModule } from './chunks/chunks.module.js';
import { EdgesModule } from './edges/edges.module.js';
import { HealthController } from './health.controller.js';
import { NotificationsModule } from './notifications/notifications.module.js';
import { PersistenceModule } from './persistence/persistence.module.js';
import { SuggestionsModule } from './suggestions/suggestions.module.js';
import { VerificationSignalsModule } from './verification-signals/verification-signals.module.js';
import { WorkspacesModule } from './workspaces/workspaces.module.js';

@Module({
  imports: [
    PersistenceModule,
    ChunksModule,
    BranchesModule,
    EdgesModule,
    AuthModule,
    SuggestionsModule,
    ArtifactsModule,
    VerificationSignalsModule,
    NotificationsModule,
    WorkspacesModule,
  ],
  controllers: [HealthController],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class AppModule {}
