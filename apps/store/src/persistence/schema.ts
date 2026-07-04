/**
 * Idempotent DDL bootstrap for the mainline chunk + edge-lineage graph, plus
 * branch-scoped delta tables (story S02).
 *
 * Sources of authority:
 * - Technical spec §"Store owns persistence", §"Tenant isolation",
 *   §"Edge lineage persistence", §"Delta-based branch storage".
 * - Story S01 out-of-scope (still true for the mainline tables): no
 *   suggestion, artifact, subscription, or notification tables belong in
 *   this schema yet.
 * - Story S02 out-of-scope: no divergence-marker column, merge transaction,
 *   suggestion, or artifact-association table belongs in this schema yet.
 *
 * Every table carries `workspace_id` in its primary key so cross-workspace
 * reads are structurally impossible without an explicit, deliberate join.
 */

import type { Pool } from 'pg';

const CREATE_CHUNKS_TABLE = `
  CREATE TABLE IF NOT EXISTS chunks (
    workspace_id TEXT NOT NULL,
    idea_label TEXT NOT NULL,
    chunk_type TEXT NOT NULL,
    discipline TEXT NOT NULL,
    context_kind TEXT NOT NULL,
    content TEXT NOT NULL,
    lifecycle_state TEXT NOT NULL,
    activity_state TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, idea_label)
  );
`;

/**
 * Additive, idempotent migration (story S07, technical spec §"Pre-merge
 * history reconstruction", `IDEA-69`) so a mainline chunk promoted from a
 * branch merge remains traceable back to that branch. `NULL` for a chunk
 * that was written directly on mainline (never promoted from a branch).
 *
 * Caveat, deliberately not "fixed" by this story: `chunks` is a single
 * mutable row per `(workspace_id, idea_label)` (story S01) rather than an
 * append-only lineage like `edge_versions`, so this column is last-writer-
 * wins — a later, unrelated branch merge that touches the same idea label
 * overwrites an earlier branch's provenance for that label. Redesigning
 * mainline chunks as an append-only history is out of this story's scope
 * (merge-execution atomicity and traceability, not a chunk-history
 * redesign); `edge_versions.origin_branch_id` below has no such limitation
 * because every version row is permanent once inserted.
 */
const MIGRATE_CHUNKS_ORIGIN_BRANCH_ID = `
  ALTER TABLE chunks ADD COLUMN IF NOT EXISTS origin_branch_id TEXT;
`;

/**
 * `lineage_seq` distinguishes successive *generations* of a relationship
 * identity (story S03, technical spec §"Edge lineage persistence"): when a
 * relationship's type is replaced, the old type's lineage is deactivated
 * (never deleted or reused) and a brand-new lineage is created for the new
 * type. If that new type is later replaced back to the original type (e.g.
 * A -> B -> A), the second "A" lineage is a new generation of the same
 * `(workspace_id, source_label, target_label, relationship_type)` identity,
 * so `lineage_seq` must be part of the primary key to avoid colliding with
 * the first (now-deactivated) A generation's rows.
 *
 * `succeeded_by_relationship_type` / `succeeded_by_lineage_seq` are set only
 * on the terminal row of a lineage generation that was replaced by a
 * relationship-type change, and together they precisely identify the
 * successor lineage generation (never just a type, which would be ambiguous
 * across repeated type changes) — satisfying the technical spec's "preserve
 * the old type as a deactivated, lineage-linked record referencing it".
 */
const CREATE_EDGE_VERSIONS_TABLE = `
  CREATE TABLE IF NOT EXISTS edge_versions (
    workspace_id TEXT NOT NULL,
    source_label TEXT NOT NULL,
    target_label TEXT NOT NULL,
    relationship_type TEXT NOT NULL,
    lineage_seq INTEGER NOT NULL DEFAULT 1,
    version INTEGER NOT NULL,
    state TEXT NOT NULL,
    succeeded_by_relationship_type TEXT,
    succeeded_by_lineage_seq INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, source_label, target_label, relationship_type, lineage_seq, version)
  );
`;

