import { Module } from '@nestjs/common';
import { DatabaseModule } from './database-pool.provider.js';
import { ChunkGraphRepository } from './chunk-graph.repository.js';
import { BranchGraphRepository } from './branch-graph.repository.js';
import { SuggestionRepository } from './suggestion.repository.js';
import { ArtifactAssociationRepository } from './artifact-association.repository.js';
import { ConflictDetectionRepository } from './conflict-detection.repository.js';
import { MergeRepository } from './merge.repository.js';
import { ConflictGatedMergeService } from './conflict-gated-merge.service.js';
import { DeliverySubscriptionRepository } from './delivery-subscription.repository.js';
import {
  DELIVERY_PUSH_PORT,
  MergeDeliveryDispatcher,
  type DeliveryPushPort,
} from './merge-delivery-dispatcher.js';
import { MergeDeliveryOrchestrator } from './merge-delivery-orchestrator.js';
import { NotificationRepository } from './notification.repository.js';

/**
 * Default push transport: intentionally a no-op. This module's slice owns
 * subscription persistence and the async dispatch boundary (story S08), not
 * a concrete webhook/egress client — that belongs to whichever future story
 * implements the actual "background queue worker" (`IDEA-63`). A consuming
 * module can override this provider with `DELIVERY_PUSH_PORT` to supply a
 * real transport.
 */
class NoopDeliveryPushPort implements DeliveryPushPort {
  async push(): Promise<void> {
    // Intentionally empty: no default transport is wired yet.
  }
}

/**
 * Persistence slice for the mainline chunk + edge-lineage graph (story S01),
 * branch-scoped delta storage/resolution (story S02), the suggestion review
 * queue plus branch-origin registration (story S04), chunk-artifact
 * association lineages (story S05), pre-merge conflict detection
 * (story S06), atomic branch-merge execution (story S07), durable
 * downstream delivery subscriptions plus async merge-triggered dispatch
 * (story S08), evaluation feedback / verification signal notification
 * routing with non-destructive acknowledgement (story S09), and a
 * conflict-gated canonical merge entrypoint (`ConflictGatedMergeService`)
 * added after rubber-duck review of Feature 01/02 against Meridian found
 * that no production merge path enforced pre-merge conflict detection.
 *
 * Not imported by `AppModule` yet: this feature's functional spec lists API
 * design as a non-goal, and this story's deliverable is explicitly "a
 * persistence adapter ... plus adapter-level tests" rather than a wired
 * route. Future stories that add controllers/MCP wiring should import this
 * module directly.
 */
@Module({
  imports: [DatabaseModule],
  providers: [
    ChunkGraphRepository,
    BranchGraphRepository,
    SuggestionRepository,
    ArtifactAssociationRepository,
    ConflictDetectionRepository,
    // MergeRepository stays a provider (ConflictGatedMergeService injects
    // it) but is deliberately NOT re-exported below — see the note on
    // `exports`.
    MergeRepository,
    ConflictGatedMergeService,
    DeliverySubscriptionRepository,
    { provide: DELIVERY_PUSH_PORT, useClass: NoopDeliveryPushPort },
    MergeDeliveryDispatcher,
    MergeDeliveryOrchestrator,
    NotificationRepository,
  ],
  exports: [
    ChunkGraphRepository,
    BranchGraphRepository,
    SuggestionRepository,
    ArtifactAssociationRepository,
    ConflictDetectionRepository,
    // `MergeRepository` is intentionally NOT exported here (fixes a gap
    // found during rubber-duck review of Feature 01/02 against Meridian:
    // `MergeRepository.mergeBranch` is an unconditional promotion
    // primitive with no pre-merge conflict check of its own, and remaining
    // freely DI-injectable made it too easy for a future consuming module
    // to bypass `ConflictGatedMergeService`'s required conflict gate
    // without noticing). A consuming module that needs to merge a branch
    // must inject `ConflictGatedMergeService`, which always runs
    // `ConflictDetectionRepository.detectConflicts` first and only then
    // delegates to `MergeRepository.mergeBranch` internally. Direct
    // construction of `MergeRepository` (bypassing Nest DI entirely, e.g.
    // in this module's own adapter tests) is unaffected by this change.
    ConflictGatedMergeService,
    DeliverySubscriptionRepository,
    MergeDeliveryDispatcher,
    MergeDeliveryOrchestrator,
    NotificationRepository,
  ],
})
export class PersistenceModule {}
