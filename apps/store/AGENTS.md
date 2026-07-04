# Store agent guide

## Purpose

`apps/store` is the NestJS knowledge store for Spool. It owns tenant-scoped idea chunks, typed graph relationships, lifecycle state, and generated document projections.

## Suggested structure

> The diagram below is a guide only. The actual files on disk are authoritative — always check the filesystem before assuming a file exists or a path is correct.

```
apps/store/
├── src/
│   ├── main.ts              # application bootstrap
│   ├── app.module.ts        # root NestJS module and feature wiring
│   ├── **/*.ts              # controllers, services, repositories, DTOs, domain modules
│   └── test/
│       └── setup.ts         # shared test setup and helpers
└── test/
    └── *.e2e-spec.ts        # end-to-end and integration tests
```

## Run tests

Tests use **[Vitest](https://vitest.dev)**. Do not use Jest.

From the repository root:

```sh
pnpm test:store
```

From `apps/store`:

```sh
pnpm test          # vitest run (all unit tests)
pnpm test:watch    # vitest watch mode
pnpm test:e2e      # vitest run test/**/*.e2e-spec.ts
pnpm test:coverage # vitest run --coverage
```

## Docker runtime

Run the store with Docker Compose from the repository root:

```sh
docker compose up --build spoolstore
```

Use the debug compose file only when debugging:

```sh
docker compose -f compose.debug.yaml up --build spoolstore
```

Do not run the store directly on the host for local development unless explicitly requested.

## Postgres persistence adapter

`apps/store/src/persistence/` holds the Postgres-backed adapter for the mainline chunk + edge-lineage graph (story S01), branch-scoped delta storage/resolution (story S02), pre-merge conflict detection (story S06), atomic branch-merge execution (story S07), durable downstream delivery subscriptions plus async merge-triggered dispatch (story S08), feedback/verification notification persistence and routing (story S09), and a conflict-gated canonical merge entrypoint plus append-only chunk merge-history (`ConflictGatedMergeService`, `chunk_history` — added post-implementation after rubber-duck review of Feature 01/02 against Meridian). It is not yet wired into `AppModule`; import `PersistenceModule` explicitly from a feature module that needs it.

Required connection env vars (see `config/store.env.example`): `STORE_DB_HOST`, `STORE_DB_PORT`, `STORE_DB_USER`, `STORE_DB_PASSWORD`, `STORE_DB_NAME`. There are no defaults — `loadDatabaseConfig()` fails fast if any is missing.

`BranchGraphRepository` stores a branch's own chunk/edge changes as delta rows (`branch_chunk_deltas`, `branch_edge_deltas`) separate from the mainline `chunks`/`edge_versions` tables, and resolves a branch's view (`resolveChunks`/`resolveEdgeLineages`) by combining those deltas with mainline records at read time, without ever mutating mainline rows.

`ChunkGraphRepository.replaceEdgeRelationshipType` (story S03) atomically changes a relationship's type: it closes out the old type's current lineage generation by appending a `deactivated` version (tagging it with a precise successor pointer — new type + generation, via `succeeded_by_relationship_type`/`succeeded_by_lineage_seq`) — never flipping the old generation's current row's state in place — and creates a brand-new active lineage generation for the new type, so history is never deleted or silently collapsed even across repeated type changes (e.g. A -> B -> A). Use `findEdgeLineageRecord`/`findPredecessorEdgeLineageRecord` to trace a relationship's history forward/backward across type changes.

Deactivation always appends a new version, whether it happens through a plain, same-type deactivation (`apps/store/src/domain/edge-lineage.ts`'s `deactivateEdge` marks the current version `superseded` and appends a `deactivated` version) or through a relationship-type change (`replaceEdgeRelationshipType`, above). A lineage's first version can only be `active`; `deactivated` can only appear as a lineage's *last* version — a lone stored row with state `deactivated` is invalid and `rowsToEdgeLineage` rejects it with `EdgeLineageError('lineage-violation')`. `saveEdgeLineage` only ever appends rows or marks an already-stored version's state to match a caller's lineage that also appends further versions; it never rewrites a stored version's terminal state without a new version appearing alongside it. (Clarified during S03 implementation review — see feature-01/feature-02 technical specs §"Edge lineage"/§"Edge lineage persistence".)

**Dev-data caveat**: local Postgres volumes created before this clarification may contain a lineage whose sole stored row is `state = 'deactivated'` (from the old in-place-mutation behavior). Such a row is invalid under the current invariant and cannot be safely backfilled; recreate the volume (`docker compose down -v` then `up -d postgres`) rather than attempting to migrate it in place.

`MergeRepository` (story S07) persists the feature-01 branch lifecycle status (`draft -> submitted -> verified -> merged`) for the first time — no prior story tracked it — via `submitBranch`/`verifyBranch`/`mergeBranch`, each of which reuses the corresponding pure guard in `domain/branch-lifecycle.ts` rather than re-deriving the state machine. `mergeBranch` runs as a single Postgres transaction: it locks the branch row `FOR UPDATE`, reads the branch's `branch_chunk_deltas`/`branch_edge_deltas` rows and its own `chunk_artifacts` rows, promotes each into mainline (reusing `resolveChunkDelta`/`resolveEdgeDelta` from `BranchGraphRepository` for chunks/edges, and a dedicated pure `resolveArtifactAssociationPromotion` matrix for associations), and flips the branch to `merged` — all under one `BEGIN`/`COMMIT`. Any failure at any step (including a legitimate domain error such as an edge delta that would reactivate an already-deactivated mainline edge) rolls back every change the merge attempted, including ones that would otherwise have succeeded on their own (AC1). Repository methods that must compose into this one transaction (`ChunkGraphRepository.saveChunk`/`findChunk`/`saveEdgeLineageOnClient`/`findEdgeLineageRecordOnClient`) accept an optional externally-supplied `client` instead of opening their own connection/transaction, while their public no-`client` forms are unchanged for non-merge callers.

Promoted chunks, edge versions, and chunk-artifact associations are stamped `origin_branch_id = branchId` so they remain traceable back to the merging branch afterward (AC3; technical spec §"Pre-merge history reconstruction", `IDEA-69`) — use `listChunksByOriginBranch`/`listEdgeIdentitiesByOriginBranch`/`listArtifactAssociationsByOriginBranch`. Every newly-written row (a promoted chunk, a newly-appended edge version, or a newly-appended chunk-artifact-association version) is stamped with the branch whose merge produced it, not with an earlier lineage's original creator — AC3 asks for traceability to the branch that produced each change, not just the branch that first created the identity. For `chunks` (a single mutable row per idea, story S01), this means provenance is only ever overwritten by an explicit merge (`saveChunk`'s `originBranchId` option); an ordinary non-merge mainline save never passes that option, so it leaves existing provenance untouched (`COALESCE`) rather than clearing it. Across successive *merges* of the same idea label by different branches, `chunks.origin_branch_id` itself is still last-writer-wins (the most recent merge's branch wins on that single mutable row) — but this is no longer a gap in the fidelity of history reconstruction: `MergeRepository.promoteChunkDelta` also appends one permanent row to the append-only `chunk_history` table (`schema.ts`) on every chunk-delta promotion, so the *full* sequence of merges that ever touched an idea label — including branches whose contribution was later overwritten on the mutable `chunks` row — remains independently reconstructable via `MergeRepository.listChunkHistoryByIdeaLabel`, ordered oldest-first by `merged_at`. (Added after rubber-duck review of Feature 01/02 against Meridian `IDEA-69` found the previous last-writer-wins-only behavior insufficient for full history reconstruction.)

Conflict detection prior to merge (`ConflictDetectionRepository`, story S06) is no longer merely an optional check callers may skip: `ConflictGatedMergeService.mergeBranch` (`conflict-gated-merge.service.ts`) is now the canonical, supported merge entrypoint and always calls `detectConflicts` first, refusing to merge (`EdgeLineageError('lineage-violation')`) if any chunk, edge, or chunk-artifact-association change was made independently on both branch and mainline since divergence. `MergeDeliveryOrchestrator` and any future controller/MCP wiring go through this service. `MergeRepository.mergeBranch` itself remains a lower-level, unconditional promotion primitive with no conflict check of its own — direct callers of it (existing tests, or a caller that has already performed its own conflict check) are unaffected, but it is no longer the recommended way to merge a branch. (Added after rubber-duck review of Feature 01/02 against Meridian found this enforcement missing from every production merge path.)

`BranchGraphRepository.saveChunkDelta`/`saveEdgeDelta` (story S11, technical spec §"Required domain error categories"; feature-01 tech spec §"Required lifecycle contracts — Branch", §"Discipline boundary") now enforce write-lock and discipline-boundary guards at the persistence layer before accepting a branch-scoped write: both methods open a transaction, lock the target branch's registration row with `SELECT ... FOR UPDATE` (mirroring `MergeRepository.lockBranchRow`, so a concurrent submit/verify cannot flip the branch out of `draft` between the check and the write), and reuse the existing pure domain guards `assertGraphWriteAllowed`/`assertDisciplineBoundaryForWrite` (`domain/branch-lifecycle.ts`) rather than re-deriving the rules. A write against an unregistered branch throws `BranchLifecycleError('not-found')`; a write against a branch that is not `draft` throws `BranchLifecycleError('write-locked')`. Discipline ownership for isolation purposes is resolved from the *existing* chunk (this branch's own prior delta for that idea label if one exists, else the mainline chunk), not merely from whatever discipline a delta's own payload declares — an engineering branch cannot launder a product-owned idea into an engineering one just by relabeling it in an override delta. If no existing chunk is found anywhere (a brand-new idea introduced entirely within this delta), the delta payload's own declared discipline is what's checked instead. Any mismatch against the branch's own registered discipline throws `BranchLifecycleError('branch-isolation-violation')`. `saveEdgeDelta` applies this same resolution to the edge's *source* label only (the target side is deliberately left unchecked, matching the technical spec's explicit allowance for a branch to "create cross-disciplinary edges to other disciplines' chunks only when it does not modify those target chunks") — if the source label has no known chunk anywhere yet, the edge write is allowed unchecked, since ownership cannot be evaluated against nothing. This supersedes the previous documented boundary decision (S02/S06/S07) that this enforcement belonged solely to a future NestJS API gateway — it is now enforced here, in addition to (not instead of) any future gateway-level check. Every caller of `saveChunkDelta`/`saveEdgeDelta` must therefore register its branch (`ConflictDetectionRepository.registerBranch` or `SuggestionRepository.acceptSuggestionAndRegisterBranch`) and complete any branch-scoped delta writes while the branch is still `draft`, before calling `MergeRepository.submitBranch`.