/**
 * Additive, idempotent migration for the `edge_versions` table so that a
 * database bootstrapped before story S03 (which only had
 * `(workspace_id, source_label, target_label, relationship_type, version)`)
 * picks up the `lineage_seq` generation column and the `succeeded_by_*`
 * cross-lineage link columns without requiring a destructive reset. Safe to
 * run repeatedly: `ADD COLUMN IF NOT EXISTS` no-ops once applied, and the
 * primary key is only re-created if it does not already include
 * `lineage_seq`.
 */
const MIGRATE_EDGE_VERSIONS_LINEAGE_SEQ = `
  ALTER TABLE edge_versions ADD COLUMN IF NOT EXISTS lineage_seq INTEGER NOT NULL DEFAULT 1;
  ALTER TABLE edge_versions ADD COLUMN IF NOT EXISTS succeeded_by_relationship_type TEXT;
  ALTER TABLE edge_versions ADD COLUMN IF NOT EXISTS succeeded_by_lineage_seq INTEGER;
  DO $$
  BEGIN
    IF NOT EXISTS (
      SELECT 1 FROM information_schema.key_column_usage
      WHERE table_name = 'edge_versions'
        AND constraint_name = 'edge_versions_pkey'
        AND column_name = 'lineage_seq'
    ) THEN
      ALTER TABLE edge_versions DROP CONSTRAINT edge_versions_pkey;
      ALTER TABLE edge_versions ADD PRIMARY KEY (
        workspace_id, source_label, target_label, relationship_type, lineage_seq, version
      );
    END IF;
  END $$;
`;

/**
 * Enforces the "at most one active edge of a given relationship type" rule
 * (technical spec §"Edge lineage persistence", feature-01 edge determinism)
 * at the database level, so concurrent writers cannot race past an
 * application-level check.
 */
const CREATE_EDGE_ACTIVE_UNIQUE_INDEX = `
  CREATE UNIQUE INDEX IF NOT EXISTS edge_versions_one_active_idx
    ON edge_versions (workspace_id, source_label, target_label, relationship_type)
    WHERE state = 'active';
`;

/**
 * Additive, idempotent migration (story S07, technical spec §"Pre-merge
 * history reconstruction", `IDEA-69`) so each persisted edge version can be
 * traced back to the branch merge that produced it. Unlike `chunks`, this
 * is a genuinely permanent provenance record: `edge_versions` is append-
 * only (a version row is never mutated or deleted once inserted — store
 * AGENTS.md), so a version's `origin_branch_id` is stamped once, at insert
 * time, and never overwritten by a later merge touching the same identity
 * (a later merge only ever appends a *new* version row with its own
 * `origin_branch_id`). `NULL` for a version written directly on mainline.
 */
const MIGRATE_EDGE_VERSIONS_ORIGIN_BRANCH_ID = `
  ALTER TABLE edge_versions ADD COLUMN IF NOT EXISTS origin_branch_id TEXT;
`;

/**
 * Branch-scoped delta record for a chunk (story S02, technical spec
 * §"Delta-based branch storage"). One row per (workspace, branch, idea
 * label): `delta_kind = 'upsert'` carries the branch's own version of the
 * chunk (addition or override); `delta_kind = 'delete'` hides a mainline
 * chunk from the branch's resolved view without mutating the mainline
 * `chunks` row. Entirely separate from `chunks` so mainline reads never
 * observe branch drafts (AC1/AC3).
 */
const CREATE_BRANCH_CHUNK_DELTAS_TABLE = `
  CREATE TABLE IF NOT EXISTS branch_chunk_deltas (
    workspace_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    idea_label TEXT NOT NULL,
    delta_kind TEXT NOT NULL,
    chunk_type TEXT,
    discipline TEXT,
    context_kind TEXT,
    content TEXT,
    lifecycle_state TEXT,
    activity_state TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, branch_id, idea_label)
  );
`;

