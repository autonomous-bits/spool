-- Adds the authoritative verification_signals table (G09 SG1), matching schema.sql column-for-
-- column (Meridian IDEA-31 ratified ADR) -- no scope deviation needed, unlike G07's suggestions
-- table.
--
-- Authoritative source: Meridian IDEA-21 (promoted: dedicated agents, tools, or humans evaluate
-- submitted branches and log feedback), IDEA-43 (promoted ADR: verification signals are recorded
-- as feedback but never auto-transition the branch), IDEA-20 (promoted: submission locks branch
-- state for verification -- basis for restricting signal submission to submitted/verified
-- branches). See file:///Users/wernerswart/repos/architecture/sql/schema.sql for the full
-- authoritative graph schema.
--
-- verifier_name is a plain non-null VARCHAR, not a stakeholder FK: IDEA-21's "agents, tools, or
-- other humans" is broader than the registered-stakeholder set, so submission needs no
-- ActorContext/session-token. Submitting a signal never changes branches.status; that
-- no-auto-transition invariant (IDEA-43) is enforced in the application layer
-- (assertReviewableStatus in branch-lifecycle.ts), not the schema.
CREATE TABLE verification_signals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id UUID REFERENCES branches(id) ON DELETE CASCADE,
    verifier_name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL CHECK (status IN ('pass', 'fail')),
    reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
