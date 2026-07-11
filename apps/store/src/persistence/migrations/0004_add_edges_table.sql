-- Adds the authoritative edges table (G03): typed, directed relationships between chunks,
-- referenced by their logical string labels rather than database ids (Meridian IDEA-36/IDEA-37),
-- so relationships remain valid across branch overrides and mainline promotions.
--
-- Authoritative source: Meridian IDEA-31 (Postgres schema ADR, promoted), IDEA-32 (branch-scoped
-- delta rows, promoted), IDEA-33 (branch-local overlay reads, promoted), IDEA-44 (composite index
-- requirement, promoted). See file:///Users/wernerswart/repos/architecture/sql/schema.sql for the
-- full authoritative graph schema.
--
-- SCOPE DEVIATION: this goal only ever writes 'active' edges (supersededByEdgeId always NULL).
-- The superseded_by_edge_id column and its self-referencing foreign key are declared now, per the
-- authoritative schema, so the supersede/deactivate lifecycle (future goal) doesn't require a
-- follow-up migration.
CREATE TABLE edges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_chunk_label VARCHAR(50) NOT NULL,
    to_chunk_label VARCHAR(50) NOT NULL,
    type VARCHAR(50) NOT NULL CHECK (type IN ('refines', 'depends-on', 'contradicts', 'derives-from', 'blocks', 'implements', 'constrains', 'feedback-on')),
    status VARCHAR(50) NOT NULL CHECK (status IN ('active', 'superseded', 'deactivated')),
    discipline VARCHAR(50) CHECK (discipline IS NULL OR discipline IN ('product', 'architecture', 'design', 'engineering', 'security', 'governance')),
    branch_id UUID REFERENCES branches(id) ON DELETE CASCADE,
    origin_branch_id UUID REFERENCES branches(id),
    superseded_by_edge_id UUID REFERENCES edges(id),
    created_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    updated_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_edges_branch_lookup ON edges (branch_id, from_chunk_label, to_chunk_label);
CREATE UNIQUE INDEX idx_edges_mainline ON edges (from_chunk_label, to_chunk_label, type) WHERE branch_id IS NULL AND status = 'active';
CREATE UNIQUE INDEX idx_edges_branch_active ON edges (from_chunk_label, to_chunk_label, type, branch_id) WHERE status = 'active';