/**
 * Branch-scoped delta record for an edge identity (story S02). One row per
 * (workspace, branch, source label, target label, relationship type):
 * `delta_kind = 'upsert'` asserts the identity is active in the branch's
 * view (a branch addition); `delta_kind = 'deactivate'` hides/deactivates
 * the identity in the branch's view. Entirely separate from
 * `edge_versions` so mainline lineage reads never observe branch drafts.
 */
const CREATE_BRANCH_EDGE_DELTAS_TABLE = `
  CREATE TABLE IF NOT EXISTS branch_edge_deltas (
    workspace_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    source_label TEXT NOT NULL,
    target_label TEXT NOT NULL,
    relationship_type TEXT NOT NULL,
    delta_kind TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, branch_id, source_label, target_label, relationship_type)
  );
`;

/**
 * Suggestion queue persistence (story S04, technical spec §"Suggestion
 * persistence"; Meridian `IDEA-49`/`IDEA-28`: "External AI-generated
 * feedback (supporting chunk and edge details) is persisted in a
 * suggestions table ... with 'pending' status, requiring human stakeholder
 * review"). `payload` is JSONB rather than a normalized chunk/edge shape:
 * this story's deliverable is "stores suggestions with their review
 * status", not chunk-artifact-association versioning (explicitly out of
 * scope), so the exact shape of a proposed chunk/edge modification is left
 * to the caller.
 *
 * The `state` CHECK constraint mirrors the feature-01 `SuggestionState`
 * machine (`pending | accepted | rejected`) at the database level so
 * malformed writes cannot bypass the domain's lifecycle invariant.
 * `suggestions_decision_matches_state` additionally enforces that decision
 * provenance (`decided_by_stakeholder_id`, `decided_at`) is present if and
 * only if the suggestion has actually been decided, so a `pending` row can
 * never carry stale decision metadata and an `accepted`/`rejected` row can
 * never lose its provenance.
 */
const CREATE_SUGGESTIONS_TABLE = `
  CREATE TABLE IF NOT EXISTS suggestions (
    workspace_id TEXT NOT NULL,
    suggestion_id TEXT NOT NULL,
    discipline TEXT NOT NULL,
    payload JSONB NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'accepted', 'rejected')),
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_by_stakeholder_id TEXT,
    decided_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, suggestion_id),
    CONSTRAINT suggestions_decision_matches_state CHECK (
      (state = 'pending' AND decided_by_stakeholder_id IS NULL AND decided_at IS NULL)
      OR (state IN ('accepted', 'rejected') AND decided_by_stakeholder_id IS NOT NULL AND decided_at IS NOT NULL)
    )
  );
`;

/**
 * Minimal branch registration table (story S04). Full branch lifecycle
 * (divergence markers, submitted/verified/merged state) is later stories'
 * scope; this table exists solely to host `origin_suggestion_id` so an
 * accepted suggestion's initiated branch durably tracks its source, per
 * Meridian `IDEA-49`: "Initiated branches track their source via an
 * `origin_suggestion_id` foreign key." `origin_suggestion_id` is nullable
 * because not every branch originates from a suggestion.
 */
const CREATE_BRANCHES_TABLE = `
  CREATE TABLE IF NOT EXISTS branches (
    workspace_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    discipline TEXT NOT NULL,
    origin_suggestion_id TEXT,
    diverged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, branch_id),
    FOREIGN KEY (workspace_id, origin_suggestion_id) REFERENCES suggestions (workspace_id, suggestion_id)
  );
`;

/**
 * Additive, idempotent migration for the `branches` table so that a database
 * bootstrapped before story S06 (which only had `workspace_id, branch_id,
 * discipline, origin_suggestion_id, created_at`) picks up the divergence
 * marker (technical spec §"Divergence tracking", `IDEA-41`: "Branches
 * divergence point is defined by recording a diverged_at timestamp").
 *
 * Backfills `diverged_at`/`updated_at` from the existing `created_at` for any
 * pre-existing row — never from `now()` — so a branch registered before this
 * migration ran still gets a divergence marker matching its actual creation
 * time, not the moment the migration happened to execute. Defaulting to
 * `now()` here would silently hide genuine mainline changes that occurred
 * between the branch's real creation and this migration running (found
 * during S06 plan review).
 */
