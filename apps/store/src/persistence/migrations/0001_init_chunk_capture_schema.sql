-- Initial schema for atomic chunk capture (G01).
--
-- Authoritative source: Meridian IDEA-31 (Postgres schema ADR, promoted), amended by IDEA-77
-- (chunk_type/context_kind columns) and IDEA-78 (branchless/draft capture +
-- idx_chunks_draft_mainline). See file:///Users/wernerswart/repos/architecture/sql/schema.sql
-- for the full authoritative graph schema.
--
-- SCOPE DEVIATION: G01 only implements chunk capture, not branch-based authoring. The
-- authoritative schema's `chunks.branch_id`/`origin_branch_id` reference a `branches` table that
-- does not exist yet in this repo. Until a future goal introduces branches, this migration:
--   1. Declares branch_id/origin_branch_id as plain UUID columns with NO foreign key to
--      `branches` (that table doesn't exist).
--   2. Adds a temporary CHECK constraint pinning both columns to NULL, since IDEA-78 ratifies
--      branchless/draft capture (branch_id NULL, status 'draft') as the only capture path in
--      scope for this goal. This CHECK MUST be dropped when branch-based authoring is
--      implemented.

CREATE TABLE stakeholders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    role VARCHAR(50) NOT NULL,
    discipline VARCHAR(50) CHECK (discipline IS NULL OR discipline IN ('product', 'architecture', 'design', 'engineering', 'security', 'governance')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    discipline VARCHAR(50) NOT NULL CHECK (discipline IN ('product', 'architecture', 'design', 'engineering', 'security', 'governance')),
    status VARCHAR(50) NOT NULL CHECK (status IN ('draft', 'promoted', 'superseded', 'deactivated')),
    chunk_type VARCHAR(50) NOT NULL CHECK (chunk_type IN ('feature', 'capability', 'constraint', 'adr', 'spike')),
    context_kind VARCHAR(50) NOT NULL CHECK (context_kind IN ('permanent', 'transient')),
    branch_id UUID,
    origin_branch_id UUID,
    created_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    updated_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_label_per_branch UNIQUE (label, branch_id),
    CONSTRAINT chk_chunks_branchless_only_g01 CHECK (branch_id IS NULL AND origin_branch_id IS NULL)
);

CREATE INDEX idx_chunks_branch_lookup ON chunks (branch_id, label);
CREATE UNIQUE INDEX idx_chunks_mainline ON chunks (label) WHERE branch_id IS NULL AND status = 'promoted';

-- Ratified by IDEA-78: tighten mainline label-uniqueness to also cover branchless drafts.
CREATE UNIQUE INDEX idx_chunks_draft_mainline ON chunks (label) WHERE branch_id IS NULL AND status = 'draft';
