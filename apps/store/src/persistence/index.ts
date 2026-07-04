export {
  loadDatabaseConfig,
  DatabaseConfigError,
  type DatabaseConfig,
} from './database-config.js';
export { DatabaseModule, PG_POOL } from './database-pool.provider.js';
export { ensureSchema } from './schema.js';
export {
  ChunkGraphRepository,
  mapPersistenceError,
  type PersistedChunk,
  type EdgeLineageRecord,
} from './chunk-graph.repository.js';
export {
  BranchGraphRepository,
  resolveChunkDelta,
  resolveEdgeDelta,
  type BranchChunkDelta,
  type BranchChunkDeltaKind,
  type BranchEdgeDelta,
  type BranchEdgeDeltaKind,
} from './branch-graph.repository.js';
export {
  SuggestionRepository,
  type PersistedSuggestion,
  type NewSuggestionInput,
  type PersistedBranchRegistration,
  type AcceptedSuggestionResult,
} from './suggestion.repository.js';
export {
  ArtifactAssociationRepository,
  type PersistedArtifactAssociation,
  type NewArtifactAssociationInput,
} from './artifact-association.repository.js';
export {
  ConflictDetectionRepository,
  type ChunkChange,
  type EdgeChange,
  type ArtifactAssociationChange,
  type MainlineChangesSinceDivergence,
  type ConflictReport,
} from './conflict-detection.repository.js';
export {
  MergeRepository as MergeRepositoryUnsafeSkipConflictCheck,
  resolveArtifactAssociationPromotion,
  type BranchLifecycleStatus,
  type MergeOutcome,
} from './merge.repository.js';
// The canonical, conflict-gated merge entrypoint: always runs pre-merge
// conflict detection before delegating to `MergeRepository.mergeBranch`
// internally. Prefer this over the unsafe re-export above.
export { ConflictGatedMergeService } from './conflict-gated-merge.service.js';
export {
  DeliverySubscriptionRepository,
  type PersistedDeliverySubscription,
  type DeliverySubscriptionInput,
} from './delivery-subscription.repository.js';
export {
  MergeDeliveryDispatcher,
  DELIVERY_PUSH_PORT,
  type MergeDeliveryEvent,
  type DeliveryPushPort,
} from './merge-delivery-dispatcher.js';
export { MergeDeliveryOrchestrator } from './merge-delivery-orchestrator.js';
export {
  NotificationRepository,
  type PersistedFeedbackItem,
  type PersistedVerificationSignal,
  type PersistedNotification,
  type AdditionalNotificationRecipients,
  type SubmissionResult,
} from './notification.repository.js';
export { PersistenceModule } from './persistence.module.js';