const MIGRATE_BRANCHES_DIVERGENCE_MARKER = `
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS diverged_at TIMESTAMPTZ;
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
  UPDATE branches SET diverged_at = created_at WHERE diverged_at IS NULL;
  UPDATE branches SET updated_at = created_at WHERE updated_at IS NULL;
  ALTER TABLE branches ALTER COLUMN diverged_at SET DEFAULT now();
  ALTER TABLE branches ALTER COLUMN diverged_at SET NOT NULL;
  ALTER TABLE branches ALTER COLUMN updated_at SET DEFAULT now();
  ALTER TABLE branches ALTER COLUMN updated_at SET NOT NULL;
`;

/**
 * Additive, idempotent migration (story S07, technical spec §"Atomic
 * merge": "...and branch-status updates") persisting the feature-01 branch
 * lifecycle state machine (`draft -> submitted -> verified -> merged`,
 * `apps/store/src/domain/branch-lifecycle.ts`) for the first time — no
 * prior story persisted it. `status` defaults to `'draft'`, matching every
 * branch registered by earlier stories (S04's `registerBranch`/
 * `acceptSuggestionAndRegisterBranch`, S06's `registerBranch`), none of
 * which could previously have been submitted/verified/merged.
 *
 * The `submitted_at`/`verified_at`/`merged_at` + matching
 * `*_by_stakeholder_id` audit columns persist the accountability already
 * modelled (but never stored) by the domain's `BranchSubmittedRecord`,
 * `BranchVerifiedRecord`, and `MergeLineage` value objects (Meridian
 * `IDEA-29`: "Auditing columns... track the human stakeholder responsible
 * for modifications"), so AC3's "review process" half is answerable, not
 * just "which branch".
 */
const MIGRATE_BRANCHES_LIFECYCLE_STATUS = `
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'draft';
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS submitted_at TIMESTAMPTZ;
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS submitted_by_stakeholder_id TEXT;
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ;
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS verified_by_stakeholder_id TEXT;
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS merged_at TIMESTAMPTZ;
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS merged_by_stakeholder_id TEXT;
  DO $$
  BEGIN
    IF NOT EXISTS (
      SELECT 1 FROM pg_constraint WHERE conname = 'branches_status_check'
    ) THEN
      ALTER TABLE branches ADD CONSTRAINT branches_status_check
        CHECK (status IN ('draft', 'submitted', 'verified', 'merged'));
    END IF;
  END $$;
`;

/**
 * Additive, idempotent migration (story S09) recording a branch's durable
 * author — the human stakeholder who created it. Nullable: this predates
 * every earlier story's branch-registration paths (`registerBranch`,
 * `acceptSuggestionAndRegisterBranch`), so pre-existing rows have no
 * recorded author and this migration does not attempt to guess one.
 *
 * This is deliberately store-owned, durable data — not a caller-supplied
 * parameter to the notification-routing persistence adapter — so that "the
 * author of the evaluated branch must be notified" (technical spec
 * §"Feedback notification routing", `IDEA-67`) can never be satisfied by an
 * unverified client claim about who the author is (AC5's provenance rule
 * applied to routing, not just to the feedback/signal record itself).
 * `NotificationRepository` resolves the author by reading this column, never
 * by trusting a request parameter.
 */
const MIGRATE_BRANCHES_AUTHOR = `
  ALTER TABLE branches ADD COLUMN IF NOT EXISTS author_stakeholder_id TEXT;
`;

