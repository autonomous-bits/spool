import { Injectable, Module, type OnApplicationShutdown } from '@nestjs/common';
import { Pool } from 'pg';
import { ArtifactRepository } from './artifact.repository.js';
import { ARTIFACT_BLOB_STORE } from './artifact-blob-store.token.js';
import { BranchRepository } from './branch.repository.js';
import { ChunkArtifactRepository } from './chunk-artifact.repository.js';
import { ChunkRepository } from './chunk.repository.js';
import { DeliveryAttemptRepository } from './delivery-attempt.repository.js';
import { DeliverySubscriptionRepository } from './delivery-subscription.repository.js';
import { EdgeRepository } from './edge.repository.js';
import { FeedbackNotificationRepository } from './feedback-notification.repository.js';
import { LocalFileBlobStore } from './local-file-blob-store.js';
import { loadLocalFileBlobStoreConfig } from './local-file-blob-store-config.js';
import { LOCAL_FILE_BLOB_STORE_CONFIG } from './local-file-blob-store-config.token.js';
import { StakeholderRepository } from './stakeholder.repository.js';
import { SuggestionRepository } from './suggestion.repository.js';
import { VerificationSignalRepository } from './verification-signal.repository.js';
import { WorkspaceRepository } from './workspace.repository.js';
import { loadDatabaseConfig } from './database-config.js';
import { PG_POOL } from './pg-pool.token.js';

/**
 * Wraps the shared `pg.Pool` in an injectable provider that closes on application shutdown, per
 * the node-memory-management guidance for long-lived resources (OnApplicationShutdown, not a
 * manual process hook).
 */
@Injectable()
class PgPoolProvider implements OnApplicationShutdown {
  readonly pool = new Pool(loadDatabaseConfig());

  async onApplicationShutdown(): Promise<void> {
    await this.pool.end();
  }
}

@Module({
  providers: [
    PgPoolProvider,
    {
      provide: PG_POOL,
      useFactory: (provider: PgPoolProvider) => provider.pool,
      inject: [PgPoolProvider],
    },
    {
      provide: LOCAL_FILE_BLOB_STORE_CONFIG,
      useFactory: () => loadLocalFileBlobStoreConfig(),
    },
    {
      provide: ARTIFACT_BLOB_STORE,
      useClass: LocalFileBlobStore,
    },
    ChunkRepository,
    BranchRepository,
    EdgeRepository,
    StakeholderRepository,
    SuggestionRepository,
    ArtifactRepository,
    ChunkArtifactRepository,
    DeliveryAttemptRepository,
    VerificationSignalRepository,
    FeedbackNotificationRepository,
    WorkspaceRepository,
    DeliverySubscriptionRepository,
  ],
  exports: [
    PG_POOL,
    ARTIFACT_BLOB_STORE,
    ChunkRepository,
    BranchRepository,
    EdgeRepository,
    StakeholderRepository,
    SuggestionRepository,
    ArtifactRepository,
    ChunkArtifactRepository,
    DeliveryAttemptRepository,
    VerificationSignalRepository,
    FeedbackNotificationRepository,
    WorkspaceRepository,
    DeliverySubscriptionRepository,
  ],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class PersistenceModule {}
