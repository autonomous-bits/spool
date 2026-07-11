-- Adds the authoritative branches table (G02) and removes G01's temporary branchless-only
-- constraint on chunks, wiring real foreign keys instead.
--
-- Authoritative source: Meridian IDEA-31 (Postgres schema ADR, promoted). See
-- file:///Users/wernerswart/repos/architecture/sql/schema.sql for the full authoritative graph
-- schema.
--
-- SCOPE DEVIATION: origin_suggestion_id is declared as a plain nullable UUID with no foreign key,
-- because the `suggestions` table does not exist yet in this repo. This mirrors G01's precedent
-- of declaring not-yet-existing relations as plain columns until the referenced table is
-- introduced by a future goal.
CREATE TABLE branches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    discipline VARCHAR(50) NOT NULL CHECK (discipline IN ('product', 'architecture', 'design', 'engineering', 'security', 'governance')),
    status VARCHAR(50) NOT NULL CHECK (status IN ('draft', 'submitted', 'verified', 'merged')),
    submitted_at TIMESTAMP WITH TIME ZONE,
    diverged_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    merged_at TIMESTAMP WITH TIME ZONE,
    origin_suggestion_id UUID,
    created_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    merged_by_stakeholder_id UUID REFERENCES stakeholders(id)
);

CREATE UNIQUE INDEX idx_active_branch_name ON branches (name) WHERE status != 'merged';

-- G01's temporary constraint pinned branch_id/origin_branch_id to NULL until branches existed.
-- Branches now exist, so drop it and wire the real foreign keys the authoritative schema
-- specifies.
ALTER TABLE chunks DROP CONSTRAINT chk_chunks_branchless_only_g01;

ALTER TABLE chunks
    ADD CONSTRAINT chunks_branch_id_fkey FOREIGN KEY (branch_id) REFERENCES branches(id) ON DELETE CASCADE;

ALTER TABLE chunks
    ADD CONSTRAINT chunks_origin_branch_id_fkey FOREIGN KEY (origin_branch_id) REFERENCES branches(id);