/**
 * Chunk-artifact association junction table (story S05, technical spec
 * §"Chunk-artifact association lifecycle"; Meridian `IDEA-62`, verified live:
 * "Chunk-artifact associations are tracked in the chunk_artifacts junction
 * table, containing branch_id, origin_branch_id, status (active, superseded,
 * deactivated), and created/updated auditing columns to allow branches to
 * version associations under a delta-based model without affecting the
 * mainline.").
 *
 * Unlike the chunks/edges pair (two separate tables), Meridian specifies a
 * *single* table here: `branch_id` is nullable, with `NULL` meaning the
 * mainline scope and a branch ID meaning that branch's own shadow lineage
 * (`IDEA-60`: "Associations are versioned per-branch, allowing branches to
 * add or remove artifact references safely without modifying the mainline
 * until merged."). `id` is a surrogate key because the natural identity
 * (workspace, chunk label, artifact, branch scope) is not usable as a
 * Postgres primary key while `branch_id` is nullable (primary key columns
 * cannot be NULL); `version` numbers each append-only lineage entry for that
 * natural identity, oldest first.
 *
 * `origin_branch_id` is set once, at a lineage's first version, and is
 * preserved unchanged on every later version of that lineage (technical spec
 * §"Pre-merge history reconstruction" / `IDEA-69`): a future merge story may
 * clear a promoted row's `branch_id`, but must never lose this provenance
 * column. It is `NULL` for a mainline-originated lineage.
 */
const CREATE_CHUNK_ARTIFACTS_TABLE = `
  CREATE TABLE IF NOT EXISTS chunk_artifacts (
    id BIGSERIAL PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    chunk_label TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    branch_id TEXT,
    origin_branch_id TEXT,
    version INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'superseded', 'deactivated')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
  );
`;

/**
 * `branch_id` is nullable, so a plain `UNIQUE (..., branch_id, version)`
 * table constraint would *not* actually enforce per-version uniqueness for
 * mainline rows: Postgres treats every `NULL` as distinct from every other
 * `NULL`, so two mainline rows sharing the same `(workspace_id, chunk_label,
 * artifact_id, version)` would NOT violate such a constraint (found during
 * implementation review). These two partial unique indexes enforce
 * per-version uniqueness correctly in both scopes instead — one keyed
 * without `branch_id` for mainline (`branch_id IS NULL`), one keyed with it
 * for branch scopes (`branch_id IS NOT NULL`).
 */
const CREATE_CHUNK_ARTIFACTS_MAINLINE_VERSION_UNIQUE_INDEX = `
  CREATE UNIQUE INDEX IF NOT EXISTS idx_chunk_artifacts_mainline_version
    ON chunk_artifacts (workspace_id, chunk_label, artifact_id, version)
    WHERE branch_id IS NULL;
`;

const CREATE_CHUNK_ARTIFACTS_BRANCH_VERSION_UNIQUE_INDEX = `
  CREATE UNIQUE INDEX IF NOT EXISTS idx_chunk_artifacts_branch_version
    ON chunk_artifacts (workspace_id, branch_id, chunk_label, artifact_id, version)
    WHERE branch_id IS NOT NULL;
`;

/**
 * `IDEA-64` (verified live): "Mainline Uniqueness Index on Chunk-Artifacts:
 * To ensure data integrity, a partial unique index is required on the
 * chunk_artifacts table to prevent duplicate active mainline associations:
 * CREATE UNIQUE INDEX idx_chunk_artifacts_mainline ON chunk_artifacts
 * (chunk_label, artifact_id) WHERE branch_id IS NULL AND status = 'active';"
 * `workspace_id` is included here (Meridian's snippet omits it) to satisfy
 * the technical spec's blanket tenant-isolation requirement — every
 * workspace-scoped uniqueness rule must itself be workspace-scoped, or one
 * workspace's active association could block another's.
 */
const CREATE_CHUNK_ARTIFACTS_MAINLINE_ACTIVE_UNIQUE_INDEX = `
  CREATE UNIQUE INDEX IF NOT EXISTS idx_chunk_artifacts_mainline
    ON chunk_artifacts (workspace_id, chunk_label, artifact_id)
    WHERE branch_id IS NULL AND status = 'active';
`;

/**
 * Branch-scoped analogue of `idx_chunk_artifacts_mainline`: at most one
 * active association per (workspace, branch, chunk label, artifact) so a
 * single branch's own shadow lineage can never accumulate two active rows
 * for the same identity either (derived from the same data-integrity
 * principle as `IDEA-64`, not itself a literal Meridian requirement).
 */
