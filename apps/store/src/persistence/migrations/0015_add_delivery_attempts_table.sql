-- Adds the delivery_attempts table (G14 SG1), per Meridian IDEA-127 (promoted: base
-- pending/succeeded/failed schema, unique(subscription_id, merge_event_id) idempotency key,
-- backoff up to 3 attempts) as amended by IDEA-129 (promoted: adds the 'in_progress' status and
-- ratifies a Postgres-native single-instance claim-queue via
-- `UPDATE ... WHERE status='pending' ... FOR UPDATE SKIP LOCKED`).
--
-- SCOPE DEVIATION (none, interim scoping per IDEA-132 gap-report + goal OQ2, user-approved):
-- this table has no lease-expiry/heartbeat column, so a delivery_attempts row stranded
-- `in_progress` by a worker crash mid-delivery is never automatically reclaimed in this goal.
-- Per-subscription FIFO ordering is enforced entirely in the repository's claimBatch query (an
-- explicit "skip a subscription that already has an in_progress row" rule), not by any
-- constraint here -- SKIP LOCKED alone does not guarantee FIFO across concurrent poll ticks.
CREATE TABLE delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES delivery_subscriptions(id),
    merge_event_id UUID NOT NULL,
    branch_id UUID NOT NULL,
    merged_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'in_progress', 'succeeded', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_attempted_at TIMESTAMPTZ NULL,
    next_retry_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (subscription_id, merge_event_id)
);

CREATE INDEX idx_delivery_attempts_subscription ON delivery_attempts (subscription_id);

-- Speeds up claimBatch's "pending rows that are due" scan (status + next_retry_at are always
-- queried together there).
CREATE INDEX idx_delivery_attempts_status_next_retry ON delivery_attempts (status, next_retry_at);
