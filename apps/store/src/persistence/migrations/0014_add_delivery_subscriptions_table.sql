-- Adds the delivery_subscriptions table (G13 SG1), per Meridian IDEA-65 (ADR: delivery
-- subscriptions tracked in a delivery_subscriptions table; registers webhooks + optional
-- discipline filters) and IDEA-104 point 3 (exact column-for-column schema).
--
-- SCOPE DEVIATION (none): this migration only adds the subscription-registration table itself.
-- The delivery worker's own tables (delivery-attempt tracking, per IDEA-104 point 2's
-- retry/ordering/idempotency invariants) are explicitly out of scope — filed as Meridian
-- IDEA-126 (draft gap-report) and deferred to a future goal per G13's Open Questions OQ1.
--
-- Soft-delete via is_active, consistent with the "supersede/deactivate, never hard-delete"
-- precedent used elsewhere (e.g. edges' superseded_by_edge_id, IDEA-26/IDEA-38).
CREATE TABLE delivery_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id),
    url TEXT NOT NULL,
    discipline_filter JSONB NULL,
    signing_secret TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_delivery_subscriptions_workspace ON delivery_subscriptions (workspace_id);
