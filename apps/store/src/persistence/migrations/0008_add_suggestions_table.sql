-- Adds the authoritative suggestions table and its state-transition log (G07), plus the FK on
-- branches.origin_suggestion_id that IDEA-82 documented as deferred until this table existed.
--
-- Authoritative source: Meridian IDEA-49 (promoted ADR: suggestions table; POST route inserts
-- pending; branches.origin_suggestion_id FK), IDEA-27 (promoted: pending/accepted/rejected
-- lifecycle, all transitions logged), IDEA-75 (promoted ADR: ActorKind human/delegated). See
-- file:///Users/wernerswart/repos/architecture/sql/schema.sql for the full authoritative graph
-- schema.
--
-- SCOPE DEVIATION (escalated upstream as promoted Meridian feedback IDEA-83, mirrors IDEA-82's
-- precedent): schema.sql's suggestions table has no column recording who/what submitted a
-- suggestion. This migration adds submitted_by_stakeholder_id (NOT NULL FK stakeholders) and
-- submitted_by_actor_kind (NOT NULL, server-assigned, never client-supplied) to support IDEA-75's
-- human/delegated distinction for suggestion actions. decided_by_stakeholder_id/decided_at are
-- additive nullable convenience columns for the same reason (populated on accept/reject, a
-- future goal).
CREATE TABLE suggestions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label VARCHAR(50),
    content TEXT,
    from_chunk_label VARCHAR(50),
    to_chunk_label VARCHAR(50),
    relationship_type VARCHAR(50) CHECK (relationship_type IS NULL OR relationship_type IN ('refines', 'depends-on', 'contradicts', 'derives-from', 'blocks', 'implements', 'constrains', 'feedback-on')),
    discipline VARCHAR(50) NOT NULL CHECK (discipline IN ('product', 'architecture', 'design', 'engineering', 'security', 'governance')),
    status VARCHAR(50) NOT NULL CHECK (status IN ('pending', 'accepted', 'rejected')),
    submitted_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    submitted_by_actor_kind VARCHAR(50) NOT NULL CHECK (submitted_by_actor_kind IN ('human', 'delegated')),
    decided_by_stakeholder_id UUID REFERENCES stakeholders(id),
    decided_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT check_suggestion_type CHECK (
        (label IS NOT NULL AND content IS NOT NULL AND from_chunk_label IS NULL AND to_chunk_label IS NULL AND relationship_type IS NULL)
        OR
        (label IS NULL AND content IS NULL AND from_chunk_label IS NOT NULL AND to_chunk_label IS NOT NULL AND relationship_type IS NOT NULL)
    )
);

CREATE UNIQUE INDEX idx_suggestions_unique ON suggestions (label, content);

CREATE TABLE suggestion_state_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    suggestion_id UUID NOT NULL REFERENCES suggestions(id),
    old_status VARCHAR(50) CHECK (old_status IS NULL OR old_status IN ('pending', 'accepted', 'rejected')),
    new_status VARCHAR(50) NOT NULL CHECK (new_status IN ('pending', 'accepted', 'rejected')),
    updated_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Closes the scope deviation documented in migration 0003/IDEA-82: the suggestions table now
-- exists, so wire the real foreign key onto the existing origin_suggestion_id column.
ALTER TABLE branches
    ADD CONSTRAINT branches_origin_suggestion_id_fkey FOREIGN KEY (origin_suggestion_id) REFERENCES suggestions(id) ON DELETE SET NULL;