const CREATE_CHUNK_ARTIFACTS_BRANCH_ACTIVE_UNIQUE_INDEX = `
  CREATE UNIQUE INDEX IF NOT EXISTS idx_chunk_artifacts_branch_active
    ON chunk_artifacts (workspace_id, branch_id, chunk_label, artifact_id)
    WHERE branch_id IS NOT NULL AND status = 'active';
`;

/**
 * Durable, workspace-scoped downstream push-delivery subscription registry
 * (story S08; Meridian `IDEA-65`: "Downstream Push consumers are tracked via
 * a dedicated `delivery_subscriptions` database table, registering webhooks
 * and optional discipline filters."). Technical spec §"Delivery subscription
 * persistence": subscriptions must be persisted independent of any single
 * delivery attempt — this table intentionally carries no delivery-attempt,
 * delivery-log, or push-outcome columns, so registering/removing a consumer
 * never depends on, and is never affected by, whether a push has ever
 * succeeded or failed.
 *
 * `disciplines` is `NULL` (no filter — every discipline is delivered) or a
 * non-empty array of `Discipline` values (only matching disciplines are
 * delivered). Re-registering the same `(workspace_id, consumer_id)` upserts
 * in place (AC1: preferences persist across sessions without needing to be
 * re-submitted; re-submission simply updates the existing record).
 */
const CREATE_DELIVERY_SUBSCRIPTIONS_TABLE = `
  CREATE TABLE IF NOT EXISTS delivery_subscriptions (
    workspace_id TEXT NOT NULL,
    consumer_id TEXT NOT NULL,
    webhook_url TEXT NOT NULL,
    disciplines TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, consumer_id)
  );
`;

/**
 * Evaluation feedback items (story S09, technical spec §"Feedback
 * notification routing", Meridian `IDEA-68`): free-text human/agent review
 * commentary associated with the branch it evaluated. Append-only from this
 * schema's point of view — no update/delete path exists for this table,
 * matching the non-destructive-acknowledgement requirement (a notification
 * referencing a row here must never cause that row to be mutated or
 * deleted).
 */
const CREATE_FEEDBACK_ITEMS_TABLE = `
  CREATE TABLE IF NOT EXISTS feedback_items (
    workspace_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    feedback_item_id TEXT NOT NULL,
    authored_by_stakeholder_id TEXT NOT NULL,
    authored_by_actor_kind TEXT NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, feedback_item_id),
    FOREIGN KEY (workspace_id, branch_id) REFERENCES branches (workspace_id, branch_id)
  );
`;

/**
 * Verification signals (feature-01 story S07 domain concept
 * `apps/store/src/domain/types/verification/verification-signal.ts`,
 * persisted for the first time by this story): advisory outcome evidence
 * (`passing`/`failing`/`mixed`) associated with the branch it evaluated.
 * Append-only, same rationale as `feedback_items` above.
 */
const CREATE_VERIFICATION_SIGNALS_TABLE = `
  CREATE TABLE IF NOT EXISTS verification_signals (
    workspace_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    signal_id TEXT NOT NULL,
    outcome TEXT NOT NULL,
    reported_by_stakeholder_id TEXT NOT NULL,
    reported_by_actor_kind TEXT NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL,
    summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, signal_id),
    FOREIGN KEY (workspace_id, branch_id) REFERENCES branches (workspace_id, branch_id)
  );
`;

/**
 * Routed notifications (story S09, Meridian `IDEA-67`/`IDEA-68`): one row
 * per recipient stakeholder per feedback/verification-signal ingestion
 * event. `source_kind`/`source_id` point at the triggering row in
 * `feedback_items` or `verification_signals` without a foreign key spanning
 * two possible parent tables (Postgres has no native polymorphic FK) — the
 * persistence adapter is the sole writer of both columns together, so this
 * is not a caller-facing integrity gap.
 *
 * `acknowledged_at` is the only mutable column (technical spec
 * §"Notification acknowledgement is non-destructive": acknowledging "must
 * not delete or mutate the underlying feedback or verification signal
 * record it references" — only this table's own row may change).
 */