`DeliverySubscriptionRepository` (story S08) persists durable, workspace-scoped downstream push-delivery subscriptions (`delivery_subscriptions`: `webhook_url` plus an optional `disciplines` filter array — `NULL`/empty means "every discipline") via `registerSubscription`, an idempotent upsert keyed on `(workspace_id, consumer_id)` so re-registering the same consumer updates its preferences in place rather than erroring or duplicating (story S08 AC1). This table carries no delivery-attempt/outcome columns at all, so a consumer's registered preferences and any pull-style read of them (`getSubscription`/`listSubscriptions`/`listSubscriptionsForDiscipline`) are always independent of whether a push has ever been attempted or has ever succeeded (AC3; technical spec §"Delivery subscription persistence", `IDEA-65`).

`MergeDeliveryDispatcher` (story S08) is the async, non-blocking dispatch boundary for push delivery triggered by a merge (technical spec §"Downstream delivery split", `IDEA-63`): `dispatchMergeCompleted` schedules subscription lookup (`listSubscriptionsForDiscipline`) and push attempts (via an injected `DeliveryPushPort`) on a later event-loop tick (`setImmediate`) and returns synchronously, so a caller can never end up waiting on it (AC2). Every failure in that scheduled work — a subscription-lookup failure, a rejecting `push()`, or a synchronously-throwing `push()` — is caught and logged (`Logger`), never left as an unhandled rejection and never thrown back to the caller. `MergeDeliveryOrchestrator` is the thin post-commit wiring that actually connects the two: `mergeBranchAndDispatchDelivery` calls `MergeRepository.mergeBranch` (unchanged — no edits to its transaction), and only after that promise resolves (i.e. after `COMMIT` has already happened) calls `dispatchMergeCompleted` with the merged branch's own `discipline` (a small additive field on `MergeOutcome`, populated from data the merge transaction already loads via its existing branch-row lock — no new query, no transaction change). The orchestrator's own returned promise never waits on the scheduled dispatch work, so a merge caller using it still gets AC2's guarantee. `PersistenceModule` wires a `NoopDeliveryPushPort` by default; a consuming module should override the `DELIVERY_PUSH_PORT` provider with a real transport when the "background queue worker" from `IDEA-63` is actually built — that full durable outbox/retry-queue implementation (surviving a crash between merge-commit and dispatch) is a larger concern not specified by any source of authority available to this story.

