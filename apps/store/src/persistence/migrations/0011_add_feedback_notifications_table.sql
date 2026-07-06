-- Adds the authoritative feedback_notifications table for G09 SG2 notification fan-out, matching
-- schema.sql column-for-column (Meridian IDEA-31 ratified ADR) and implementing IDEA-68/IDEA-67's
-- requirement that each verification signal be delivered as unread stakeholder feedback.
--
-- Authoritative source: Meridian IDEA-68 (verification activity emits stakeholder-facing
-- notifications), IDEA-67 (feedback delivery fans out to stakeholders), IDEA-31 (promoted Postgres
-- schema ADR), IDEA-11 (human stakeholders participate in branch governance). See
-- file:///Users/wernerswart/repos/architecture/sql/schema.sql for the full authoritative graph
-- schema.
--
-- Recipient-scope decision for this increment: every stakeholder in the global stakeholders table
-- receives one unread notification per verification signal. No branch-watcher or tenant-recipient
-- join table is introduced here; that flat fan-out is already ratified for G09 SG2 and stays
-- consistent with G05's "any human stakeholder may act as a merging authority" rule.
CREATE TABLE feedback_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    stakeholder_id UUID NOT NULL REFERENCES stakeholders(id) ON DELETE CASCADE,
    signal_id UUID NOT NULL REFERENCES verification_signals(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL CHECK (status IN ('unread', 'read')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_feedback_notifications_stakeholder ON feedback_notifications (stakeholder_id, status);