const CREATE_NOTIFICATIONS_TABLE = `
  CREATE TABLE IF NOT EXISTS notifications (
    workspace_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    notification_id TEXT NOT NULL,
    recipient_stakeholder_id TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('feedback-item', 'verification-signal')),
    source_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_at TIMESTAMPTZ,
    PRIMARY KEY (workspace_id, notification_id),
    FOREIGN KEY (workspace_id, branch_id) REFERENCES branches (workspace_id, branch_id)
  );
`;

/**
 * Supports `listNotificationsForStakeholder` (AC1/AC2/AC3) without a
 * sequential scan of the whole table as notification volume grows.
 */
const CREATE_NOTIFICATIONS_RECIPIENT_INDEX = `
  CREATE INDEX IF NOT EXISTS idx_notifications_recipient
    ON notifications (workspace_id, recipient_stakeholder_id, created_at);
`;

/**
 * Postgres advisory lock key used to serialize concurrent `ensureSchema`
 * bootstraps. `CREATE TABLE IF NOT EXISTS` is not safe under true
 * concurrency: two connections racing to create the same table can both
 * pass the existence check and collide on Postgres's internal `pg_type`
 * catalog. Multiple e2e spec files each call `ensureSchema` in their own
 * `beforeAll` and vitest runs spec files in parallel forked processes, so
 * this lock is required for the bootstrap to be reliable, not just
 * theoretically nice-to-have.
 */
const SCHEMA_BOOTSTRAP_LOCK_KEY = 875_302_114;

export async function ensureSchema(pool: Pool): Promise<void> {
  const client = await pool.connect();
  try {
    await client.query('SELECT pg_advisory_lock($1)', [SCHEMA_BOOTSTRAP_LOCK_KEY]);
    await client.query(CREATE_CHUNKS_TABLE);
    await client.query(MIGRATE_CHUNKS_ORIGIN_BRANCH_ID);
    await client.query(CREATE_EDGE_VERSIONS_TABLE);
    await client.query(MIGRATE_EDGE_VERSIONS_LINEAGE_SEQ);
    await client.query(MIGRATE_EDGE_VERSIONS_ORIGIN_BRANCH_ID);
    await client.query(CREATE_EDGE_ACTIVE_UNIQUE_INDEX);
    await client.query(CREATE_BRANCH_CHUNK_DELTAS_TABLE);
    await client.query(CREATE_BRANCH_EDGE_DELTAS_TABLE);
    await client.query(CREATE_SUGGESTIONS_TABLE);
    await client.query(CREATE_BRANCHES_TABLE);
    await client.query(MIGRATE_BRANCHES_DIVERGENCE_MARKER);
    await client.query(MIGRATE_BRANCHES_LIFECYCLE_STATUS);
    await client.query(MIGRATE_BRANCHES_AUTHOR);
    await client.query(CREATE_CHUNK_ARTIFACTS_TABLE);
    await client.query(CREATE_CHUNK_ARTIFACTS_MAINLINE_VERSION_UNIQUE_INDEX);
    await client.query(CREATE_CHUNK_ARTIFACTS_BRANCH_VERSION_UNIQUE_INDEX);
    await client.query(CREATE_CHUNK_ARTIFACTS_MAINLINE_ACTIVE_UNIQUE_INDEX);
    await client.query(CREATE_CHUNK_ARTIFACTS_BRANCH_ACTIVE_UNIQUE_INDEX);
    await client.query(CREATE_DELIVERY_SUBSCRIPTIONS_TABLE);
    await client.query(CREATE_FEEDBACK_ITEMS_TABLE);
    await client.query(CREATE_VERIFICATION_SIGNALS_TABLE);
    await client.query(CREATE_NOTIFICATIONS_TABLE);
    await client.query(CREATE_NOTIFICATIONS_RECIPIENT_INDEX);
  } finally {
    await client.query('SELECT pg_advisory_unlock($1)', [SCHEMA_BOOTSTRAP_LOCK_KEY]);
    client.release();
  }
}