`NotificationRepository` (story S09) persists advisory evaluation feedback (`feedback_items`) and verification signals (`verification_signals`) against a branch, and immediately routes each submission into a dedicated `notifications` table addressed to recipient stakeholders (technical spec §"Feedback notification routing", `IDEA-67`/`IDEA-68`). The recipient set is always resolved from durable data the repository itself looks up inside the same transaction as the insert — never from a caller-supplied recipient parameter — because trusting a request-supplied author would let a malicious or buggy caller misroute a notification to the wrong stakeholder. Concretely, `submitFeedbackItem`/`submitVerificationSignal` lock the target branch row `FOR UPDATE`, read its `author_stakeholder_id` (a new nullable column on `branches`, additive migration `MIGRATE_BRANCHES_AUTHOR`), and use that as the primary recipient; a branch with no recorded author (legacy data predating this story) fails the submission with `NotificationError('not-found')` rather than silently dropping the notification. `author_stakeholder_id` is populated going forward by `ConflictDetectionRepository.registerBranch`'s new optional trailing `authorStakeholderId` parameter and by `SuggestionRepository.acceptSuggestionAndRegisterBranch`, which now defaults it to the accepted suggestion's `decision.decidedByStakeholderId` automatically — both changes are backward compatible (existing call sites without the new argument are unaffected). `acknowledgeNotification` is intentionally non-destructive: it only ever sets `notifications.acknowledged_at` (`SET acknowledged_at = COALESCE(acknowledged_at, $3)`, so re-acknowledging is idempotent and first-ack-wins) and never touches `feedback_items`/`verification_signals` or any branch lifecycle column — evaluation history and branch state remain queryable and unaffected regardless of acknowledgement (AC4). Feedback/verification submission deliberately does not call `assertHumanActor`; per feature-01's "delegated agents" rule, advisory feedback and verification signals may come from delegated (agent) actors, not only humans — the provenance guarantee this story requires (AC5) is structural instead: `authoredByStakeholderId`/`reportedByStakeholderId` is always taken from the authenticated actor's own `stakeholderId`, never from a separate, independently-supplied field a caller could spoof. Duplicate resubmission of the same `feedback_item_id`/`signal_id` (a unique-constraint violation, SQLSTATE `23505`) maps to `NotificationError('invalid-state-transition')` rather than leaking a raw `pg` error, reusing the same category `BranchLifecycleError`/`ArtifactAssociationError` already use for a duplicate-identity write — the whole submission transaction (including any notification rows already inserted for that same call) rolls back together, so no partial state survives a duplicate-id failure.

**Known limitation**: `author_stakeholder_id` has no backfill for branches that existed before this story or that were registered through a call site that omitted it — submission against such a branch fails fast with `NotificationError('not-found')` rather than silently misrouting or dropping the notification. `PersistenceModule` is not yet wired into `AppModule` (no controller exposes this repository), so no such branches exist in any running deployment yet; wiring a controller for this repository must also guarantee every branch-creation path supplies an author before it can rely on notification routing.

To run the adapter's integration tests (`test/graph-persistence-adapter.e2e-spec.ts`, `test/branch-graph-persistence-adapter.e2e-spec.ts`, `test/edge-lineage-history-persistence-adapter.e2e-spec.ts`, `test/conflict-detection-persistence-adapter.e2e-spec.ts`, `test/merge-persistence-adapter.e2e-spec.ts`, `test/delivery-subscription-persistence-adapter.e2e-spec.ts`, `test/notification-persistence-adapter.e2e-spec.ts`) locally, start the compose Postgres service and export matching env vars for the host-published port:

```sh
docker compose up -d postgres
export STORE_DB_HOST=localhost STORE_DB_PORT=5433 \
  STORE_DB_USER=spool STORE_DB_PASSWORD=spool_dev STORE_DB_NAME=spool
pnpm test:e2e
```

These tests require a real containerized Postgres — they do not run against an in-memory substitute.

